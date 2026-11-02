package store

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/whyjp/abbd/internal/model"
)

// A draft keeps only what a writer says; a file there with no frontmatter is
// a stray, and one named no flow key is read but not known.
func TestFlowsAreReadFromTheirOwnFiles(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("package a\nfunc A() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := Open(root)
	f, err := ParseFlowDraft([]byte("---\nquestion: ' Q? '\nwriter: human\nsteps:\n  - unit: MOD-0001\n    does: runs A\n    evidence: [{path: a.go, quote: func A()}]\n    code: {x: y}\n---\n\ncaption\n"))
	if err != nil {
		t.Fatal(err)
	}
	if f.Question != "Q?" || f.Writer != "" || f.Steps[0].Code != nil || f.Caption != "caption" {
		t.Fatalf("a draft keeps the question, steps and caption only: %+v", f)
	}
	f.Key, f.Writer, f.Origin = "run", model.Reader, model.Inferred
	if err := s.SaveFlow(f); err == nil {
		t.Fatal("an unstamped flow must be refused")
	}
	if err := s.SaveFlow(s.StampFlow(f)); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"README.md": "notes\n", "Bad Name.md": "---\nquestion: q\n---\n"} {
		if err := os.WriteFile(filepath.Join(root, Dir, "flows", name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	flows, strays, err := s.LoadFlows()
	if err != nil {
		t.Fatal(err)
	}
	if len(strays) != 1 || strays[0].File != Dir+"/flows/README.md" {
		t.Errorf("a file without frontmatter is a stray: %v", strays)
	}
	known := map[string]bool{}
	for _, f := range flows {
		known[string(f.Key)] = f.Known
	}
	if !known["run"] || known["Bad Name"] || len(flows) != 2 {
		t.Errorf("run is a flow, Bad Name is read but no flow key: %v", known)
	}
	if fl, _, _ := s.LoadFlow("run"); fl.Steps[0].Stale() {
		t.Errorf("a flow stamped on the code as it is reads current: %+v", fl.Steps[0])
	}
}

// A step written by hand with no evidence, or with no code stamped, has
// nothing that shows it is still true, so it reads STALE.
func TestAFlowStepWithNoCodeReadsStale(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("package a\nfunc A() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := Open(root)
	if err := os.MkdirAll(filepath.Join(root, Dir, "flows"), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\nquestion: Q?\norigin: INFERRED\nformat: 1\nsteps:\n  - unit: MOD-0001\n    does: runs A\n    evidence: []\n    code: {}\n  - unit: MOD-0001\n    does: runs A again\n    evidence: [{path: a.go, quote: func A()}]\n---\n"
	if err := os.WriteFile(filepath.Join(root, Dir, "flows", "run.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	fl, _, err := s.LoadFlow("run")
	if err != nil {
		t.Fatal(err)
	}
	for i, st := range fl.Steps {
		if !st.Stale() {
			t.Errorf("step %d cites no code to check and must read STALE: %+v", i+1, st)
		}
	}
}

// A flow file abbd cannot read is a stray with the reason, so one bad file
// does not stop every reader of the flows.
func TestAMalformedFlowFileIsAStray(t *testing.T) {
	root := t.TempDir()
	s := Open(root)
	if err := os.MkdirAll(filepath.Join(root, Dir, "flows"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"open.md": "---\nquestion: q\n", "bad.md": "---\nsteps: [unclosed\n---\n"} {
		if err := os.WriteFile(filepath.Join(root, Dir, "flows", name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	flows, strays, err := s.LoadFlows()
	if err != nil {
		t.Fatalf("a malformed file must not stop the flows from loading: %v", err)
	}
	if len(flows) != 0 || len(strays) != 2 || strays[0].File != Dir+"/flows/bad.md" || strays[0].Why == "" || strays[1].File != Dir+"/flows/open.md" {
		t.Errorf("each malformed file is a stray with its reason: %+v %+v", flows, strays)
	}
}
