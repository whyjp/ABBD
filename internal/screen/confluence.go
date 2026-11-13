package screen

import (
	"fmt"
	"slices"
	"sort"
)

// Lines are bus routes on a highway (rule 4). The ink is the road: lines of
// one style that run side by side are drawn as one road, which they join
// and leave where their ways part, with a dot at each split and merge
// (rule 17). Each line keeps its own route, its whole way from the shape
// it leaves to the shape it reaches, unseen at rest and lit whole when a
// reader points at it, so the road may join places no one line joins
// (rule 18) while hover and the model never do.
//
// confluent moves straight runs of the routes onto their neighbours: a run
// within roadRoom of a parallel run of its style, beside it for roadMeet or
// more, moves onto it; a fork's branch that turns off its stem at another
// height than its siblings moves onto theirs; and a run before a stair
// moves onto the run past it, as TALA's ShortcutEdgeRoutes takes out a
// bend the grid forced (re-implemented, no code copied). A route's first
// and last run move only along the edge span its line may meet. A move is
// kept only where no route then passes through a box, turns back, folds
// onto itself or gains a jog, no route's first or last run comes to lie on
// a line of another kind or dotting wherever it lands (a local road), and
// the view gains no crossing, turn, fork height or split far from the
// places it splits toward (J1), and loses one, or runs less ink beside
// other ink. The moves run twice, stairs first and merges first, since
// either order can shut out the other's best moves, and the view keeps the
// roads that weigh less (roadCost.weight). Then the road is drawn from the
// routes.

const (
	roadRoom = 12
	roadMeet = 16
	// roadReach is how far apart two long runs, roadLong or more side by
	// side, of one kind may lie and still merge, once the
	// nearer merges are settled: a highway gathers the lanes the grid laid
	// a few tracks apart.
	roadReach = 64
	roadLong  = 96
	// roadEnd is the least a route's first and last run keep, so an
	// arrowhead and an origin mark still have a straight stretch.
	roadEnd = 20
	// roadMoves bounds the moves one view takes, and roadClear is how far a
	// moved run keeps off a box.
	roadMoves = 400
	roadClear = 2
	// roadBeside is how far road running beside road weighs as much as a
	// crossing or a turn, when two settled roads are weighed.
	roadBeside = 200
)

// straightRun is a straight piece of road: the collinear runs of routes of
// one style that touch or overlap. at is its x when it is vertical and its
// y when it is not, and lo, hi the span it covers.
type straightRun struct {
	vertical   bool
	at, lo, hi int
	style      string
	// segs are the route runs it holds, by route and run index.
	segs [][2]int
	// slide is how far it may move: a route's first or last run meets a
	// box's top or bottom, so it moves only along the span of the edge its
	// line may meet, and one that meets a box's side not at all.
	slide [2]int
}

// moves says the run may move to at.
func (r straightRun) moves(at int) bool { return r.slide[0] <= at && at <= r.slide[1] }

func runOf(a, b pt) (vertical bool, at, lo, hi int) {
	if a.x == b.x {
		return true, a.x, min(a.y, b.y), max(a.y, b.y)
	}
	return false, a.y, min(a.x, b.x), max(a.x, b.x)
}

// roadRuns groups the routes' runs into straight pieces of road.
func roadRuns(ways [][]pt, rv roadView) []straightRun { return roadRunsBy(ways, rv, false) }

// roadRunsBy is roadRuns, with the runs that go each way along a track
// grouped apart when byWay is set.
func roadRunsBy(ways [][]pt, rv roadView, byWay bool) []straightRun {
	style := rv.style
	type key struct {
		style    string
		vertical bool
		at, way  int
	}
	type seg struct {
		lo, hi int
		at     [2]int
		slide  [2]int
	}
	by := map[key][]seg{}
	var keys []key
	for i, w := range ways {
		for j := 1; j < len(w); j++ {
			if w[j-1] == w[j] {
				continue
			}
			v, at, lo, hi := runOf(w[j-1], w[j])
			k := key{style[i], v, at, 0}
			if byWay {
				k.way = heading(w[j-1], w[j])
			}
			if _, ok := by[k]; !ok {
				keys = append(keys, k)
			}
			slide := [2]int{-1 << 30, 1 << 30}
			for side, meets := range []bool{j == 1, j == len(w)-1} {
				switch e := rv.ends[i][side]; {
				case !meets:
				case !v:
					slide = [2]int{1, 0}
				default:
					slide = [2]int{max(slide[0], e.lo), min(slide[1], e.hi)}
				}
			}
			by[k] = append(by[k], seg{lo, hi, [2]int{i, j - 1}, slide})
		}
	}
	sort.Slice(keys, func(a, b int) bool {
		if keys[a].style != keys[b].style {
			return keys[a].style < keys[b].style
		}
		if keys[a].vertical != keys[b].vertical {
			return keys[a].vertical
		}
		if keys[a].at != keys[b].at {
			return keys[a].at < keys[b].at
		}
		return keys[a].way < keys[b].way
	})
	var out []straightRun
	for _, k := range keys {
		ss := by[k]
		sort.SliceStable(ss, func(a, b int) bool { return ss[a].lo < ss[b].lo })
		first := len(out)
		for _, s := range ss {
			if n := len(out); n > first && s.lo <= out[n-1].hi {
				r := &out[n-1]
				r.hi = max(r.hi, s.hi)
				r.segs = append(r.segs, s.at)
				r.slide = [2]int{max(r.slide[0], s.slide[0]), min(r.slide[1], s.slide[1])}
				continue
			}
			out = append(out, straightRun{k.vertical, k.at, s.lo, s.hi, k.style, [][2]int{s.at}, s.slide})
		}
	}
	return out
}

// moved is the routes with run r moved to at.
func moved(ways [][]pt, r straightRun, at int) [][]pt {
	out := slices.Clone(ways)
	touched := map[int]bool{}
	for _, s := range r.segs {
		i, j := s[0], s[1]
		if !touched[i] {
			out[i] = slices.Clone(ways[i])
			touched[i] = true
		}
		for _, k := range []int{j, j + 1} {
			if r.vertical {
				out[i][k].x = at
			} else {
				out[i][k].y = at
			}
		}
	}
	return out
}

// folds says the route runs back over itself somewhere.
func folds(w []pt) bool {
	for k := 2; k < len(w); k++ {
		a, b, c := w[k-2], w[k-1], w[k]
		if a == b || b == c {
			continue
		}
		if (a.x == b.x && b.x == c.x && (b.y-a.y)*(c.y-b.y) < 0) || (a.y == b.y && b.y == c.y && (b.x-a.x)*(c.x-b.x) < 0) {
			return true
		}
	}
	return false
}

// backTurns counts the runs that head against the run two before them.
func backTurns(w []pt) int {
	n := 0
	for k := 3; k < len(w); k++ {
		d1 := pt{sign(w[k-2].x - w[k-3].x), sign(w[k-2].y - w[k-3].y)}
		d2 := pt{sign(w[k].x - w[k-1].x), sign(w[k].y - w[k-1].y)}
		if d1.x == -d2.x && d1.y == -d2.y && d1 != (pt{}) {
			n++
		}
	}
	return n
}

func jogsOf(w []pt) int {
	n := 0
	for k := 2; k < len(w)-1; k++ {
		if abs(w[k].x-w[k-1].x)+abs(w[k].y-w[k-1].y) < jogLen {
			n++
		}
	}
	return n
}

// crosses says the run a-c passes through the inside of box b, its two
// ends excepted, where it meets the box it leaves or reaches.
func crosses(a, c pt, b box) bool {
	lo, hi := pt{min(a.x, c.x), min(a.y, c.y)}, pt{max(a.x, c.x), max(a.y, c.y)}
	return lo.x < b.x+b.w-1 && hi.x > b.x+1 && lo.y < b.y+b.h-1 && hi.y > b.y+1
}

// routeShape is a route as a move finds it, read once for every move of
// it. keeps says the route a move leaves still reads as a line: its ends
// stay on their edges with room for a head, it passes and runs along no
// box, never folds and gains no turn-back or jog.
type routeShape struct {
	w                 []pt
	back, jogs, boxed int
	room              []pt
}

func routeShapeOf(was []pt, boxes []box) *routeShape {
	return &routeShape{w: was, back: backTurns(was), jogs: jogsOf(was), boxed: boxed(was, boxes, 1<<30)}
}

func (s *routeShape) keeps(w []pt, boxes []box) bool {
	was := s.w
	if folds(w) {
		return false
	}
	s.room = straightInto(s.room, w)
	w = s.room
	if len(w) < 2 || w[0].y != was[0].y || w[len(w)-1].y != was[len(was)-1].y {
		return false
	}
	end := func(a, b pt) int { return abs(a.x-b.x) + abs(a.y-b.y) }
	if end(w[0], w[1]) < min(roadEnd, end(was[0], was[1])) || end(w[len(w)-1], w[len(w)-2]) < min(roadEnd, end(was[len(was)-1], was[len(was)-2])) {
		return false
	}
	if backTurns(w) > s.back || jogsOf(w) > s.jogs {
		return false
	}
	return boxed(w, boxes, s.boxed) <= s.boxed
}

// boxed counts the runs of route w that pass through a box or within
// roadClear of one, so they read as running along it, but where its first
// or last run meets the shape at its end. It stops counting past limit.
func boxed(w []pt, boxes []box, limit int) int {
	toward := func(p, q pt, d int) pt { return pt{p.x + d*sign(q.x-p.x), p.y + d*sign(q.y-p.y)} }
	n := 0
	for k := 1; k < len(w) && n <= limit; k++ {
		a, c := w[k-1], w[k]
		if k == 1 {
			a = toward(a, c, roadClear+1)
		}
		if k == len(w)-1 {
			c = toward(c, a, roadClear+1)
		}
		for _, b := range boxes {
			if crosses(a, c, box{b.x - roadClear, b.y - roadClear, b.w + 2*roadClear, b.h + 2*roadClear}) {
				n++
			}
		}
	}
	return n
}

// roadCost is what a reader of the drawn road meets: where pieces of road
// cross, how often the routes turn, how far road runs beside road, how far
// pieces of different styles run on one another, the splits far from
// their ends, and the forks whose branches leave at different heights.
type roadCost struct {
	crossings, turns, beside, clash, near, far, forks, tees, unseen, combs int
}

// roadView is what confluent weighs routes against: each route's style and
// ends, the places it leaves and reaches, the row position (look.position)
// of the place it reaches, and the position of a height on the page.
type roadView struct {
	style []string
	// plan says each route's relations all arise only in design, so it is
	// drawn dotted where it rides alone, and kinds is each route's kind.
	// lead, nil where no lead is drawn, says the view's lead rides a route.
	plan, lead []bool
	kinds      []string
	// ends are each route's two ends, whose spans its first and last runs
	// may slide along.
	ends     [][2]end
	from, to []string
	dest     []int
	position func(y int) int
	// forkGroups are the routes of one style from one place into one row,
	// and of one dotting too when dotting is set, since a dotted branch need
	// not share its solid siblings' track; fromGroups are those from one
	// place, each in order, with the group each route is in.
	forkGroups, fromGroups [][]int
	dotting                bool
	forkOf, fromOf         []int
}

// dotted says route i shows dotted at rest, so its dots must show
// somewhere (unseenOf): it is plan, and no lead hop paints it in solid flow
// ink, which would hide its dots on any track it took.
func (v roadView) dotted(i int) bool { return v.plan[i] && (v.lead == nil || !v.lead[i]) }

// grouped is v with its routes grouped.
func (v roadView) grouped() roadView {
	type fork struct {
		style, from string
		plan        bool
		dest        int
	}
	forks, froms := map[fork]int{}, map[string]int{}
	v.forkOf, v.fromOf = make([]int, len(v.style)), make([]int, len(v.style))
	for i := range v.style {
		f := fork{v.style[i], v.from[i], v.dotting && v.plan[i], v.dest[i]}
		g, ok := forks[f]
		if !ok {
			g = len(v.forkGroups)
			forks[f] = g
			v.forkGroups = append(v.forkGroups, nil)
		}
		v.forkOf[i], v.forkGroups[g] = g, append(v.forkGroups[g], i)
		h, ok := froms[v.from[i]]
		if !ok {
			h = len(v.fromGroups)
			froms[v.from[i]] = h
			v.fromGroups = append(v.fromGroups, nil)
		}
		v.fromOf[i], v.fromGroups[h] = h, append(v.fromGroups[h], i)
	}
	return v
}

func (v roadView) cost(ways [][]pt) roadCost { return newRoadIndex(v, ways).cost }

// weight weighs settled roads against each other: a crossing, a turn and
// roadBeside of road beside road weigh alike.
func (c roadCost) weight() int { return c.crossings + c.turns + c.combs + c.beside/roadBeside }

func (c roadCost) better(o roadCost) bool {
	if c.crossings > o.crossings || c.turns > o.turns || c.clash > o.clash || c.near > o.near || c.far > o.far || c.forks > o.forks || c.tees > o.tees || c.unseen > o.unseen || c.combs > o.combs {
		return false
	}
	return c.crossings < o.crossings || c.turns < o.turns || c.forks < o.forks || c.combs < o.combs || c.beside < o.beside
}

// withers says o is no worse than c on any count.
func (c roadCost) withers(o roadCost) bool {
	return o.crossings <= c.crossings && o.turns <= c.turns && o.clash <= c.clash && o.near <= c.near && o.far <= c.far &&
		o.forks <= c.forks && o.tees <= c.tees && o.unseen <= c.unseen && o.combs <= c.combs && o.beside <= c.beside
}

// stub is the way a route's first or last run leaves or reaches its place:
// lines of one kind and dotting that leave or reach one place the same way
// by stubs combGap or less apart draw a comb (lint's combs), which a hand
// drawing brings out of the box as one stem.
type stub struct {
	place, kind string
	plan, last  bool
	way         pt
}

// stubOf is the stub route i's run k makes, if it is the route's first or
// last run and stands upright on its place's top or bottom.
func (v roadView) stubOf(ways [][]pt, i, k int) (stub, bool) {
	w := ways[i]
	last := k == len(w)-2
	if k != 0 && !last || w[k].x != w[k+1].x || w[k] == w[k+1] {
		return stub{}, false
	}
	place, a, b := v.from[i], w[0], w[1]
	if k != 0 {
		place, a, b = v.to[i], w[len(w)-1], w[len(w)-2]
	}
	return stub{place, v.kinds[i], v.plan[i], k != 0, pt{0, sign(b.y - a.y)}}, true
}

// combed says runs a and b each hold a stub of one comb.
func (v roadView) combed(ways [][]pt, a, b straightRun) bool {
	if !a.vertical || !b.vertical || a.at == b.at || abs(a.at-b.at) > combGap {
		return false
	}
	of := map[stub]bool{}
	for _, s := range a.segs {
		if st, ok := v.stubOf(ways, s[0], s[1]); ok {
			of[st] = true
		}
	}
	for _, s := range b.segs {
		if st, ok := v.stubOf(ways, s[0], s[1]); ok && of[st] {
			return true
		}
	}
	return false
}

// forks are pairs of routes that leave one place for boxes in one row and
// turn off their stem onto flat runs at different heights, as forkLevels
// reads them, by route. byDotting pairs only routes of one dotting, as
// roadCost counts forks when v.dotting is set; the moves still try to put a
// branch of either dotting onto its siblings' track.
func (v roadView) forks(ways [][]pt, byDotting bool) [][2]int {
	type fork struct {
		style, from string
		plan        bool
		dest        int
	}
	first := map[fork]int{}
	var out [][2]int
	for i, w := range ways {
		if len(w) < 3 || w[0].x != w[1].x {
			continue
		}
		f := fork{v.style[i], v.from[i], byDotting && v.plan[i], v.dest[i]}
		j, ok := first[f]
		switch {
		case !ok:
			first[f] = i
		case ways[j][1].y != w[1].y:
			out = append(out, [2]int{j, i})
		}
	}
	return out
}

// joins says runs a and b may ride one road. In the middle any lines
// may, a highway whatever their kind or dotting; a run that holds a line's
// first or last run is a local road, which only lines of one kind and
// dotting whose ends there are the same sort of shape, a box or a chip,
// ride, so each line keeps its own look where it leaves and arrives.
func (v roadView) joins(ways [][]pt, a, b straightRun) bool {
	local, kind, asset := false, "", ""
	for _, r := range []straightRun{a, b} {
		for _, s := range r.segs {
			i, j := s[0], s[1]
			for side, end := range []bool{j == 0, j == len(ways[i])-2} {
				if !end {
					continue
				}
				local = true
				if k := fmt.Sprint(v.ends[i][side].node != ""); asset == "" {
					asset = k
				} else if asset != k {
					return false
				}
			}
		}
	}
	if !local {
		return true
	}
	for _, r := range []straightRun{a, b} {
		for _, s := range r.segs {
			if k := fmt.Sprint(v.kinds[s[0]], v.plan[s[0]]); kind == "" {
				kind = k
			} else if kind != k {
				return false
			}
		}
	}
	return true
}

// oneKind says every route run on a and b is of one kind.
func (v roadView) oneKind(a, b straightRun) bool {
	k := v.kinds[a.segs[0][0]]
	for _, r := range []straightRun{a, b} {
		for _, s := range r.segs {
			if v.kinds[s[0]] != k {
				return false
			}
		}
	}
	return true
}

// solidPart is run r with only its solid lines, when it holds both solid
// and dotted ones: a dotted line hidden under solid ink reads as solid, so
// a road of mixed ink may join a solid road only without its dotted lines.
// A run that holds a route's first or last run is a box's stub, which stays
// whole: split, its lines would leave the box as two stubs side by side.
func (v roadView) solidPart(ways [][]pt, r straightRun) (straightRun, bool) {
	out := r
	out.segs = nil
	out.lo, out.hi = 1<<30, -1<<30
	for _, s := range r.segs {
		if s[1] == 0 || s[1] == len(ways[s[0]])-2 {
			return r, false
		}
		if v.plan[s[0]] {
			continue
		}
		out.segs = append(out.segs, s)
		_, _, lo, hi := runOf(ways[s[0]][s[1]], ways[s[0]][s[1]+1])
		out.lo, out.hi = min(out.lo, lo), max(out.hi, hi)
	}
	return out, len(out.segs) > 0 && len(out.segs) < len(r.segs)
}

// mix says route runs a and b, side by side on one track, break the local
// road rule (joins): one of them is its route's first or last run, and the
// two differ in kind or dotting, or both are ends and meet no shape of one
// sort. A route of one run has both its ends there, so either sort will do.
func (v roadView) mix(ways [][]pt, a, b [2]int) bool {
	assets := func(s [2]int) map[bool]bool {
		out := map[bool]bool{}
		for side, end := range []bool{s[1] == 0, s[1] == len(ways[s[0]])-2} {
			if end {
				out[v.ends[s[0]][side].node != ""] = true
			}
		}
		return out
	}
	x, y := assets(a), assets(b)
	switch {
	case len(x) == 0 && len(y) == 0:
		return false
	case v.kinds[a[0]] != v.kinds[b[0]] || v.plan[a[0]] != v.plan[b[0]]:
		return true
	case len(x) == 0 || len(y) == 0:
		return false
	}
	for k := range x {
		if y[k] {
			return false
		}
	}
	return true
}

// confluent moves the routes onto shared roads and draws the road.
func (r *routing) confluent(reqs []wireReq, position func(y int) int, rows int) {
	ink, plan, lead, kinds := make([]string, len(reqs)), make([]bool, len(reqs)), make([]bool, len(reqs)), make([]string, len(reqs))
	dest, to, from := make([]int, len(reqs)), make([]string, len(reqs)), make([]string, len(reqs))
	for i, q := range reqs {
		to[i], from[i] = place(q.to), place(q.from)
		if q.from.node != "" {
			from[i] = "node|" + q.from.node
		}
		ink[i], plan[i], lead[i], kinds[i] = fmt.Sprint(r.back[i]), q.plan, q.lead, q.kind
		dest[i] = min(max(2*q.to.row+1, -1), 2*rows+1)
	}
	ends := make([][2]end, len(reqs))
	for i, q := range reqs {
		ends[i] = [2]end{q.from, q.to}
	}
	ways := make([][]pt, len(r.ways))
	for i, w := range r.ways {
		ways[i] = straight(w)
	}
	v := roadView{style: ink, plan: plan, lead: lead, kinds: kinds, ends: ends, from: from, to: to, dest: dest, position: position}.grouped()
	routed := ways
	settle := func(stairsFirst bool) ([][]pt, roadCost) {
		ways, cost := routed, v.cost(routed)
		for n := 0; n < roadMoves; n++ {
			next, ok := r.step(ways, v, cost, stairsFirst)
			if !ok {
				break
			}
			ways = next
			for i := range ways {
				ways[i] = straight(ways[i])
			}
			cost = v.cost(ways)
		}
		return ways, cost
	}
	// The two orders read the same routes and share nothing else, so they
	// settle side by side.
	var stairs [][]pt
	var sc roadCost
	done := make(chan struct{})
	go func() {
		stairs, sc = settle(true)
		close(done)
	}()
	ways, mc := settle(false)
	<-done
	if sc.weight() <= mc.weight() {
		ways = stairs
	}
	// Then long runs of one kind merge from up to roadReach apart, where
	// that runs no more ink beside other ink and hides no dotted line.
	for n, cost := 0, v.cost(ways); n < roadMoves; n++ {
		next, ok := r.shift(ways, v, cost, false, true)
		if !ok {
			// A long merge may open a nearer one it had shut out: a run
			// that split from a road short of where the others turned now
			// turns with them.
			next, ok = r.step(ways, v, cost, false)
		}
		if !ok {
			break
		}
		ways = next
		for i := range ways {
			ways[i] = straight(ways[i])
		}
		cost = v.cost(ways)
	}
	// Last, a fork's heights count apart by dotting, so solid branches may
	// join a road their dotted sibling may not ride, where no move of a
	// whole fork has set the dotted branch on their track already.
	d := v
	d.dotting, d.forkGroups, d.fromGroups = true, nil, nil
	d = d.grouped()
	for n, cost := 0, d.cost(ways); n < roadMoves; n++ {
		next, ok := r.step(ways, d, cost, false)
		if !ok {
			break
		}
		ways = next
		for i := range ways {
			ways[i] = straight(ways[i])
		}
		cost = d.cost(ways)
	}
	ways = r.twoLanes(ways, v)
	if roadsDrawn != nil {
		roadsDrawn(r.ways, ways)
	}
	r.ways = ways
	r.drawn = roads(ways, ink, reqs, r.back)
}

// roadsDrawn, when a test sets it, sees each view's routes as routed and
// as the roads leave them.
var roadsDrawn func(routed, ways [][]pt)

// step finds the first move that reads better than cost, in order: runs
// onto a neighbour beside them, longest shared stretch first, then forks
// set on one track, with stairs straightened before both when stairsFirst,
// so a route that could drop straight does before a road bends it onto its
// track, and after both otherwise; then a stair taken the long way round.
func (r *routing) step(ways [][]pt, v roadView, cost roadCost, stairsFirst bool) ([][]pt, bool) {
	if next, ok := r.shift(ways, v, cost, stairsFirst, false); ok {
		return next, true
	}
	return r.wide(ways, v, cost, stairsFirst)
}

// shift is step's moves of whole runs of road; wide takes only the merges
// of long runs of one kind from past roadRoom.
func (r *routing) shift(ways [][]pt, v roadView, cost roadCost, stairsFirst, wide bool) ([][]pt, bool) {
	rs := roadRuns(ways, v)
	type move struct {
		run      straightRun
		at, gain int
	}
	// near calls f with each run of run i's style and direction that lies
	// within reach of it, in order: roadRuns sorts the runs so, and a run
	// may move only that far.
	near := func(i, reach int, f func(b straightRun)) {
		a, k := rs[i], i
		for k > 0 && rs[k-1].style == a.style && rs[k-1].vertical == a.vertical && a.at-rs[k-1].at <= reach {
			k--
		}
		for ; k < len(rs) && rs[k].style == a.style && rs[k].vertical == a.vertical && rs[k].at-a.at <= reach; k++ {
			if k != i {
				f(rs[k])
			}
		}
	}
	reach := roadRoom
	if wide {
		reach = roadReach
	}
	var moves []move
	for i, a := range rs {
		near(i, reach, func(b straightRun) {
			if !a.moves(b.at) {
				return
			}
			try := func(a straightRun) {
				d, o := abs(a.at-b.at), min(a.hi, b.hi)-max(a.lo, b.lo)
				if wide && (d <= roadRoom || o < roadLong || !v.oneKind(a, b)) || !wide && d == 0 {
					return
				}
				if o >= roadMeet && v.joins(ways, a, b) {
					moves = append(moves, move{a, b.at, o})
				}
			}
			try(a)
			if s, ok := v.solidPart(ways, a); ok {
				try(s)
			}
		})
	}
	// at is the piece of road each route run lies on.
	at := map[[2]int]int{}
	for k, run := range rs {
		for _, s := range run.segs {
			at[s] = k
		}
	}
	// A fork: lines of one place into one row that turn off their stem at
	// different heights. Moving one's first turn onto another's puts the
	// branches on one track.
	for _, f := range v.forks(ways, false) {
		if wide {
			break
		}
		a, b := at[[2]int{f[0], 1}], at[[2]int{f[1], 1}]
		if !v.joins(ways, rs[a], rs[b]) {
			continue
		}
		if rs[a].moves(rs[b].at) {
			moves = append(moves, move{rs[a], rs[b].at, 0})
		}
		if rs[b].moves(rs[a].at) {
			moves = append(moves, move{rs[b], rs[a].at, 0})
		}
	}
	// A comb: stubs of one place, kind and way that leave or reach it apart.
	// Moving one stub onto another brings the lines out as one stem.
	for i, a := range rs {
		if wide {
			break
		}
		near(i, combGap, func(b straightRun) {
			if a.moves(b.at) && v.combed(ways, a, b) && v.joins(ways, a, b) {
				moves = append(moves, move{a, b.at, 0})
			}
		})
	}
	// A stair: a run, a short step across, and a run on past it the same
	// way. Moving either run onto the other takes out the step.
	var stairs []move
	for i, w := range ways {
		if wide {
			break
		}
		for j := 1; j+1 < len(w)-1; j++ {
			a, b := at[[2]int{i, j - 1}], at[[2]int{i, j + 1}]
			if rs[a].vertical != rs[b].vertical {
				continue
			}
			if rs[a].moves(rs[b].at) {
				stairs = append(stairs, move{rs[a], rs[b].at, 0})
			}
			if rs[b].moves(rs[a].at) {
				stairs = append(stairs, move{rs[b], rs[a].at, 0})
			}
		}
	}
	sort.SliceStable(moves, func(i, j int) bool { return moves[i].gain > moves[j].gain })
	if stairsFirst {
		moves = append(stairs, moves...)
	} else {
		moves = append(moves, stairs...)
	}
	var x *roadIndex
	shapes := make([]*routeShape, len(ways))
	for _, m := range moves {
		next := moved(ways, m.run, m.at)
		ok := true
		for _, s := range m.run.segs {
			if shapes[s[0]] == nil {
				shapes[s[0]] = routeShapeOf(ways[s[0]], r.boxes)
			}
			if !shapes[s[0]].keeps(next[s[0]], r.boxes) {
				ok = false
				break
			}
		}
		if !ok {
			continue
		}
		touched := map[int]bool{}
		var ts []int
		for _, s := range m.run.segs {
			next[s[0]] = straight(next[s[0]])
			if !touched[s[0]] {
				ts = append(ts, s[0])
			}
			touched[s[0]] = true
		}
		if bendsMore(ways, next, touched, wide) {
			continue
		}
		if x == nil {
			x = newRoadIndex(v, ways)
		}
		ch := x.change(next, ts)
		nc, ok := ch.costWithin(cost)
		if !ok || wide && nc.beside > cost.beside {
			continue
		}
		if was, now := ch.endMixes(); now <= was && nc.better(cost) {
			return next, true
		}
	}
	return nil, false
}

// bendsMore says a move turns one of the routes it touches more often, or
// gives it another dogleg. roadCost weighs a view's turns together, so
// without this a move could bend one route to straighten another.
func bendsMore(ways, next [][]pt, touched map[int]bool, legs bool) bool {
	for i := range touched {
		if len(straight(next[i])) > len(straight(ways[i])) || legs && doglegs(fpts(next[i])) > doglegs(fpts(ways[i])) {
			return true
		}
	}
	return false
}

func fpts(w []pt) []fpt {
	out := make([]fpt, len(w))
	for i, p := range w {
		out[i] = fpt{float64(p.x), float64(p.y)}
	}
	return out
}

// wideStep is the step, and wideReach the farthest, that wide looks past a
// stair's far run for a clear way down.
const (
	wideStep  = 8
	wideReach = 96
)

// wide takes one route's stair another way round. Where a route runs down,
// steps across and runs on down, its step may cross at another height and
// its far run come down further out, clear of every box: a line that comes
// down the middle, crosses a crowded row and swings round the box under it
// may step across higher, where nothing runs, and come down beside that
// box. Only that route moves, so the road it leaves keeps its other
// riders, and the move is kept on the terms of every other.
func (r *routing) wide(ways [][]pt, v roadView, cost roadCost, stairsFirst bool) ([][]pt, bool) {
	var ix *roadIndex
	var next [][]pt
	// raw, nw and sw are the room each move's route is laid out in; a
	// route kept past the move is copied out.
	var raw, nw, sw []pt
	for i, w := range ways {
		was, turns := routeShapeOf(w, r.boxes), len(straight(w))
		for j := 2; j+3 < len(w); j++ {
			a, b, c, d := w[j-1], w[j], w[j+1], w[j+2]
			if a.x != b.x || c.x != d.x || b.y != c.y || (b.y-a.y)*(d.y-c.y) <= 0 {
				continue
			}
			away, down := sign(c.x-b.x), sign(d.y-a.y)
			for s := 0; s <= wideReach; s += wideStep {
				x := c.x + away*s
				for y := a.y; (d.y-y)*down >= roadEnd; y += down * wideStep {
					if s == 0 && y == b.y {
						continue
					}
					raw = append(append(append(raw[:0], w[:j]...), pt{b.x, y}, pt{x, y}, pt{x, d.y}), w[j+3:]...)
					nw = straightInto(nw, raw)
					if !was.keeps(nw, r.boxes) {
						continue
					}
					if sw = straightInto(sw, nw); len(sw) > turns {
						continue
					}
					// Each move changes route i alone, so one copy of the
					// routes serves them all, with route i put back after.
					if next == nil {
						next = slices.Clone(ways)
					}
					next[i] = nw
					if ix == nil {
						ix = newRoadIndex(v, ways)
					}
					nc, ok := ix.change(next, []int{i}).costWithin(cost)
					if !ok {
						continue
					}
					if nc.better(cost) {
						next[i] = slices.Clone(nw)
						return next, true
					}
					// A move that costs nothing may free a run another route
					// shared with this one, so it is kept where a move of a
					// whole run then reads better.
					if !cost.withers(nc) {
						continue
					}
					next[i] = slices.Clone(nw)
					if then, ok := r.shift(next, v, nc, stairsFirst, false); ok && v.cost(then).better(cost) {
						return then, true
					}
				}
			}
		}
		if next != nil {
			next[i] = w
		}
	}
	return nil, false
}

// roads draws the road the routes ride: one path for each stretch that the
// same routes ride, from a shape, a split or a merge to the next.
func roads(ways [][]pt, style []string, reqs []wireReq, back []bool) []drawn {
	type edge struct{ a, b pt }
	type node struct {
		style string
		p     pt
	}
	riders := map[string]map[edge][]int{}
	var styles []string
	cuts := map[string][]pt{}
	for i, w := range ways {
		if _, ok := riders[style[i]]; !ok {
			riders[style[i]] = map[edge][]int{}
			styles = append(styles, style[i])
		}
		cuts[style[i]] = append(cuts[style[i]], w...)
	}
	sort.Strings(styles)
	norm := func(a, b pt) edge {
		if b.x < a.x || b.x == a.x && b.y < a.y {
			a, b = b, a
		}
		return edge{a, b}
	}
	for i, w := range ways {
		for j := 1; j < len(w); j++ {
			a, b := w[j-1], w[j]
			var cut []pt
			for _, p := range cuts[style[i]] {
				if p != a && p != b && on(p, a, b) {
					cut = append(cut, p)
				}
			}
			sort.Slice(cut, func(x, y int) bool {
				return abs(cut[x].x-a.x)+abs(cut[x].y-a.y) < abs(cut[y].x-a.x)+abs(cut[y].y-a.y)
			})
			ps := append(append([]pt{a}, slices.Compact(cut)...), b)
			for k := 1; k < len(ps); k++ {
				e := norm(ps[k-1], ps[k])
				if !slices.Contains(riders[style[i]][e], i) {
					riders[style[i]][e] = append(riders[style[i]][e], i)
				}
			}
		}
	}
	ends := map[node]bool{}
	for i, w := range ways {
		ends[node{style[i], w[0]}], ends[node{style[i], w[len(w)-1]}] = true, true
	}
	var out []drawn
	for _, s := range styles {
		es := riders[s]
		at := map[pt][]edge{}
		var keys []edge
		for e, is := range es {
			slices.Sort(is)
			at[e.a], at[e.b] = append(at[e.a], e), append(at[e.b], e)
			keys = append(keys, e)
		}
		sort.Slice(keys, func(i, j int) bool {
			a, b := keys[i], keys[j]
			if a.a != b.a {
				return a.a.y < b.a.y || a.a.y == b.a.y && a.a.x < b.a.x
			}
			return a.b.y < b.b.y || a.b.y == b.b.y && a.b.x < b.b.x
		})
		// A path runs on through a point that only it passes, by the same
		// routes, and that ends no route.
		onward := func(p pt, e edge) (edge, bool) {
			if ends[node{s, p}] || len(at[p]) != 2 {
				return edge{}, false
			}
			o := at[p][0]
			if o == e {
				o = at[p][1]
			}
			return o, slices.Equal(es[o], es[e])
		}
		done := map[edge]bool{}
		for _, e := range keys {
			if done[e] {
				continue
			}
			done[e] = true
			pts := []pt{e.a, e.b}
			for cur, p := e, e.b; ; {
				o, ok := onward(p, cur)
				if !ok || done[o] {
					break
				}
				done[o] = true
				p = map[bool]pt{true: o.b, false: o.a}[o.a == p]
				pts, cur = append(pts, p), o
			}
			for cur, p := e, e.a; ; {
				o, ok := onward(p, cur)
				if !ok || done[o] {
					break
				}
				done[o] = true
				p = map[bool]pt{true: o.b, false: o.a}[o.a == p]
				pts, cur = append([]pt{p}, pts...), o
			}
			out = append(out, road(pts, es[e], ways, reqs, back))
		}
	}
	// A bus is the paths of the lines that leave one place, or that reach
	// one, as rule 4 bundles them: one network to a reader, whose pieces
	// neither cross nor stand beside one another as two lines do. A path
	// that leaves a box and reaches one joins both buses into one.
	root := map[string]string{}
	find := func(k string) string {
		for root[k] != "" && root[k] != k {
			k = root[k]
		}
		return k
	}
	keys := make([][]string, len(out))
	for k, d := range out {
		var at [2]string
		for side := range 2 {
			placeOf := func(i int) string {
				e := [2]end{reqs[i].from, reqs[i].to}[side]
				if e.node != "" {
					return "node|" + e.node
				}
				return place(e)
			}
			p := placeOf(d.reqs[0])
			if !slices.ContainsFunc(d.reqs, func(i int) bool { return placeOf(i) != p }) {
				at[side] = fmt.Sprint(side, p, d.kind, d.back)
			}
		}
		leaves, arrives := at[0] != "" && d.starts[0] != noStart, at[1] != "" && !d.joins
		switch {
		case leaves && arrives:
			keys[k] = []string{at[0], at[1]}
		case leaves || at[0] != "" && !arrives:
			keys[k] = []string{at[0]}
		case at[1] != "":
			keys[k] = []string{at[1]}
		}
		for _, key := range keys[k][min(1, len(keys[k])):] {
			if a, b := find(keys[k][0]), find(key); a != b {
				root[b] = a
			}
		}
	}
	nets := map[string]int{}
	for k := range out {
		if len(keys[k]) == 0 {
			continue
		}
		r := find(keys[k][0])
		if _, ok := nets[r]; !ok {
			nets[r] = len(nets) + 1
		}
		out[k].net = nets[r]
	}
	return out
}

// road is one path of the road, ridden by the routes is, turned the way
// they run, with a count where it meets a shape and carries more than one.
func road(pts []pt, is []int, ways [][]pt, reqs []wireReq, back []bool) drawn {
	first := ways[is[0]]
	pos := func(p pt) int {
		for k := 1; k < len(first); k++ {
			if on(p, first[k-1], first[k]) {
				return k*100000 + abs(p.x-first[k-1].x) + abs(p.y-first[k-1].y)
			}
		}
		return -1
	}
	if pos(pts[0]) > pos(pts[len(pts)-1]) {
		slices.Reverse(pts)
	}
	for _, i := range is {
		if ways[i][0] == pts[len(pts)-1] {
			slices.Reverse(pts)
			break
		}
	}
	// A piece several kinds ride wears the kind most of them have, the
	// first by name among equals: only its ends, local roads of one kind,
	// carry heads.
	n := map[string]int{}
	for _, i := range is {
		n[reqs[i].kind]++
	}
	kind := reqs[is[0]].kind
	for k, c := range n {
		if c > n[kind] || c == n[kind] && k < kind {
			kind = k
		}
	}
	d := drawn{D: polyline(pts), parts: [][]pt{pts}, reqs: is, kind: kind, back: back[is[0]], road: true}
	last := pts[len(pts)-1]
	arrives := false
	for _, i := range is {
		st := noStart
		if ways[i][0] == pts[0] {
			st = pts[0]
		}
		d.starts = append(d.starts, st)
		arrives = arrives || ways[i][len(ways[i])-1] == last
	}
	d.joins = !arrives
	if len(is) > 1 {
		switch {
		case arrives:
			d.count, d.own = len(is), [2]pt{pts[len(pts)-2], last}
		case d.starts[0] != noStart:
			d.count, d.own = len(is), [2]pt{pts[0], pts[1]}
		}
		if d.count > 0 {
			d.counted = is
			d.label = pt{d.own[0].x + 5, (d.own[0].y+d.own[1].y)/2 + 4}
		}
	}
	return d
}
