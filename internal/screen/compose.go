package screen

import (
	"fmt"
	"slices"
	"strings"

	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/store"
)

// An overview drawn from a stored record whose primary flow shows two
// stages or more and whose record names a zone is composed, the way the
// exemplars are drawn, instead of ranked by its lines: the record says
// where each box stands before any line is routed.
//
// The flow runs left to right along one row, the axis. The record's zones
// are rooms: a zone that holds stages stands on the axis around them, in
// the order of its first stage, with its other boxes under its stages. A
// zone that holds no stage is a room on a band of its own, above the axis
// when its lines mostly leave it, else below. A box no zone holds stands
// alone where its lines lead. A box with no line drawn stands on the
// bottom row, the boxes most used marked as the shared base. Contracts
// nothing in the repository provides stand on an outside band right of
// the axis. Pairs the record leaves undrawn take no part. The lines are
// routed as every overview's are (route.go), on roads.
//
// Stage numbers stand on the boxes, not the lines. A flow line is drawn
// bold only between two stages a drawn line joins; the gaps are omissions
// of the record (FlowGaps).

// room is one rectangle of boxes on the composed overview: a zone, the
// stages of no zone, a lone box, the bottom row or the outside band.
type room struct {
	title string
	// ids stand in reading order: the first lead ids on the axis row, the
	// rest row by row in cols columns under them.
	ids  []string
	lead int
	cols int
	// mark says the room is the shared base, drawn with its band only.
	mark bool
	// at is the room's left x once placed, row its first row.
	at, row int
}

// composer is the plan of one composed overview.
type composer struct {
	stage map[string]int
	zone  map[string]string
	names []string
	// order is every staged box, by stage then id.
	order []string
	rooms []room
	gaps  []string
}

// roomGap is how far apart two rooms of a band stand: room for a road of
// lines and the bands' edges.
const roomGap = 72

// stageR is how large a stage's number stands on its box's corner, and
// stageGap how far right of the corner its name starts.
const stageR, stageGap = 9, 13

// roomHead is the least gap between two rows: room for the band under one
// row's room and the title over the next.
const roomHead = 52

// baseTitle says what the bottom row's marked boxes share, and no more:
// the record names no such grouping.
const baseTitle = "여러 곳이 쓰는 상자 · 선 생략"

// baseUses is how many undrawn pairs mark a box with no line as the shared
// base.
const baseUses = 3

// composable says o, planned without a cut, is composed under its record:
// the primary flow shows two stages and the record a zone with a box.
func composable(o *overview, ov store.Overview) bool {
	if o.flow == nil || o.flow.stagesOn(o.boxOf) == nil {
		return false
	}
	for _, z := range ov.Zones {
		for _, u := range z.Claim.Units {
			if o.boxOf(u) != "" {
				return true
			}
		}
	}
	return false
}

// boxOf is the top-level box that shows unit u on the whole overview.
func (o *overview) boxOf(u model.ID) string {
	if c := o.tree.chain(u); len(c) > 0 && slices.Contains(o.modules, c[0]) {
		return string(c[0])
	}
	return ""
}

// drawnPairs are the lines the planned overview will draw, from user to
// what it uses.
func (o *overview) drawnPairs() []ledge {
	var out []ledge
	for _, k := range o.pairs {
		out = append(out, ledge{string(k[0]), string(k[1])})
	}
	for _, e := range o.relations {
		if e.Kind == model.DependsOn && slices.Contains(o.modules, e.From) && slices.Contains(o.modules, e.To) {
			out = append(out, ledge{string(e.From), string(e.To)})
		}
	}
	return out
}

// compose plans the rooms of o, already cut, under the record ov.
func compose(o *overview, ov store.Overview) *composer {
	c := &composer{stage: o.flow.stagesOn(o.boxOf), zone: map[string]string{}, names: o.flow.names}
	var zones []string
	members := map[string][]string{}
	for _, z := range ov.Zones {
		for _, u := range z.Claim.Units {
			b := o.boxOf(u)
			if b == "" || c.zone[b] != "" {
				continue
			}
			if !slices.Contains(zones, z.Claim.Name) {
				zones = append(zones, z.Claim.Name)
			}
			c.zone[b] = z.Claim.Name
			members[z.Claim.Name] = append(members[z.Claim.Name], b)
		}
	}
	for b := range c.stage {
		c.order = append(c.order, b)
	}
	slices.SortFunc(c.order, func(a, b string) int {
		if c.stage[a] != c.stage[b] {
			return c.stage[a] - c.stage[b]
		}
		return strings.Compare(a, b)
	})
	drawn := o.drawnPairs()
	lined := map[string]bool{}
	for _, e := range drawn {
		lined[e.from], lined[e.to] = true, true
	}
	placed := map[string]bool{}
	// The axis: runs of stages in one zone, or in none, each a room.
	for i := 0; i < len(c.order); {
		z := c.zone[c.order[i]]
		j := i
		for j < len(c.order) && c.zone[c.order[j]] == z {
			j++
		}
		r := room{title: z, ids: slices.Clone(c.order[i:j]), lead: j - i}
		for _, b := range r.ids {
			placed[b] = true
		}
		if z != "" {
			for _, b := range members[z] {
				if !placed[b] {
					r.ids = append(r.ids, b)
					placed[b] = true
				}
			}
		}
		r.cols = max(r.lead, min(len(r.ids)-r.lead, 3))
		c.rooms = append(c.rooms, r)
		i = j
	}
	for _, z := range zones {
		var ids []string
		for _, b := range members[z] {
			if !placed[b] {
				ids = append(ids, b)
				placed[b] = true
			}
		}
		if len(ids) > 0 {
			c.rooms = append(c.rooms, room{title: z, ids: ids, cols: min(len(ids), 3)})
		}
	}
	var base, rest, outside []string
	for _, m := range o.modules {
		id := string(m)
		switch {
		case placed[id]:
		case lined[id]:
			c.rooms = append(c.rooms, room{ids: []string{id}, cols: 1})
		case o.more[m] >= baseUses:
			base = append(base, id)
		default:
			rest = append(rest, id)
		}
	}
	for _, ct := range o.contracts {
		if len(o.providers[ct]) == 0 {
			outside = append(outside, string(ct))
		}
	}
	slices.SortStableFunc(base, func(a, b string) int { return o.more[model.ID(b)] - o.more[model.ID(a)] })
	if len(base) > 0 {
		c.rooms = append(c.rooms, room{title: baseTitle, ids: base, cols: len(base), mark: true})
	}
	if len(rest) > 0 {
		c.rooms = append(c.rooms, room{ids: rest, cols: len(rest), mark: true})
	}
	if len(outside) > 0 {
		c.rooms = append(c.rooms, room{title: "외부 계약", ids: outside, cols: 1})
	}
	c.gaps = flowGaps(c.order, c.stage, o.flow.names, drawn, o.name)
	return c
}

// flowGaps names each pair of neighbouring stages no drawn line joins in
// stage order: a hole in the flow the record leaves.
func flowGaps(order []string, stage map[string]int, names []string, drawn []ledge, name func(model.ID) string) []string {
	var out []string
	last := 0
	for _, s := range order {
		last = max(last, stage[s])
	}
	for k := 1; k < last; k++ {
		joined := false
		for _, e := range drawn {
			if stage[e.from] == k && stage[e.to] == k+1 {
				joined = true
			}
		}
		if joined {
			continue
		}
		var from, to []string
		for _, s := range order {
			switch stage[s] {
			case k:
				from = append(from, name(model.ID(s)))
			case k + 1:
				to = append(to, name(model.ID(s)))
			}
		}
		if len(from) == 0 || len(to) == 0 {
			continue
		}
		out = append(out, fmt.Sprintf("stage %d %s (%s) to stage %d %s (%s)", k, names[k-1], strings.Join(from, ", "), k+1, names[k], strings.Join(to, ", ")))
	}
	return out
}

// FlowGaps are the holes in the primary flow a composed overview leaves
// without a line: each pair of neighbouring stages the record draws no
// line between, in stage order. It is empty for an overview that is not
// composed.
func FlowGaps(in Input) []string {
	ls, fs := layerings(in), walks(in)
	o := planOverview(in.Units, "", nil, tactic{}, ls, fs, nil)
	c := recordCut(in, o, ls, fs)
	if c == nil || c.said == nil || !composable(o, in.Overview) {
		return nil
	}
	p := planOverview(in.Units, "", nil, tactic{}, ls, fs, c)
	return compose(p, in.Overview).gaps
}

// lay sets out c's rooms as rows and x and returns o's layout for them,
// the lines' chains threaded through the rows between their ends.
func (c *composer) lay(o *overview, t tactic) layout {
	size := o.layout.pos
	// Each room's columns and rows, relative to its corner.
	type cell struct{ r, col int }
	at := map[string]cell{}
	colW := make([][]int, len(c.rooms))
	rowsOf := make([]int, len(c.rooms))
	for k, r := range c.rooms {
		colW[k] = make([]int, max(r.cols, 1))
		for i, id := range r.ids {
			var ce cell
			switch {
			case i < r.lead:
				ce = cell{0, i}
			case r.lead > 0:
				j := i - r.lead
				ce = cell{1 + j/r.cols, j % r.cols}
			default:
				ce = cell{i / r.cols, i % r.cols}
			}
			at[id] = ce
			colW[k][ce.col] = max(colW[k][ce.col], size[id].W)
			rowsOf[k] = max(rowsOf[k], ce.r+1)
		}
	}
	width := func(k int) int {
		w := -gapX
		for _, cw := range colW[k] {
			w += cw + gapX
		}
		return w
	}
	drawn := o.drawnPairs()
	// Bands: 0 above the axis, 1 the axis, 2 below it, 3 the bottom row.
	band := make([]int, len(c.rooms))
	centre := map[string]int{}
	x := padX
	for k, r := range c.rooms {
		switch {
		case r.lead > 0:
			band[k] = 1
			c.rooms[k].at = x
			x += width(k) + roomGap
		case r.title == "외부 계약" && !r.mark:
			band[k] = 1
		case r.mark:
			band[k] = 3
		default:
			band[k] = 2
		}
	}
	for k, r := range c.rooms {
		// The outside band stands right of the axis where the page has
		// room for it, else on the bottom row.
		if band[k] == 1 && r.lead == 0 {
			if x+width(k) > overviewBudget-padX {
				band[k] = 3
				continue
			}
			c.rooms[k].at = x
			x += width(k) + roomGap
		}
	}
	cx := func(k int, id string) int {
		off := c.rooms[k].at
		for i := 0; i < at[id].col; i++ {
			off += colW[k][i] + gapX
		}
		return off + (colW[k][at[id].col]-size[id].W)/2
	}
	roomOf := map[string]int{}
	for k, r := range c.rooms {
		for _, id := range r.ids {
			roomOf[id] = k
		}
	}
	for k, r := range c.rooms {
		if band[k] == 1 {
			for _, id := range r.ids {
				centre[id] = cx(k, id) + size[id].W/2
			}
		}
	}
	// A room off the axis stands above it when more of its lines leave it
	// than reach it, and as near as it can to the mean x of what it joins.
	want := map[int]int{}
	for k, r := range c.rooms {
		if band[k] != 2 {
			continue
		}
		out, in, sum, n := 0, 0, 0, 0
		for _, e := range drawn {
			mine, other := slices.Contains(r.ids, e.from), e.to
			if !mine {
				if !slices.Contains(r.ids, e.to) {
					continue
				}
				other = e.from
			}
			if slices.Contains(r.ids, other) {
				continue
			}
			if mine {
				out++
			} else {
				in++
			}
			if x, ok := centre[other]; ok {
				sum, n = sum+x, n+1
			}
		}
		if out > in {
			band[k] = 0
		}
		want[k] = padX + width(k)/2
		if n > 0 {
			want[k] = sum / n
		}
	}
	// Rows: each band as tall as its shelves, top down.
	tall := [4]int{}
	sub := make([]int, len(c.rooms))
	for _, b := range []int{0, 2, 3} {
		var ks []int
		for k := range c.rooms {
			if band[k] == b {
				ks = append(ks, k)
			}
		}
		slices.SortStableFunc(ks, func(p, q int) int { return want[p] - want[q] })
		// A room past the page's width starts a shelf of the band under
		// the rooms before it.
		edge, shelf, h := padX, 0, 0
		for _, k := range ks {
			spot := func() int {
				if b == 3 {
					return edge
				}
				return max(edge, want[k]-width(k)/2)
			}
			l := spot()
			if l+width(k) > overviewBudget-padX && edge > padX {
				shelf, h, edge = shelf+h, 0, padX
				l = min(spot(), max(padX, overviewBudget-padX-width(k)))
			}
			c.rooms[k].at, sub[k] = l, shelf
			h = max(h, rowsOf[k])
			edge = l + width(k) + roomGap
		}
		tall[b] = shelf + h
	}
	for k := range c.rooms {
		if band[k] == 1 {
			tall[1] = max(tall[1], rowsOf[k])
		}
	}
	first := [4]int{}
	for b := 1; b < 4; b++ {
		first[b] = first[b-1] + tall[b-1]
	}
	l := layout{pos: map[string]rect{}, row: map[string]int{}, lanes: map[[2]int]int{}, heights: map[string]int{}, minGap: roomHead, endGap: 12, pass: map[[2]int]int{}, counted: t.counted, roads: t.roads, through: t.through, top: padY, recorded: o.layout.recorded}
	n := first[3] + tall[3]
	grid := make([][]string, n)
	for k, r := range c.rooms {
		c.rooms[k].row = first[band[k]]
		for _, id := range r.ids {
			row := first[band[k]] + sub[k] + at[id].r
			grid[row] = append(grid[row], id)
			l.pos[id] = rect{X: cx(k, id), W: size[id].W, H: size[id].H}
			l.heights[id] = size[id].H
		}
	}
	var rows [][]string
	for _, ids := range grid {
		if len(ids) == 0 {
			continue
		}
		slices.SortFunc(ids, func(a, b string) int { return l.pos[a].X - l.pos[b].X })
		for _, id := range ids {
			l.row[id] = len(rows)
		}
		rows = append(rows, ids)
	}
	for k, r := range c.rooms {
		if len(r.ids) > 0 {
			c.rooms[k].row = l.row[r.ids[0]]
		}
	}
	l.rows = rows
	// The chains, as arrange threads them, set straight at the x nearest
	// their ends' mean that keeps clear of every box and chain they pass.
	lines := map[[2]string]int{}
	out := map[string][]string{}
	for _, e := range drawn {
		if _, ok := l.pos[e.from]; !ok {
			continue
		}
		if _, ok := l.pos[e.to]; !ok || e.from == e.to {
			continue
		}
		lines[pairKey(e.from, e.to)]++
		if !slices.Contains(out[e.from], e.to) {
			out[e.from] = append(out[e.from], e.to)
		}
	}
	var pairs [][2]string
	for k := range lines {
		pairs = append(pairs, k)
	}
	slices.SortFunc(pairs, func(a, b [2]string) int {
		if c := strings.Compare(a[0], b[0]); c != 0 {
			return c
		}
		return strings.Compare(a[1], b[1])
	})
	w := map[string]int{}
	for id, p := range l.pos {
		w[id] = p.W
	}
	g := proper(rows, w, pairs, lines)
	right := 0
	for _, p := range l.pos {
		right = max(right, p.X+p.W)
	}
	taken := map[int][][2]int{}
	for ch, chn := range g.chains {
		cw := chainLane * (chn.lanes - 1)
		a, b := l.pos[chn.upper], l.pos[chn.lower]
		ideal := (a.X + a.W/2 + b.X + b.W/2) / 2
		free := func(x int) bool {
			for r := chn.top; r <= chn.bottom; r++ {
				for _, id := range rows[r] {
					p := l.pos[id]
					if x+cw+chainSep > p.X && x-chainSep < p.X+p.W {
						return false
					}
				}
			}
			// A chain in the row just above or below runs on into the gap
			// this one runs in, so the two keep apart there too.
			for r := chn.top - 1; r <= chn.bottom+1; r++ {
				for _, s := range taken[r] {
					if x+cw+chainSep > s[0] && x-chainSep < s[1] {
						return false
					}
				}
			}
			return true
		}
		best := right + chainSep
		for d := 0; d <= right+roomGap; d += 4 {
			if free(ideal-d) && ideal-d >= padX {
				best = ideal - d
				break
			}
			if free(ideal + d) {
				best = ideal + d
				break
			}
		}
		for r := chn.top; r <= chn.bottom; r++ {
			l.pass[[2]int{ch, r}] = best
			taken[r] = append(taken[r], [2]int{best, best + cw})
		}
		right = max(right, best+cw)
		l.chains = append(l.chains, chn)
	}
	l.width = right + padX
	l.oneWay = true
	l.axis = axis{basis: byFlow, cycle: cyclic(out), flow: &flowAxis{across: true, stage: c.stage}}
	return l
}

// draw draws o, planned with the record's cut, composed.
func (c *composer) draw(o *overview, href func(model.ID) string) Drawing {
	o.layout = c.lay(o, tactic{roads: true})
	o.stageWords = map[string]string{}
	seen := map[int]bool{}
	for _, id := range c.order {
		if s := c.stage[id]; !seen[s] {
			seen[s] = true
			o.stageWords[id] = c.names[s-1]
		}
	}
	// A line that leaves its room, or runs against the flow, takes its
	// word before the lines inside one room: its word says the most.
	room := map[string]int{}
	for k, r := range c.rooms {
		for _, id := range r.ids {
			room[id] = k
		}
	}
	o.first = func(w Wire) bool {
		return w.Back || len(w.ends) == 2 && room[w.ends[0]] != room[w.ends[1]]
	}
	return o.draw(href)
}

// mark numbers the stages on their boxes, each with its stage's name, and
// draws bold only the lines that join one stage to the next in stage
// order. The lead keeps what lead found for it, its entry and its quiet
// lines; its hops are the stages.
func (c *composer) mark(d *Drawing, key, question string) {
	boxes := map[string]Box{}
	for _, b := range d.Boxes {
		boxes[b.ID] = b
	}
	if d.Lead == nil {
		d.Lead = &Lead{Key: key, Title: question, Primary: true, Hue: hueKey(-1)}
		if len(c.order) > 0 {
			d.Lead.Entry = c.order[0]
			if b, ok := boxes[c.order[0]]; ok && b.Hue != "" {
				d.Lead.Hue = b.Hue
			}
		}
	}
	uses := d.uses()
	var hops []Hop
	seen := map[int]bool{}
	for _, id := range c.order {
		s := c.stage[id]
		b, ok := boxes[id]
		if seen[s] || !ok {
			continue
		}
		seen[s] = true
		h := Hop{N: s, X: b.X, Y: b.Y, R: stageR, Word: c.names[s-1], WX: b.X + stageGap, WY: b.Y - 5}
		for _, u := range uses {
			if c.stage[u.to] == s && c.stage[u.from] == s-1 && s > 1 {
				r := d.ways[u.way]
				h.D, h.Head, h.Tips, h.way = r.D, r.Head, r.Tips, u.way
				if u.way < len(d.Routes) {
					h.Lit = d.Routes[u.way].Lit
				}
				break
			}
		}
		hops = append(hops, h)
	}
	d.Lead.Hops, d.Lead.Stay, d.Lead.OnBoxes = hops, nil, true
	// A stage's name always sits right of its number, over its box: the
	// lines' words kept off that place (reserved).
}

// bands draws a band around each titled room, numbered by nothing: the
// stages carry the numbers.
func (c *composer) bands(d *Drawing) []string {
	var lost []string
	for _, r := range c.rooms {
		// A mark around one box says nothing its +N does not.
		if r.title == "" || r.mark && len(r.ids) < 2 {
			continue
		}
		if z, ok := d.band(group{title: r.title, ids: r.ids}); ok {
			// A room on the axis keeps the room over its boxes for the
			// stages' numbers and names, so its title goes under them.
			if r.lead > 0 {
				lo := 1 << 30
				for _, b := range d.Boxes {
					if slices.Contains(r.ids, b.ID) {
						lo = min(lo, b.Y)
					}
				}
				// The title takes the foot's left corner, else its right,
				// else the first place along it clear of ink, else stays
				// where band set it.
				hi := z.Y + z.H
				under := z
				under.Lines = nil
				under.Y, under.H = lo-zoneHead, hi-lo+zoneHead+14
				under.TY = under.Y + under.H - 6
				w := zoneWidth(under.Label())
				xs := []int{under.X + zoneSide + 2, under.X + under.W - zoneSide - 2 - w}
				for x := under.X + zoneSide + 6; x < under.X+under.W-zoneSide-2-w; x += 4 {
					xs = append(xs, x)
				}
				for _, x := range xs {
					under.TX = x
					if a := grow(under.titleArea(), inkGap); d.clearOfInk(a, nil) && d.crossed(a) == 0 {
						z = under
						d.Height = max(d.Height, z.Y+z.H+4)
						break
					}
				}
			}
			d.Zones = append(d.Zones, z)
		} else {
			lost = append(lost, r.title)
		}
	}
	return lost
}

// unclash drops a word the reader would read twice or not at all: a
// road's shared phrase that stands so near another label that Hangul
// glyphs, taller than labelH, touch, and the second of two labels that say
// the same, the phrase first. The lines under a dropped word keep their
// own words, and every word stays in its line's tooltip.
func (c *composer) unclash(d *Drawing) {
	drop := func(i, j int) bool {
		l, o := d.Wires[i].Label, d.Wires[j].Label
		if l == nil || o == nil || i == j {
			return false
		}
		switch {
		case l.S == o.S:
			return l.Said && !o.Said || l.Said == o.Said && j < i
		case l.Said:
			return hit(grow(l.area(), 3), o.area())
		}
		return false
	}
	for i := range d.Wires {
		for j := range d.Wires {
			if drop(i, j) {
				d.Wires[i].Label = nil
				break
			}
		}
	}
}

// legend sets the key where card finds room, and where the drawing has
// none, grows it by the least height that gives the key a place: a
// composed overview is short, and its key is the only place its dashes
// and dots are said.
func (c *composer) legend(d *Drawing) {
	d.roam = true
	h := d.Height
	for more := 0; d.Card == nil && more <= cardHead+7*cardRow+cardPad+2*padY; more += 4 {
		d.Height = h + more
		d.card()
	}
}
