package screen

import (
	"math"
	"sort"
)

// A reader follows one line along a road by its route, and loses it where
// the road turns or parts (Ware et al. 2002: a path that runs on straight
// through a junction is followed best). So lint reads, on the routes of a
// drawing that holds them, how often each line turns while it rides ink it
// shares with another line, and how many junctions it passes: points on its
// way, away from its two ends, where another line joins or leaves its ink.
// A junction counts once for each line that passes it. It also counts the
// bent junctions, once each, where of the lines that come in one way more
// go out one other way than run on straight, though one runs on, or the
// same read from where they go out: the road a reader follows bends, and a
// lone line runs on. Nothing bounds them; they are diagnostics.

// roadWalk is what walking every route of a drawing finds.
type roadWalk struct {
	// turns is each route's turns on shared ink, junctions the junctions it
	// passes, and bent the junctions that bend the road.
	turns, junctions, bent int
	// bentAt are the bent junctions.
	bentAt []fpt
}

// walkLeg is one straight stretch of a route: vertical or not, at its x or
// y, over lo..hi.
type walkLeg struct {
	vertical bool
	at       float64
	lo, hi   float64
}

func stretchOf(a, b fpt) (walkLeg, bool) {
	switch {
	case math.Abs(a.x-b.x) < 0.5 && math.Abs(a.y-b.y) >= 0.5:
		return walkLeg{true, a.x, math.Min(a.y, b.y), math.Max(a.y, b.y)}, true
	case math.Abs(a.y-b.y) < 0.5 && math.Abs(a.x-b.x) >= 0.5:
		return walkLeg{false, a.y, math.Min(a.x, b.x), math.Max(a.x, b.x)}, true
	}
	return walkLeg{}, false
}

// along is where p stands along the stretch, and across how far off its
// line.
func (a walkLeg) along(p fpt) float64 {
	if a.vertical {
		return p.y
	}
	return p.x
}

func (a walkLeg) across(p fpt) float64 {
	if a.vertical {
		return math.Abs(p.x - a.at)
	}
	return math.Abs(p.y - a.at)
}

func (a walkLeg) point(v float64) fpt {
	if a.vertical {
		return fpt{a.at, v}
	}
	return fpt{v, a.at}
}

// shareMin is the least two collinear stretches overlap before they read as
// one ink.
const shareMin = 1.0

// walkRoads reads the routes rs, each by its turning points.
func walkRoads(rs [][]fpt) roadWalk {
	var w roadWalk
	stretches := make([][]walkLeg, len(rs))
	for i, r := range rs {
		for k := 1; k < len(r); k++ {
			if a, ok := stretchOf(r[k-1], r[k]); ok {
				stretches[i] = append(stretches[i], a)
			}
		}
	}
	// sharedOn says another route than i runs along a on [lo, hi].
	sharedOn := func(i int, a walkLeg, lo, hi float64) bool {
		for j, ss := range stretches {
			if j == i {
				continue
			}
			for _, b := range ss {
				if b.vertical == a.vertical && math.Abs(b.at-a.at) < 0.5 && math.Min(hi, b.hi)-math.Max(lo, b.lo) >= shareMin-1e-9 {
					return true
				}
			}
		}
		return false
	}
	key := func(p fpt) [2]int { return [2]int{int(math.Round(p.x)), int(math.Round(p.y))} }
	// wayAt is how route i comes into p and goes out of it, false where p
	// is no point of its way between its ends.
	wayAt := func(i int, p fpt) ([2]fpt, bool) {
		ts := rs[i]
		for k := 1; k < len(ts)-1; k++ {
			if dist(ts[k], p) < 0.75 {
				return [2]fpt{dirOf(ts[k-1], ts[k]), dirOf(ts[k], ts[k+1])}, true
			}
		}
		for k := 1; k < len(ts); k++ {
			if a, ok := stretchOf(ts[k-1], ts[k]); ok && a.across(p) < 0.75 && a.along(p) > a.lo && a.along(p) < a.hi {
				d := dirOf(ts[k-1], ts[k])
				return [2]fpt{d, d}, true
			}
		}
		return [2]fpt{}, false
	}
	// ways[p] are the ways the routes through junction p come in and go out
	// there.
	ways := map[[2]int][][2]fpt{}
	for i, r := range rs {
		if len(r) < 2 {
			continue
		}
		// A turn rides the road where the stretch into it or out of it is
		// shared right at the turn.
		for k := 1; k < len(r)-1; k++ {
			in, ok1 := stretchOf(r[k-1], r[k])
			out, ok2 := stretchOf(r[k], r[k+1])
			on := ok1 && sharedOn(i, in, math.Max(in.lo, in.along(r[k])-shareMin), math.Min(in.hi, in.along(r[k])+shareMin))
			on = on || ok2 && sharedOn(i, out, math.Max(out.lo, out.along(r[k])-shareMin), math.Min(out.hi, out.along(r[k])+shareMin))
			if on {
				w.turns++
			}
		}
		// The junctions are the points along the route, away from its two
		// ends, where another route's shared ink starts or stops: inside the
		// route's stretch, or at its end where the other runs on past it, or
		// where both start or stop there but come in, or go out, different
		// ways, as two lines from either side that merge onto one trunk.
		first, last := key(r[0]), key(r[len(r)-1])
		seen := map[[2]int]fpt{}
		for _, a := range stretches[i] {
			for j, ss := range stretches {
				if j == i {
					continue
				}
				for _, b := range ss {
					if b.vertical != a.vertical || math.Abs(b.at-a.at) >= 0.5 {
						continue
					}
					lo, hi := math.Max(a.lo, b.lo), math.Min(a.hi, b.hi)
					if hi-lo < shareMin-1e-9 {
						continue
					}
					for _, v := range []float64{lo, hi} {
						p := a.point(v)
						joins := v > a.lo && v < a.hi || v == a.lo && v > b.lo || v == a.hi && v < b.hi
						if !joins {
							wi, oki := wayAt(i, p)
							wj, okj := wayAt(j, p)
							joins = oki && okj && wi != wj
						}
						if k := key(p); joins && k != first && k != last {
							seen[k] = p
						}
					}
				}
			}
		}
		for k, p := range seen {
			if wa, ok := wayAt(i, p); ok {
				w.junctions++
				ways[k] = append(ways[k], wa)
			}
		}
	}
	ps := make([][2]int, 0, len(ways))
	for p := range ways {
		ps = append(ps, p)
	}
	sort.Slice(ps, func(a, b int) bool { return ps[a][0] < ps[b][0] || ps[a][0] == ps[b][0] && ps[a][1] < ps[b][1] })
	for _, p := range ps {
		if bentAt(ways[p]) {
			w.bent++
			w.bentAt = append(w.bentAt, fpt{float64(p[0]), float64(p[1])})
		}
	}
	return w
}

// bentAt says a junction bends the road: of the lines that come in one way,
// more go out one other way than run on straight, though one runs on; or,
// of the lines that go out one way, more come in from one other way than
// from straight behind, though one does.
func bentAt(ways [][2]fpt) bool {
	for side := range 2 {
		by := map[fpt]map[fpt]int{}
		for _, w := range ways {
			at, other := w[side], w[1-side]
			if by[at] == nil {
				by[at] = map[fpt]int{}
			}
			by[at][other]++
		}
		for at, n := range by {
			most := 0
			for d, c := range n {
				if d != at {
					most = max(most, c)
				}
			}
			if n[at] > 0 && most > n[at] {
				return true
			}
		}
	}
	return false
}
