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
// way where another line joins or leaves its ink. It also counts the bent
// junctions, where more of the lines through a junction turn there than run
// on straight, though one runs on: the road a reader follows bends, and a
// lone line runs on. Nothing bounds them; they are diagnostics.

// roadWalk is what walking every route of a drawing finds.
type roadWalk struct {
	// turns is each route's turns on shared ink, junctions the junctions it
	// passes, and bent the junctions where more lines turn than run on.
	turns, junctions, bent int
	// bentAt are the bent junctions.
	bentAt []fpt
}

// walkLeg is one straight stretch of a route: vertical or not, at its x or y,
// over lo..hi.
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
	// shared says another route than i runs along a on [lo, hi].
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
	pointOf := func(a walkLeg, v float64) fpt {
		if a.vertical {
			return fpt{a.at, v}
		}
		return fpt{v, a.at}
	}
	key := func(p fpt) [2]int { return [2]int{int(math.Round(p.x)), int(math.Round(p.y))} }
	// ways[p] are the ways the routes through junction p come in and go
	// out there.
	ways := map[[2]int][][2]fpt{}
	for i, r := range rs {
		ts := turns(r)
		if len(ts) < 2 {
			continue
		}
		// wayAt is how the route comes into p and goes out of it.
		wayAt := func(p [2]int) [2]fpt {
			for k := 1; k < len(ts)-1; k++ {
				if key(ts[k]) == p {
					return [2]fpt{dirOf(ts[k-1], ts[k]), dirOf(ts[k], ts[k+1])}
				}
			}
			for k := 1; k < len(ts); k++ {
				q := fpt{float64(p[0]), float64(p[1])}
				if a, ok := stretchOf(ts[k-1], ts[k]); ok {
					if v := map[bool]float64{true: q.y, false: q.x}[a.vertical]; math.Abs(map[bool]float64{true: q.x, false: q.y}[a.vertical]-a.at) < 1 && v >= a.lo && v <= a.hi {
						d := dirOf(ts[k-1], ts[k])
						return [2]fpt{d, d}
					}
				}
			}
			return [2]fpt{}
		}
		// A turn rides the road where the stretch into it or out of it is
		// shared right at the turn.
		for k := 1; k < len(ts)-1; k++ {
			in, ok1 := stretchOf(ts[k-1], ts[k])
			out, ok2 := stretchOf(ts[k], ts[k+1])
			v := func(a walkLeg, p fpt) float64 {
				if a.vertical {
					return p.y
				}
				return p.x
			}
			on := ok1 && sharedOn(i, in, math.Max(in.lo, v(in, ts[k])-shareMin), math.Min(in.hi, v(in, ts[k])+shareMin))
			on = on || ok2 && sharedOn(i, out, math.Max(out.lo, v(out, ts[k])-shareMin), math.Min(out.hi, v(out, ts[k])+shareMin))
			if on {
				w.turns++
			}
		}
		// The junctions are the points along the route, away from its two
		// ends, where another route's shared ink starts or stops.
		first, last := key(r[0]), key(r[len(r)-1])
		seen := map[[2]int]bool{}
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
						// Where the other route's ink and this one's both
						// end, they part at a turn of neither: no junction.
						if v > a.lo && v < a.hi || v == a.lo && (v > b.lo) || v == a.hi && (v < b.hi) {
							p := key(pointOf(a, v))
							if p != first && p != last && !seen[p] {
								seen[p] = true
							}
						}
					}
				}
			}
		}
		for p := range seen {
			w.junctions++
			ways[p] = append(ways[p], wayAt(p))
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
