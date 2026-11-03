package store

import (
	"bytes"
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
	f, err := ParseFigureDraft[model.Step]([]byte("---\nquestion: ' Q? '\nwriter: human\nsteps:\n  - unit: MOD-0001\n    does: runs A\n    evidence: [{path: a.go, quote: func A()}]\n    code: {x: y}\n---\n\ncaption\n"))
	if err != nil {
		t.Fatal(err)
	}
	if f.Question != "Q?" || f.Parts[0].Writer != "" || f.Parts[0].Code != nil || f.Caption != "caption" {
		t.Fatalf("a draft keeps the question, steps and caption only: %+v", f)
	}
	f.Key, f.Parts[0].Writer = "run", model.Reader
	if err := SaveFigure(s, Flow{Key: "bare", Question: "Q?", Parts: []Part[model.Step]{{Claim: model.Step{Unit: "MOD-0001", Does: "runs A"}}}}); err == nil {
		t.Fatal("a step that cites no code must be refused")
	}
	if err := SaveFigure(s, f); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"README.md": "notes\n", "Bad Name.md": "---\nquestion: q\n---\n"} {
		if err := os.WriteFile(filepath.Join(root, Dir, "flows", name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	figs, err := s.LoadFigures()
	if err != nil {
		t.Fatal(err)
	}
	if len(figs.Strays) != 1 || figs.Strays[0].File != Dir+"/flows/README.md" {
		t.Errorf("a file without frontmatter is a stray: %v", figs.Strays)
	}
	known := map[string]bool{}
	for _, f := range figs.Flows {
		known[string(f.Key)] = f.Known
	}
	if !known["run"] || known["Bad Name"] || len(figs.Flows) != 2 {
		t.Errorf("run is a flow, Bad Name is read but no flow key: %v", known)
	}
	if fl, _, _ := LoadFigure[model.Step](s, "run"); fl.Parts[0].Stale() {
		t.Errorf("a flow stamped on the code as it is reads current: %+v", fl.Parts[0])
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
	fl, _, err := LoadFigure[model.Step](s, "run")
	if err != nil {
		t.Fatal(err)
	}
	for i, st := range fl.Parts {
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
	figs, err := s.LoadFigures()
	if err != nil {
		t.Fatalf("a malformed file must not stop the flows from loading: %v", err)
	}
	if len(figs.Flows) != 0 || len(figs.Strays) != 2 || figs.Strays[0].File != Dir+"/flows/bad.md" || figs.Strays[0].Why == "" || figs.Strays[1].File != Dir+"/flows/open.md" {
		t.Errorf("each malformed file is a stray with its reason: %+v %+v", figs.Flows, figs.Strays)
	}
}

// A stack is a figure of its own kind: its file sits under .abbd/stacks and
// lists its layers under their own key, and a file there that is no stack is
// a stray of that directory.
func TestAStackIsStoredUnderItsOwnKind(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("package a\nfunc A() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := Open(root)
	f, err := ParseFigureDraft[model.Layer]([]byte("---\nquestion: What are the layers?\nlayers:\n  - name: top\n    role: runs A\n    units: [MOD-0001]\n    evidence: [{path: a.go, quote: func A()}]\n---\n\ncaption\n"))
	if err != nil {
		t.Fatal(err)
	}
	f.Key = "cli"
	if err := SaveFigure(s, f); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(root, Dir, "stacks", "cli.md"))
	if err != nil || !bytes.Contains(b, []byte("\nlayers:\n")) || bytes.Contains(b, []byte("steps:")) {
		t.Fatalf("a stack's file lists its layers: %v\n%s", err, b)
	}
	if err := os.WriteFile(filepath.Join(root, Dir, "stacks", "README.md"), []byte("notes\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	figs, err := s.LoadFigures()
	if err != nil {
		t.Fatal(err)
	}
	if len(figs.Flows) != 0 || len(figs.Stacks) != 1 || figs.Stacks[0].Parts[0].Claim.Units[0] != "MOD-0001" || figs.Stacks[0].Parts[0].Stale() {
		t.Errorf("the stack loads back as stored, among the stacks only: %+v", figs)
	}
	if len(figs.Strays) != 1 || figs.Strays[0].File != Dir+"/stacks/README.md" {
		t.Errorf("a file there that is no stack is a stray of the stacks: %v", figs.Strays)
	}
}

// A flow file reads and writes back to the same bytes, its retired steps
// and a human's word included, so the flows stored before figures had kinds
// stay as they are. testdata/flows/sync.md is this repository's own flow.
func TestAFlowFileWritesBackTheSameBytes(t *testing.T) {
	for _, key := range []model.FigureKey{"sync", "charge"} {
		want, err := os.ReadFile(filepath.Join("testdata", "flows", string(key)+".md"))
		if err != nil {
			t.Fatal(err)
		}
		root := t.TempDir()
		s := Open(root)
		if err := os.MkdirAll(filepath.Join(root, Dir, "flows"), 0o755); err != nil {
			t.Fatal(err)
		}
		file := filepath.Join(root, Dir, "flows", string(key)+".md")
		if err := os.WriteFile(file, want, 0o644); err != nil {
			t.Fatal(err)
		}
		f, ok, err := LoadFigure[model.Step](s, key)
		if err != nil || !ok {
			t.Fatalf("flow %s: %v", key, err)
		}
		if err := writeFigure(s, f); err != nil {
			t.Fatal(err)
		}
		if got, _ := os.ReadFile(file); !bytes.Equal(got, want) {
			t.Errorf("flow %s must write back as it was read:\n%s", key, got)
		}
	}
}
