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
	if b, _ := s.ReadBlueprint("index.html"); string(b) != "a" {
		t.Errorf("the index page is gone or changed: %q", b)
	}
}
