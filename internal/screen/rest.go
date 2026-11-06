package screen

import (
	"math"
	"slices"
	"sort"
)

// These are what a reader meets in a drawing at rest, before pointing at
// anything: ink that runs beside other ink, the straight pieces the eye
// takes in, and forks whose branches leave their stem at different heights.
// They measure the drawn ink alone, never a criterion's breaches.

// besideRoom is how close two straight pieces of ink, running the same way,
// may stand before they read as lines side by side that one road could
// carry.
const besideRoom = 10.0

type inkRun struct {
	// at is the stretch's x when it is vertical, its y when horizontal, and
	// lo, hi the span it covers along its run.
	at, lo, hi float64
	vertical   bool
	stroke     int
}

func stretchesOf(s sheet) []inkRun {
	var out []inkRun
	for k, w := range s.strokes {
		for i := 1; i < len(w.pts); i++ {
			a, b := w.pts[i-1], w.pts[i]
			switch {
			case dist(a, b) < 0.5:
			case math.Abs(a.x-b.x) < 0.5:
				out = append(out, inkRun{a.x, math.Min(a.y, b.y), math.Max(a.y, b.y), true, k})
			case math.Abs(a.y-b.y) < 0.5:
				out = append(out, inkRun{a.y, math.Min(a.x, b.x), math.Max(a.x, b.x), false, k})
			}
		}
	}
	return out
}

// besideInk is how far, in px summed over pairs, pieces of different paths
// run the same way within besideRoom of each other and not on one another.
func besideInk(s sheet) int {
	ss := stretchesOf(s)
	n := 0.0
	for i, a := range ss {
		for _, b := range ss[i+1:] {
			if a.vertical != b.vertical || a.stroke == b.stroke {
				continue
			}
			if d := math.Abs(a.at - b.at); d < 0.5 || d > besideRoom {
				continue
			}
			n += math.Max(0, math.Min(a.hi, b.hi)-math.Max(a.lo, b.lo))
		}
	}
	return int(math.Round(n))
}

// straightPieces is how many straight pieces the ink shows: collinear pieces that
// touch or overlap read as one, whichever paths draw them.
func straightPieces(s sheet) int {
	type line struct {
		vertical bool
		at       int
	}
	spans := map[line][][2]float64{}
	for _, st := range stretchesOf(s) {
		k := line{st.vertical, int(math.Round(st.at))}
		spans[k] = append(spans[k], [2]float64{st.lo, st.hi})
	}
	n := 0
	for _, sp := range spans {
		sort.Slice(sp, func(i, j int) bool { return sp[i][0] < sp[j][0] })
		end := math.Inf(-1)
		for _, s := range sp {
			if s[0] > end+0.5 {
				n++
			}
			end = math.Max(end, s[1])
		}
	}
	return n
}

// inkCrossings counts the points where a straight piece of ink passes
// through another, each point once, however the ink is cut into paths: a
// junction, where one piece ends on another, is no crossing.
func inkCrossings(s sheet) int {
	type line struct {
		vertical bool
		at       int
	}
	merged := map[line][][2]float64{}
	for _, st := range stretchesOf(s) {
		k := line{st.vertical, int(math.Round(st.at))}
		merged[k] = append(merged[k], [2]float64{st.lo, st.hi})
	}
	var vs, hs []inkRun
	for k, sp := range merged {
		sort.Slice(sp, func(i, j int) bool { return sp[i][0] < sp[j][0] })
		for i := 0; i < len(sp); i++ {
			lo, hi := sp[i][0], sp[i][1]
			for i+1 < len(sp) && sp[i+1][0] <= hi+0.5 {
				i++
				hi = math.Max(hi, sp[i][1])
			}
			r := inkRun{at: float64(k.at), lo: lo, hi: hi, vertical: k.vertical}
			if k.vertical {
				vs = append(vs, r)
			} else {
				hs = append(hs, r)
			}
		}
	}
	n := 0
	for _, v := range vs {
		for _, h := range hs {
			if v.at > h.lo+0.5 && v.at < h.hi-0.5 && h.at > v.lo+0.5 && h.at < v.hi-0.5 {
				n++
			}
		}
	}
	return n
}

// forkLevels counts, for each place's lines of one kind that drop out of it
// and turn toward boxes in one row, the heights past the first at which
// they turn: a fork whose branches leave its stem on one track has none.
func (l *look) forkLevels() int {
	type fork struct {
		from, kind string
		row        int
	}
	ys := map[fork][]int{}
	for _, w := range l.trace.Ways {
		ps := turns(w.pts)
		if len(ps) < 3 || math.Abs(ps[0].x-ps[1].x) > 0.5 {
			continue
		}
		f := fork{w.from, w.kind, l.placeAt(w.to)}
		if y := int(math.Round(ps[1].y)); !slices.Contains(ys[f], y) {
			ys[f] = append(ys[f], y)
		}
	}
	n := 0
	for _, y := range ys {
		n += len(y) - 1
	}
	return n
}
