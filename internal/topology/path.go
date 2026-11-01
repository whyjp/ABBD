package topology

import (
	"cmp"
	"slices"

	"github.com/whyjp/abbd/internal/model"
)

// Hop is one link of a path. Forward says the path walks it from From to To;
// otherwise it walks it backwards, from To to From.
//
// Shares is set on a hop that walks back along a depends_on or consumes link
// into the unit the previous hop used as well. The units on either side of
// that unit only use the same thing; neither relates to the other. Provider
// is a unit that provides Shares, when one does.
type Hop struct {
	Link
	Forward          bool
	Shares, Provider model.ID
}

func uses(k model.RelationKind) bool { return k == model.DependsOn || k == model.Consumes }

// cost orders walks: fewest joints where two users of one unit meet, then
// fewest hops, then the strongest weakest link, kept negated so smaller is
// better throughout.
type cost struct{ joints, hops, weak int }

func (c cost) less(d cost) bool {
	return cmp.Or(cmp.Compare(c.joints, d.joints), cmp.Compare(c.hops, d.hops), cmp.Compare(c.weak, d.weak)) < 0
}

// state is where a walk stands and whether it got there by using that unit,
// along a depends_on or consumes link walked forward.
type state struct {
	at   model.ID
	used bool
}

// Path is the walk from a to b over the live links, each walked in either
// direction. It routes the way units talk: two units that both use a third
// do not reach each other through it while any other way exists, however
// long. Then it takes the fewest hops, and of those the one whose weakest
// link is strongest, so a guess shows only where nothing firmer connects the
// two. A joint that remains is marked on its hop. ok is false when no walk
// exists.
func Path(units []model.Unit, a, b model.ID) (hops []Hop, ok bool) {
	return walk(Links(units), a, b)
}

func walk(links []Link, a, b model.ID) ([]Hop, bool) {
	start := state{a, false}
	best := map[state]cost{start: {weak: -(strength[model.Extracted] + 1)}}
	type step struct {
		from state
		hop  Hop
	}
	prev := map[state]step{}
	done := map[state]bool{}
	order := []state{start}
	for {
		var x state
		found := false
		for _, s := range order {
			if !done[s] && (!found || best[s].less(best[x])) {
				x, found = s, true
			}
		}
		if !found {
			break
		}
		done[x] = true
		c := best[x]
		for _, l := range links {
			forward := l.From == x.at
			y := l.To
			switch {
			case forward:
			case l.To == x.at:
				y = l.From
			default:
				continue
			}
			n := cost{c.joints, c.hops + 1, max(c.weak, -strength[l.Origin])}
			if x.used && !forward && uses(l.Kind) {
				n.joints++
			}
			s := state{y, forward && uses(l.Kind)}
			old, seen := best[s]
			if seen && !n.less(old) {
				continue
			}
			if !seen {
				order = append(order, s)
			}
			best[s], prev[s] = n, step{x, Hop{Link: l, Forward: forward}}
		}
	}
	end := state{b, false}
	if c, ok := best[state{b, true}]; ok {
		if d, ok := best[end]; !ok || c.less(d) {
			end = state{b, true}
		}
	}
	if _, ok := best[end]; !ok {
		return nil, false
	}
	var hops []Hop
	for s := end; s != start; s = prev[s].from {
		hops = append(hops, prev[s].hop)
	}
	slices.Reverse(hops)
	return markJoints(links, unloop(a, hops)), true
}

func (h Hop) next() model.ID {
	if h.Forward {
		return h.To
	}
	return h.From
}

// unloop drops any stretch that leaves a unit and comes back to it. Avoiding
// a joint can take the walk from a contract to its provider and back, which
// says no more than the joint itself, so the joint stays and is marked.
func unloop(a model.ID, hops []Hop) []Hop {
	for {
		at := map[model.ID]int{a: 0}
		cut := false
		for i, h := range hops {
			if j, seen := at[h.next()]; seen {
				hops = append(hops[:j:j], hops[i+1:]...)
				cut = true
				break
			}
			at[h.next()] = i + 1
		}
		if !cut {
			return hops
		}
	}
}

func markJoints(links []Link, hops []Hop) []Hop {
	for i := 1; i < len(hops); i++ {
		p, h := hops[i-1], hops[i]
		if !p.Forward || !uses(p.Kind) || h.Forward || !uses(h.Kind) {
			continue
		}
		hops[i].Shares = p.To
		if j := slices.IndexFunc(links, func(l Link) bool { return l.To == p.To && l.Kind == model.Provides }); j >= 0 {
			hops[i].Provider = links[j].From
		}
	}
	return hops
}

// Weakest is the origin of the path's least certain hop.
func Weakest(hops []Hop) model.Origin {
	w := model.Extracted
	for _, h := range hops {
		if strength[h.Origin] < strength[w] {
			w = h.Origin
		}
	}
	return w
}
