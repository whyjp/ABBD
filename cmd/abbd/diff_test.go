package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const diffBase = `{"units":[
 {"key":"billing","kind":"MOD","name":"billing","anchors":[{"path":"billing/charge.go","symbol":"Charge"}]},
 {"key":"ledger","kind":"MOD","name":"ledger","anchors":[{"path":"ledger/book.go","symbol":"Post"}]},
 {"key":"api","kind":"IF","name":"http api","signature":"GET /charges","digest":"a1","anchors":[{"path":"api/http.go","symbol":"Serve"}]},
 {"key":"events","kind":"EVT","name":"charge events","signature":"charged{id}","digest":"e1","anchors":[{"path":"billing/events.go","symbol":"Charged"}]}
],"relations":[
 {"from":"billing","to":"api","kind":"provides"},
 {"from":"billing","to":"ledger","kind":"depends_on"}
]}`

// The api's signature changes, the events' code changes under the same
// signature, ledger goes, notify arrives, and an inside unit of billing comes
// to depend on notify, which is billing's own new relation.
const diffNow = `{"units":[
 {"key":"billing","kind":"MOD","name":"billing","anchors":[{"path":"billing/charge.go","symbol":"Charge"}]},
 {"key":"charger","kind":"MOD","name":"charger","parent":"billing","anchors":[{"path":"billing/charger.go","symbol":"Run"}]},
 {"key":"notify","kind":"MOD","name":"notify","anchors":[{"path":"notify/mail.go","symbol":"Send"}]},
 {"key":"api","kind":"IF","name":"http api","signature":"GET /charges?page=N","digest":"a2","anchors":[{"path":"api/http.go","symbol":"Serve"}]},
 {"key":"events","kind":"EVT","name":"charge events","signature":"charged{id}","digest":"e2","anchors":[{"path":"billing/events.go","symbol":"Charged"}]}
],"relations":[
 {"from":"billing","to":"api","kind":"provides"},
 {"from":"charger","to":"notify","kind":"depends_on"}
]}`

func rev(t *testing.T, dir string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(out))
}

func diffProject(t *testing.T) (*project, string) {
	t.Helper()
	p, exe := bare(t)
	p.extract(diffBase)
	p.setup(0, "synced: 4 unit(s)", "--", exe)
	git(t, p.root, "add", "-A")
	git(t, p.root, "commit", "-qm", "base")
	git(t, p.root, "branch", "-M", "main")
	base := rev(t, p.root)
	git(t, p.root, "checkout", "-qb", "work")
	p.code("notify/mail.go", "package notify\n\nfunc Send() { api.Serve() }\n")
	p.extract(diffNow)
	p.want(0, "", "sync")
	p.want(0, "INFERRED", "record", "relation", "MOD-0004", "consumes", "IF-0001", "--via", "call", "--evidence", "notify/mail.go:3:api.Serve()")
	return p, base
}

func TestDiffNamesEveryCategorySinceTheBase(t *testing.T) {
	p, base := diffProject(t)
	out := p.want(0, "topology since "+base[:12], "diff", "--base", base)
	for _, want := range []string{
		"+ unit MOD-0003 charger",
		"+ unit MOD-0004 notify",
		"- unit MOD-0002 ledger",
		`~ contract IF-0001 http api signature: "GET /charges" -> "GET /charges?page=N"`,
		"~ contract EVT-0001 charge events: its code changed under the same signature",
		"+ relation MOD-0001 depends_on MOD-0004 (billing depends_on notify)",
		"- relation MOD-0001 depends_on MOD-0002 (billing depends_on ledger)",
		`+ inferred MOD-0004 consumes IF-0001 (notify consumes http api) via call: notify/mail.go:3 "api.Serve()"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "MOD-0003 depends_on MOD-0004") {
		t.Errorf("an inside unit's relation counts as its module's:\n%s", out)
	}
	if def := p.want(0, "", "diff"); def != out {
		t.Errorf("the default base is the merge base with main:\n%s\n---\n%s", def, out)
	}

	var d struct {
		Base           string
		Signatures     []struct{ ID, Old, New string }   `json:"contract_signatures_changed"`
		RelationsAdded []struct{ From, Kind, To string } `json:"relations_added"`
		InferredAdded  []struct{ Via, Quote string }     `json:"inferred_added"`
	}
	if err := json.Unmarshal([]byte(p.want(0, "", "diff", "--base", base, "--json")), &d); err != nil {
		t.Fatal(err)
	}
	if d.Base != base || len(d.Signatures) != 1 || d.Signatures[0].New != "GET /charges?page=N" || len(d.RelationsAdded) != 1 || d.RelationsAdded[0].To != "MOD-0004" || len(d.InferredAdded) != 1 {
		t.Fatalf("--json should carry the same categories: %+v", d)
	}
}

// --check informs a CI label: exit 1 when a contract's signature changed, a
// contract was added or removed, or a relation between top-level units
// appeared or disappeared, 0 when the change stays inside modules.
func TestDiffCheckFlagsOnlyBoundaryChanges(t *testing.T) {
	p, base := diffProject(t)
	p.want(1, "reaches the boundary", "diff", "--base", base, "--check")
	git(t, p.root, "add", "-A")
	git(t, p.root, "commit", "-qm", "work")
	p.want(0, "no topology change", "diff", "--base", "HEAD", "--check")

	inside := strings.Replace(diffNow, `{"from":"charger","to":"notify","kind":"depends_on"}`,
		`{"from":"charger","to":"notify","kind":"depends_on"},{"from":"billing","to":"charger","kind":"depends_on"}`, 1)
	p.extract(inside)
	p.want(0, "", "sync")
	if out := p.want(0, "", "diff", "--base", "HEAD", "--check"); strings.Contains(out, "relation") {
		t.Fatalf("a relation inside one module is not a topology change between modules:\n%s", out)
	}
}

func TestDiffAgainstARevisionWithoutState(t *testing.T) {
	p, exe := bare(t)
	first := rev(t, p.root)
	p.extract(diffBase)
	p.setup(0, "synced", "--", exe)
	out := p.want(0, "+ unit MOD-0001 billing", "diff", "--base", first)
	if strings.Contains(out, "- unit") {
		t.Fatalf("a revision before ABBD has no units to remove:\n%s", out)
	}
}

// A CI checkout such as actions/checkout leaves origin/main but no origin/HEAD
// and no local main; the default base still finds the merge base.
func TestDiffDefaultBaseFindsOriginMainWithoutOriginHead(t *testing.T) {
	p, exe := bare(t)
	p.extract(diffBase)
	p.setup(0, "synced", "--", exe)
	git(t, p.root, "add", "-A")
	git(t, p.root, "commit", "-qm", "base")
	git(t, p.root, "update-ref", "refs/remotes/origin/main", "HEAD")
	git(t, p.root, "branch", "-M", "work")
	p.want(0, "topology since", "diff")
}

// A contract's code change under the same signature is reported but leaves
// --check at 0, or CI would flag every edit to an interface file. A new
// contract and a vanished relation between modules each fail it.
func TestDiffCheckFailsOnANewContractButNotOnACodeChange(t *testing.T) {
	p, exe := bare(t)
	p.extract(diffBase)
	p.setup(0, "synced", "--", exe)
	git(t, p.root, "add", "-A")
	git(t, p.root, "commit", "-qm", "base")
	base := rev(t, p.root)

	code := strings.Replace(diffBase, `"digest":"e1"`, `"digest":"e2"`, 1)
	p.extract(code)
	p.want(0, "", "sync")
	p.want(0, "its code changed under the same signature", "diff", "--base", base, "--check")

	p.extract(strings.Replace(code, `],"relations"`, `,{"key":"cfg","kind":"CFG","name":"rates","signature":"rate=N","anchors":[{"path":"cfg/rates.go","symbol":"Rate"}]}],"relations"`, 1))
	p.want(0, "", "sync")
	p.want(1, "reaches the boundary", "diff", "--base", base, "--check")

	p.extract(strings.Replace(diffBase, `,
 {"from":"billing","to":"ledger","kind":"depends_on"}`, "", 1))
	p.want(0, "", "sync")
	p.want(1, "- relation MOD-0001 depends_on MOD-0002", "diff", "--base", base, "--check")
}

// --unit keeps one module's share of the change, and --page writes the
// blueprint at the base beside the current one outside blueprint/, which
// stays the current state alone.
func TestDiffComparesOneModuleOutsideTheBlueprint(t *testing.T) {
	p, base := diffProject(t)
	before := p.blueprint()
	out := p.want(0, "topology inside MOD-0004 since "+base[:12], "diff", "--base", base, "--unit", "MOD-0004", "--page")
	contains(t, out, "+ unit MOD-0004 notify", "+ relation MOD-0001 depends_on MOD-0004", "compare view:")
	if strings.Contains(out, "MOD-0002") || strings.Contains(out, "IF-0001 http api signature") {
		t.Errorf("a change outside MOD-0004 is listed:\n%s", out)
	}
	page, err := os.ReadFile(filepath.Join(p.root, ".abbd", "compare", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	contains(t, string(page), `src="../blueprint/MOD-0004.html"`, "unit MOD-0004 notify")
	if _, err := os.Stat(filepath.Join(p.root, ".abbd", "compare", "base", "index.html")); err != nil {
		t.Errorf("the base's blueprint is not kept: %v", err)
	}
	if p.blueprint() != before {
		t.Errorf("the compare view changed the blueprint")
	}
	p.want(0, "blueprint: CURRENT", "status")
	p.want(1, "is in neither", "diff", "--unit", "MOD-0099")
}
