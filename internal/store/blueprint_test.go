package store

import (
	"os"
	"path/filepath"
	"testing"
)

// The blueprint is the pages of the last render and nothing else: a page
// of a module that left the state goes with it.
func TestSavingTheBlueprintDropsPagesItNoLongerHas(t *testing.T) {
	dir := t.TempDir()
	s := Open(dir)
	if err := s.SaveBlueprint(map[string][]byte{"index.html": []byte("a"), "MOD-0001.html": []byte("b"), "graph.json": []byte("{}")}); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveBlueprint(map[string][]byte{"index.html": []byte("a"), "graph.json": []byte("{}")}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, Dir, "blueprint", "MOD-0001.html")); !os.IsNotExist(err) {
		t.Errorf("the page of a module no render holds is still there")
	}
	if b, _ := os.ReadFile(filepath.Join(dir, Dir, "blueprint", "index.html")); string(b) != "a" {
		t.Errorf("the index page is gone or changed: %q", b)
	}
}

// The pages are committed, so a sync drops the .gitignore of "*" an earlier
// abbd wrote beside them, and leaves one it did not write alone.
func TestSavingTheBlueprintDropsOnlyItsOwnOldGitignore(t *testing.T) {
	for _, c := range []struct {
		ignore string
		kept   bool
	}{{"*\n", false}, {"*\r\n", false}, {"*.tmp\n", true}} {
		dir := t.TempDir()
		s := Open(dir)
		ignore := filepath.Join(dir, Dir, "blueprint", ".gitignore")
		if err := writeFile(ignore, []byte(c.ignore)); err != nil {
			t.Fatal(err)
		}
		if err := s.SaveBlueprint(map[string][]byte{"index.html": []byte("a")}); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(ignore); (err == nil) != c.kept {
			t.Errorf("a blueprint .gitignore of %q kept: %v, want %v", c.ignore, err == nil, c.kept)
		}
		if h, _ := s.BlueprintHash(); h != HashRender(map[string][]byte{"index.html": []byte("a")}) {
			t.Errorf("a blueprint .gitignore of %q changes the blueprint's hash", c.ignore)
		}
	}
}
