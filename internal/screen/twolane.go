package screen

// Lines that run opposite ways side by side read at rest as one line drawn
// twice (#232). They are drawn as a two-lane road instead: each keeps to
// its own right, as traffic does, and the two lanes stand twoLaneGap apart,
// closer than lane, the pitch of lines that merely run near each other, so
// the pair reads as one road both ways. Where no lane can take its own
// right without a crossing or a turn, the two run apart instead, as a hand
// drawing keeps lines it does not merge.

const (
	// twoLaneGap is how far apart, in px, the two lanes of a two-lane road
	// are drawn: under lane, and not under rule 6's 5px, where lines of two
	// styles blur into one.
	twoLaneGap = 6
	// twoLaneApart is how far apart, in px, a pair that cannot be a
	// two-lane road is set: two lane pitches, clear of besideGap.
	twoLaneApart = 2 * lane
)

// rightOf is which way across its track, -1 or 1, a lane that goes way
// along it keeps: its own right. A lane going right (1) along a flat
// track keeps below, one going down an upright track keeps to the left.
func rightOf(vertical bool, way int) int {
	if vertical {
		return -way
	}
	return way
}

// heading is the way a run from a to b goes along its track: 1 right or
// down, -1 left or up.
func heading(a, b pt) int { return sign(b.x - a.x + b.y - a.y) }

// runWay is the way every route run of r goes along its track, or 0 when
// they go both ways.
func runWay(ways [][]pt, r straightRun) int {
	way := 0
	for _, s := range r.segs {
		w := ways[s[0]]
		h := heading(w[s[1]], w[s[1]+1])
		if way != 0 && h != way {
			return 0
		}
		way = h
	}
	return way
}

// twoLanes sets each pair of pieces of road that run opposite ways side by
// side, besideGap or less apart on two tracks, as a two-lane road: one
// piece moves to twoLaneGap from the other, on its own right, the back
// piece first, so the forward flow keeps its track; failing both, one
// moves twoLaneApart from the other. A move is kept only where every route
// it moves still reads as a line and the view gains nothing roadCost
// counts, ink beside other ink included. Lines that
// share one track both ways stay there: moving them apart reveals the
// crossings the shared ink hides (see the pull request of #232).
func (r *routing) twoLanes(ways [][]pt, v roadView) [][]pt {
	tried := map[[3]int]bool{}
	for n := 0; n < roadMoves; n++ {
		next, ok := r.twoLane(ways, v, tried)
		if !ok {
			break
		}
		ways = next
	}
	return ways
}

// twoLane makes the first move twoLanes keeps, if any. tried holds the
// moves weighed already, by the run's first route run and where it goes.
func (r *routing) twoLane(ways [][]pt, v roadView, tried map[[3]int]bool) ([][]pt, bool) {
	rs := roadRunsBy(ways, v, true)
	way := make([]int, len(rs))
	for i, run := range rs {
		way[i] = runWay(ways, run)
	}
	cost := v.cost(ways)
	var x *roadIndex
	for i, a := range rs {
		for j, b := range rs[i+1:] {
			j += i + 1
			d := b.at - a.at
			if a.vertical != b.vertical || way[i] == 0 || way[i] != -way[j] || d == 0 || abs(d) > besideGap ||
				min(a.hi, b.hi) <= max(a.lo, b.lo) || d == twoLaneGap*rightOf(b.vertical, way[j]) {
				continue
			}
			type move struct {
				run straightRun
				at  int
			}
			moves := []move{
				{b, a.at + twoLaneGap*rightOf(b.vertical, way[j])}, {a, b.at + twoLaneGap*rightOf(a.vertical, way[i])},
				{b, a.at + sign(d)*twoLaneApart}, {a, b.at - sign(d)*twoLaneApart},
			}
			if a.style == "true" && b.style != "true" {
				moves[0], moves[1], moves[2], moves[3] = moves[1], moves[0], moves[3], moves[2]
			}
			for _, m := range moves {
				k := [3]int{m.run.segs[0][0], m.run.segs[0][1], m.at}
				if tried[k] || !m.run.moves(m.at) {
					continue
				}
				tried[k] = true
				if x == nil {
					x = newRoadIndex(v, ways)
				}
				if next, ok := r.keepsLanes(ways, m.run, m.at, x, cost); ok {
					return next, true
				}
			}
		}
	}
	return nil, false
}

// keepsLanes is the routes with run moved to at, where twoLanes keeps the
// move, x being the routes' road index and cost what it counts.
func (r *routing) keepsLanes(ways [][]pt, run straightRun, at int, x *roadIndex, cost roadCost) ([][]pt, bool) {
	next := moved(ways, run, at)
	var ts []int
	touched := map[int]bool{}
	for _, s := range run.segs {
		if !routeShapeOf(ways[s[0]], r.boxes).keeps(next[s[0]], r.boxes) {
			return nil, false
		}
		if !touched[s[0]] {
			touched[s[0]] = true
			ts = append(ts, s[0])
		}
	}
	for _, t := range ts {
		next[t] = straight(next[t])
	}
	if bendsMore(ways, next, touched, true) {
		return nil, false
	}
	if forkLevelsOf(next, x.v) > forkLevelsOf(ways, x.v) {
		return nil, false
	}
	ch := x.change(next, ts)
	if nc, ok := ch.costWithin(cost); !ok || nc.beside > cost.beside {
		return nil, false
	}
	if was, now := ch.endMixes(); now > was {
		return nil, false
	}
	return next, true
}

// forkLevelsOf counts, as the lint's forkLevels reads them, the flat tracks
// past the first that a fork's branches turn onto: the lines of one kind
// between one place and two or more places of one row, leaving it or
// arriving at it, each by the flat run nearest that place. roadCost counts
// only forks that leave, by style, so a move may split an arrival.
func forkLevelsOf(ways [][]pt, v roadView) int {
	type key struct {
		place, kind string
		row         int
		out         bool
	}
	ys, fars := map[key]map[float64]bool{}, map[key]map[string]bool{}
	for i, w := range ways {
		for _, out := range []bool{true, false} {
			k, far := key{v.from[i], v.kinds[i], v.dest[i], true}, v.to[i]
			if !out {
				k, far = key{v.to[i], v.kinds[i], v.position(w[0].y), false}, v.from[i]
			}
			if fars[k] == nil {
				ys[k], fars[k] = map[float64]bool{}, map[string]bool{}
			}
			fars[k][far] = true
			if y, ok := flatNear(fpts(w), !out); ok {
				ys[k][y] = true
			}
		}
	}
	n := 0
	for k, f := range fars {
		if len(f) >= 2 && len(ys[k]) > 1 {
			n += len(ys[k]) - 1
		}
	}
	return n
}
