package screen

import (
	"bytes"
	"fmt"
	"maps"
	"math"
	"slices"
	"sort"
	"strings"
)

// This file holds every drawing of a model to docs/blueprint-svg.md's
// Drawing criteria and reports each breach by the boxes, lines and chips it
// involves. It reads the rendered SVG, as legible.go and trace.go do, so it
// judges what a reader sees.

// Criterion is one drawing criterion. A hard one must hold. A target one is
// minimized: each breach is counted. B1 and B2 feed the excess turns, and
// the placement criteria the placement debt, the two totals a change to the
// renderer is gated by.
type Criterion struct {
	ID        string `json:"id"`
	Hard      bool   `json:"hard"`
	Placement bool   `json:"placement,omitempty"`
	Says      string `json:"says"`
	// check finds the breaches of the criterion in one drawing.
	check func(l *look) []Breach
}

// Breach is one place where a drawing breaks a criterion: what it involves,
// what it measures against what the criterion allows, and why where the
// criterion asks, such as the obstacle a line turns around. Got is never
// more than Allowed when the criterion holds. Debt says a hard breach is
// one its view has recorded, so it does not fail abbd lint.
type Breach struct {
	View      string `json:"view"`
	Criterion string `json:"criterion"`
	Hard      bool   `json:"hard"`
	Debt      bool   `json:"debt"`
	Items     []Item `json:"items,omitempty"`
	Got       int    `json:"got"`
	Allowed   int    `json:"allowed"`
	Why       string `json:"why,omitempty"`
	Text      string `json:"text"`
}

// Item is a box or a chip a breach involves: the name the text gives it,
// and its id, a unit ID for a box.
type Item struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Totals are the per-view sums a change to the renderer is weighed by:
// the hard and target breaches, the breaches of each criterion, the excess
// turns and the placement debt, and the path measures of the trace.
type Totals struct {
	Hard     int            `json:"hard"`
	Target   int            `json:"target"`
	Criteria map[string]int `json:"criteria"`
	// ExcessTurns is each line's turns past its B1 allowance, plus every
	// jog. Placement is the breaches of the placement criteria.
	ExcessTurns int `json:"excess_turns"`
	Placement   int `json:"placement"`
	Steps       int `json:"steps"`
	Jogs        int `json:"jogs"`
	Crossings   int `json:"crossings"`
	// Tees are the points where a line's straight drop crosses a flat
	// stretch of another line from the same place, with no junction dot:
	// a T a reader takes for a branch. Each is one of the crossings.
	Tees        int `json:"tees"`
	Phantoms    int `json:"phantoms"`
	PathsOver2  int `json:"paths_over2"`
	TracedBends int `json:"traced_bends"`
	Back        int `json:"back"`
	// Weighed is the crossings plus stepCrossings for each excess turn, so
	// a turn saved may cost at most stepCrossings crossings.
	Weighed int `json:"weighed"`
	// Detours are the times lines turn back (B3) plus the pairs of
	// stretches that run beside their own line or bundle (B4).
	Detours int `json:"detours"`
	// Height is the drawing's height in px, so a change that runs a view
	// taller or shorter shows in the sum.
	Height int `json:"height"`
	// Beside, Pieces, InkCrossings and ForkLevels are what the drawing
	// shows at rest (rest.go), and Lines how many relations it traces.
	Beside       int `json:"beside"`
	Pieces       int `json:"pieces"`
	InkCrossings int `json:"ink_crossings"`
	ForkLevels   int `json:"fork_levels"`
	Lines        int `json:"lines"`
}

// Sum adds up the totals of several views.
func Sum(views []ViewLint) Totals {
	t := Totals{Criteria: map[string]int{}}
	for _, v := range views {
		o := v.Totals
		t.Hard, t.Target, t.ExcessTurns, t.Placement = t.Hard+o.Hard, t.Target+o.Target, t.ExcessTurns+o.ExcessTurns, t.Placement+o.Placement
		t.Steps, t.Jogs, t.Crossings, t.Tees = t.Steps+o.Steps, t.Jogs+o.Jogs, t.Crossings+o.Crossings, t.Tees+o.Tees
		t.Phantoms, t.PathsOver2, t.TracedBends, t.Back, t.Weighed = t.Phantoms+o.Phantoms, t.PathsOver2+o.PathsOver2, t.TracedBends+o.TracedBends, t.Back+o.Back, t.Weighed+o.Weighed
		t.Detours += o.Detours
		t.Height += o.Height
		t.Beside, t.Pieces, t.InkCrossings, t.ForkLevels, t.Lines = t.Beside+o.Beside, t.Pieces+o.Pieces, t.InkCrossings+o.InkCrossings, t.ForkLevels+o.ForkLevels, t.Lines+o.Lines
		for k, n := range o.Criteria {
			t.Criteria[k] += n
		}
	}
	return t
}

// ViewLint is what linting one drawing found.
type ViewLint struct {
	View     string   `json:"view"`
	Breaches []Breach `json:"breaches"`
	Totals   Totals   `json:"totals"`
}

// The thresholds of the criteria. They are this file's own constants, as
// legible.go's are, so a change to the renderer cannot loosen them.
const (
	// hubUsers is how many places must use a box before it is a hub.
	hubUsers = 3
	// cornerBends is how often a line between boxes in different columns
	// may turn: out of its start, across, and down into its end.
	cornerBends = 2
	// aroundBends is how many more turns an obstacle in the way allows:
	// aside and back within a column, as rule 4's exit bundle runs, and
	// round the obstacle between columns.
	aroundBends = 2
	// laneStep is how finely the obstacle search tries the rows' gaps.
	laneStep = 4.0
)

var criteria = []Criterion{
	{ID: "P1", Placement: true, Says: "a user stands in a row above what it uses", check: (*look).usersAbove},
	{ID: "P2", Placement: true, Says: "boxes used by exactly the same places stand in one row", check: (*look).siblingsInARow},
	{ID: "B1", Says: "a line turns 0 times within a column whose straight drop is clear and 2 when it is blocked, 2 between columns and 4 when every corner is blocked, 4 between ends that share a row, 2 for a branch of a fork into the row below", check: (*look).bendsAllowed},
	{ID: "B2", Says: "no stretch between two turns of a line is shorter than 24px", check: (*look).noJogs},
	{ID: "B3", Says: "a line never runs past a turn and comes back: no stretch runs against the stretch two before it", check: (*look).noTurnBacks},
	{ID: "B4", Says: "no two stretches of one line, or of one drawn bundle, run side by side within 12px for 24px or more", check: (*look).noDoubles},
	{ID: "J1", Hard: true, Says: "a bundle splits only in a gap next to the row of a place it splits toward", check: (*look).splitsAtEnds},
	{ID: "J2", Hard: true, Says: "every split and merge carries a dot (rule 17)", check: (*look).dotted},
	{ID: "J3", Hard: true, Says: "the ink joins no two places that no relation joins, but where lines ride roads and light whole (rule 18)", check: (*look).noPhantoms},
	{ID: "H1", Placement: true, Says: "a box used by 3 or more places stands within the span of its users' centres", check: (*look).hubsCentred},
	{ID: "H2", Placement: true, Says: "the forward lines of one kind into a hub reach it as one trunk", check: (*look).hubsOneTrunk},
	{ID: "O1", Placement: true, Says: "a box that only uses stands in the top rows", check: (*look).sourcesOnTop},
	{ID: "O2", Placement: true, Says: "a box that is only used stands in the bottom rows", check: (*look).sinksAtBottom},
	{ID: "O3", Hard: true, Says: "a line climbs only to close a cycle", check: (*look).climbsCloseCycles},
	{ID: "F1", Placement: true, Says: "a frame chip stands over or under the boxes its lines reach", check: (*look).chipsOverTheirBoxes},
	{ID: "R", Hard: true, Says: "rules 1 to 15 and 21 hold, and every relation is traced", check: (*look).rules},
	{ID: "S", Says: fmt.Sprintf("the drawing fits one screen, %dpx wide and %dpx tall less what its page sets above it", screenW, screenH), check: (*look).fitsOneScreen},
}

// screenW and screenH are the room a drawing has on one screen: a browser
// viewport of 1600x1000 CSS pixels at 100% zoom, the drawing box's width
// there, and the height below the page's header and the drawing's heading,
// 22px left below. What a page sets between the heading and the drawing
// takes from that height (room): the overview's list of questions,
// questionGroup for each kind of question and questionLine for each, and
// the note over a module's page and a focus view's page, one line and two
// at that width. These are measured on the page at that size; a longer
// question that wraps, a smaller window or a larger zoom or display
// scaling leaves less.
const (
	screenW, screenH            = 1320, 860
	questionGroup, questionLine = 37, 29
	insideNote, focusNote       = 22, 45
)

// overviewRoom is the height the overview has under a list of flows,
// stacks and tables of those counts.
func overviewRoom(counts ...int) int {
	h := 0
	for _, n := range counts {
		if n > 0 {
			h += questionGroup + n*questionLine
		}
	}
	if h > 0 {
		h += 4
	}
	return screenH - h
}

// pastScreen is how far, in pixels, d runs past one screen whose drawing
// has room px of height.
func pastScreen(d Drawing, room int) int {
	return max(0, d.Width-screenW, d.Height-room)
}

// Criteria lists the drawing criteria in the order lint checks them.
func Criteria() []Criterion { return slices.Clone(criteria) }

// view is one drawing of a model, by the name the checks give it.
// room is the height its drawing has on one screen (screenH).
type view struct {
	name   string
	d      Drawing
	budget int
	room   int
}

func views(m Model) []view {
	out := []view{{"overview", m.Overview, overviewWidth, overviewRoom(len(m.Flows), len(m.Stacks), len(m.Tables))}}
	focus := screenH - focusNote
	for _, f := range m.Focus {
		out = append(out, view{"overview focus " + f.Unit + " " + f.Name, f.Drawing, overviewWidth, focus})
	}
	for _, ins := range m.Insides {
		out = append(out, view{ins.ID + " " + ins.Name, ins.Drawing, insideWidth, screenH - insideNote})
		for _, f := range ins.Focus {
			out = append(out, view{ins.ID + " focus " + f.Unit + " " + f.Name, f.Drawing, overviewWidth, focus})
		}
	}
	return out
}

// ViewNames lists the names of every drawing of m, in the order Lint
// checks them.
func ViewNames(m Model) []string {
	var out []string
	for _, v := range views(m) {
		out = append(out, v.name)
	}
	return out
}

// sheetOf reads a drawing back from the SVG the page embeds.
func sheetOf(d Drawing) (sheet, error) {
	var b bytes.Buffer
	if err := site.ExecuteTemplate(&b, "drawing", d); err != nil {
		return sheet{}, err
	}
	return readSVG(b.String())
}

// Lint checks every drawing of m whose view name matches only, or every
// drawing when only is empty. A name matches when it starts with only or
// holds it as a whole word, so a unit ID selects its inside and its focus
// views.
func Lint(m Model, only string) ([]ViewLint, error) {
	var out []ViewLint
	for _, v := range views(m) {
		if only != "" && !strings.HasPrefix(v.name, only) && !strings.Contains(" "+v.name+" ", " "+only+" ") {
			continue
		}
		got, err := lintView(v)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", v.name, err)
		}
		out = append(out, got)
	}
	return out, nil
}

func lintView(v view) (ViewLint, error) {
	s, err := sheetOf(v.d)
	if err != nil {
		return ViewLint{}, err
	}
	l := newLook(v, s)
	vl := ViewLint{View: v.name, Totals: Totals{Criteria: map[string]int{}}}
	for _, c := range criteria {
		for _, b := range c.check(l) {
			b.View, b.Criterion, b.Hard = v.name, c.ID, c.Hard
			vl.Breaches = append(vl.Breaches, b)
			vl.Totals.Criteria[c.ID]++
			if c.Hard {
				vl.Totals.Hard++
			} else {
				vl.Totals.Target++
			}
			if c.Placement {
				vl.Totals.Placement++
			}
			if c.ID == "B3" || c.ID == "B4" {
				vl.Totals.Detours += b.Got
			}
		}
	}
	t := l.trace
	vl.Totals.Tees = len(tees(l))
	vl.Totals.Steps, vl.Totals.Jogs, vl.Totals.Crossings = steps(t), sum(t.Jogs), l.metrics.Crossings+vl.Totals.Tees
	vl.Totals.Phantoms, vl.Totals.PathsOver2, vl.Totals.TracedBends, vl.Totals.Back = t.Phantom, over2(t.Bends), sum(t.Bends), l.metrics.Back
	vl.Totals.ExcessTurns = vl.Totals.Jogs
	for _, ln := range l.lines() {
		vl.Totals.ExcessTurns += max(0, ln.turns-ln.allowed)
	}
	vl.Totals.Weighed = vl.Totals.Crossings + (teeCrossings-1)*vl.Totals.Tees + stepCrossings*vl.Totals.ExcessTurns
	vl.Totals.Height = v.d.Height
	vl.Totals.Beside, vl.Totals.Pieces, vl.Totals.InkCrossings = besideInk(s), straightPieces(s), inkCrossings(s)
	vl.Totals.ForkLevels, vl.Totals.Lines = l.forkLevels(), len(t.Ways)
	return vl, nil
}

// tees are the points where a traced line's straight drop crosses a flat
// stretch of another line leaving the same place, inside both, where no
// junction dot joins them and R1 has not counted a crossing.
func tees(l *look) []fpt {
	var out []fpt
	near := func(p fpt, ps []fpt, r float64) bool {
		for _, q := range ps {
			if dist(p, q) < r {
				return true
			}
		}
		return false
	}
	var dots []fpt
	for _, j := range l.s.junctions {
		dots = append(dots, j.at)
	}
	ws := l.trace.Ways
	for i, a := range ws {
		if len(a.pts) != 2 || a.pts[0].x != a.pts[1].x {
			continue
		}
		x, y0, y1 := a.pts[0].x, min(a.pts[0].y, a.pts[1].y), max(a.pts[0].y, a.pts[1].y)
		for j, b := range ws {
			if i == j || b.from != a.from {
				continue
			}
			for k := 1; k < len(b.pts); k++ {
				p, q := b.pts[k-1], b.pts[k]
				if p.y != q.y || p.y <= y0 || p.y >= y1 || x <= min(p.x, q.x) || x >= max(p.x, q.x) {
					continue
				}
				at := fpt{x, p.y}
				if !near(at, out, 1) && !near(at, dots, 3) && !near(at, l.metrics.crossed, 6) {
					out = append(out, at)
				}
			}
		}
	}
	return out
}

// look is one drawing read for the criteria: its shapes by id, the rows its
// boxes stand in, and the relations between its places.
type look struct {
	v       view
	s       sheet
	trace   Trace
	metrics Metrics
	// at is the shape index of each place id, and row the row of each box.
	at  map[string]int
	row map[int]int
	// rows is how many rows of boxes the drawing has.
	rows int
	// rels are the view's relations between two shapes, by shape index,
	// and boxRels those between two boxes.
	rels, boxRels [][2]int
}

func newLook(v view, s sheet) *look {
	l := &look{v: v, s: s, trace: trace(s, v.d.Relations), metrics: measure(s), at: map[string]int{}, row: map[int]int{}}
	for i, sh := range s.shapes {
		if _, ok := l.at[sh.id]; !ok {
			l.at[sh.id] = i
		}
		for _, id := range strings.Fields(sh.id) {
			if _, ok := l.at[id]; !ok {
				l.at[id] = i
			}
		}
	}
	// A row is a band of boxes whose heights overlap.
	var boxes []int
	for i, sh := range s.shapes {
		if sh.kind == "box" {
			boxes = append(boxes, i)
		}
	}
	sort.Slice(boxes, func(a, b int) bool { return s.shapes[boxes[a]].r.y < s.shapes[boxes[b]].r.y })
	bottom := math.Inf(-1)
	for _, i := range boxes {
		r := s.shapes[i].r
		if r.y >= bottom {
			l.rows++
		}
		bottom = math.Max(bottom, r.y+r.h)
		l.row[i] = l.rows - 1
	}
	seen := map[[2]int]bool{}
	for _, r := range v.d.Relations {
		a, okA := l.at[r[0]]
		b, okB := l.at[r[1]]
		if !okA || !okB || a == b || seen[[2]int{a, b}] {
			continue
		}
		seen[[2]int{a, b}] = true
		l.rels = append(l.rels, [2]int{a, b})
		if l.isBox(a) && l.isBox(b) {
			l.boxRels = append(l.boxRels, [2]int{a, b})
		}
	}
	return l
}

func (l *look) isBox(i int) bool { return l.s.shapes[i].kind == "box" }

// onFrame says shape i is a chip on the frame's edge, not on a box.
func (l *look) onFrame(i int) bool {
	sh := l.s.shapes[i]
	if sh.kind != "port" || l.s.frame == nil {
		return false
	}
	for _, b := range l.s.shapes {
		if b.kind == "box" && b.r.holds(sh.r) {
			return false
		}
	}
	return true
}

// name is how a breach names a shape: its label, and a box's id.
func (l *look) name(i int) string {
	sh := l.s.shapes[i]
	label := strings.Join(sh.lines, " ")
	if sh.kind == "port" {
		return `chip "` + label + `"`
	}
	return fmt.Sprintf(`box "%s" (%s)`, label, strings.Fields(sh.id + " ?")[0])
}

func (l *look) shapeName(id string) string {
	if i, ok := l.at[id]; ok {
		return l.name(i)
	}
	return id
}

// item is shape i as a breach lists it.
func (l *look) item(i int) Item {
	sh := l.s.shapes[i]
	if sh.kind == "port" {
		return Item{Kind: "chip", ID: sh.id, Name: l.name(i)}
	}
	return Item{Kind: "box", ID: strings.Fields(sh.id + " ?")[0], Name: l.name(i)}
}

func (l *look) items(is ...int) []Item {
	var out []Item
	for _, i := range is {
		out = append(out, l.item(i))
	}
	return out
}

// itemsOf lists the places with these ids, and an id no shape carries as
// a place of its own.
func (l *look) itemsOf(ids ...string) []Item {
	var out []Item
	for _, id := range ids {
		if i, ok := l.at[id]; ok {
			out = append(out, l.item(i))
		} else {
			out = append(out, Item{Kind: "place", ID: id, Name: id})
		}
	}
	return out
}

// byName sorts shape indexes by the names breaches give them.
func (l *look) byName(is []int) {
	sort.Slice(is, func(a, b int) bool { return l.name(is[a]) < l.name(is[b]) })
}

func names(items []Item) []string {
	var out []string
	for _, it := range items {
		out = append(out, it.Name)
	}
	return out
}

// plural is n with the noun, in the plural unless n is 1.
func plural(n int, noun string) string {
	switch {
	case n == 1:
		return "1 " + noun
	case strings.HasSuffix(noun, "ch"):
		return fmt.Sprintf("%d %ses", n, noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// P1. Rule 13 bounds how many lines may climb. This lists each relation
// between two boxes whose user does not stand in a row above what it uses,
// with how many rows short of that it stands: 1 in the same row, 2 one row
// below. Two boxes joined by one line with a head at each end stand
// well in one row or in rows next to each other: that line reads both
// ways, so no row order holds both of their relations. Standing further
// apart, they are one breach, with how many rows they stand past the next.
func (l *look) usersAbove() []Breach {
	var out []Breach
	both, far := map[[2]int]bool{}, map[[2]int]bool{}
	for _, w := range l.v.d.Wires {
		if w.Kind != "mutual" || len(w.ends) != 2 {
			continue
		}
		a, okA := l.at[w.ends[0]]
		b, okB := l.at[w.ends[1]]
		if okA && okB {
			both[[2]int{a, b}], both[[2]int{b, a}] = true, true
		}
	}
	for _, r := range l.boxRels {
		ru, rv := l.row[r[0]], l.row[r[1]]
		if both[r] {
			apart, pair := abs(ru-rv), [2]int{min(r[0], r[1]), max(r[0], r[1])}
			if apart > 1 && !far[pair] {
				far[pair] = true
				out = append(out, Breach{Items: l.items(r[0], r[1]), Got: apart - 1, Allowed: 0,
					Text: fmt.Sprintf("%s and %s use each other and stand %s apart, %s past rows next to each other, allowed 0", l.name(r[0]), l.name(r[1]), plural(apart, "row"), plural(apart-1, "row"))})
			}
			continue
		}
		if ru < rv {
			continue
		}
		where := "in the same row as"
		if ru > rv {
			where = plural(ru-rv, "row") + " below"
		}
		short := ru - rv + 1
		out = append(out, Breach{Items: l.items(r[0], r[1]), Got: short, Allowed: 0,
			Text: fmt.Sprintf("%s stands %s %s, which it uses, %s short of standing above it, allowed 0", l.name(r[0]), where, l.name(r[1]), plural(short, "row"))})
	}
	return out
}

// P2. Boxes reached from exactly the same places share an arrival bus that
// splits only in the gap above them (rule 4), so they stand in one row.
func (l *look) siblingsInARow() []Breach {
	users := map[int][]int{}
	for _, r := range l.rels {
		if l.isBox(r[1]) && !strings.HasSuffix(l.s.shapes[r[1]].id, twinMark) {
			users[r[1]] = append(users[r[1]], r[0])
		}
	}
	groups := map[string][]int{}
	for b, us := range users {
		sort.Ints(us)
		k := fmt.Sprint(us)
		groups[k] = append(groups[k], b)
	}
	var keys []string
	for k, bs := range groups {
		if len(bs) > 1 {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	var out []Breach
	for _, k := range keys {
		bs := groups[k]
		sort.Ints(bs)
		rows := map[int]bool{}
		for _, b := range bs {
			rows[l.row[b]] = true
		}
		if len(rows) > 1 {
			items := l.items(bs...)
			out = append(out, Breach{Items: items, Got: len(rows), Allowed: 1,
				Text: fmt.Sprintf("%s are used by exactly the same places and stand in %d rows, allowed 1", strings.Join(names(items), ", "), len(rows))})
		}
	}
	return out
}

// turnsOf is one traced line between two shapes with how often it turns, and
// how often its ends allow and why.
type turnsOf struct {
	a, b           int
	turns, allowed int
	why            string
}

// lines are the traced lines whose two ends the drawing shows.
func (l *look) lines() []turnsOf {
	var out []turnsOf
	for i, w := range l.trace.Ways {
		a, okA := l.at[w.from]
		b, okB := l.at[w.to]
		if !okA || !okB || len(w.pts) < 2 {
			continue
		}
		allowed, why := l.allowance(a, b)
		if allowed < cornerBends && l.forks(w, a, b) {
			allowed, why = cornerBends, "it forks off a stem its box shares, onto one track into the row below"
		}
		out = append(out, turnsOf{a, b, l.trace.Bends[i], allowed, why})
	}
	return out
}

// forks says line w, from box a to box b in the row right below, leaves a
// down a stem that another of a's lines of its kind drops straight down,
// turns onto one flat track in the gap between the rows that every branch
// off the stem turns onto and no other place's line runs along, and drops
// into b. A reader sees one line splitting into a row, not a line that
// bends, so it may turn twice.
func (l *look) forks(w tracedWay, a, b int) bool {
	p := w.pts
	sa, sb := l.s.shapes[a], l.s.shapes[b]
	if len(p) != 4 || sa.kind != "box" || sb.kind != "box" {
		return false
	}
	ra, rb := sa.r, sb.r
	near := func(u, v float64) bool { return math.Abs(u-v) <= 2 }
	if !near(p[0].x, p[1].x) || !near(p[1].y, p[2].y) || !near(p[2].x, p[3].x) ||
		!near(p[0].y, ra.y+ra.h) || !near(p[3].y, rb.y) || p[1].y <= p[0].y || p[3].y <= p[1].y {
		return false
	}
	// nextRow says a line from a's bottom down to top passes no row.
	nextRow := func(top float64) bool {
		for _, sh := range l.s.shapes {
			if sh.r.y >= ra.y+ra.h && sh.r.y+sh.r.h <= top {
				return false
			}
		}
		return top > p[1].y
	}
	if !nextRow(rb.y) {
		return false
	}
	// Every line down the stem ends in that row, so the stem is no trunk
	// that runs on past it, and every branch turns onto w's track.
	over := false
	for _, o := range l.trace.Ways {
		if o.from != w.from || o.to == w.to || len(o.pts) < 2 || o.pts[0] != p[0] {
			continue
		}
		if o.kind != w.kind || len(o.pts) > 4 || len(o.pts) == 3 || !nextRow(o.pts[len(o.pts)-1].y) {
			return false
		}
		if len(o.pts) == 4 && !near(o.pts[1].y, p[1].y) {
			return false
		}
		over = over || len(o.pts) == 2 && near(o.pts[1].x, p[0].x)
	}
	if !over {
		return false
	}
	lo, hi := min(p[1].x, p[2].x), max(p[1].x, p[2].x)
	for _, o := range l.trace.Ways {
		if o.from == w.from {
			continue
		}
		for i := 1; i < len(o.pts); i++ {
			u, v := o.pts[i-1], o.pts[i]
			if near(u.y, p[1].y) && near(v.y, p[1].y) && min(max(u.x, v.x), hi)-max(min(u.x, v.x), lo) > 2 {
				return false
			}
		}
	}
	return true
}

// B1. A line's allowed turns follow from where its ends stand: none within
// one column whose straight drop is clear, two when the drop is blocked,
// a corner of two between columns, two more when every corner is blocked,
// and four between ends that share a row.
func (l *look) bendsAllowed() []Breach {
	var out []Breach
	for _, ln := range l.lines() {
		if ln.turns > ln.allowed {
			out = append(out, Breach{Items: l.items(ln.a, ln.b), Got: ln.turns, Allowed: ln.allowed, Why: ln.why,
				Text: fmt.Sprintf("the line from %s to %s turns %s, allowed %d, %s in excess (%s)", l.name(ln.a), l.name(ln.b), plural(ln.turns, "time"), ln.allowed, plural(ln.turns-ln.allowed, "turn"), ln.why)})
		}
	}
	return out
}

// allowance is how often a line between shapes a and b may turn, and why.
// It reads only where the two shapes stand, never the drawn route, so a
// route cannot buy itself a larger allowance by how it leaves or arrives.
func (l *look) allowance(a, b int) (int, string) {
	ra, rb := l.s.shapes[a].r, l.s.shapes[b].r
	// Judge the line from the upper end down.
	if rb.y+rb.h <= ra.y {
		ra, rb = rb, ra
	}
	if rb.y < ra.y+ra.h {
		return cornerBends + aroundBends, "the ends share a row"
	}
	column := ra.x < rb.x+rb.w && rb.x < ra.x+ra.w
	var others []frect
	var names []string
	for k, sh := range l.s.shapes {
		if k == a || k == b || sh.r.overlaps(l.s.shapes[a].r) || sh.r.overlaps(l.s.shapes[b].r) {
			continue
		}
		others = append(others, sh.r)
		names = append(names, l.name(k))
	}
	if column && clearDrop(ra, rb, others) {
		return 0, "same column, the straight drop is clear"
	}
	if !column && clearCorner(ra, rb, others) {
		return cornerBends, "different columns, a corner fits"
	}
	// The obstacles are the shapes that stand between the two ends.
	allowed, what := aroundBends, "same column"
	lo, hi := math.Max(ra.x, rb.x), math.Min(ra.x+ra.w, rb.x+rb.w)
	if !column {
		allowed, what = cornerBends+aroundBends, "different columns"
		lo, hi = math.Min(ra.x, rb.x), math.Max(ra.x+ra.w, rb.x+rb.w)
	}
	between := frect{lo, ra.y + ra.h, hi - lo, rb.y - ra.y - ra.h}
	var in []string
	for k, r := range others {
		if r.overlaps(between) {
			in = append(in, names[k])
		}
	}
	if len(in) == 0 {
		return allowed, what + ", no room between the rows"
	}
	return allowed, what + ", around " + strings.Join(in, ", ")
}

// free is what is left of [lo, hi] once the spans of the shapes that meet
// the band (top, bottom) are taken out.
func free(lo, hi, top, bottom float64, others []frect) [][2]float64 {
	spans := [][2]float64{{lo, hi}}
	for _, r := range others {
		if r.y >= bottom || r.y+r.h <= top {
			continue
		}
		var next [][2]float64
		for _, sp := range spans {
			if r.x+r.w+1 <= sp[0] || r.x-1 >= sp[1] {
				next = append(next, sp)
				continue
			}
			if r.x-1 > sp[0] {
				next = append(next, [2]float64{sp[0], r.x - 1})
			}
			if r.x+r.w+1 < sp[1] {
				next = append(next, [2]float64{r.x + r.w + 1, sp[1]})
			}
		}
		spans = next
	}
	return spans
}

// clearDrop says a straight line can drop from a's bottom into b's top.
func clearDrop(a, b frect, others []frect) bool {
	lo, hi := math.Max(a.x, b.x), math.Min(a.x+a.w, b.x+b.w)
	return len(free(lo, hi, a.y+a.h, b.y, others)) > 0
}

// clearCorner says a line can drop from a's bottom, cross at some height
// and drop into b's top, turning twice.
func clearCorner(a, b frect, others []frect) bool {
	top, bottom := a.y+a.h, b.y
	for y := top + laneStep/2; y < bottom; y += laneStep {
		outs := free(a.x, a.x+a.w, top, y, others)
		ins := free(b.x, b.x+b.w, y, bottom, others)
		across := free(math.Inf(-1), math.Inf(1), y, y+0.01, others)
		for _, c := range across {
			if meets(c, outs) && meets(c, ins) {
				return true
			}
		}
	}
	return false
}

func meets(c [2]float64, spans [][2]float64) bool {
	for _, sp := range spans {
		if sp[0] <= c[1] && c[0] <= sp[1] {
			return true
		}
	}
	return false
}

// B2. A jog is a step a person drawing the line by hand would not make.
func (l *look) noJogs() []Breach {
	var out []Breach
	for _, w := range l.trace.Ways {
		if n := jogs(w.pts); n > 0 {
			out = append(out, Breach{Items: l.itemsOf(w.from, w.to), Got: n, Allowed: 0,
				Text: fmt.Sprintf("the line from %s to %s has %s under %gpx between turns, allowed 0, at %s", l.shapeName(w.from), l.shapeName(w.to), plural(n, "stretch"), jogLen, pointList(w.pts))})
		}
	}
	return out
}

// B3. A line turns back where a stretch runs one way, the next stretch
// crosses, and the one after runs back the other way: it rode past the
// turn it needed and came back on a lane beside its own. Each breach gives
// how many times the line turns back and how far it rode past, the shorter
// of the two stretches that run against each other. Lines that share a
// turn-back, as the lines of one trunk do, count it once, on the first
// line traced along it. A back line, which runs up or leftward to its end,
// climbs out through a gutter beside its row and turns back by its nature,
// so rule 13 counts it and B3 does not.
func (l *look) noTurnBacks() []Breach {
	var out []Breach
	seen := map[[2]fpt]bool{}
	for _, w := range l.trace.Ways {
		if backWay(w.pts) {
			continue
		}
		n, past := 0, 0.0
		for _, tb := range turnsBack(w.pts) {
			if !seen[tb.step] {
				seen[tb.step] = true
				n++
				past += tb.past
			}
		}
		if n > 0 {
			out = append(out, Breach{Items: l.itemsOf(w.from, w.to), Got: n, Allowed: 0,
				Text: fmt.Sprintf("the line from %s to %s turns back %s, riding %.0fpx past its turn, at %s", l.shapeName(w.from), l.shapeName(w.to), plural(n, "time"), past, pointList(w.pts))})
		}
	}
	return out
}

// backWay says a path runs against the reading direction, up to its end or
// leftward along one row, as rule 13's back lines do.
func backWay(pts []fpt) bool {
	a, b := pts[0], pts[len(pts)-1]
	dy, dx := b.y-a.y, b.x-a.x
	return !(dy > 2 || math.Abs(dy) <= 2 && dx >= 0)
}

// turnBack is one place a path runs back against the stretch two before:
// the stretch that crosses between them, and how far it rode past.
type turnBack struct {
	step [2]fpt
	past float64
}

// turnsBack lists where a path of turning points runs back against the
// stretch two before.
func turnsBack(pts []fpt) []turnBack {
	var out []turnBack
	for i := 0; i+3 < len(pts); i++ {
		a, b := dirOf(pts[i], pts[i+1]), dirOf(pts[i+2], pts[i+3])
		if a.x == -b.x && a.y == -b.y {
			out = append(out, turnBack{[2]fpt{pts[i+1], pts[i+2]}, math.Min(dist(pts[i], pts[i+1]), dist(pts[i+2], pts[i+3]))})
		}
	}
	return out
}

// doubleGap is how close two parallel stretches of one line run before a
// reader sees one doubled line, and jogLen how long they must run so.
const doubleGap = 12.0

// B4. A doubled track is two stretches of one traced line, or of one drawn
// bundle, that run the same way side by side closer than doubleGap for
// jogLen or more: a reader sees the line twice. Two stretches that run
// against each other are a turn-back, which B3 counts. A pair of a
// bundle's subpaths along a pair a traced line already counts is that
// line's. Each breach gives how many such pairs the line or bundle has.
func (l *look) noDoubles() []Breach {
	var out []Breach
	var counted [][2][2]fpt
	for _, w := range l.trace.Ways {
		segs, n := stretches(w.pts), 0
		for i := range segs {
			for _, q := range segs[i+1:] {
				if sideBySide(segs[i], q) {
					counted = append(counted, [2][2]fpt{segs[i], q})
					n++
				}
			}
		}
		if n > 0 {
			out = append(out, Breach{Items: l.itemsOf(w.from, w.to), Got: n, Allowed: 0,
				Text: fmt.Sprintf("the line from %s to %s runs beside itself %s, at %s", l.shapeName(w.from), l.shapeName(w.to), plural(n, "time"), pointList(w.pts))})
		}
	}
	// A bundle's own subpaths beside each other: a stroke beside itself is
	// its line's, which the trace counts above.
	groups := map[int][]int{}
	for k, st := range l.s.strokes {
		groups[st.group] = append(groups[st.group], k)
	}
	for _, g := range indexes(groups) {
		n := 0
		for i, a := range groups[g] {
			for _, b := range groups[g][i+1:] {
				for _, p := range stretches(l.s.strokes[a].pts) {
					for _, q := range stretches(l.s.strokes[b].pts) {
						if sideBySide(p, q) && !slices.ContainsFunc(counted, func(c [2][2]fpt) bool {
							return onStretch(p, c[0]) && onStretch(q, c[1]) || onStretch(p, c[1]) && onStretch(q, c[0])
						}) {
							n++
						}
					}
				}
			}
		}
		if n > 0 {
			out = append(out, Breach{Got: n, Allowed: 0, Text: fmt.Sprintf("a drawn bundle runs beside itself %s", plural(n, "time"))})
		}
	}
	return out
}

// stretches are the straight pieces of a path of points.
func stretches(pts []fpt) [][2]fpt {
	var out [][2]fpt
	for i := 1; i < len(pts); i++ {
		out = append(out, [2]fpt{pts[i-1], pts[i]})
	}
	return out
}

// sideBySide says two straight stretches run the same way in parallel,
// apart but closer than doubleGap, over jogLen or more.
func sideBySide(p, q [2]fpt) bool {
	if dirOf(p[0], p[1]) != dirOf(q[0], q[1]) {
		return false
	}
	ph, qh := math.Abs(p[0].y-p[1].y) < 0.5, math.Abs(q[0].y-q[1].y) < 0.5
	pv, qv := math.Abs(p[0].x-p[1].x) < 0.5, math.Abs(q[0].x-q[1].x) < 0.5
	var gap, run float64
	switch {
	case ph && qh:
		gap, run = math.Abs(p[0].y-q[0].y), spanOf(p[0].x, p[1].x, q[0].x, q[1].x)
	case pv && qv:
		gap, run = math.Abs(p[0].x-q[0].x), spanOf(p[0].y, p[1].y, q[0].y, q[1].y)
	default:
		return false
	}
	return gap > 0.5 && gap < doubleGap+0.5 && run >= jogLen
}

// onStretch says the stretch p lies on the line of the stretch q and shares
// jogLen or more of it.
func onStretch(p, q [2]fpt) bool {
	switch {
	case math.Abs(p[0].y-p[1].y) < 0.5 && math.Abs(q[0].y-q[1].y) < 0.5 && math.Abs(p[0].y-q[0].y) < 0.5:
		return spanOf(p[0].x, p[1].x, q[0].x, q[1].x) >= jogLen
	case math.Abs(p[0].x-p[1].x) < 0.5 && math.Abs(q[0].x-q[1].x) < 0.5 && math.Abs(p[0].x-q[0].x) < 0.5:
		return spanOf(p[0].y, p[1].y, q[0].y, q[1].y) >= jogLen
	}
	return false
}

// spanOf is how far the spans a..b and c..d overlap.
func spanOf(a, b, c, d float64) float64 {
	return math.Min(math.Max(a, b), math.Max(c, d)) - math.Max(math.Min(a, b), math.Min(c, d))
}

// position places a height among the rows: 2k+1 inside row k, 2k in the
// gap above it. A frame's top chips stand at -1 and its bottom chips past
// the last row.
func (l *look) position(y float64) int {
	p := 0
	for k := 0; k < l.rows; k++ {
		top, bottom := math.Inf(1), math.Inf(-1)
		for i, r := range l.row {
			if r == k {
				top, bottom = math.Min(top, l.s.shapes[i].r.y), math.Max(bottom, l.s.shapes[i].r.y+l.s.shapes[i].r.h)
			}
		}
		switch {
		case y < top:
			return p
		case y <= bottom:
			return p + 1
		}
		p += 2
	}
	return p
}

// placeAt is the row position of the shape a place id names.
func (l *look) placeAt(id string) int {
	i := l.at[id]
	if r, ok := l.row[i]; ok {
		return 2*r + 1
	}
	sh := l.s.shapes[i].r
	if l.s.frame != nil && sh.y+sh.h/2 < l.s.frame.y+l.s.frame.h/2 {
		return -1
	}
	return 2*l.rows + 1
}

// J1. Rule 4 lets lines from different places merge midway, so only a
// split is held. A split is a dot where the lines through it arrive as one
// strand, from one side, and leave it on two sides or more. It stands in a
// gap next to the row of a place one of those lines goes to.
func (l *look) splitsAtEnds() []Breach {
	var out []Breach
	for _, j := range l.s.junctions {
		if !l.splitAt(j) {
			continue
		}
		var tos []string
		var ends []int
		for _, w := range l.trace.Ways {
			if _, _, ok := passing(j.at, w.pts); ok && w.kind == j.kind && !slices.Contains(tos, w.to) {
				tos = append(tos, w.to)
				ends = append(ends, l.placeAt(w.to))
			}
		}
		p := l.position(j.at.y)
		far := math.MaxInt
		for _, e := range ends {
			far = min(far, absInt(p-e))
		}
		if far > 1 {
			sort.Strings(tos)
			items := l.itemsOf(tos...)
			out = append(out, Breach{Items: items, Got: far / 2, Allowed: 0,
				Text: fmt.Sprintf("a bundle splits toward %s at %g,%g, %s from the nearest of their rows, allowed 0", strings.Join(names(items), ", "), j.at.x, j.at.y, plural(far/2, "row"))})
		}
	}
	return out
}

// splitAt says the lines of j's kind through dot j arrive from one side
// and leave on more than one, to more than one place.
func (l *look) splitAt(j junction) bool {
	ins, outs, tos := map[fpt]bool{}, map[fpt]bool{}, map[string]bool{}
	for _, w := range l.trace.Ways {
		in, out, ok := passing(j.at, w.pts)
		if !ok || w.kind != j.kind {
			continue
		}
		ins[in], outs[out], tos[w.to] = true, true, true
	}
	return len(ins) == 1 && len(outs) > 1 && len(tos) > 1
}

// passing says a path passes point p, and in which direction it arrives
// there and leaves. A path that starts at p arrives from nowhere, the zero
// direction, and one that ends there leaves to nowhere.
func passing(p fpt, pts []fpt) (in, out fpt, ok bool) {
	for i := 1; i < len(pts); i++ {
		if segDist(p, pts[i-1], pts[i]) >= 1 {
			continue
		}
		d := dirOf(pts[i-1], pts[i])
		switch {
		case i == 1 && dist(p, pts[0]) < 1:
			return fpt{}, d, true
		case dist(p, pts[i]) >= 1:
			return d, d, true
		case i == len(pts)-1:
			return d, fpt{}, true
		}
		return d, dirOf(pts[i], pts[i+1]), true
	}
	return fpt{}, fpt{}, false
}

func absInt(x int) int { return max(x, -x) }

// J2 is rule 17.
func (l *look) dotted() []Breach {
	var out []Breach
	for _, p := range l.trace.Unmarked {
		out = append(out, Breach{Got: 1, Allowed: 0, Text: fmt.Sprintf("ink splits or merges at %g,%g without a dot", p.x, p.y)})
	}
	return out
}

// J3 is rule 18, held at zero but where lines ride roads (confluence.go)
// and the drawing holds their routes: there ink may join places no
// relation joins, since each line's route lights whole under the pointer.
func (l *look) noPhantoms() []Breach {
	if l.v.d.roads && l.s.routed {
		return nil
	}
	var out []Breach
	for _, w := range l.trace.Phantoms {
		out = append(out, Breach{Items: l.itemsOf(w.from, w.to), Got: 1, Allowed: 0,
			Text: fmt.Sprintf("the ink leads from %s to %s, which no relation joins, at %s", l.shapeName(w.from), l.shapeName(w.to), pointList(w.pts))})
	}
	return out
}

// hubs are the boxes hubUsers or more places use, each with its users.
func (l *look) hubs() map[int][]int { return hubsOf(l.rels, l.isBox) }

// hubsOf reads the hubs off a view's relations between drawn places: the
// boxes hubUsers or more other places use, each with its users in the
// order they first appear. Lint and route both call it, so a line route
// drops straight never ends on a box lint holds to H2.
func hubsOf[K comparable](rels [][2]K, isBox func(K) bool) map[K][]K {
	users, seen := map[K][]K{}, map[[2]K]bool{}
	for _, r := range rels {
		if r[0] == r[1] || seen[r] || !isBox(r[1]) {
			continue
		}
		seen[r] = true
		users[r[1]] = append(users[r[1]], r[0])
	}
	for b, us := range users {
		if len(us) < hubUsers {
			delete(users, b)
		}
	}
	return users
}

func indexes[V any](m map[int]V) []int { return slices.Sorted(maps.Keys(m)) }

// H1. A hub's centre lies within the span of its users' centres, so its
// lines come down to it from both sides.
func (l *look) hubsCentred() []Breach {
	var out []Breach
	hs := l.hubs()
	for _, h := range indexes(hs) {
		lo, hi := math.Inf(1), math.Inf(-1)
		for _, u := range hs[h] {
			c := l.s.shapes[u].r.center().x
			lo, hi = math.Min(lo, c), math.Max(hi, c)
		}
		c := l.s.shapes[h].r.center().x
		off := math.Max(lo-c, c-hi)
		if off > 1 {
			out = append(out, Breach{Items: l.items(h), Got: int(math.Round(off)), Allowed: 0,
				Text: fmt.Sprintf("hub %s, used by %d places, stands %dpx outside the span of their centres, allowed 0", l.name(h), len(hs[h]), int(math.Round(off)))})
		}
	}
	return out
}

// H2. Rule 4 bundles the lines of one kind into one edge of a box. A hub's
// forward lines of one kind reach it as one bundle, whichever edge they
// reach. The dashed lines that climb to it are rule 13's, not H2's.
func (l *look) hubsOneTrunk() []Breach {
	var out []Breach
	hs := l.hubs()
	for _, h := range indexes(hs) {
		r := l.s.shapes[h].r.grow(5)
		groups := map[string]map[int]bool{}
		for _, w := range l.s.strokes {
			if len(w.pts) < 2 || strings.Contains(" "+w.class+" ", " joins ") || strings.Contains(" "+w.class+" ", " back ") || !r.contains(w.pts[len(w.pts)-1], 0) || r.contains(w.pts[0], 0) {
				continue
			}
			k := colour(w.class)
			if groups[k] == nil {
				groups[k] = map[int]bool{}
			}
			groups[k][w.group] = true
		}
		for _, k := range slices.Sorted(maps.Keys(groups)) {
			if n := len(groups[k]); n > 1 {
				out = append(out, Breach{Items: l.items(h), Got: n, Allowed: 1,
					Text: fmt.Sprintf("hub %s takes %d bundles of %s lines, allowed 1", l.name(h), n, k)})
			}
		}
	}
	return out
}

// oneWay sorts the boxes that join other boxes into those that only use,
// those that are only used, and the rest.
func (l *look) oneWay() (sources, sinks map[int]bool) {
	in, out := map[int]bool{}, map[int]bool{}
	for _, r := range l.boxRels {
		out[r[0]], in[r[1]] = true, true
	}
	sources, sinks = map[int]bool{}, map[int]bool{}
	for b := range out {
		if !in[b] {
			sources[b] = true
		}
	}
	for b := range in {
		if !out[b] {
			sinks[b] = true
		}
	}
	return sources, sinks
}

// frameSide says which of the frame's edges box b has a line to: -1 the
// top, 1 the bottom, 0 neither or both.
func (l *look) frameSide(b int) int {
	top, bottom := false, false
	for _, r := range l.rels {
		o := -1
		switch b {
		case r[0]:
			o = r[1]
		case r[1]:
			o = r[0]
		}
		if o < 0 || !l.onFrame(o) {
			continue
		}
		if l.placeAt(l.s.shapes[o].id) < 0 {
			top = true
		} else {
			bottom = true
		}
	}
	switch {
	case top && !bottom:
		return -1
	case bottom && !top:
		return 1
	}
	return 0
}

// edgeRows checks that each box of set stands where only boxes of set, or
// boxes that join no other box, stand beyond it toward one edge: dir -1 the
// top, 1 the bottom. A box whose only frame line pulls it the other way is
// left to rule 10.
func (l *look) edgeRows(set map[int]bool, dir int, what string) []Breach {
	joined := map[int]bool{}
	for _, r := range l.boxRels {
		joined[r[0]], joined[r[1]] = true, true
	}
	var out []Breach
	for _, b := range indexes(set) {
		if l.frameSide(b) == -dir {
			continue
		}
		var past []int
		for k, r := range l.row {
			if (r-l.row[b])*dir > 0 && joined[k] && !set[k] {
				past = append(past, k)
			}
		}
		if len(past) > 0 {
			l.byName(past)
			side := "above"
			if dir > 0 {
				side = "below"
			}
			verb := "stand"
			if len(past) == 1 {
				verb = "stands"
			}
			items := l.items(append([]int{b}, past...)...)
			out = append(out, Breach{Items: items, Got: len(past), Allowed: 0,
				Text: fmt.Sprintf("%s %s, but %s %s %s it, allowed 0", l.name(b), what, strings.Join(names(items[1:]), ", "), verb, side)})
		}
	}
	return out
}

// O1.
func (l *look) sourcesOnTop() []Breach {
	sources, _ := l.oneWay()
	return l.edgeRows(sources, -1, "only uses")
}

// O2.
func (l *look) sinksAtBottom() []Breach {
	_, sinks := l.oneWay()
	return l.edgeRows(sinks, 1, "is only used")
}

// O3. A relation whose user stands in a row below what it uses climbs, and
// the climb is allowed only when the used box reaches the user back, so
// the two stand in one cycle.
func (l *look) climbsCloseCycles() []Breach {
	next := map[int][]int{}
	for _, r := range l.boxRels {
		next[r[0]] = append(next[r[0]], r[1])
	}
	reaches := func(from, to int) bool {
		seen := map[int]bool{from: true}
		queue := []int{from}
		for len(queue) > 0 {
			c := queue[0]
			queue = queue[1:]
			for _, n := range next[c] {
				if n == to {
					return true
				}
				if !seen[n] {
					seen[n] = true
					queue = append(queue, n)
				}
			}
		}
		return false
	}
	var out []Breach
	for _, r := range l.boxRels {
		if l.row[r[0]] > l.row[r[1]] && !reaches(r[1], r[0]) {
			out = append(out, Breach{Items: l.items(r[0], r[1]), Got: l.row[r[0]] - l.row[r[1]], Allowed: 0,
				Text: fmt.Sprintf("the line from %s climbs %s to %s, which does not reach it back", l.name(r[0]), plural(l.row[r[0]]-l.row[r[1]], "row"), l.name(r[1]))})
		}
	}
	return out
}

// F1. A frame chip's span meets the span its boxes cover, so its line can
// drop straight or with one corner into them.
func (l *look) chipsOverTheirBoxes() []Breach {
	boxes := map[int][]int{}
	for _, r := range l.rels {
		for _, e := range [][2]int{r, {r[1], r[0]}} {
			if l.onFrame(e[0]) && l.isBox(e[1]) && !slices.Contains(boxes[e[0]], e[1]) {
				boxes[e[0]] = append(boxes[e[0]], e[1])
			}
		}
	}
	var out []Breach
	for _, c := range indexes(boxes) {
		lo, hi := math.Inf(1), math.Inf(-1)
		for _, b := range boxes[c] {
			r := l.s.shapes[b].r
			lo, hi = math.Min(lo, r.x), math.Max(hi, r.x+r.w)
		}
		r := l.s.shapes[c].r
		off := math.Max(lo-r.x-r.w, r.x-hi)
		if off > 0 {
			side := "right"
			if r.x+r.w <= lo {
				side = "left"
			}
			l.byName(boxes[c])
			items := l.items(append([]int{c}, boxes[c]...)...)
			out = append(out, Breach{Items: items, Got: int(math.Round(off)), Allowed: 0,
				Text: fmt.Sprintf("%s stands %dpx %s of %s, which its lines reach, allowed 0", l.name(c), int(math.Round(off)), side, strings.Join(names(items[1:]), ", "))})
		}
	}
	return out
}

// S. A drawing past the screen makes the reader scroll to see it whole, so
// what it shows can no longer be read at a glance.
func (l *look) fitsOneScreen() []Breach {
	over := pastScreen(l.v.d, l.v.room)
	if over == 0 {
		return nil
	}
	return []Breach{{Got: over, Allowed: 0,
		Text: fmt.Sprintf("the drawing is %dx%d px, %dpx past one screen's %dx%d", l.v.d.Width, l.v.d.Height, over, screenW, l.v.room)}}
}

// R. The numbered rules the checker already holds, one breach per rule a
// drawing breaks, and every relation lost from the ink.
func (l *look) rules() []Breach {
	var out []Breach
	for _, v := range l.metrics.violations(l.v.budget) {
		rule, msg, _ := strings.Cut(v, ": ")
		out = append(out, Breach{Got: 1, Allowed: 0, Why: "rule " + rule, Text: "rule " + rule + ": " + msg})
	}
	for _, r := range l.trace.Missing {
		out = append(out, Breach{Items: l.itemsOf(r[0], r[1]), Got: 1, Allowed: 0, Why: "rule 2",
			Text: fmt.Sprintf("rule 2: no ink carries the relation from %s to %s", l.shapeName(r[0]), l.shapeName(r[1]))})
	}
	if l.trace.SizeSpread > maxSpread {
		out = append(out, Breach{Got: int(math.Round(l.trace.SizeSpread * 100)), Allowed: int(maxSpread * 100), Why: "rule 21",
			Text: fmt.Sprintf("rule 21: the tallest box is %.2f times the median, limit %.1f", l.trace.SizeSpread, maxSpread)})
	}
	return out
}
