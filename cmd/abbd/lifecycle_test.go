package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The test binary doubles as the project extraction script and, given the
// fake-verify argument, as the project verify command.
func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "fake-verify" {
		b, _ := os.ReadFile(os.Getenv("ABBD_FAKE_VERIFY"))
		// "pass edit PATH" passes after writing to PATH, as a verify that formats code would.
		result, edit, _ := strings.Cut(strings.TrimSpace(string(b)), " edit ")
		if edit != "" {
			os.WriteFile(edit, []byte("package x\n// formatted\n"), 0o644)
		}
		if result != "pass" {
			fmt.Println("FAIL: 1 test failed")
			os.Exit(1)
		}
		fmt.Println("ok: all tests passed")
		os.Exit(0)
	}
	if p := os.Getenv("ABBD_FAKE_EXTRACT"); p != "" {
		b, err := os.ReadFile(p)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		// {{RUN}} plays a script whose output changes on every run.
		if bytes.Contains(b, []byte("{{RUN}}")) {
			n, _ := os.ReadFile(p + ".n")
			n = append(n, 'x')
			os.WriteFile(p+".n", n, 0o644)
			b = bytes.ReplaceAll(b, []byte("{{RUN}}"), n)
		}
		b = bytes.ReplaceAll(b, []byte("{{PYTHONHASHSEED}}"), []byte(os.Getenv("PYTHONHASHSEED")))
		os.Stdout.Write(b)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

type project struct {
	t          *testing.T
	root       string
	extraction string
}

func newProject(t *testing.T) *project {
	t.Helper()
	p := &project{t: t, root: newRepo(t), extraction: filepath.Join(t.TempDir(), "extraction.json")}
	t.Setenv("ABBD_FAKE_EXTRACT", p.extraction)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(p.root, ".abbd", "project.yaml"), fmt.Sprintf("extract: [%q]\n", exe))
	return p
}

// extract sets what the extraction script reports next time it runs.
func (p *project) extract(json string) { write(p.t, p.extraction, json) }

func (p *project) cmd(stdin string, args ...string) (int, string) {
	p.t.Helper()
	var out, errb bytes.Buffer
	code := run(append(args, "--root", p.root), strings.NewReader(stdin), &out, &errb)
	return code, out.String() + errb.String()
}

func (p *project) want(code int, contains string, args ...string) string {
	p.t.Helper()
	got, out := p.cmd("", args...)
	if got != code || !strings.Contains(out, contains) {
		p.t.Fatalf("abbd %v: want exit %d containing %q, got exit %d:\n%s", args, code, contains, got, out)
	}
	return out
}

func (p *project) hook(session, file string) {
	p.t.Helper()
	in := fmt.Sprintf(`{"session_id":%q,"tool_name":"Edit","tool_input":{"file_path":%q}}`, session, file)
	if code, out := p.cmd(in, "hook", "PostToolUse"); code != 0 || out != "" {
		p.t.Fatalf("hook must exit 0 silently, got %d: %s", code, out)
	}
}

func (p *project) journal(session string) string {
	b, _ := os.ReadFile(filepath.Join(p.root, ".abbd", "sessions", session+".journal"))
	return string(b)
}

// editCode makes a change outside the mode so the working tree differs.
func (p *project) editCode(name string) {
	write(p.t, filepath.Join(p.root, name), "package x\n// "+name+"\n")
}

func (p *project) read(rel string) string {
	b, err := os.ReadFile(filepath.Join(p.root, ".abbd", rel))
	if err != nil {
		p.t.Fatal(err)
	}
	return string(b)
}

const topoV1 = `{"units":[
 {"key":"billing","kind":"MOD","name":"billing","anchors":[{"path":"billing/charge.go","symbol":"Charge"}]},
 {"key":"ledger","kind":"MOD","name":"ledger","anchors":[{"path":"ledger/book.go","symbol":"Post"}]},
 {"key":"api","kind":"IF","name":"http api","signature":"GET /charges","anchors":[{"path":"api/http.go","symbol":"Serve"}]}
],"relations":[
 {"from":"billing","to":"api","kind":"provides"}
]}`

func aligned(t *testing.T) *project {
	t.Helper()
	p := newProject(t)
	p.extract(topoV1)
	p.want(0, "MOD-0001 new", "ingest", p.extraction)
	p.want(0, "aligned", "align", "--all")
	return p
}

func TestModeIsInertUntilAligned(t *testing.T) {
	p := newProject(t)
	p.extract(topoV1)
	p.want(0, "MOD-0001 new", "ingest", p.extraction)
	p.want(0, "phase: bootstrapped", "status")

	p.want(2, "bootstrapped: run setup and align", "mode", "on", "--session", "s1")
	p.hook("s1", "billing/charge.go")
	if j := p.journal("s1"); j != "" {
		t.Fatalf("no turn may run before alignment, journal:\n%s", j)
	}
}

func TestHooksWorkOnlyWhileActive(t *testing.T) {
	p := aligned(t)
	p.want(0, "active", "mode", "on", "--session", "s1")
	p.hook("s1", "billing/charge.go")
	if !strings.Contains(p.journal("s1"), "changed billing/charge.go") {
		t.Fatalf("an active session should record changed files, journal:\n%s", p.journal("s1"))
	}

	p.want(0, "mode off", "mode", "off", "--session", "s1")
	before := p.journal("s1")
	p.hook("s1", "ledger/book.go")
	p.hook("other-session", "ledger/book.go")
	if p.journal("s1") != before || p.journal("other-session") != "" {
		t.Fatalf("hooks must do nothing while the mode is off")
	}
}

func TestChangeOutsideModeCrossingBoundaryIsBlocked(t *testing.T) {
	p := aligned(t)
	p.want(0, "active", "mode", "on", "--session", "s1")
	p.want(0, "mode off", "mode", "off", "--session", "s1")

	p.editCode("billing/charge.go")
	p.extract(strings.Replace(strings.Replace(topoV1, `"name":"billing"`, `"name":"billing core"`, 1),
		`{"from":"billing","to":"api","kind":"provides"}`,
		`{"from":"billing","to":"api","kind":"provides"},{"from":"billing","to":"ledger","kind":"depends_on"}`, 1))

	out := p.want(2, "BLOCKED new-relation:MOD-0001.depends_on.MOD-0002", "mode", "on", "--session", "s1")
	if !strings.Contains(out, "drifted") {
		t.Fatalf("mode on should report drift:\n%s", out)
	}
	if !strings.Contains(p.read("architecture/MOD-0001.md"), "billing core") {
		t.Fatalf("the external change should update EXTRACTED facts without asking")
	}
	p.hook("s1", "billing/charge.go")
	if p.journal("s1") != "" {
		t.Fatalf("a blocked project must not run hooks")
	}

	p.want(2, "pending approval", "align")
	p.want(0, "approved new-relation:MOD-0001.depends_on.MOD-0002 as ADR-0001", "approve", "new-relation:MOD-0001.depends_on.MOD-0002")
	if adr := p.read("decisions/ADR-0001.md"); !strings.Contains(adr, "origin: USER_STATED") || !strings.Contains(adr, "grade: C") {
		t.Fatalf("approval should leave a grade C ADR:\n%s", adr)
	}
	p.want(0, "active", "mode", "on", "--session", "s1")
}

func TestContractChangeOutsideModeIsBlocked(t *testing.T) {
	p := aligned(t)
	p.editCode("api/http.go")
	p.extract(strings.Replace(topoV1, `GET /charges`, `GET /v2/charges`, 1))
	p.want(2, `BLOCKED contract-changed:IF-0001 ("GET /charges" -> "GET /v2/charges")`, "mode", "on", "--session", "s1")
	p.want(0, "as ADR-0001", "approve", "contract-changed:IF-0001")
	p.want(0, "active", "mode", "on", "--session", "s1")
	if !strings.Contains(p.read("intended.yaml"), "GET /v2/charges") {
		t.Fatalf("the approved signature should become intended")
	}
}

func TestDesignUnitChangeNeedsOnlyShortRealign(t *testing.T) {
	p := aligned(t)
	p.editCode("ledger/book.go")
	p.extract(`{"units":[
 {"key":"billing","kind":"MOD","name":"billing","anchors":[{"path":"billing/charge.go","symbol":"Charge"}]},
 {"key":"api","kind":"IF","name":"http api","signature":"GET /charges","anchors":[{"path":"api/http.go","symbol":"Serve"}]}
],"relations":[{"from":"billing","to":"api","kind":"provides"}]}`)

	out := p.want(2, "REALIGN unit-removed:MOD-0002", "mode", "on", "--session", "s1")
	if strings.Contains(out, "BLOCKED") {
		t.Fatalf("removing a unit is a realignment, not a blocked change:\n%s", out)
	}
	p.want(0, "aligned", "align")
	p.want(0, "active", "mode", "on", "--session", "s1")
	if strings.Contains(p.read("intended.yaml"), "MOD-0002") {
		t.Fatalf("realignment should drop the removed unit from the intended topology")
	}
}

func TestNewUnitIsPromotionCandidateNotDrift(t *testing.T) {
	p := aligned(t)
	p.editCode("audit/log.go")
	p.extract(strings.Replace(topoV1, `"units":[`,
		`"units":[{"key":"audit","kind":"MOD","name":"audit","anchors":[{"path":"audit/log.go","symbol":"Log"}]},`, 1))
	out := p.want(0, "active", "mode", "on", "--session", "s1")
	if !strings.Contains(out, "PROMOTE unit-added:MOD-0003") {
		t.Fatalf("a new unit should surface as a promotion candidate:\n%s", out)
	}
	if strings.Contains(p.read("intended.yaml"), "MOD-0003") {
		t.Fatalf("a promotion candidate must not enter the intended topology on its own")
	}
}

func TestStaleIngestCannotBypassTheGate(t *testing.T) {
	p := aligned(t)
	p.editCode("billing/charge.go")
	p.extract(strings.Replace(topoV1, `{"from":"billing","to":"api","kind":"provides"}`,
		`{"from":"billing","to":"api","kind":"provides"},{"from":"billing","to":"ledger","kind":"depends_on"}`, 1))
	stale := filepath.Join(t.TempDir(), "stale.json")
	write(t, stale, topoV1)
	p.want(0, "matched", "ingest", stale)
	p.want(2, "BLOCKED new-relation:MOD-0001.depends_on.MOD-0002", "mode", "on", "--session", "s1")
}

func TestContractReplacedUnderNewIDIsBlocked(t *testing.T) {
	p := aligned(t)
	p.editCode("api/http.go")
	p.extract(strings.Replace(strings.Replace(topoV1, `GET /charges`, `GET /v2/charges`, 1), `"Serve"`, `"ServeV2"`, 1))
	p.want(2, "BLOCKED unit-removed:IF-0001", "mode", "on", "--session", "s1")
	p.want(2, "pending approval", "align")
}

func TestRemovingAContractIsBlocked(t *testing.T) {
	p := aligned(t)
	p.editCode("api/http.go")
	p.extract(`{"units":[
 {"key":"billing","kind":"MOD","name":"billing","anchors":[{"path":"billing/charge.go","symbol":"Charge"}]},
 {"key":"ledger","kind":"MOD","name":"ledger","anchors":[{"path":"ledger/book.go","symbol":"Post"}]}
]}`)
	out := p.want(2, "BLOCKED unit-removed:IF-0001", "mode", "on", "--session", "s1")
	if strings.Contains(out, "REALIGN unit-removed:IF-0001") {
		t.Fatalf("a removed contract must be blocked, not realigned:\n%s", out)
	}
	p.want(0, "as ADR-0001", "approve", "unit-removed:IF-0001")
	p.want(0, "active", "mode", "on", "--session", "s1")
	if strings.Contains(p.read("intended.yaml"), "IF-0001") {
		t.Fatalf("an approved removal should drop the contract and its relations:\n%s", p.read("intended.yaml"))
	}
}

func TestEmptyExtractionIsAFailureNotDrift(t *testing.T) {
	p := aligned(t)
	p.editCode("billing/charge.go")
	p.extract(`{"units":[]}`)
	p.want(1, "reported no units", "mode", "on", "--session", "s1")
	p.want(0, "phase: aligned", "status")
}

func TestNewCouplingThroughANewUnitNeedsRealign(t *testing.T) {
	p := aligned(t)
	p.editCode("audit/log.go")
	v2 := strings.Replace(topoV1, `"units":[`,
		`"units":[{"key":"audit","kind":"MOD","name":"audit","anchors":[{"path":"audit/log.go","symbol":"Log"}]},`, 1)
	v2 = strings.Replace(v2, `{"from":"billing","to":"api","kind":"provides"}`,
		`{"from":"billing","to":"api","kind":"provides"},{"from":"billing","to":"audit","kind":"depends_on"},{"from":"audit","to":"ledger","kind":"depends_on"}`, 1)
	p.extract(v2)
	out := p.want(2, "REALIGN new-relation:MOD-0001.depends_on.MOD-0003", "mode", "on", "--session", "s1")
	if !strings.Contains(out, "REALIGN new-relation:MOD-0003.depends_on.MOD-0002") {
		t.Fatalf("both new edges touching intended units should surface:\n%s", out)
	}
	p.want(0, "aligned", "align")
	p.want(0, "active", "mode", "on", "--session", "s1")
}

func TestNewConsumerOfAContractIsBlocked(t *testing.T) {
	p := aligned(t)
	p.editCode("audit/log.go")
	v2 := strings.Replace(topoV1, `"units":[`,
		`"units":[{"key":"audit","kind":"MOD","name":"audit","anchors":[{"path":"audit/log.go","symbol":"Log"}]},`, 1)
	v2 = strings.Replace(v2, `{"from":"billing","to":"api","kind":"provides"}`,
		`{"from":"billing","to":"api","kind":"provides"},{"from":"audit","to":"api","kind":"consumes"}`, 1)
	p.extract(v2)
	p.want(2, "BLOCKED new-relation:MOD-0003.consumes.IF-0001", "mode", "on", "--session", "s1")
}

func TestRefusedModeOnTurnsTheSessionOff(t *testing.T) {
	p := aligned(t)
	p.want(0, "active", "mode", "on", "--session", "s1")
	p.editCode("api/http.go")
	p.extract(strings.Replace(topoV1, `GET /charges`, `GET /v2/charges`, 1))
	p.want(2, "BLOCKED", "mode", "on", "--session", "s1")
	p.want(0, "mode: off", "status", "--session", "s1")
	p.want(0, "as ADR-0001", "approve", "contract-changed:IF-0001")
	p.hook("s1", "api/http.go")
	if p.journal("s1") != "" {
		t.Fatalf("hooks must stay off until mode on succeeds again")
	}
}

func TestHookFindsTheProjectFromItsInput(t *testing.T) {
	p := aligned(t)
	t.Setenv("CLAUDE_PROJECT_DIR", "")
	p.want(0, "active", "mode", "on", "--session", "s1")
	sub := filepath.Join(p.root, "billing")
	file := filepath.Join(p.root, "billing", "charge.go")
	in := fmt.Sprintf(`{"session_id":"s1","cwd":%q,"tool_name":"Edit","tool_input":{"file_path":%q}}`, sub, file)
	var out, errb bytes.Buffer
	if code := run([]string{"hook", "PostToolUse"}, strings.NewReader(in), &out, &errb); code != 0 {
		t.Fatalf("hook exit %d: %s", code, errb.String())
	}
	if !strings.Contains(p.journal("s1"), "changed billing/charge.go") {
		t.Fatalf("the hook should find .abbd from cwd and record a root-relative path, journal:\n%s", p.journal("s1"))
	}
}
