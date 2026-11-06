package screen

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/whyjp/abbd/internal/store"
)

// lintFixture lints every view of one fixture and holds it to the debt
// the fixture records in its .abbd/lint-debt.yaml, as abbd lint does. It
// returns the views, every way they break the gate, and what the debt file
// records above what they measure.
func lintFixture(t *testing.T, name string) (views []ViewLint, fails, lowerable []string) {
	t.Helper()
	views, err := Lint(fixture(t, name), "")
	if err != nil {
		t.Fatal(err)
	}
	debt, err := store.Open(filepath.Join("testdata", name)).LoadLintDebt()
	if err != nil {
		t.Fatal(err)
	}
	return views, Gate(views, debt, true), Lowerable(views, debt, true)
}

// lintColumns are the per-view totals of the lint table, after the view.
func lintColumns() []string {
	cols := []string{"hard", "target"}
	for _, c := range criteria {
		cols = append(cols, c.ID)
	}
	return append(cols, "excess_turns", "placement", "crossings", "weighed", "steps", "jogs", "phantoms", "paths_over2", "traced_bends", "back", "detours", "height", "tees", "parallel_ink", "segments", "lines", "fork_levels", "ink_crossings")
}

func lintRow(v ViewLint) []int {
	row := []int{v.Totals.Hard, v.Totals.Target}
	for _, c := range criteria {
		row = append(row, v.Totals.Criteria[c.ID])
	}
	t := v.Totals
	return append(row, t.ExcessTurns, t.Placement, t.Crossings, t.Weighed, t.Steps, t.Jogs, t.Phantoms, t.PathsOver2, t.TracedBends, t.Back, t.Detours, t.Height, t.Tees, t.ParallelInk, t.Segments, t.Lines, t.ForkLevels, t.InkCrossings)
}

// Every fixture keeps the gate its .abbd/lint-debt.yaml sets, the one abbd
// lint holds a repository to: no view breaks a hard criterion past its
// debt, the weighed sum stays at or under its ceiling, and the placement
// debt passes its ceiling by at most 2%. A debt or a ceiling above what the
// fixture measures fails too, so each kept change lowers them with abbd lint
// --record. With ABBD_LINT_FIXTURES set to a file, the per-view totals of
// every fixture go there as a table, and with ABBD_LINT_BREACHES set, every
// breach.
func TestLintFixtures(t *testing.T) {
	cols := lintColumns()
	totals := make([]int, len(cols))
	var rows, breaches []string
	for _, name := range fixtures {
		views, fails, lowerable := lintFixture(t, name)
		for _, f := range fails {
			t.Errorf("%s: %s", name, f)
		}
		for _, f := range lowerable {
			t.Errorf("%s: %s; lower it with abbd lint --record --root internal/screen/testdata/%s", name, f, name)
		}
		for _, v := range views {
			view := name + " " + v.View
			cells := []string{view}
			for i, n := range lintRow(v) {
				totals[i] += n
				cells = append(cells, fmt.Sprint(n))
			}
			rows = append(rows, strings.Join(cells, "\t"))
			for _, b := range v.Breaches {
				breaches = append(breaches, strings.Join([]string{view, b.Criterion, fmt.Sprint(b.Hard), fmt.Sprint(b.Debt), fmt.Sprint(b.Got), fmt.Sprint(b.Allowed), b.Text}, "\t"))
			}
		}
	}
	for i, c := range cols {
		t.Logf("%s %d", c, totals[i])
	}
	write := func(env, head string, lines []string) {
		if f := os.Getenv(env); f != "" {
			if err := os.WriteFile(f, []byte(head+"\n"+strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	write("ABBD_LINT_FIXTURES", "view\t"+strings.Join(cols, "\t"), rows)
	write("ABBD_LINT_BREACHES", "view\tcriterion\thard\tdebt\tgot\tallowed\ttext", breaches)
}
