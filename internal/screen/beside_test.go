package screen

import (
	"maps"
	"slices"
	"strings"
	"testing"
)

// A box whose only lines are one to a partner and one back stands in its
// partner's row under the beside tactic, and no row is left empty behind it.
func TestBesideStandsAPairOnlyBoxInItsPartnersRow(t *testing.T) {
	nodes := []lnode{{id: "x", w: 100, h: 60}, {id: "p", w: 100, h: 60}, {id: "v", w: 100, h: 60}, {id: "w", w: 100, h: 60}}
	edges := []ledge{{"x", "p"}, {"p", "v"}, {"v", "p"}, {"p", "w"}}
	plain := arrange(slices.Clone(nodes), edges, overviewBudget, nil, tactic{})
	beside := arrange(slices.Clone(nodes), edges, overviewBudget, nil, tactic{beside: true})
	if plain.row["v"] == plain.row["p"] {
		t.Fatal("the plain drawing already stands v in p's row; the case tests nothing")
	}
	if beside.row["v"] != beside.row["p"] {
		t.Errorf("beside stands v in row %d and its partner p in row %d", beside.row["v"], beside.row["p"])
	}
	if beside.partner["v"] != "p" || len(beside.partner) != 1 {
		t.Errorf("beside's partners are %v, want only v beside p", beside.partner)
	}
	if beside.idle {
		t.Error("a beside tactic that placed a box says it is idle")
	}
	for r, row := range beside.rows {
		if len(row) == 0 {
			t.Errorf("row %d is empty after v moved", r)
		}
	}
}

// A box can stand beside its partner only on its left or right, so a box
// that three pair-only boxes meet keeps them in their rows.
func TestBesideLeavesThreePartnersOfOneBoxInTheirRows(t *testing.T) {
	nodes := []lnode{{id: "p", w: 100, h: 60}}
	var edges []ledge
	for _, v := range []string{"a", "b", "c"} {
		nodes = append(nodes, lnode{id: v, w: 100, h: 60})
		edges = append(edges, ledge{"p", v}, ledge{v, "p"})
	}
	l := arrange(nodes, edges, overviewBudget, nil, tactic{beside: true})
	if len(l.partner) != 0 {
		t.Errorf("beside placed %v beside one box", slices.Sorted(maps.Keys(l.partner)))
	}
}

// Only a mutual pair between a box beside placed and its partner is
// one hop with a head at each end: one line each way, of one kind and one
// dotting, neither of them mutual, a bypass or the lead, in a view that
// folds its pairs.
func TestFoldsHopsFoldsOnlyATrueReversePair(t *testing.T) {
	l := layout{pairs: foldAll, partner: map[string]string{"v": "p"}}
	req := func(from, to, kind string) wireReq {
		return wireReq{from: end{node: from, key: from}, to: end{node: to, key: to}, kind: kind}
	}
	k := [2]string{"p", "v"}
	pair := func(a, b wireReq) bool { return l.foldsHops([]wireReq{a, b}, k, []int{0, 1}) }
	if !pair(req("v", "p", "depends_on"), req("p", "v", "depends_on")) {
		t.Error("a mutual pair beside its partner does not fold")
	}
	for name, q := range map[string]wireReq{
		"two kinds": req("p", "v", "consumes"),
		"one way":   req("v", "p", "depends_on"),
	} {
		if pair(req("v", "p", "depends_on"), q) {
			t.Errorf("%s fold", name)
		}
	}
	for _, kind := range []string{kindMutual, kindBypass} {
		if pair(req("v", "p", kind), req("p", "v", kind)) {
			t.Errorf("two %s lines fold", kind)
		}
	}
	dotted := req("p", "v", "depends_on")
	dotted.plan = true
	if pair(req("v", "p", "depends_on"), dotted) {
		t.Error("a solid and a dotted line fold")
	}
	l.pairs = foldNone
	if pair(req("v", "p", "depends_on"), req("p", "v", "depends_on")) {
		t.Error("a view that folds no pair folds a hop")
	}
	l.pairs, l.partner = foldAll, nil
	if pair(req("v", "p", "depends_on"), req("p", "v", "depends_on")) {
		t.Error("a pair no beside tactic placed folds as a hop")
	}
}

// foldsAll holds where every two routes that join two boxes opposite ways
// by one kind are one folded line.
func TestFoldsAllAsksEveryReversePairToFold(t *testing.T) {
	d := Drawing{Boxes: []Box{{ID: "a"}, {ID: "b"}}}
	d.Routes = []Route{{Kind: "depends_on", ends: []string{"a", "b"}}, {Kind: "depends_on", ends: []string{"b", "a"}}}
	if foldsAll(d) {
		t.Error("two opposite routes drawn apart fold all")
	}
	d.Routes[0].Both, d.Routes[1].Both = true, true
	if !foldsAll(d) {
		t.Error("a folded pair does not fold all")
	}
	d.Routes[1].Kind = "consumes"
	d.Routes[0].Both, d.Routes[1].Both = false, false
	if !foldsAll(d) {
		t.Error("two opposite routes of two kinds, no pair, do not fold all")
	}
}

// In the orca fixture's focus views of an inside, a box on both sides of
// the focus is drawn once where the view reads no worse so: its two lines
// are one route each with a head at each end, and where it stands in the
// focus's row they run as one straight hop across.
func TestAFocusDrawsABoxOnBothSidesOnceWithItsPairFolded(t *testing.T) {
	m := fixture(t, "orca")
	once, across := 0, 0
	for _, ins := range m.Insides {
		for _, f := range ins.Focus {
			box := map[string]Box{}
			for _, b := range f.Drawing.Boxes {
				box[b.ID] = b
			}
			for _, r := range f.Drawing.Routes {
				if len(r.ends) != 2 || r.ends[0] != f.Unit || !r.Both || r.Kind == kindMutual {
					continue
				}
				o := r.ends[1]
				if strings.HasSuffix(o, twinMark) {
					t.Errorf("%s: a folded line ends on the twin %s", f.ID, o)
				}
				if _, ok := box[o+twinMark]; ok {
					t.Errorf("%s: %s is folded with the focus and drawn twice", f.ID, o)
				}
				back := slices.IndexFunc(f.Drawing.Routes, func(q Route) bool {
					return len(q.ends) == 2 && q.ends[0] == o && q.ends[1] == f.Unit && q.Kind == r.Kind
				})
				if back < 0 || !f.Drawing.Routes[back].Both || f.Drawing.Routes[back].D != r.D {
					t.Errorf("%s: the line back from %s is not the folded line %q", f.ID, o, r.D)
				}
				once++
				if a, b := box[f.Unit], box[o]; a.Y == b.Y {
					if strings.Count(r.D, "L") != 1 || !horizontal(r.D) {
						t.Errorf("%s: %s stands beside the focus but its line runs %q", f.ID, o, r.D)
					}
					across++
				}
			}
		}
	}
	if once == 0 || across == 0 {
		t.Fatalf("orca's inside focus views fold %d pairs, %d of them across a row; want some of each", once, across)
	}
}

// horizontal says path d of two points keeps one height.
func horizontal(d string) bool {
	f := strings.Fields(strings.NewReplacer("M", "", "L", "").Replace(d))
	return len(f) == 4 && f[1] == f[3]
}

// A box that moves to its partner's row may leave its own row empty, and
// the rows close up behind it, with or without a frame.
func TestBesideLeavesNoEmptyRow(t *testing.T) {
	for _, es := range [][]ledge{
		{{"a", "b"}, {"b", "a"}},
		{{"a", "p"}, {"p", "a"}, {"p", "q"}, {"q", "r"}},
	} {
		seen := map[string]bool{}
		var ns []lnode
		for _, e := range es {
			for _, id := range []string{e.from, e.to} {
				if !seen[id] {
					seen[id] = true
					ns = append(ns, lnode{id: id, w: 100, h: 60})
				}
			}
		}
		l := arrange(ns, es, overviewBudget, nil, tactic{beside: true})
		if len(l.partner) == 0 {
			t.Fatalf("%v: beside placed no box; the case tests nothing", es)
		}
		for r, row := range l.rows {
			if len(row) == 0 {
				t.Errorf("%v: row %d is empty: %v", es, r, l.rows)
			}
		}
	}
}

// A beside tactic that places no box draws the rows the plain drawing
// does, and says it is idle.
func TestAnIdleBesideDrawsThePlainRows(t *testing.T) {
	nodes := []lnode{{id: "a", w: 100, h: 60}, {id: "b", w: 100, h: 60}, {id: "c", w: 100, h: 60}, {id: "d", w: 100, h: 60}}
	edges := []ledge{{"a", "b"}, {"a", "c"}, {"b", "d"}, {"c", "d"}}
	plain := arrange(slices.Clone(nodes), edges, overviewBudget, nil, tactic{})
	beside := arrange(slices.Clone(nodes), edges, overviewBudget, nil, tactic{beside: true})
	if len(beside.partner) != 0 {
		t.Fatalf("beside placed %v; the case tests nothing", beside.partner)
	}
	if !beside.idle {
		t.Error("a beside tactic that placed nothing is not idle")
	}
	if !slices.EqualFunc(plain.rows, beside.rows, slices.Equal) {
		t.Errorf("an idle beside draws rows %v, the plain drawing %v", beside.rows, plain.rows)
	}
}
