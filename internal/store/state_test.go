package store

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/whyjp/abbd/internal/model"
)

// A merge can keep both sides' lines of the state, by union or as conflict
// markers. The state then reads as merged, keeps the first side's values and
// the higher of each kind's high water, so no ID is minted twice.
func TestAStateWithTwoSyncsReadsMerged(t *testing.T) {
	for name, text := range map[string]string{
		"union":    "revision: ra\nfingerprint: fa\nblueprint: ba\nrevision: rb\nfingerprint: fb\nblueprint: bb\nhigh_water:\n    DATA: 3\n    DATA: 5\n    MOD: 2\n",
		"conflict": "<<<<<<< HEAD\r\nrevision: ra\r\nfingerprint: fa\r\nblueprint: ba\r\n=======\r\nrevision: rb\r\nfingerprint: fb\r\nblueprint: bb\r\n>>>>>>> b\r\nhigh_water:\r\n<<<<<<< HEAD\r\n    DATA: 3\r\n||||||| base\r\n    DATA: 1\r\n=======\r\n    DATA: 5\r\n>>>>>>> b\r\n    MOD: 2\r\n",
	} {
		dir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(dir, Dir), 0o755); err != nil {
			t.Fatal(err)
		}
		os.WriteFile(filepath.Join(dir, Dir, "state.yaml"), []byte(text), 0o644)
		st, err := Open(dir).LoadState()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !st.Merged || st.Revision != "ra" || st.Fingerprint != "fa" || st.Blueprint != "ba" {
			t.Errorf("%s: want a merged state with the first side's values, got %+v", name, st)
		}
		if st.HighWater[model.Data] != 5 || st.HighWater[model.Module] != 2 {
			t.Errorf("%s: want the higher high water of each kind, got %v", name, st.HighWater)
		}
	}
}

func TestAStateOfOneSyncIsNotMerged(t *testing.T) {
	dir := t.TempDir()
	s := Open(dir)
	os.MkdirAll(filepath.Join(dir, Dir), 0o755)
	want := State{Revision: "r", Fingerprint: "f", Blueprint: "b", HighWater: map[model.Kind]int{model.Data: 1}}
	if err := s.SaveState(want); err != nil {
		t.Fatal(err)
	}
	got, err := s.LoadState()
	if err != nil || got.Merged || got.Fingerprint != "f" || got.Blueprint != "b" || got.HighWater[model.Data] != 1 {
		t.Fatalf("a saved state should read back as itself, got %+v, %v", got, err)
	}
}
