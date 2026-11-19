package screen

import "testing"

// choose keeps a view drawn on roads with a stem's branches gathered only
// where every line that says a word at rest drawn stemless still says one,
// and the gathered drawing crosses, doglegs and breaks its ink no more.
func TestChooseKeepsGatheredBranchesOnlyWhereTheyReadNoWorse(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name           string
		words, bare    []bool
		rest, bareRest []int
		want           string
	}{
		{"every word stays", []bool{true, true, false}, []bool{true, false, false}, []int{3, 5, 20}, []int{3, 5, 21}, "stemmed"},
		{"a line loses its word", []bool{true, false, true}, []bool{true, true, false}, []int{2, 5, 20}, []int{3, 5, 21}, "stemless"},
		{"one more dogleg", []bool{true, true}, []bool{true, true}, []int{2, 6, 19}, []int{3, 5, 21}, "stemless"},
	} {
		draw := func(tc tactic, read *cost) (string, bool) {
			switch {
			case !tc.roads:
				return "plain", true
			case tc.stemless:
				return "stemless", true
			}
			read.stemmed = true
			return "stemmed", true
		}
		judge := func(v string, read *cost) {
			read.words, read.rest = c.words, c.rest
			if v == "stemless" {
				read.words, read.rest = c.bare, c.bareRest
			}
		}
		if got := choose([]tactic{{}}, draw, judge); got != c.want {
			t.Errorf("%s: choose kept the %s drawing, want the %s one", c.name, got, c.want)
		}
	}
}
