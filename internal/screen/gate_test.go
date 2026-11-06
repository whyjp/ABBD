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

// The weighed sum prices an excess turn at one crossing, so a drawing that
// saves three crossings by bending two lines twice more weighs one less,
// while the reader follows two more bends. The gate holds orca's excess
// turns at their ceiling, so such a change fails it.
func TestTheGateRefusesBendsTheWeighedSumWouldBuy(t *testing.T) {
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
	traded := false
	for i := range views {
		if tt := &views[i].Totals; tt.Crossings-tt.Tees >= 3 {
			tt.Crossings, tt.ExcessTurns, tt.Weighed = tt.Crossings-3, tt.ExcessTurns+2, tt.Weighed-1
			traded = true
			break
		}
	}
	if !traded {
		t.Fatal("no orca view has three crossings to trade")
	}
	fails := Gate(views, debt, true)
	if len(fails) != 1 || !strings.HasPrefix(fails[0], "excess_turns ") {
		t.Errorf("two more excess turns for three crossings fails the gate with %q, want only the excess turns past their ceiling", fails)
	}
}
