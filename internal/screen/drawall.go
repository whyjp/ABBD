package screen

import (
	"cmp"
	"fmt"
	"os"
	"slices"
	"strconv"

	"github.com/whyjp/abbd/internal/model"
)

// Prototype only (branch drawall): the overview draws every pair between
// the boxes it draws, and, with ABBD_FOLD=N, folds boxes until N or fewer
// stand. Both read only the store, so the same input gives the same picture.

// drawAll is the record's cut with every pair of o drawn: the record keeps
// its words, and no box counts a pair it does not draw.
func drawAll(o *overview, c *cut) *cut {
	if os.Getenv("ABBD_DRAWALL") == "" || c == nil || c.said == nil {
		return c
	}
	keep := map[pair]bool{}
	for _, e := range o.drawnPairs() {
		keep[pairOf(model.ID(e.from), model.ID(e.to))] = true
	}
	return &cut{keep: keep, more: map[model.ID]int{}, said: c.said}
}

// folded is in with its top-level boxes folded until ABBD_FOLD or fewer
// modules stand, by one rule over stored data only:
//  1. A record zone that holds no stage of the primary flow folds into one
//     box named for the zone, its boxes inside it: the zone with the fewest
//     live pairs first, then by its place in the record.
//  2. Past the zones, a module no zone holds, on no stage, that no record
//     line names, leaves the overview: the fewest live pairs first, then by
//     ID. It stays one level down, in the focus views of what uses it.
//
// A zone or a stage is never split, and a box the record draws a line to
// never leaves.
func folded(in Input) Input {
	n, err := strconv.Atoi(os.Getenv("ABBD_FOLD"))
	if err != nil || n <= 0 || !in.Overview.Known {
		return in
	}
	units := slices.Clone(in.Units)
	t := newTree(units)
	top := func(id model.ID) model.ID {
		if c := t.chain(id); len(c) > 0 {
			return c[0]
		}
		return ""
	}
	var mods []model.ID
	for _, u := range model.Present(units) {
		if u.Parent == "" && (u.Kind == model.Module || u.Kind == model.Feature) {
			mods = append(mods, u.ID)
		}
	}
	pairs := map[model.ID]map[model.ID]bool{}
	for _, u := range model.Present(units) {
		for _, r := range u.Relations {
			a, b := top(u.ID), top(r.To)
			if a == "" || b == "" || a == b || r.Dead {
				continue
			}
			for _, p := range [][2]model.ID{{a, b}, {b, a}} {
				if pairs[p[0]] == nil {
					pairs[p[0]] = map[model.ID]bool{}
				}
				pairs[p[0]][p[1]] = true
			}
		}
	}
	staged := map[model.ID]bool{}
	if p, ok := in.Figures.Primary(); ok {
		for _, part := range p.Parts {
			for _, u := range part.Claim.Units {
				staged[top(u)] = true
			}
		}
	}
	lined := map[model.ID]bool{}
	for _, l := range in.Overview.Lines {
		lined[top(l.Claim.From)], lined[top(l.Claim.To)] = true, true
	}
	zoned := map[model.ID]bool{}
	type zone struct {
		at, pairs int
		name     string
		boxes    []model.ID
	}
	var zs []zone
	for i, z := range in.Overview.Zones {
		var bs []model.ID
		stage := false
		for _, u := range z.Claim.Units {
			if b := top(u); b != "" && !slices.Contains(bs, b) {
				bs = append(bs, b)
				zoned[b] = true
				stage = stage || staged[b]
			}
		}
		if stage || len(bs) < 2 {
			continue
		}
		p := 0
		for _, b := range bs {
			p += len(pairs[b])
		}
		zs = append(zs, zone{i, p, z.Claim.Name, bs})
	}
	slices.SortStableFunc(zs, func(a, b zone) int { return cmp.Or(a.pairs-b.pairs, a.at-b.at) })
	count := len(mods)
	byID := map[model.ID]int{}
	for i, u := range units {
		byID[u.ID] = i
	}
	for _, z := range zs {
		if count <= n {
			break
		}
		id := model.ID(fmt.Sprintf("MOD-Z%03d", z.at+1))
		units = append(units, model.Unit{ID: id, Kind: model.Module, Name: model.Fact{Value: z.name}})
		for _, b := range z.boxes {
			units[byID[b]].Parent = id
		}
		count -= len(z.boxes) - 1
	}
	var loose []model.ID
	for _, m := range mods {
		if !zoned[m] && !staged[m] && !lined[m] {
			loose = append(loose, m)
		}
	}
	slices.SortStableFunc(loose, func(a, b model.ID) int { return cmp.Or(len(pairs[a])-len(pairs[b]), cmp.Compare(a, b)) })
	gone := map[model.ID]bool{}
	for _, m := range loose {
		if count <= n {
			break
		}
		gone[m] = true
		count--
	}
	for i, u := range units {
		if gone[top(u.ID)] {
			units[i].Absent = true
		}
	}
	in.Units = units
	return in
}
