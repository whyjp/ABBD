package screen

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/whyjp/abbd/internal/store"
)

// A line that drops straight across its own bus saves the two turns it pays
// to take the bus and costs one crossing, so the weighed sum falls by one
// while the reader sees a branch that is not there. The gate holds orca's
// tees at their ceiling, so such a change fails it.
func TestTheGateRefusesATeeTheWeighedSumWouldBuy(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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

// Each page a reader looks at keeps its colour switches, doubled lines,
// lines abreast and doglegs at what the debt file records for it, whatever a sum over
// every view would trade, and recording a rise never raises its ceiling.
// The fixture's debt file is what abbd lint --record writes.
func TestTheGateHoldsEachPageAtItsTotals(t *testing.T) {
	t.Parallel()
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
	tmp := t.TempDir()
	if err := store.Open(tmp).SaveLintDebt(Record(views, debt)); err != nil {
		t.Fatal(err)
	}
	want, _ := os.ReadFile(filepath.Join("testdata", "orca", ".abbd", "lint-debt.yaml"))
	if got, _ := os.ReadFile(filepath.Join(tmp, ".abbd", "lint-debt.yaml")); string(got) != strings.ReplaceAll(string(want), "\r\n", "\n") {
		t.Errorf("abbd lint --record writes orca's debt file as\n%s\nnot as recorded\n%s", got, want)
	}
	const page = "MOD-0072 desktop renderer UI"
	at := slices.IndexFunc(views, func(v ViewLint) bool { return v.View == page })
	if at < 0 || debt.Pages[page] == nil {
		t.Fatalf("orca draws no page %q the debt file bounds", page)
	}
	for _, rise := range []func(*Totals){
		func(t *Totals) { t.ColourSwitches++ },
		func(t *Totals) { t.ParallelInk++ },
		func(t *Totals) { t.Abreast++ },
		func(t *Totals) { t.Combs++ },
		func(t *Totals) { t.Doglegs++ },
		func(t *Totals) { t.LabelRepeats++ },
		func(t *Totals) { t.Labelled-- },
	} {
		risen := slices.Clone(views)
		rise(&risen[at].Totals)
		fails := Gate(risen, debt, true)
		if len(fails) != 1 || !strings.HasPrefix(fails[0], page+": ") {
			t.Errorf("one more on %s fails the gate with %q, want only that page past its bound", page, fails)
		}
		if got := Record(risen, debt).Pages[page]; !maps.Equal(got, debt.Pages[page]) {
			t.Errorf("recording a rise on %s sets its bounds %v, want them kept at %v", page, got, debt.Pages[page])
		}
	}
	risen := slices.Clone(views)
	risen[at].Totals.Criteria = maps.Clone(risen[at].Totals.Criteria)
	risen[at].Totals.Criteria["J3"]++
	if fails := Gate(risen, debt, true); !slices.Contains(fails, fmt.Sprintf("%s: false_joins %d, past its page bound %d", page, risen[at].Totals.Criteria["J3"], debt.Pages[page]["false_joins"])) {
		t.Errorf("a false join on %s fails the gate with %q, want its page past its false_joins bound", page, fails)
	}
	if got := Record(risen, debt).Pages[page]; !maps.Equal(got, debt.Pages[page]) {
		t.Errorf("recording a false join on %s sets its bounds %v, want them kept at %v", page, got, debt.Pages[page])
	}
}
