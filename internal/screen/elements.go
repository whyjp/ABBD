package screen

import (
	"cmp"
	"math"
	"slices"
)

// Elements is how much one drawing asks a reader to take in at rest, counted
// the way Störrle sizes a diagram: its shapes, its line segments and its
// labels, and the badges set on them. It is a diagnostic. Nothing bounds it,
// so it never fails abbd lint.
//
// Lines are counted as the ink a reader sees: each straight stretch of drawn
// ink once, so a bundle that carries several lines on one trunk counts its
// trunk once, and every bend starts another stretch. Fewer bends or more
// merged ink lower the count.
//
// A fixed number of elements misjudges views of different sizes, so the
// count is read against the view's own range. Min is its skeleton: its boxes
// and chips with only the primary flow's lines, each one straight stretch,
// and their numbers. Max is every line the view could draw between its
// places drawn apart, each with one run more than the turns rule B1 allows
// it and a word. Percent is where the count stands between the two, and WarnAt the
// count at WarnShare of the range, past which Over is set.
type Elements struct {
	// Shapes are the boxes and the chips drawn, a folded row's "+N" chip
	// once. Ink is the straight stretches of drawn ink, and Bends how many
	// of them start at a turn of the stretch before. Words are the labels
	// at rest, on lines and beside the lead's hops. Badges are the ×N on
	// bundles, the +N pills on boxes, the lead's hop numbers and its stay
	// pill.
	Shapes int `json:"shapes"`
	Ink    int `json:"ink"`
	Bends  int `json:"bends"`
	Words  int `json:"words"`
	Badges int `json:"badges"`
	Count  int `json:"count"`
	// Lines are the lines a reader follows, one per request. Possible are
	// the lines the view could draw: one for each pair of places and kind
	// of relation it joins, drawn or counted on a box (overview_cut).
	// Stored relations lifted onto one pair count once, as the view never
	// draws them apart. Apart is the ink those lines take drawn one by one,
	// and Flow the lines the primary flow rides.
	Lines    int `json:"lines"`
	Possible int `json:"possible"`
	Apart    int `json:"apart"`
	Flow     int `json:"flow"`
	Min      int `json:"min"`
	Max      int `json:"max"`
	// Percent, rounded up so that a view past WarnShare never reads at it,
	// and WarnAt are left out where Min and Max are one number.
	Percent *int `json:"percent,omitempty"`
	WarnAt  *int `json:"warn_at,omitempty"`
	Over    bool `json:"over"`
}

// WarnShare is the share of a view's own range, in percent, past which its
// count is reported: a view passes it when 100*(Count-Min) > WarnShare*span.
// It is set from the positions of 829 views with a range in 15 repositories
// (docs/blueprint-svg.md, Elements), where about one view in ten passes it.
const WarnShare = 85

// inkMeasure is what a drawn view's ink measures for its elements: the
// straight runs a reader sees, the turns its paths make, and, for its traced
// lines, the runs they may take drawn apart under rule B1 (room).
type inkMeasure struct{ runs, bends, room, traced int }

// measureInk measures a view's ink for its elements. A run is straight ink
// as a reader sees it: stretches on one row or column that overlap or meet
// end to end read as one, whichever paths draw them, so a trunk several
// paths share counts once and a stub that runs on into its bus counts once.
// A bend is a turn within a drawn path. Each traced line may take one run
// more than the turns B1 allows it.
func measureInk(stretches []inkStretch, lines []turnsOf) inkMeasure {
	var k inkMeasure
	type track struct {
		upright bool
		at      float64
	}
	spans := map[track][][2]float64{}
	last := inkStretch{path: -1}
	for _, st := range stretches {
		if st.path == last.path && st.a == last.b {
			k.bends++
		}
		last = st
		switch {
		case st.a.x == st.b.x:
			t := track{true, st.a.x}
			spans[t] = append(spans[t], [2]float64{min(st.a.y, st.b.y), max(st.a.y, st.b.y)})
		case st.a.y == st.b.y:
			t := track{false, st.a.y}
			spans[t] = append(spans[t], [2]float64{min(st.a.x, st.b.x), max(st.a.x, st.b.x)})
		default:
			k.runs++
		}
	}
	for _, ss := range spans {
		slices.SortFunc(ss, func(a, b [2]float64) int { return cmp.Compare(a[0], b[0]) })
		end := math.Inf(-1)
		for _, s := range ss {
			if s[0] > end+1 {
				k.runs++
			}
			end = max(end, s[1])
		}
	}
	for _, ln := range lines {
		k.room += 1 + ln.allowed
		k.traced++
	}
	return k
}

// elementsOf counts d's elements, its ink measured k, and its range.
func elementsOf(d Drawing, k inkMeasure) Elements {
	type chip struct {
		x, y  int
		label string
	}
	chips := map[chip]bool{}
	for _, p := range d.Ports {
		chips[chip{p.X, p.Y, p.Label}] = true
	}
	e := Elements{Shapes: len(d.Boxes) + len(chips), Ink: k.runs, Bends: k.bends, Lines: len(d.ways)}
	for _, w := range d.Wires {
		e.Words += len(w.Words())
		if w.Count > 0 {
			e.Badges++
		}
	}
	for _, b := range d.Boxes {
		if b.More > 0 {
			e.Badges++
		}
	}
	// numbers are the lead's hop numbers and stay pill, which the skeleton
	// draws with the flow, and steps the words beside its hops.
	numbers, steps := 0, 0
	if l := d.Lead; l != nil {
		ways := map[int]bool{}
		for _, h := range l.Hops {
			ways[h.way] = true
			if h.Word != "" {
				steps++
			}
		}
		e.Flow, numbers = len(ways), len(l.Hops)
		if l.Stay != nil {
			numbers++
		}
		e.Words, e.Badges = e.Words+steps, e.Badges+numbers
	}
	e.Possible = len(d.ways) + len(d.Counted)
	e.Apart = k.room + max(0, e.Possible-k.traced)*(1+cornerBends)
	e.Count = e.Shapes + e.Ink + e.Words + e.Badges
	e.Min = e.Shapes + e.Flow + numbers
	e.Max = e.Shapes + e.Apart + e.Possible + numbers + steps
	if span := e.Max - e.Min; span > 0 {
		p := (100*(e.Count-e.Min) + span - 1) / span
		w := e.Min + span*WarnShare/100
		e.Percent, e.WarnAt, e.Over = &p, &w, e.Count > w
	}
	return e
}

// ElementTotals add up the element counts of several views, and name the
// views whose count passes their own warning point. The sums say how much
// the views hold together; no warning point is read from them.
type ElementTotals struct {
	Count int      `json:"count"`
	Min   int      `json:"min"`
	Max   int      `json:"max"`
	Over  []string `json:"over"`
}

// SumElements adds up the element counts of views.
func SumElements(views []ViewLint) ElementTotals {
	t := ElementTotals{Over: []string{}}
	for _, v := range views {
		e := v.Elements
		t.Count, t.Min, t.Max = t.Count+e.Count, t.Min+e.Min, t.Max+e.Max
		if e.Over {
			t.Over = append(t.Over, v.View)
		}
	}
	return t
}
