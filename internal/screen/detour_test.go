package screen

import (
	"strings"
	"testing"
)

// On ripgrep's overview grep sends lines down a trunk to the boxes below
// and onto the bus that ripgrep shares into the row under it. grep-regex
// stands almost under grep, so its line drops onto that bus and into it:
// it never rides on to another box's drop and back (B3), and no line of
// the view runs beside itself (B4).
func TestALineToTheBoxBelowDoesNotRidePastIt(t *testing.T) {
	got, err := Lint(fixture(t, "ripgrep"), "overview")
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range got[0].Breaches {
		if b.Criterion == "B4" || b.Criterion == "B3" && strings.Contains(b.Text, `"grep" (MOD-0006) to box "grep-regex"`) {
			t.Errorf("%s: %s", b.Criterion, b.Text)
		}
	}
}
