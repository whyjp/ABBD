package screen

import (
	"math"
	"slices"
	"sort"
)

// besideGap is how near, in px, two stretches of ink of different drawn
// paths may run the same way before they read as lines doubled side by
// side, which only a hover tells apart.
const besideGap = 10.0

// inkStretch is one straight stretch of drawn ink, between two turns of the
// path.w it is part of.
type inkStretch struct {
	path int
	a, b fpt
}

// inkStretches are the straight stretches of every drawn path of s, a
// subpath that runs on from where the last one stopped, the same way,
// merged into it, since it draws one stretch.
func inkStretches(s sheet) []inkStretch {
	var out []inkStretch
	for _, st := range s.strokes {
		if len(st.pts) < 2 {
			continue
		}
		pts := turns(st.pts)
		for i := 1; i < len(pts); i++ {
			a, b := pts[i-1], pts[i]
			if a == b {
				continue
			}
			if k := len(out) - 1; k >= 0 && out[k].path == st.path && out[k].b == a && dirOf(out[k].a, out[k].b) == dirOf(a, b) {
				out[k].b = b
				continue
			}
			out = append(out, inkStretch{st.path, a, b})
		}
	}
	return out
}

// axisRun is a flat or upright stretch of a drawn path: it stands at at
// across the way it runs, from lo to hi along it.
type axisRun struct {
	path       int
	at, lo, hi float64
}

// axisRuns are the flat and the upright stretches of s's drawn paths.
func axisRuns(s sheet) (flat, upright []axisRun) {
	for _, p := range inkStretches(s) {
		switch {
		case p.a.y == p.b.y:
			flat = append(flat, axisRun{p.path, p.a.y, math.Min(p.a.x, p.b.x), math.Max(p.a.x, p.b.x)})
		case p.a.x == p.b.x:
			upright = append(upright, axisRun{p.path, p.a.x, math.Min(p.a.y, p.b.y), math.Max(p.a.y, p.b.y)})
		}
	}
	return flat, upright
}

// parallelInk is how far, in px, stretches of two different drawn paths
// run along each other the same way, flat or upright, no more than
// besideGap apart: lines doubled side by side, which a hand drawing would
// merge into one bus, or keep apart. Each pair of stretches counts once.
func parallelInk(s sheet) int {
	flat, upright := axisRuns(s)
	total := 0.0
	for _, rs := range [][]axisRun{flat, upright} {
		sort.Slice(rs, func(i, j int) bool { return rs[i].at < rs[j].at })
		for i, r := range rs {
			for _, q := range rs[i+1:] {
				if q.at-r.at > besideGap {
					break
				}
				if q.path != r.path {
					total += math.Max(0, math.Min(r.hi, q.hi)-math.Max(r.lo, q.lo))
				}
			}
		}
	}
	return int(math.Round(total))
}

// abreastGap is the width, in px, across which three or more drawn paths
// running the same way read as lines abreast that a hand drawing would
// draw as one.
const abreastGap = 30.0

// abreastInk is how far, in px, three or more different drawn paths run
// the same way, flat or upright, within abreastGap across: each stretch of
// the way counts once for every separate group of such paths.
func abreastInk(s sheet) int {
	flat, upright := axisRuns(s)
	total := 0.0
	for _, rs := range [][]axisRun{flat, upright} {
		var cuts []float64
		for _, r := range rs {
			cuts = append(cuts, r.lo, r.hi)
		}
		slices.Sort(cuts)
		cuts = slices.Compact(cuts)
		for k := 1; k < len(cuts); k++ {
			lo, hi := cuts[k-1], cuts[k]
			var on []axisRun
			for _, r := range rs {
				if r.lo <= lo && r.hi >= hi {
					on = append(on, r)
				}
			}
			sort.Slice(on, func(i, j int) bool { return on[i].at < on[j].at })
			for i := 0; i < len(on); {
				paths, j := map[int]bool{}, i
				for ; j < len(on) && on[j].at-on[i].at <= abreastGap; j++ {
					paths[on[j].path] = true
				}
				if len(paths) >= 3 {
					total += hi - lo
					i = j
				} else {
					i++
				}
			}
		}
	}
	return int(math.Round(total))
}

// forkLevels counts, over every fork, the flat tracks its branches turn
// onto past the first: a fork is the lines of one kind between one place
// and two or more places of one row, boxes and chips banded together,
// leaving it (the flat stretch they drop from into the row) or arriving
// at it (the flat stretch they rise onto out of the row). A hand drawing
// splits a fork off one level.
func forkLevels(l *look) int {
	row := bands(l.s)
	type key struct {
		place, kind string
		row         int
		out         bool
	}
	ys, fars := map[key]map[float64]bool{}, map[key]map[string]bool{}
	for _, w := range l.trace.Ways {
		for _, out := range []bool{true, false} {
			place, far := w.from, w.to
			if !out {
				place, far = far, place
			}
			i, ok := l.at[far]
			if !ok {
				continue
			}
			k := key{place, w.kind, row[i], out}
			if fars[k] == nil {
				ys[k], fars[k] = map[float64]bool{}, map[string]bool{}
			}
			fars[k][far] = true
			if y, ok := flatNear(w.pts, out); ok {
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

// bands numbers the rows of s's boxes and chips, by shape index: a row is
// a run of shapes, by top, whose heights overlap, as look bands boxes.
func bands(s sheet) map[int]int {
	var at []int
	for i := range s.shapes {
		at = append(at, i)
	}
	sort.SliceStable(at, func(a, b int) bool { return s.shapes[at[a]].r.y < s.shapes[at[b]].r.y })
	row, n, bottom := map[int]int{}, 0, math.Inf(-1)
	for _, i := range at {
		r := s.shapes[i].r
		if r.y >= bottom {
			n++
		}
		bottom = math.Max(bottom, r.y+r.h)
		row[i] = n
	}
	return row
}

// flatNear is the y of the flat stretch of a line's points nearest their
// last point, or their first.
func flatNear(pts []fpt, last bool) (float64, bool) {
	for i := range len(pts) - 1 {
		j := i
		if last {
			j = len(pts) - 2 - i
		}
		if pts[j].y == pts[j+1].y && pts[j].x != pts[j+1].x {
			return pts[j].y, true
		}
	}
	return 0, false
}

// colourSwitches counts, over every line a road drawing routes, the
// times its ink changes colour on its way from its start to its end: at
// rest a reader follows a line by its colour, and a line that turns
// neutral for a stretch and back reads as broken.
func colourSwitches(d Drawing) int {
	n := 0
	for k, r := range d.Routes {
		way := runs(r.D)
		at := func(p pt) int {
			run := 0
			for _, q := range way {
				if on(p, q[0], q[1]) {
					return run + abs(p.x-q[0].x) + abs(p.y-q[0].y)
				}
				run += abs(q[1].x-q[0].x) + abs(q[1].y-q[0].y)
			}
			return -1
		}
		type piece struct {
			at  int
			hue string
		}
		var ps []piece
		for _, w := range d.Wires {
			if !slices.Contains(w.rides, k) {
				continue
			}
			rs := runs(w.D)
			if len(rs) == 0 {
				continue
			}
			a, b := at(rs[0][0]), at(rs[len(rs)-1][1])
			ps = append(ps, piece{min(a, b), w.Hue})
		}
		slices.SortFunc(ps, func(a, b piece) int { return a.at - b.at })
		for i := 1; i < len(ps); i++ {
			if ps[i].hue != ps[i-1].hue {
				n++
			}
		}
	}
	return n
}

// inkCrossings counts the points where a straight stretch of ink passes
// through another, each point once, however the ink is cut into paths: a
// road and a bus cut the same ink at different points, and a junction,
// where one stretch ends on another, is no crossing.
func inkCrossings(s sheet) int {
	type line struct {
		vertical bool
		at       int
	}
	spans := map[line][][2]float64{}
	for _, st := range inkStretches(s) {
		a, b := st.a, st.b
		switch {
		case math.Abs(a.x-b.x) < 0.5:
			k := line{true, int(math.Round(a.x))}
			spans[k] = append(spans[k], [2]float64{math.Min(a.y, b.y), math.Max(a.y, b.y)})
		case math.Abs(a.y-b.y) < 0.5:
			k := line{false, int(math.Round(a.y))}
			spans[k] = append(spans[k], [2]float64{math.Min(a.x, b.x), math.Max(a.x, b.x)})
		}
	}
	type run struct{ at, lo, hi float64 }
	var vs, hs []run
	for k, sp := range spans {
		sort.Slice(sp, func(i, j int) bool { return sp[i][0] < sp[j][0] })
		for i := 0; i < len(sp); i++ {
			lo, hi := sp[i][0], sp[i][1]
			for i+1 < len(sp) && sp[i+1][0] <= hi+0.5 {
				i++
				hi = math.Max(hi, sp[i][1])
			}
			if r := (run{float64(k.at), lo, hi}); k.vertical {
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
