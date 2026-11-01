package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// neverRead fails the test if a command reads its stdin, which is how a CLI asks.
type neverRead struct{ t *testing.T }

func (r neverRead) Read([]byte) (int, error) {
	r.t.Fatal("setup must never ask: it read stdin")
	return 0, nil
}

// bare is a code-only repository with no .abbd at all.
func bare(t *testing.T) (*project, string) {
	t.Helper()
	p := &project{t: t, root: newRepo(t), extraction: filepath.Join(t.TempDir(), "extraction.json")}
	t.Setenv("ABBD_FAKE_EXTRACT", p.extraction)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return p, exe
}

func (p *project) setup(want int, contains string, args ...string) string {
	p.t.Helper()
	var out, errb bytes.Buffer
	code := run(append([]string{"setup", "--root", p.root}, args...), neverRead{p.t}, &out, &errb)
	got := out.String() + errb.String()
	if code != want || !strings.Contains(got, contains) {
		p.t.Fatalf("abbd setup %v: want exit %d containing %q, got exit %d:\n%s", args, want, contains, code, got)
	}
	return got
}

func (p *project) file(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	write(t, path, content)
	return path
}

// units returns the unit files only, the actual topology.
func units(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for k, v := range snapshot(t, root) {
		if strings.HasPrefix(k, "architecture/") || strings.HasPrefix(k, "features/") {
			out[k] = v
		}
	}
	return out
}

const codeOnly = `{"units":[
 {"key":"billing","kind":"MOD","name":"billing","anchors":[{"path":"billing/charge.go","symbol":"Charge"}],"purpose":"takes payments"},
 {"key":"ledger","kind":"MOD","name":"ledger","anchors":[{"path":"ledger/book.go","symbol":"Post"}]},
 {"key":"api","kind":"IF","name":"http api","signature":"GET /charges","anchors":[{"path":"api/http.go","symbol":"Serve"}]}
],"relations":[
 {"from":"billing","to":"api","kind":"provides"},
 {"from":"billing","to":"ledger","kind":"depends_on"}
]}`

// The swarm sees that ledger reads the table billing writes; no import shows it.
const supplement = `{"units":[
 {"key":"b","kind":"MOD","name":"billing","anchors":[{"path":"billing/charge.go","symbol":"Charge"}],"responsibility":"owns the charges table"},
 {"key":"l","kind":"MOD","name":"ledger","anchors":[{"path":"ledger/book.go","symbol":"Post"}],"purpose":"keeps the books"}
],"relations":[
 {"from":"l","to":"b","kind":"consumes"}
]}`

func TestCodeOnlyRepoReachesActiveByConfirmingDesignUnitsOnly(t *testing.T) {
	p, exe := bare(t)
	p.extract(codeOnly)
	p.setup(0, "bootstrapped: 3 design unit candidate(s)", "--", exe)
	if !strings.Contains(p.read("project.yaml"), filepath.Base(exe)) {
		t.Fatalf("setup should record the extraction script:\n%s", p.read("project.yaml"))
	}
	if r := p.read("bootstrap-report.md"); !strings.Contains(r, "MOD-0001 billing") || !strings.Contains(r, "abbd align --all") || !strings.Contains(r, "Three runs") {
		t.Fatalf("the bootstrap report should list candidates and the next step:\n%s", r)
	}
	p.want(2, "bootstrapped: run setup and align", "mode", "on", "--session", "s1")
	p.want(2, "MOD-0002 ledger", "align")

	p.want(0, "aligned: 2 design unit(s)", "align", "--exclude", "MOD-0002")
	out := p.want(0, "active", "mode", "on", "--session", "s1")
	if !strings.Contains(out, "PROMOTE unit-added:MOD-0002") {
		t.Fatalf("the unit left out should stay below the line as a promotion candidate:\n%s", out)
	}
	p.want(0, "MOD-0001.purpose \"takes payments\" (design unit)", "confirm", "--list")
	if !strings.Contains(p.read("architecture/MOD-0001.md"), "confirmation: UNCONFIRMED") {
		t.Fatalf("an inferred purpose should stay unconfirmed; alignment does not need it")
	}
}

func TestConfirmingAContractAloneStillAligns(t *testing.T) {
	p, exe := bare(t)
	p.extract(codeOnly)
	p.setup(0, "bootstrapped", "--", exe)
	p.want(1, "not a design unit candidate", "align", "--confirm", "MOD-0009")
	p.want(0, "aligned: 1 design unit(s)", "align", "--confirm", "IF-0001")
	p.want(0, "active", "mode", "on", "--session", "s1")
	p.want(1, "choose the first design units only", "align", "--all")
}

func TestSupplementFeedsTheConfirmQueueAndSurvivesReextraction(t *testing.T) {
	p, exe := bare(t)
	p.extract(codeOnly)
	p.setup(0, "4 item(s) in the confirm queue", "--supplement", p.file(t, "sup.json", supplement), "--", exe)
	if md := p.read("architecture/MOD-0002.md"); !strings.Contains(md, "origin: INFERRED") || !strings.Contains(md, "to: MOD-0001") {
		t.Fatalf("the supplement should add an inferred purpose and coupling:\n%s", md)
	}
	queue := p.want(0, "MOD-0002.consumes.MOD-0001", "confirm", "--list")
	if !strings.Contains(queue, "MOD-0001.responsibility") || !strings.Contains(queue, "MOD-0002.purpose") {
		t.Fatalf("supplement why-fields should wait for confirmation:\n%s", queue)
	}

	p.want(0, "aligned", "align", "--all")
	if strings.Contains(p.read("intended.yaml"), "consumes") {
		t.Fatalf("an unconfirmed coupling must not enter the intended topology:\n%s", p.read("intended.yaml"))
	}
	p.want(0, "active", "mode", "on", "--session", "s1")
	if !strings.Contains(p.read("architecture/MOD-0002.md"), "kind: consumes") {
		t.Fatalf("re-extraction by the script must keep a coupling the script cannot see")
	}

	p.want(0, "confirmed MOD-0001.responsibility", "confirm", "MOD-0001.responsibility")
	p.want(0, "confirmed MOD-0002.purpose", "confirm", "MOD-0002.purpose", "--value", "posts journal entries")
	md1, md2 := p.read("architecture/MOD-0001.md"), p.read("architecture/MOD-0002.md")
	if !strings.Contains(md1, "owns the charges table\n    origin: INFERRED\n    confirmation: CONFIRMED") {
		t.Fatalf("confirming as written keeps the origin and records the confirmation:\n%s", md1)
	}
	if !strings.Contains(md2, "posts journal entries\n    origin: USER_STATED") {
		t.Fatalf("a human-given value should be USER_STATED:\n%s", md2)
	}

	p.want(0, "confirmed MOD-0001.purpose", "confirm", "MOD-0001.purpose")
	p.want(0, "approved new-relation:MOD-0002.consumes.MOD-0001", "confirm", "MOD-0002.consumes.MOD-0001")
	p.want(0, "active", "mode", "on", "--session", "s1")
	if q := p.want(0, "", "confirm", "--list"); strings.TrimSpace(q) != "" {
		t.Fatalf("every item was confirmed, queue:\n%s", q)
	}
}

func TestConfirmedCouplingBeforeAlignmentBecomesIntended(t *testing.T) {
	p, exe := bare(t)
	p.extract(codeOnly)
	p.setup(0, "bootstrapped", "--supplement", p.file(t, "sup.json", supplement), "--", exe)
	p.want(0, "confirmed", "confirm", "MOD-0002.consumes.MOD-0001")
	p.want(0, "aligned", "align", "--all")
	if !strings.Contains(p.read("intended.yaml"), "kind: consumes") {
		t.Fatalf("a coupling a human registered at alignment should be intended:\n%s", p.read("intended.yaml"))
	}
	p.want(0, "active", "mode", "on", "--session", "s1")
}

func TestSupplementNamingUnknownCodeIsRefused(t *testing.T) {
	p, exe := bare(t)
	p.extract(codeOnly)
	bad := `{"units":[{"key":"x","kind":"MOD","name":"ghost","anchors":[{"path":"ghost/x.go","symbol":"X"}],"purpose":"haunts"}]}`
	p.setup(1, "fix the script", "--supplement", p.file(t, "sup.json", bad), "--", exe)
	if _, err := os.Stat(filepath.Join(p.root, ".abbd")); err == nil {
		t.Fatalf("a refused setup must write nothing")
	}
}

func TestAnotherSessionReusesTheBlueprint(t *testing.T) {
	p, exe := bare(t)
	p.extract(codeOnly)
	p.setup(0, "bootstrapped", "--", exe)
	p.want(0, "aligned", "align", "--all")
	p.want(0, "active", "mode", "on", "--session", "s1")
	before := units(t, p.root)

	p.want(0, "active", "mode", "on", "--session", "s2")
	p.want(0, "mode: on", "status", "--session", "s2")
	p.hook("s2", "billing/charge.go")
	if !strings.Contains(p.journal("s2"), "changed billing/charge.go") {
		t.Fatalf("a second session should work on the stored blueprint without a new setup")
	}
	if after := units(t, p.root); !equal(before, after) {
		t.Fatalf("a second session must not rewrite the blueprint")
	}
}

const adoptDoc = `{"units":[
 {"key":"b","kind":"MOD","name":"Billing service","anchors":[{"path":"billing/charge.go","symbol":"Charge"}],"purpose":"takes card payments"},
 {"key":"a","kind":"IF","name":"Charges API","signature":"GET /v1/charges","anchors":[{"path":"api/http.go","symbol":"Serve"}]},
 {"key":"n","kind":"MOD","name":"notifier","anchors":[{"path":"notify/mail.go","symbol":"Send"}]}
],"relations":[
 {"from":"b","to":"a","kind":"consumes"}
]}`

func TestAdoptRecordsOriginAndReportsConflictsAndGaps(t *testing.T) {
	p, exe := bare(t)
	p.extract(codeOnly)
	p.setup(0, "bootstrapped", "--adopt", p.file(t, "doc.json", adoptDoc), "--", exe)
	md := p.read("architecture/MOD-0001.md")
	if !strings.Contains(md, "value: Billing service\n    origin: ADOPTED") || !strings.Contains(md, "takes card payments\n    origin: ADOPTED") {
		t.Fatalf("adopted name and purpose should carry origin ADOPTED:\n%s", md)
	}
	if !strings.Contains(p.read("architecture/IF-0001.md"), "GET /charges") {
		t.Fatalf("the code's signature must win over the document's")
	}
	r := p.read("bootstrap-report.md")
	for _, want := range []string{
		`documents signature "GET /v1/charges", code has "GET /charges"`,
		`adopt relation b -consumes-> a is not in the code`,
		`adopt MOD "notifier": no MOD in the code shares an anchor`,
		`code MOD "ledger" is not in the adopt document`,
	} {
		if !strings.Contains(r, want) {
			t.Fatalf("the report should say %q:\n%s", want, r)
		}
	}
	if strings.Contains(p.want(0, "", "confirm", "--list"), "MOD-0001.purpose") {
		t.Fatalf("an adopted purpose is intent, not a guess for the confirm queue")
	}
}

func TestBrokenAdoptNeverBreaksPlainBootstrap(t *testing.T) {
	plain, exe := bare(t)
	plain.extract(codeOnly)
	plain.setup(0, "bootstrapped", "--", exe)

	for name, doc := range map[string]string{"unparsable": "<html>not an extraction</html>", "missing": ""} {
		t.Run(name, func(t *testing.T) {
			p, exe := bare(t)
			p.extract(codeOnly)
			path := filepath.Join(t.TempDir(), "missing.json")
			if doc != "" {
				path = p.file(t, "doc.json", doc)
			}
			p.setup(0, "adopt not applied", "--adopt", path, "--", exe)
			if !equal(units(t, plain.root), units(t, p.root)) {
				t.Fatalf("a failed adopt must leave the code-only result untouched")
			}
			if !strings.Contains(p.read("bootstrap-report.md"), "Not applied") {
				t.Fatalf("the report should name the adopt failure:\n%s", p.read("bootstrap-report.md"))
			}
		})
	}
}

func TestReextractingUnchangedCodeKeepsTheActualTopology(t *testing.T) {
	p, exe := bare(t)
	p.extract(codeOnly)
	p.setup(0, "bootstrapped", "--supplement", p.file(t, "sup.json", supplement), "--", exe)
	first := snapshot(t, p.root)

	p.setup(0, "bootstrapped", "--supplement", p.file(t, "sup2.json", supplement))
	if again := snapshot(t, p.root); !equal(first, again) {
		t.Fatalf("setup again on unchanged code must rewrite nothing")
	}
	p.want(0, "aligned", "align", "--all")
	p.want(0, "active", "mode", "on", "--session", "s1")
	if !equal(units(t, p.root), pick(first)) {
		t.Fatalf("re-extraction at mode on must not change the actual topology")
	}
}

func TestNondeterministicScriptIsRefused(t *testing.T) {
	p, exe := bare(t)
	p.extract(strings.Replace(codeOnly, `"name":"ledger"`, `"name":"ledger {{RUN}}"`, 1))
	p.setup(1, "not deterministic", "--", exe)
	if _, err := os.Stat(filepath.Join(p.root, ".abbd")); err == nil {
		t.Fatalf("a refused setup must not record the script or any unit")
	}
}

func TestGreenfieldStatedUnitsAreAlignedAtOnce(t *testing.T) {
	root := t.TempDir()
	stated := filepath.Join(t.TempDir(), "stated.json")
	write(t, stated, `{"units":[
 {"key":"shop","kind":"MOD","name":"shop","anchors":[{"path":"shop/"}],"purpose":"sells things"},
 {"key":"cart","kind":"IF","name":"cart api","signature":"POST /cart","anchors":[{"path":"shop/cart.go"}]}
],"relations":[{"from":"shop","to":"cart","kind":"provides"}]}`)
	var out, errb bytes.Buffer
	if code := run([]string{"setup", "--root", root, "--greenfield", stated}, neverRead{t}, &out, &errb); code != 0 || !strings.Contains(out.String(), "aligned: 2") {
		t.Fatalf("greenfield setup: exit %d\n%s%s", code, out.String(), errb.String())
	}
	p := &project{t: t, root: root}
	p.want(0, "phase: aligned", "status")
	md := p.read("architecture/MOD-0001.md")
	if !strings.Contains(md, "value: shop\n    origin: USER_STATED") || !strings.Contains(md, "sells things\n    origin: USER_STATED") {
		t.Fatalf("stated units should be USER_STATED:\n%s", md)
	}
	if in := p.read("intended.yaml"); !strings.Contains(in, "kind: provides") {
		t.Fatalf("the stated relation should be intended:\n%s", in)
	}
	if code := run([]string{"setup", "--root", root, "--greenfield", stated}, neverRead{t}, &out, &errb); code != 1 {
		t.Fatalf("greenfield on a project that has units must be refused, got %d", code)
	}
}

func pick(snap map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range snap {
		if strings.HasPrefix(k, "architecture/") || strings.HasPrefix(k, "features/") {
			out[k] = v
		}
	}
	return out
}

func equal(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func TestConfirmingACouplingAfterAlignmentApprovesIt(t *testing.T) {
	p, exe := bare(t)
	p.extract(codeOnly)
	p.setup(0, "", "--supplement", p.file(t, "sup.json", supplement), "--", exe)
	p.want(0, "aligned", "align", "--all")
	p.want(0, "active", "mode", "on", "--session", "s1")
	p.want(0, "approved new-relation:MOD-0002.consumes.MOD-0001 as ADR-0001", "confirm", "MOD-0002.consumes.MOD-0001")
	if !strings.Contains(p.read("intended.yaml"), "kind: consumes") {
		t.Fatalf("a confirmed coupling between design units should be intended:\n%s", p.read("intended.yaml"))
	}
	p.want(0, "phase: aligned", "status")
	p.want(0, "active", "mode", "on", "--session", "s1")
}

func TestConfirmingACouplingToABelowTheLineUnitPointsAtRealign(t *testing.T) {
	p, exe := bare(t)
	p.extract(codeOnly)
	p.setup(0, "", "--supplement", p.file(t, "sup.json", supplement), "--", exe)
	p.want(0, "aligned", "align", "--exclude", "MOD-0001")
	p.want(0, "REALIGN new-relation:MOD-0002.consumes.MOD-0001: run abbd align", "confirm", "MOD-0002.consumes.MOD-0001")
	p.want(0, "aligned", "align")
	p.want(0, "active", "mode", "on", "--session", "s1")
}

func TestSetupResolvesFilesAgainstTheRoot(t *testing.T) {
	p, exe := bare(t)
	p.extract(codeOnly)
	write(t, filepath.Join(p.root, ".abbd", "extractor", "supplement.json"), supplement)
	p.setup(0, "4 item(s) in the confirm queue", "--supplement", ".abbd/extractor/supplement.json", "--", exe)
}

func TestGreenfieldRefusesADirectoryThatHoldsCode(t *testing.T) {
	p, _ := bare(t)
	p.setup(1, "empty directory", "--greenfield", p.file(t, "stated.json", codeOnly))
	if _, err := os.Stat(filepath.Join(p.root, ".abbd")); err == nil {
		t.Fatalf("a refused greenfield setup must write nothing")
	}
}

func TestGreenfieldIgnoresAgentHarnessFiles(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, ".claude", "settings.json"), "{}")
	write(t, filepath.Join(root, "CLAUDE.md"), "# notes")
	write(t, filepath.Join(root, "AGENTS.md"), "# notes")
	write(t, filepath.Join(root, ".gitignore"), "bin/")
	p := &project{t: t, root: root}
	stated := p.file(t, "stated.json", codeOnly)
	p.setup(0, "aligned: 3", "--greenfield", stated)

	other := t.TempDir()
	write(t, filepath.Join(other, "docs", "CLAUDE.md"), "# not at the root")
	q := &project{t: t, root: other}
	q.setup(1, "docs/CLAUDE.md is already there", "--greenfield", stated)
}

func TestAlignedProjectKeepsItsRecordedScript(t *testing.T) {
	p, exe := bare(t)
	p.extract(codeOnly)
	p.setup(0, "bootstrapped", "--", exe)
	p.want(0, "aligned", "align", "--all")
	recorded := p.read("project.yaml")

	p.setup(1, "extraction script is fixed", "--", exe, "frozen-arg")
	if got := p.read("project.yaml"); got != recorded {
		t.Fatalf("a refused setup must not replace the recorded script:\n%s", got)
	}
	p.setup(0, "aligned", "--", exe)
	p.setup(0, "aligned")
}

func greenfield(t *testing.T) (*project, string) {
	t.Helper()
	root := t.TempDir()
	p := &project{t: t, root: root, extraction: filepath.Join(t.TempDir(), "extraction.json")}
	t.Setenv("ABBD_FAKE_EXTRACT", p.extraction)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(root, ".abbd", "extractor", "stated.json"), codeOnly)
	p.setup(0, "aligned: 3", "--greenfield", ".abbd/extractor/stated.json")
	write(t, filepath.Join(root, "billing", "charge.go"), "package billing\n\nfunc Charge() {}\n")
	return p, exe
}

func TestGreenfieldRegistersItsFirstScriptOnce(t *testing.T) {
	p, exe := greenfield(t)
	p.extract(codeOnly)
	p.setup(0, "registered the extraction script", "--", exe)
	if !strings.Contains(p.read("project.yaml"), filepath.Base(exe)) {
		t.Fatalf("the first script of a greenfield project should be recorded:\n%s", p.read("project.yaml"))
	}
	p.setup(1, "extraction script is fixed", "--", exe, "other")
	p.want(0, "active", "mode", "on", "--session", "s1")
}

func TestGreenfieldRegistrationStillProvesDeterminism(t *testing.T) {
	p, exe := greenfield(t)
	p.extract(strings.Replace(codeOnly, `"name":"ledger"`, `"name":"ledger {{RUN}}"`, 1))
	p.setup(1, "not deterministic", "--", exe)
	if b, _ := os.ReadFile(filepath.Join(p.root, ".abbd", "project.yaml")); strings.Contains(string(b), filepath.Base(exe)) {
		t.Fatalf("a refused registration must not record the script")
	}
}

func TestScriptThatDependsOnTheHashSeedIsRefused(t *testing.T) {
	t.Setenv("PYTHONHASHSEED", "0")
	p, exe := bare(t)
	p.extract(strings.Replace(codeOnly, `"name":"ledger"`, `"name":"ledger{{PYTHONHASHSEED}}"`, 1))
	p.setup(1, "PYTHONHASHSEED", "--", exe)
	if _, err := os.Stat(filepath.Join(p.root, ".abbd")); err == nil {
		t.Fatalf("a refused setup must write nothing")
	}
}

func TestAdoptDuplicateUnitAddsNoSelfLoop(t *testing.T) {
	p, exe := bare(t)
	p.extract(codeOnly)
	adopt := `{"units":[
 {"key":"a","kind":"MOD","name":"pay","anchors":[{"path":"billing/charge.go","symbol":"Charge"}]},
 {"key":"b","kind":"MOD","name":"pay2","anchors":[{"path":"billing/charge.go","symbol":"Charge"}]}
],"relations":[{"from":"a","to":"b","kind":"depends_on"}]}`
	p.setup(0, "bootstrapped", "--adopt", p.file(t, "a.json", adopt), "--", exe)
	if md := p.read("architecture/MOD-0001.md"); strings.Contains(md, "to: MOD-0001") {
		t.Fatalf("a duplicate adopt unit must not turn its relation into a self-loop:\n%s", md)
	}
	if q := p.want(0, "", "confirm", "--list"); strings.Contains(q, "MOD-0001.depends_on.MOD-0001") {
		t.Fatalf("a self-loop must never reach the confirm queue:\n%s", q)
	}
	r := p.read("bootstrap-report.md")
	if !strings.Contains(r, "describes the same code") || !strings.Contains(r, "adopt relation a -depends_on-> b names a unit that matched no code") {
		t.Fatalf("the report should name the duplicate and the relation it orphaned:\n%s", r)
	}
}

func TestAdoptRelationAlsoGuessedBySupplementIsAdopted(t *testing.T) {
	p, exe := bare(t)
	p.extract(codeOnly)
	adopt := strings.ReplaceAll(supplement, `"responsibility":"owns the charges table"`, `"purpose":"x"`)
	p.setup(0, "bootstrapped", "--supplement", p.file(t, "sup.json", supplement), "--adopt", p.file(t, "a.json", adopt), "--", exe)
	if r := p.read("bootstrap-report.md"); !strings.Contains(r, "adopt relation l -consumes-> b is not in the code") {
		t.Fatalf("an adopted relation the code lacks is a conflict even when the supplement guessed it too:\n%s", r)
	}
	if md := p.read("architecture/MOD-0002.md"); !strings.Contains(md, "kind: consumes\n      origin: ADOPTED") {
		t.Fatalf("the document's relation should carry origin ADOPTED:\n%s", md)
	}
}

func TestCorrectedSupplementWithdrawsEarlierGuesses(t *testing.T) {
	p, exe := bare(t)
	p.extract(codeOnly)
	p.setup(0, "", "--supplement", p.file(t, "sup.json", supplement), "--", exe)
	corrected := `{"units":[
 {"key":"b","kind":"MOD","name":"billing","anchors":[{"path":"billing/charge.go","symbol":"Charge"}],"responsibility":"owns the refunds table"}
]}`
	p.setup(0, "", "--supplement", p.file(t, "sup2.json", corrected))
	q := p.want(0, `MOD-0001.responsibility "owns the refunds table"`, "confirm", "--list")
	if strings.Contains(q, "consumes") || strings.Contains(q, "MOD-0002.purpose") {
		t.Fatalf("guesses the corrected supplement dropped must leave the queue:\n%s", q)
	}
	if md := p.read("architecture/MOD-0002.md"); strings.Contains(md, "consumes") || strings.Contains(md, "keeps the books") {
		t.Fatalf("withdrawn guesses must leave the unit:\n%s", md)
	}
}

func TestConfirmedGuessesSurviveALaterSupplement(t *testing.T) {
	p, exe := bare(t)
	p.extract(codeOnly)
	p.setup(0, "", "--supplement", p.file(t, "sup.json", supplement), "--", exe)
	p.want(0, "confirmed", "confirm", "MOD-0002.consumes.MOD-0001")
	p.want(0, "confirmed", "confirm", "MOD-0002.purpose")
	p.setup(0, "")
	md := p.read("architecture/MOD-0002.md")
	if !strings.Contains(md, "kind: consumes") || !strings.Contains(md, "keeps the books") {
		t.Fatalf("a human confirmation must outlive the supplement that proposed it:\n%s", md)
	}
}

func TestSetupRegistersTheVerifyCommandATurnThenRuns(t *testing.T) {
	p, exe := bare(t)
	p.extract(codeOnly)
	verify := "'" + exe + "' fake-verify"
	p.setup(0, "registered the verify command", "--verify", verify, "--", exe)
	recorded := p.read("project.yaml")
	if !strings.Contains(recorded, "fake-verify") {
		t.Fatalf("setup should record the verify command:\n%s", recorded)
	}
	p.setup(1, "the verify command is fixed", "--verify", "true", "--", exe)
	p.setup(0, "bootstrapped", "--verify", verify, "--", exe)
	if got := p.read("project.yaml"); got != recorded {
		t.Fatalf("a refused or repeated --verify must leave project.yaml as it was:\n%s", got)
	}

	p.want(0, "aligned", "align", "--all")
	p.want(0, "active", "mode", "on", "--session", "s1")
	verifyFile := filepath.Join(t.TempDir(), "verify")
	t.Setenv("ABBD_FAKE_VERIFY", verifyFile)
	write(t, verifyFile, "pass")
	p.want(0, "OPEN", "turn", "start", "--session", "s1", "--seeds", "MOD-0001", "--request", "make the change")
	p.want(0, "run-0001 PASSED", "turn", "verify", "--session", "s1")
}

func TestSetupRefusesAVerifyCommandThatIsNotOneCommand(t *testing.T) {
	p, exe := bare(t)
	p.extract(codeOnly)
	p.setup(1, "one plain command", "--verify", "go test ./... | tee out", "--", exe)
	if _, err := os.Stat(filepath.Join(p.root, ".abbd", "project.yaml")); !os.IsNotExist(err) {
		t.Fatalf("a refused --verify must record nothing, got %v", err)
	}
}

// A project commits .abbd/ with its code. Turning the mode on afterwards
// must leave that commit clean, or every commit would ask for another.
func TestCommittedBlueprintStaysCleanWhenTheModeTurnsOn(t *testing.T) {
	p, exe := bare(t)
	p.extract(codeOnly)
	p.setup(0, "bootstrapped", "--", exe)
	p.want(0, "aligned", "align", "--all")
	git(t, p.root, "add", "-A")
	git(t, p.root, "commit", "-qm", "blueprint")
	p.want(0, "active", "mode", "on", "--session", "s1")
	out, err := exec.Command("git", "-C", p.root, "status", "--porcelain").CombinedOutput()
	if err != nil || len(out) != 0 {
		t.Fatalf("mode on after committing .abbd/ should change nothing git sees, got %v:\n%s", err, out)
	}
}
