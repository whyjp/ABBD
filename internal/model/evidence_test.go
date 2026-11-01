package model

import "testing"

// ABBD's own files repeat every quote they store, so evidence citing them would
// never die.
func TestEvidenceMustCiteTheCode(t *testing.T) {
	for _, p := range []string{".abbd/architecture/MOD-0001.md", ".abbd", ".git/HEAD"} {
		if err := (Evidence{Path: p, Quote: "x"}).Validate(); err == nil {
			t.Errorf("%s was accepted as evidence", p)
		}
	}
	if err := (Evidence{Path: "src/.abbdrc.rs", Quote: "x"}).Validate(); err != nil {
		t.Errorf("a code path that only starts like .abbd was refused: %v", err)
	}
}
