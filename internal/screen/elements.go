package screen

// Elements is how much one drawing asks a reader to take in at rest, counted
// the way Störrle sizes a diagram: its shapes, its lines and its labels, and
// the badges set on them. It is a diagnostic. Nothing bounds it, so it never
// fails abbd lint.
//
// A fixed number of elements misjudges views of different sizes, so the
// count is read against the view's own range. Min is its skeleton: its boxes
// and chips with only the primary flow's lines and their numbers. Max is
// every line the view could draw between its places drawn apart, each with
// a word, and no bundle badges. Percent is where the count stands between
// the two, past 100 where badges sit on a view that already draws every
// line with a word, and WarnAt the count at WarnShare of the range, past
// which Over is set.
type Elements struct {
	// Shapes are the boxes and the chips drawn, a folded row's "+N" chip
	// once. Lines are the lines a reader follows, one per request, however
	// many pieces of ink draw it. Words are the labels at rest, on lines
	// and beside the lead's hops. Badges are the ×N on bundles, the +N
	// pills on boxes, the lead's hop numbers and its stay pill.
	Shapes int `json:"shapes"`
	Lines  int `json:"lines"`
	Words  int `json:"words"`
	Badges int `json:"badges"`
	Count  int `json:"count"`
	// Possible are the lines the view could draw: one for each pair of
	// places and kind of relation it joins, drawn or counted on a box
	// (overview_cut). Stored relations lifted onto one pair count once, as
	// the view never draws them apart. Flow is the lines the primary flow
	// rides.
	Possible int `json:"possible"`
	Flow     int `json:"flow"`
	Min      int `json:"min"`
	Max      int `json:"max"`
	// Percent and WarnAt are left out where Min and Max are one number.
	Percent *int `json:"percent,omitempty"`
	WarnAt  *int `json:"warn_at,omitempty"`
	Over    bool `json:"over"`
}

// WarnShare is the share of a view's own range, in percent, past which its
// count is reported: a view passes it when 100*(Count-Min) > WarnShare*span.
// A view with no lead that draws every line it could and gives half of them
// a word stands there; a lead puts that drawing a little lower. It is
// chosen, not measured.
const WarnShare = 75

// elementsOf counts d's elements and its range.
func elementsOf(d Drawing) Elements {
	type chip struct {
		x, y  int
		label string
	}
	chips := map[chip]bool{}
	for _, p := range d.Ports {
		chips[chip{p.X, p.Y, p.Label}] = true
	}
	e := Elements{Shapes: len(d.Boxes) + len(chips), Lines: len(d.ways)}
	for _, w := range d.Wires {
		if w.Label != nil {
			e.Words++
		}
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
	e.Count = e.Shapes + e.Lines + e.Words + e.Badges
	e.Min = e.Shapes + e.Flow + numbers
	e.Max = e.Shapes + 2*e.Possible + numbers + steps
	if span := e.Max - e.Min; span > 0 {
		p := 100 * (e.Count - e.Min) / span
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
