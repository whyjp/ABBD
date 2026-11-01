package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// audit consumes the http api, so changing the api's contract affects it.
var topoConsumed = strings.Replace(strings.Replace(topoV1, `"units":[`,
	`"units":[{"key":"audit","kind":"MOD","name":"audit","anchors":[{"path":"reports/log.go","symbol":"Log"}]},`, 1),
	`{"from":"billing","to":"api","kind":"provides"}`,
	`{"from":"billing","to":"api","kind":"provides"},{"from":"audit","to":"api","kind":"consumes"}`, 1)

var apiV2 = func(topo string) string { return strings.Replace(topo, `GET /charges`, `GET /v2/charges`, 1) }

type turnProject struct {
	*project
	exe, verifyFile string
}

func activeTurnProject(t *testing.T, topo string) *turnProject {
	t.Helper()
	p := newProject(t)
	p.extract(topo)
	p.want(0, "new", "ingest", p.extraction)
	p.want(0, "aligned", "align", "--all")
	p.want(0, "active", "mode", "on", "--session", "s1")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	tp := &turnProject{project: p, exe: exe, verifyFile: filepath.Join(t.TempDir(), "verify")}
	t.Setenv("ABBD_FAKE_VERIFY", tp.verifyFile)
	return tp
}

// verify runs the fake verify command, which passes or fails as told.
func (p *turnProject) verify(result string, want int, contains string) {
	p.t.Helper()
	write(p.t, p.verifyFile, result)
	var out, errb bytes.Buffer
	code := run([]string{"turn", "verify", "--session", "s1", "--root", p.root, "--", p.exe, "fake-verify"}, neverRead{p.t}, &out, &errb)
	if got := out.String() + errb.String(); code != want || !strings.Contains(got, contains) {
		p.t.Fatalf("abbd turn verify: want exit %d containing %q, got %d:\n%s", want, contains, code, got)
	}
}

func (p *turnProject) start(seeds string) string {
	p.t.Helper()
	return p.want(0, "OPEN", "turn", "start", "--session", "s1", "--seeds", seeds, "--request", "make the change")
}

func (p *project) intended() string { return p.read("intended.yaml") }

func TestTurnInsideTheBoundarySyncsWithoutAHuman(t *testing.T) {
	p := activeTurnProject(t, topoV1)
	p.start("MOD-0001")
	if req := p.read("requirements/REQ-0001.md"); !strings.Contains(req, "origin: USER_STATED") || !strings.Contains(req, "make the change") {
		t.Fatalf("the request should be recorded as stated:\n%s", req)
	}
	p.want(0, "CHG-0001 declares 1 unit(s) and 0 change(s)", "turn", "declare", "--session", "s1", "--units", "MOD-0001")
	for _, id := range []string{"MOD-0001", "IF-0001"} {
		p.want(0, "scanned", "turn", "scan", "--session", "s1", "--unit", id, "--by", "how")
	}
	p.editCode("billing/charge.go")
	p.want(0, "", "turn", "diff", "--session", "s1")
	p.verify("pass", 0, "run-0001 PASSED")
	before := p.intended()

	var out, errb bytes.Buffer
	if code := run([]string{"turn", "end", "--session", "s1", "--root", p.root}, neverRead{t}, &out, &errb); code != 0 || !strings.Contains(out.String(), "CHG-0001 SYNCED") {
		t.Fatalf("a turn inside the boundary should sync on its own, exit %d:\n%s%s", code, out.String(), errb.String())
	}
	if _, err := os.Stat(filepath.Join(p.root, ".abbd", "decisions")); err == nil {
		t.Fatalf("a grade A turn should leave no decision")
	}
	if p.intended() != before {
		t.Fatalf("a grade A turn should not change the intended topology")
	}
	if run := p.read("verification/runs/run-0001.yaml"); !strings.Contains(run, "result: PASSED") || !strings.Contains(run, "output: verification/runs/run-0001.log") {
		t.Fatalf("the run should record its result and output:\n%s", run)
	}
	if !strings.Contains(p.read("verification/runs/run-0001.log"), "all tests passed") {
		t.Fatalf("the run output should be kept")
	}
	if chg := p.read("changes/CHG-0001.md"); !strings.Contains(chg, "state: SYNCED") {
		t.Fatalf("the turn record should say SYNCED:\n%s", chg)
	}
}

func TestTurnNeedsAnActiveAlignedSession(t *testing.T) {
	p := activeTurnProject(t, topoV1)
	p.want(1, "not active", "turn", "start", "--session", "s2", "--seeds", "MOD-0001", "--request", "x")
	p.want(1, "no turn to declare", "turn", "declare", "--session", "s1", "--units", "MOD-0001")

	p.editCode("api/http.go")
	p.extract(apiV2(topoV1))
	out := p.want(2, "drifted", "turn", "start", "--session", "s1", "--seeds", "MOD-0001", "--request", "x")
	if !strings.Contains(out, "BLOCKED contract-changed:IF-0001") {
		t.Fatalf("a change made between turns should be shown:\n%s", out)
	}
}

func TestTurnCommandsBelongToTheTurnsSession(t *testing.T) {
	p := activeTurnProject(t, topoV1)
	p.start("MOD-0001")
	p.editCode("billing/charge.go")
	for _, sub := range []string{"impact", "declare", "diff", "verify", "end"} {
		p.want(64, "turn "+sub+" needs --session", "turn", sub)
		p.want(1, "CHG-0001 belongs to session s1, not s2", "turn", sub, "--session", "s2")
	}
	p.want(0, "CHG-0001 OPEN", "turn", "show")
	p.want(0, "CHG-0001 OPEN", "turn", "show", "--session", "s2")
}

func TestImpactFollowsConsumersAndCannotShrink(t *testing.T) {
	p := activeTurnProject(t, topoConsumed)
	out := p.start("MOD-0001")
	for _, want := range []string{"impact MOD-0001: seed", "impact IF-0001: provided by MOD-0001", "impact MOD-0003: consumes IF-0001"} {
		if !strings.Contains(out, want) {
			t.Fatalf("impact should reach the contract's consumers; missing %q:\n%s", want, out)
		}
	}
	p.want(0, "impact MOD-0002: added by the agent (agent, direct)", "turn", "impact", "--session", "s1", "--add", "MOD-0002")
	p.want(64, "flag provided but not defined", "turn", "impact", "--session", "s1", "--remove", "MOD-0003")

	path := filepath.Join(p.root, ".abbd", "changes", "CHG-0001.md")
	b, _ := os.ReadFile(path)
	cut := regexp.MustCompile(`(?m)^ *- unit: MOD-0003\n.*\n.*\n`).ReplaceAllString(string(b), "")
	if cut == string(b) {
		t.Fatalf("test setup: could not remove the impact item from:\n%s", b)
	}
	write(t, path, cut)
	got := p.want(0, "impact MOD-0003: consumes IF-0001 (computed, direct)", "turn", "show")
	if !strings.Contains(got, "cannot be removed; restored") || !strings.Contains(p.read("changes/CHG-0001.md"), "unit: MOD-0003") {
		t.Fatalf("a removed computed item should come back:\n%s", got)
	}
}

// topoTiered adds a module that leans on the contract's consumer, two hops
// from the seed billing, and one that depends on billing itself.
var topoTiered = strings.Replace(strings.Replace(topoConsumed, `"units":[`,
	`"units":[{"key":"digest","kind":"MOD","name":"digest","anchors":[{"path":"reports/digest.go","symbol":"Mail"}]},`+
		`{"key":"cron","kind":"MOD","name":"cron","anchors":[{"path":"zcron/run.go","symbol":"Run"}]},`, 1),
	`{"from":"audit","to":"api","kind":"consumes"}`,
	`{"from":"audit","to":"api","kind":"consumes"},{"from":"digest","to":"audit","kind":"depends_on"},{"from":"cron","to":"billing","kind":"depends_on"}`, 1)

func TestImpactSplitsIntoDirectAndFarTiers(t *testing.T) {
	p := activeTurnProject(t, topoTiered)
	direct := []string{
		"impact MOD-0001: seed (computed, direct)",
		"impact IF-0001: provided by MOD-0001 (computed, direct)",
		"impact MOD-0004: consumes IF-0001 (computed, direct)",
		"impact MOD-0005: depends_on MOD-0001 (computed, direct)",
	}
	far := "impact MOD-0003: depends_on MOD-0004 (computed, far)"
	readsDirectFirst := func(out string) {
		t.Helper()
		for _, d := range direct {
			if i := strings.Index(out, d); i < 0 || i > strings.Index(out, far) {
				t.Fatalf("want %q before the far item %q:\n%s", d, far, out)
			}
		}
	}
	readsDirectFirst(p.start("MOD-0001"))
	readsDirectFirst(p.want(0, "impact MOD-0002: added by the agent (agent, direct)", "turn", "impact", "--session", "s1", "--add", "MOD-0002"))
	readsDirectFirst(p.want(0, far, "turn", "show"))

	record := p.read("changes/CHG-0001.md")
	if !strings.Contains(record, "- MOD-0003: depends_on MOD-0004 (computed, far)") || !strings.Contains(record, "- MOD-0004: consumes IF-0001 (computed, direct)") {
		t.Fatalf("the change record should show each item's tier:\n%s", record)
	}
	if strings.Contains(record, "tier:") {
		t.Fatalf("the tier is read from the intended graph, so the record's frontmatter keeps its shape:\n%s", record)
	}

	path := filepath.Join(p.root, ".abbd", "changes", "CHG-0001.md")
	cut := regexp.MustCompile(`(?m)^ *- unit: MOD-0003\n.*\n.*\n`).ReplaceAllString(record, "")
	if cut == record {
		t.Fatalf("test setup: could not remove the far item from:\n%s", record)
	}
	write(t, path, cut)
	if got := p.want(0, far, "turn", "show"); !strings.Contains(got, "MOD-0003 is computed impact and cannot be removed; restored") {
		t.Fatalf("a far item is still computed impact and comes back:\n%s", got)
	}

	html := p.screen().view(t)
	html = html[strings.Index(html, `<section id="turn">`):]
	at := func(s string) int {
		i := strings.Index(html, s)
		if i < 0 {
			t.Fatalf("the screen should show %q", s)
		}
		return i
	}
	if !(at("직접") < at("MOD-0004") && at("MOD-0004") < at("멀리") && at("멀리") < at("MOD-0003")) {
		t.Fatalf("the screen should list direct items, then far ones")
	}
}

func TestDeclaredBoundaryCrossingIsStillBlockedAtTheEnd(t *testing.T) {
	p := activeTurnProject(t, topoV1)
	p.start("MOD-0001")
	edge := "new-relation:MOD-0001.depends_on.MOD-0002"
	p.want(2, "C QUESTION "+edge, "turn", "declare", "--session", "s1", "--units", "MOD-0001", "--changes", edge)

	p.editCode("billing/charge.go")
	p.extract(strings.Replace(topoV1, `{"from":"billing","to":"api","kind":"provides"}`,
		`{"from":"billing","to":"api","kind":"provides"},{"from":"billing","to":"ledger","kind":"depends_on"}`, 1))
	out := p.want(2, "C BLOCKED "+edge, "turn", "diff", "--session", "s1")
	if !strings.Contains(out, "stop:") {
		t.Fatalf("diff should tell the skill to stop:\n%s", out)
	}
	p.hook("s1", "billing/charge.go")
	if !strings.Contains(p.journal("s1"), "changed billing/charge.go") {
		t.Fatalf("hooks should keep recording while a turn drifts the project")
	}
	p.verify("pass", 0, "PASSED")
	p.want(2, "CHG-0001 BLOCKED", "turn", "end", "--session", "s1")
	if strings.Contains(p.intended(), "depends_on") {
		t.Fatalf("a blocked turn must not touch the intended topology:\n%s", p.intended())
	}

	p.want(0, "approved "+edge+" for CHG-0001 as ADR-0001", "approve", edge)
	if adr := p.read("decisions/ADR-0001.md"); !strings.Contains(adr, "turn: CHG-0001") || !strings.Contains(adr, "origin: USER_STATED") {
		t.Fatalf("a turn approval should leave a USER_STATED ADR naming the turn:\n%s", adr)
	}
	if strings.Contains(p.intended(), "depends_on") {
		t.Fatalf("a turn approval enters the intended topology only when the turn syncs")
	}
	p.want(0, "C APPROVED "+edge+" by ADR-0001", "turn", "end", "--session", "s1")
	if !strings.Contains(p.intended(), "kind: depends_on") {
		t.Fatalf("the approved relation should be intended after sync:\n%s", p.intended())
	}
	p.want(0, "phase: aligned", "status")
}

func TestUndeclaredRelationIsUnexpectedAndRealignIsGradeB(t *testing.T) {
	p := activeTurnProject(t, topoV1)
	p.start("MOD-0001")
	p.want(0, "", "turn", "declare", "--session", "s1", "--units", "MOD-0001")
	p.editCode("audit/log.go")
	p.extract(strings.Replace(strings.Replace(topoV1, `"units":[`,
		`"units":[{"key":"audit","kind":"MOD","name":"audit","anchors":[{"path":"audit/log.go","symbol":"Log"}]},`, 1),
		`{"from":"billing","to":"api","kind":"provides"}`,
		`{"from":"billing","to":"api","kind":"provides"},{"from":"billing","to":"audit","kind":"depends_on"}`, 1))
	out := p.want(0, "B PROCEED new-relation:MOD-0001.depends_on.MOD-0003", "turn", "diff", "--session", "s1")
	if !strings.Contains(out, "UNEXPECTED new-relation:MOD-0001.depends_on.MOD-0003") || !strings.Contains(out, "A PROMOTE unit-added:MOD-0003") {
		t.Fatalf("an undeclared relation should be flagged as unexpected:\n%s", out)
	}
	p.verify("pass", 0, "PASSED")
	p.want(0, "CHG-0001 SYNCED", "turn", "end", "--session", "s1")
	if adr := p.read("decisions/ADR-0001.md"); !strings.Contains(adr, "grade: B") || !strings.Contains(adr, "confirmation: UNCONFIRMED") {
		t.Fatalf("a grade B default should leave an UNCONFIRMED ADR:\n%s", adr)
	}
	p.want(0, "phase: aligned", "status")
}

func TestContractChangeWithoutConsumersIsGradeB(t *testing.T) {
	p := activeTurnProject(t, topoV1)
	p.start("IF-0001")
	p.want(0, "B PROCEED contract-changed:IF-0001", "turn", "declare", "--session", "s1", "--changes", "contract-changed:IF-0001")
	p.editCode("api/http.go")
	p.extract(apiV2(topoV1))
	p.want(0, `B PROCEED contract-changed:IF-0001 ("GET /charges" -> "GET /v2/charges")`, "turn", "diff", "--session", "s1")
	p.verify("pass", 0, "PASSED")
	p.want(0, "CHG-0001 SYNCED", "turn", "end", "--session", "s1")
	adr := p.read("decisions/ADR-0001.md")
	for _, want := range []string{"grade: B", "origin: INFERRED", "confirmation: UNCONFIRMED", "subject: contract-changed:IF-0001"} {
		if !strings.Contains(adr, want) {
			t.Fatalf("the default should be recorded with %q:\n%s", want, adr)
		}
	}
	if !strings.Contains(p.intended(), "GET /v2/charges") {
		t.Fatalf("a grade B contract change should be intended after sync:\n%s", p.intended())
	}
}

func TestContractChangeWithConsumersNeedsApproval(t *testing.T) {
	p := activeTurnProject(t, topoConsumed)
	p.start("IF-0001")
	key := "contract-changed:IF-0001"
	p.want(2, "C QUESTION "+key, "turn", "declare", "--session", "s1", "--changes", key)
	p.want(0, "approved "+key+" for CHG-0001 as ADR-0001", "approve", key)
	p.want(0, "C APPROVED "+key+" by ADR-0001", "turn", "declare", "--session", "s1", "--changes", key)
	if adr := p.read("decisions/ADR-0001.md"); !strings.Contains(adr, `question: contract-changed:IF-0001 from "GET /charges"`) {
		t.Fatalf("the ADR should name the question it answers:\n%s", adr)
	}
	p.editCode("api/http.go")
	p.extract(apiV2(topoConsumed))
	p.want(0, "C APPROVED "+key, "turn", "diff", "--session", "s1")
	p.verify("pass", 0, "PASSED")
	p.want(0, "CHG-0001 SYNCED", "turn", "end", "--session", "s1")
	if !strings.Contains(p.intended(), "GET /v2/charges") {
		t.Fatalf("the approved contract should be intended after sync:\n%s", p.intended())
	}

	p.start("IF-0001")
	p.want(2, "C QUESTION "+key, "turn", "declare", "--session", "s1", "--changes", key)
}

func TestExternalConsumersMakeAContractChangeGradeC(t *testing.T) {
	p := activeTurnProject(t, topoV1)
	p.want(1, "not an intended contract", "confirm", "MOD-0001.external_consumers", "--value", "true")
	p.want(0, "confirmed IF-0001.external_consumers true", "confirm", "IF-0001.external_consumers", "--value", "true")
	p.start("IF-0001")
	p.want(2, "C QUESTION contract-changed:IF-0001", "turn", "declare", "--session", "s1", "--changes", "contract-changed:IF-0001")
	p.editCode("api/http.go")
	p.extract(apiV2(topoV1))
	p.verify("pass", 0, "PASSED")
	p.want(2, "CHG-0001 BLOCKED", "turn", "end", "--session", "s1")
}

func TestFailedVerificationLeavesTheTurnDraft(t *testing.T) {
	p := activeTurnProject(t, topoConsumed)
	p.start("IF-0001")
	key := "contract-changed:IF-0001"
	p.want(2, "QUESTION", "turn", "declare", "--session", "s1", "--changes", key)
	p.want(0, "as ADR-0001", "approve", key)
	p.editCode("api/http.go")
	p.extract(apiV2(topoConsumed))
	p.verify("fail", 1, "run-0001 FAILED (exit 1)")
	p.want(2, "run-0001 FAILED", "turn", "end", "--session", "s1")
	p.want(0, "CHG-0001 DRAFT", "turn", "show")
	if strings.Contains(p.intended(), "/v2/") {
		t.Fatalf("a DRAFT turn must not commit its approval to the intended topology:\n%s", p.intended())
	}
	if !strings.Contains(p.read("decisions/ADR-0001.md"), "decision: approved") {
		t.Fatalf("the approval should stay in its ADR")
	}

	p.verify("pass", 0, "run-0002 PASSED")
	p.want(0, "CHG-0001 SYNCED", "turn", "end", "--session", "s1")
	if !strings.Contains(p.intended(), "/v2/") {
		t.Fatalf("a resumed DRAFT turn that passes should sync:\n%s", p.intended())
	}
}

func TestMissingVerificationLeavesTheTurnDraft(t *testing.T) {
	p := activeTurnProject(t, topoV1)
	p.start("MOD-0001")
	p.editCode("billing/charge.go")
	p.want(1, "names no verify command", "turn", "verify", "--session", "s1")
	p.want(2, "no verification ran in this turn", "turn", "end", "--session", "s1")
	p.want(0, "CHG-0001 DRAFT", "turn", "show")
}

func TestEditAfterVerifyBlocksCompletion(t *testing.T) {
	p := activeTurnProject(t, topoV1)
	p.start("MOD-0001")
	p.editCode("billing/charge.go")
	p.verify("pass", 0, "PASSED")
	write(t, filepath.Join(p.root, "billing", "charge.go"), "package billing\n// edited after verify\n")
	p.want(2, "the working tree changed after run-0001 passed", "turn", "end", "--session", "s1")
	p.verify("pass", 0, "run-0002 PASSED")
	p.want(0, "CHG-0001 SYNCED", "turn", "end", "--session", "s1")
}

func TestVerifyCommandIsFixedOnceRegistered(t *testing.T) {
	p := activeTurnProject(t, topoV1)
	p.start("MOD-0001")
	p.verify("pass", 0, "registered the verify command")
	var out, errb bytes.Buffer
	if code := run([]string{"turn", "verify", "--session", "s1", "--root", p.root, "--", "true"}, nil, &out, &errb); code != 1 || !strings.Contains(errb.String(), "the verify command is fixed") {
		t.Fatalf("a second verify command should be refused, exit %d: %s", code, errb.String())
	}
	if !strings.Contains(p.read("project.yaml"), "fake-verify") {
		t.Fatalf("the verify command should stay recorded:\n%s", p.read("project.yaml"))
	}
}

func TestReportShowsDeclaredVersusActual(t *testing.T) {
	p := activeTurnProject(t, topoV1)
	p.start("MOD-0001")
	p.want(0, "B PROCEED", "turn", "declare", "--session", "s1", "--units", "MOD-0002", "--changes", "contract-changed:IF-0001")
	p.editCode("billing/charge.go")
	p.verify("pass", 0, "PASSED")
	out := p.want(0, "CHG-0001 SYNCED", "turn", "end", "--session", "s1")
	for _, want := range []string{
		"declared but not done: MOD-0002",
		"declared but not done: contract-changed:IF-0001",
		"done but not declared: MOD-0001",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("the report should contain %q:\n%s", want, out)
		}
	}
	if chg := p.read("changes/CHG-0001.md"); !strings.Contains(chg, "## Declared but not done") || !strings.Contains(chg, "## Done but not declared") {
		t.Fatalf("the turn record should keep the report:\n%s", chg)
	}
}

func TestTurnCompletesOnlyThroughEnd(t *testing.T) {
	p := activeTurnProject(t, topoV1)
	p.start("IF-0001")
	p.want(0, "", "turn", "declare", "--session", "s1", "--changes", "contract-changed:IF-0001")
	p.editCode("api/http.go")
	p.extract(apiV2(topoV1))
	p.verify("pass", 0, "PASSED")

	p.want(0, "CHG-0001 OPEN", "turn", "show")
	if strings.Contains(p.intended(), "/v2/") {
		t.Fatalf("verification alone must not complete the turn")
	}
	p.want(1, "CHG-0001 is OPEN", "turn", "start", "--session", "s1", "--seeds", "MOD-0001", "--request", "next")
	p.want(0, "CHG-0001 SYNCED", "turn", "end", "--session", "s1")
	if !strings.Contains(p.intended(), "/v2/") {
		t.Fatalf("turn end should complete it:\n%s", p.intended())
	}
}

func TestReadOnlyTurnClosesWithoutAGate(t *testing.T) {
	p := activeTurnProject(t, topoV1)
	p.start("MOD-0001")
	p.want(0, "", "turn", "declare", "--session", "s1", "--units", "MOD-0001")
	p.want(0, "CHG-0001 CLOSED", "turn", "end", "--session", "s1")
	if chg := p.read("changes/CHG-0001.md"); !strings.Contains(chg, "state: CLOSED") || !strings.Contains(chg, "nothing to gate") {
		t.Fatalf("a read-only turn should be recorded as CLOSED:\n%s", chg)
	}
	p.start("MOD-0001")
}

func TestQuestionsPerTurnAreCapped(t *testing.T) {
	p := activeTurnProject(t, topoV1)
	p.start("MOD-0001")
	four := "new-relation:MOD-0001.depends_on.MOD-0002,new-relation:MOD-0002.depends_on.MOD-0001,new-relation:MOD-0002.consumes.IF-0001,unit-removed:IF-0001"
	p.want(1, "at most 3", "turn", "declare", "--session", "s1", "--changes", four)
	if strings.Contains(p.read("changes/CHG-0001.md"), "questions:") {
		t.Fatalf("a refused declaration must record nothing")
	}
	p.want(2, "QUESTION", "turn", "declare", "--session", "s1", "--changes", "new-relation:MOD-0001.depends_on.MOD-0002,new-relation:MOD-0002.depends_on.MOD-0001")
	p.want(1, "at most 3", "turn", "declare", "--session", "s1", "--changes", "new-relation:MOD-0002.consumes.IF-0001,unit-removed:IF-0001")
	p.want(1, "does not change the intended topology", "turn", "declare", "--session", "s1", "--changes", "new-relation:MOD-0001.provides.IF-0001")
}

func TestAnsweredQuestionIsNotAskedAgain(t *testing.T) {
	p := activeTurnProject(t, topoV1)
	edge := "new-relation:MOD-0001.depends_on.MOD-0002"
	p.start("MOD-0001")
	p.want(2, "QUESTION", "turn", "declare", "--session", "s1", "--changes", edge)
	p.want(0, "as ADR-0001", "approve", edge)
	p.editCode("billing/charge.go")
	p.want(2, "CHG-0001 DRAFT", "turn", "end", "--session", "s1")

	p.want(0, "CHG-0002 OPEN", "turn", "start", "--session", "s1", "--seeds", "MOD-0001", "--request", "again")
	p.want(0, "C APPROVED "+edge+" by ADR-0001, answered before", "turn", "declare", "--session", "s1", "--changes", edge)
}

// Scenario 10: an empty directory, stated units, then two turns that both sync.
func TestGreenfieldTwoTurns(t *testing.T) {
	root := t.TempDir()
	git(t, root, "init", "-q")
	write(t, filepath.Join(root, ".claude", "settings.json"), "{}")
	write(t, filepath.Join(root, "CLAUDE.md"), "# project")
	stated := `{"units":[
 {"key":"billing","kind":"MOD","name":"billing","anchors":[{"path":"billing/charge.go","symbol":"Charge"}]},
 {"key":"api","kind":"IF","name":"http api","signature":"GET /charges","anchors":[{"path":"api/http.go","symbol":"Serve"}]}
],"relations":[{"from":"billing","to":"api","kind":"provides"}]}`
	write(t, filepath.Join(root, ".abbd", "extractor", "stated.json"), stated)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	p := &turnProject{
		project: &project{t: t, root: root, extraction: filepath.Join(t.TempDir(), "extraction.json")},
		exe:     exe, verifyFile: filepath.Join(t.TempDir(), "verify"),
	}
	t.Setenv("ABBD_FAKE_EXTRACT", p.extraction)
	t.Setenv("ABBD_FAKE_VERIFY", p.verifyFile)
	p.setup(0, "aligned: 2", "--greenfield", ".abbd/extractor/stated.json")
	p.want(0, "active", "mode", "on", "--session", "s1")

	p.start("MOD-0001")
	write(t, filepath.Join(root, "billing", "charge.go"), "package billing\n\nfunc Charge() {}\n")
	write(t, filepath.Join(root, "api", "http.go"), "package api\n\nfunc Serve() {}\n")
	p.want(1, "names no extraction script", "turn", "diff", "--session", "s1")
	p.extract(stated)
	p.setup(0, "registered the extraction script", "--", exe)
	p.want(0, "", "turn", "diff", "--session", "s1")
	p.verify("pass", 0, "PASSED")
	p.want(0, "CHG-0001 SYNCED", "turn", "end", "--session", "s1")

	p.start("IF-0001")
	p.want(0, "B PROCEED", "turn", "declare", "--session", "s1", "--changes", "contract-changed:IF-0001")
	write(t, filepath.Join(root, "api", "http.go"), "package api\n\nfunc Serve() {} // v2\n")
	p.extract(apiV2(stated))
	p.verify("pass", 0, "PASSED")
	p.want(0, "CHG-0002 SYNCED", "turn", "end", "--session", "s1")
	if !strings.Contains(p.intended(), "GET /v2/charges") {
		t.Fatalf("the second turn's change should be intended:\n%s", p.intended())
	}
}

func TestGreenfieldModeOnRefusesCodeWithoutAScript(t *testing.T) {
	root := t.TempDir()
	p := &project{t: t, root: root}
	stated := p.file(t, "stated.json", `{"units":[{"kind":"MOD","name":"billing","anchors":[{"path":"billing/charge.go"}]}]}`)
	p.setup(0, "aligned: 1", "--greenfield", stated)
	write(t, filepath.Join(root, "billing", "charge.go"), "package billing\n")
	p.want(1, "names no extraction script", "mode", "on", "--session", "s1")
}

// Deleting a contract's only consumer must not pass as a realignment, or the
// next turn changes the contract as if no one had ever consumed it.
func TestRemovingAConsumerCannotLaunderAContractChange(t *testing.T) {
	p := activeTurnProject(t, topoConsumed)
	p.start("MOD-0001")
	p.editCode("reports/log.go")
	p.extract(topoV1)
	p.verify("pass", 0, "PASSED")
	edge := "removed-relation:MOD-0003.consumes.IF-0001"
	p.want(2, "C BLOCKED "+edge, "turn", "end", "--session", "s1")
	if !strings.Contains(p.intended(), "kind: consumes") {
		t.Fatalf("a blocked turn must keep the consumer intended:\n%s", p.intended())
	}
	p.want(0, "approved "+edge, "approve", edge)
	p.want(0, "CHG-0001 SYNCED", "turn", "end", "--session", "s1")
	if strings.Contains(p.intended(), "kind: consumes") {
		t.Fatalf("an approved removal should drop the consumer:\n%s", p.intended())
	}
}

func TestApproveDuringADraftTurnWaitsForTheTurn(t *testing.T) {
	p := activeTurnProject(t, topoV1)
	p.start("MOD-0001")
	p.editCode("billing/charge.go")
	p.verify("fail", 1, "FAILED")
	p.want(2, "CHG-0001 DRAFT", "turn", "end", "--session", "s1")
	edge := "new-relation:MOD-0001.depends_on.MOD-0002"
	p.extract(withLedgerDep)
	p.want(2, "C BLOCKED "+edge, "turn", "diff", "--session", "s1")
	p.want(0, "approved "+edge+" for CHG-0001", "approve", edge)
	if strings.Contains(p.intended(), "depends_on") {
		t.Fatalf("an approval in a DRAFT turn enters the intended topology only when the turn syncs:\n%s", p.intended())
	}
	p.verify("pass", 0, "PASSED")
	p.want(0, "C APPROVED "+edge, "turn", "end", "--session", "s1")
	if !strings.Contains(p.intended(), "kind: depends_on") {
		t.Fatalf("the approved relation should be intended after sync:\n%s", p.intended())
	}
}

var withLedgerDep = strings.Replace(topoV1, `{"from":"billing","to":"api","kind":"provides"}`,
	`{"from":"billing","to":"api","kind":"provides"},{"from":"billing","to":"ledger","kind":"depends_on"}`, 1)

func TestDependsOnIntoAContractIsAConsumer(t *testing.T) {
	topo := strings.Replace(topoV1, `{"from":"billing","to":"api","kind":"provides"}`,
		`{"from":"billing","to":"api","kind":"provides"},{"from":"ledger","to":"api","kind":"depends_on"}`, 1)
	p := activeTurnProject(t, topo)
	p.start("IF-0001")
	p.want(2, "C QUESTION contract-changed:IF-0001", "turn", "declare", "--session", "s1", "--changes", "contract-changed:IF-0001")
	p.editCode("api/http.go")
	p.extract(apiV2(topo))
	p.verify("pass", 0, "PASSED")
	p.want(2, "CHG-0001 BLOCKED", "turn", "end", "--session", "s1")
}

// An answer allows a relation against the design it was given for. Once the
// edge has entered and left the intended topology, adding it again is a new question.
func TestRelationAnswerLapsesOnceTheDesignMoves(t *testing.T) {
	edge := "new-relation:MOD-0001.depends_on.MOD-0002"
	p := activeTurnProject(t, topoV1)
	p.start("MOD-0001")
	p.want(2, "C QUESTION", "turn", "declare", "--session", "s1", "--changes", edge)
	p.want(0, "approved", "approve", edge)
	p.editCode("billing/charge.go")
	p.extract(withLedgerDep)
	p.verify("pass", 0, "PASSED")
	p.want(0, "CHG-0001 SYNCED", "turn", "end", "--session", "s1")

	p.start("MOD-0001")
	write(t, filepath.Join(p.root, "billing", "charge.go"), "package billing\n// v2\n")
	p.extract(topoV1)
	p.verify("pass", 0, "PASSED")
	p.want(0, "B PROCEED removed-relation:MOD-0001.depends_on.MOD-0002", "turn", "end", "--session", "s1")

	p.start("MOD-0001")
	p.want(2, "C QUESTION "+edge, "turn", "declare", "--session", "s1", "--changes", edge)
}

func TestVerifyThatEditsTheTreeFails(t *testing.T) {
	p := activeTurnProject(t, topoV1)
	p.start("MOD-0001")
	p.editCode("billing/charge.go")
	p.verify("pass edit "+filepath.Join(p.root, "billing", "charge.go"), 1, "it wrote billing/charge.go")
	if run := p.read("verification/runs/run-0001.yaml"); !strings.Contains(run, "result: FAILED") {
		t.Fatalf("a verify that edits the tree should be recorded FAILED:\n%s", run)
	}
	p.want(2, "run-0001 FAILED", "turn", "end", "--session", "s1")
}

func TestPrunedBaselineStillGatesTheTurn(t *testing.T) {
	p := activeTurnProject(t, topoV1)
	p.editCode("ledger/uncommitted.go")
	p.start("MOD-0001")
	p.want(0, "", "turn", "declare", "--session", "s1", "--units", "MOD-0001")
	p.editCode("billing/charge.go")
	p.verify("pass", 0, "PASSED")
	git(t, p.root, "prune", "--expire=now")
	out := p.want(0, "CHG-0001 SYNCED", "turn", "end", "--session", "s1")
	if !strings.Contains(out, "baseline snapshot") || strings.Contains(out, "declared but not done: MOD-0001") {
		t.Fatalf("a lost baseline should be reported, not read as an untouched unit:\n%s", out)
	}
}

// The tree fingerprint leaves .abbd/ out, so the end gate checks the
// extraction script itself: a turn may not end on a script it changed, in
// .abbd/extractor/ or wherever project.yaml names it.
func TestTurnCannotEndOnAChangedExtractor(t *testing.T) {
	p := activeTurnProject(t, topoV1)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	helper := filepath.Join(p.root, ".abbd", "extractor", "helper.txt")
	rules := filepath.Join(p.root, "tools", "rules.txt")
	write(t, helper, "strict\n")
	write(t, rules, "strict\n")
	git(t, p.root, "add", "-A")
	git(t, p.root, "commit", "-qm", "rules")
	write(t, filepath.Join(p.root, ".abbd", "project.yaml"), fmt.Sprintf("extract: [%q, tools/rules.txt]\n", exe))
	p.start("MOD-0001")
	p.editCode("billing/charge.go")
	for _, f := range []string{helper, rules} {
		write(t, f, "anything goes\n")
		p.want(1, "the extraction script changed during CHG-0001", "turn", "end", "--session", "s1")
		if !strings.Contains(p.read("changes/CHG-0001.md"), "state: OPEN") {
			t.Fatalf("a refused end leaves the turn open:\n%s", p.read("changes/CHG-0001.md"))
		}
		write(t, f, "strict\n")
	}
	p.want(2, "CHG-0001 DRAFT", "turn", "end", "--session", "s1")
}

func TestSyncedTurnPublishesCurrentFilesForTheNewDesign(t *testing.T) {
	p := activeTurnProject(t, topoV1)
	p.start("IF-0001")
	p.editCode("api/http.go")
	p.extract(apiV2(topoV1))
	p.verify("pass", 0, "PASSED")
	p.want(0, "CHG-0001 SYNCED", "turn", "end", "--session", "s1")
	p.want(0, "generated: CURRENT", "status")
	p.want(0, "actual topology: CURRENT", "status")
	if !strings.Contains(p.intended(), "revision: 2") || !strings.Contains(p.read("generated/graph.json"), `"intended_revision": 2`) {
		t.Fatalf("the published files should carry the design the turn synced:\n%s", p.read("generated/graph.json"))
	}
	if !strings.Contains(p.read("generated/explorer.html"), "GET /v2/charges") {
		t.Fatalf("the explorer should show the synced contract")
	}
}

var withDigest = func(topo, digest string) string {
	return strings.Replace(topo, `"signature":"GET /charges",`, `"signature":"GET /charges","digest":"`+digest+`",`, 1)
}

// A turn that rewrites a consumed contract's code without changing its
// signature may still change what it means; the gate cannot read meaning, so
// it proceeds and leaves a human a grade B card.
func TestSilentChangeToAConsumedContractLeavesAGradeBCard(t *testing.T) {
	p := activeTurnProject(t, withDigest(topoConsumed, "d1"))
	p.start("MOD-0001")
	p.editCode("api/http.go")
	p.extract(withDigest(topoConsumed, "d2"))
	p.verify("pass", 0, "PASSED")
	out := p.want(0, "CHG-0001 SYNCED", "turn", "end", "--session", "s1")
	for _, want := range []string{"B PROCEED contract-touched:IF-0001"} {
		if !strings.Contains(out, want) {
			t.Fatalf("want %q in:\n%s", want, out)
		}
	}
	if adr := p.read("decisions/ADR-0001.md"); !strings.Contains(adr, "subject: contract-touched:IF-0001") || !strings.Contains(adr, "grade: B") {
		t.Fatalf("the silent change should be on record for a human:\n%s", adr)
	}
}

func TestChangeToAContractNobodyConsumesLeavesNoCard(t *testing.T) {
	p := activeTurnProject(t, withDigest(topoV1, "d1"))
	p.start("MOD-0001")
	p.editCode("api/http.go")
	p.extract(withDigest(topoV1, "d2"))
	p.verify("pass", 0, "PASSED")
	if out := p.want(0, "CHG-0001 SYNCED", "turn", "end", "--session", "s1"); strings.Contains(out, "contract-touched") {
		t.Fatalf("a contract no one consumes can change its code freely:\n%s", out)
	}
}

// Units that share a file are told apart by digest: only the one whose code
// changed counts as touched.
func TestDigestTellsApartUnitsThatShareAFile(t *testing.T) {
	admin := `{"key":"admin","kind":"IF","name":"admin api","signature":"GET /admin","digest":"a1","anchors":[{"path":"api/http.go","symbol":"Admin"}]},`
	p := activeTurnProject(t, withUnit(withDigest(topoV1, "d1"), admin))
	p.start("MOD-0001")
	p.editCode("api/http.go")
	p.extract(withUnit(withDigest(topoV1, "d2"), admin))
	p.verify("pass", 0, "PASSED")
	out := p.want(0, "CHG-0001 SYNCED", "turn", "end", "--session", "s1")
	if strings.Count(out, "done but not declared: IF-") != 1 {
		t.Fatalf("only the unit whose digest changed was touched:\n%s", out)
	}
}
