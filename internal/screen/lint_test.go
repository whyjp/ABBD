package screen

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// lintFixtures lints every view of the fixtures, each named by its fixture.
func lintFixtures(t *testing.T) []ViewLint {
	t.Helper()
	var out []ViewLint
	for _, name := range fixtures {
		got, err := Lint(fixture(t, name), "")
		if err != nil {
			t.Fatal(err)
		}
		for _, v := range got {
			v.View = name + " " + v.View
			for i := range v.Breaches {
				v.Breaches[i].View = v.View
			}
			out = append(out, v)
		}
	}
	return out
}

// lintColumns are the per-view totals of the lint table, after the view.
func lintColumns() []string {
	cols := []string{"hard", "target"}
	for _, c := range criteria {
		cols = append(cols, c.ID)
	}
	return append(cols, "steps", "jogs", "crossings", "phantoms", "paths_over2", "traced_bends", "back", "weighed")
}

func lintRow(v ViewLint) []int {
	row := []int{v.Totals.Hard, v.Totals.Target}
	for _, c := range criteria {
		row = append(row, v.Totals.Criteria[c.ID])
	}
	t := v.Totals
	return append(row, t.Steps, t.Jogs, t.Crossings, t.Phantoms, t.PathsOver2, t.TracedBends, t.Back, t.Weighed)
}

// Every view of the fixtures keeps the hard criteria, but for the debt
// recorded below, and the fixtures' target totals stay at or under their
// ceilings. With ABBD_LINT_FIXTURES set to a file, the per-view totals go
// there as a table, and with ABBD_LINT_BREACHES set, every breach.
func TestLintFixtures(t *testing.T) {
	views := lintFixtures(t)
	cols := lintColumns()
	totals := make([]int, len(cols))
	var rows, breaches []string
	for _, v := range views {
		row := lintRow(v)
		cells := []string{v.View}
		for i, n := range row {
			totals[i] += n
			cells = append(cells, fmt.Sprint(n))
		}
		rows = append(rows, strings.Join(cells, "\t"))
		owed := lintDebt[v.View]
		for _, c := range criteria {
			got := v.Totals.Criteria[c.ID]
			switch {
			case !c.Hard:
			case got > owed[c.ID]:
				for _, b := range v.Breaches {
					if b.Criterion == c.ID {
						t.Errorf("%s: %s: %s", v.View, c.ID, b.Text)
					}
				}
				if owed[c.ID] > 0 {
					t.Errorf("%s: %d breaches of %s, recorded %d", v.View, got, c.ID, owed[c.ID])
				}
			case got < owed[c.ID]:
				t.Errorf("%s: %d breaches of %s, fewer than the %d recorded; lower its debt", v.View, got, c.ID, owed[c.ID])
			}
		}
		for _, b := range v.Breaches {
			breaches = append(breaches, strings.Join([]string{v.View, b.Criterion, fmt.Sprint(b.Hard), fmt.Sprint(b.Got), fmt.Sprint(b.Allowed), b.Text}, "\t"))
		}
	}
	for i, c := range cols {
		if ceiling, ok := lintCeilings[c]; ok && totals[i] > ceiling {
			t.Errorf("the fixtures total %d %s, ceiling %d", totals[i], c, ceiling)
		}
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
	write("ABBD_LINT_BREACHES", "view\tcriterion\thard\tgot\tallowed\ttext", breaches)
}

// lintDebt are the views of the fixtures that break a hard criterion now,
// with how many breaches each has. They may only fall. The rule breaches
// are the views fixtures_test.go lists as debt. pstack's overview joins
// pairs of skill families that use each other with one line that reads
// both ways (docs/blueprint-svg.md, "Known gaps", "Dense overviews").
var lintDebt = map[string]map[string]int{
	"self MOD-0030 internal/model":                                {"R": 1},
	"self MOD-0047 internal/topology":                             {"R": 1},
	"orca MOD-0007 mobile app (Expo)":                             {"R": 1},
	"orca MOD-0042 desktop main process":                          {"R": 1},
	"orca MOD-0056 Orca runtime (RPC host)":                       {"R": 1},
	"orca MOD-0072 desktop renderer UI":                           {"R": 1},
	"orca MOD-0081 shared types and schemas":                      {"R": 2},
	"pstack MOD-0001 focus FEAT-0008 docs/harness/claude-code.md": {"R": 1},
	"pstack overview":                                             {"J3": 52},
}

// lintCeilings are the fixtures' totals when the criteria were set, from
// eval/lint/baseline.tsv. A change to the renderer may lower them and may
// not raise them. The steps are the measure to lower. B1 and B2 are the
// steps line by line, so they have no ceiling of their own, and the
// crossings are held only through the weighed sum, since a step may buy
// up to stepCrossings crossings (Staircases first).
var lintCeilings = map[string]int{
	"P1": 117, "P2": 21, "H1": 15, "H2": 36, "O1": 1, "O2": 8, "F1": 198,
	"steps": 1438, "phantoms": 52, "paths_over2": 417, "traced_bends": 4960, "back": 74, "weighed": 6017,
}
