package screen

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/whyjp/abbd/internal/store"
)

// A line that drops straight across its own bus saves the two turns it pays
// to take the bus and costs one crossing, so the weighed sum falls by one
// while the reader sees a branch that is not there. The gate holds orca's
// tees at their ceiling, so such a change fails it.
func TestTheGateRefusesATeeTheWeighedSumWouldBuy(t *testing.T) {
	views, err := Lint(fixture(t, "orca"), "")
	if err != nil {
		t.Fatal(err)
	}
	debt, err := store.Open(filepath.Join("testdata", "orca")).LoadLintDebt()
	if err != nil {
		t.Fatal(err)
	}
	if fails := Gate(views, debt, true); len(fails) > 0 {
		t.Fatalf("orca breaks the gate as drawn: %v", fails)
	}
	for i := range views {
		if tt := &views[i].Totals; tt.ExcessTurns >= 2 {
			tt.Tees, tt.Crossings, tt.ExcessTurns, tt.Weighed = tt.Tees+1, tt.Crossings+1, tt.ExcessTurns-2, tt.Weighed-1
			break
		}
	}
	fails := Gate(views, debt, true)
	if len(fails) != 1 || !strings.HasPrefix(fails[0], "tees ") {
		t.Errorf("one more tee for two excess turns fails the gate with %q, want only the tees past their ceiling", fails)
	}
}
