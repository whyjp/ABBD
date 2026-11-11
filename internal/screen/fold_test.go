package screen

import (
	"slices"
	"testing"
)

// A chain of places that use each other ranks a row each when drawn
// plainly. Folding leaves their two-way edges out of the ranking, so the
// pairs share rows; with no two-way pair it has nothing to change.
func TestFoldRanksTwoWayPairsTogether(t *testing.T) {
	nodes := []lnode{{id: "a", w: 100, h: 60}, {id: "b", w: 100, h: 60}, {id: "c", w: 100, h: 60}, {id: "d", w: 100, h: 60}}
	edges := []ledge{{"a", "b"}, {"b", "a"}, {"b", "c"}, {"c", "b"}, {"c", "d"}, {"d", "c"}}
	rows := func(es []ledge, tc tactic) layout {
		return arrange(slices.Clone(nodes), es, overviewBudget, nil, tc)
	}
	plain, fold := rows(edges, tactic{}), rows(edges, tactic{fold: true})
	if len(fold.rows) >= len(plain.rows) {
		t.Errorf("folded drawing has %d rows, the plain one %d", len(fold.rows), len(plain.rows))
	}
	if fold.idle {
		t.Error("a fold that changed the ranking says it is idle")
	}
	if !rows([]ledge{{"a", "b"}, {"b", "c"}}, tactic{fold: true}).idle {
		t.Error("a fold with no two-way pair is not idle")
	}
}
