package topology

import (
	"slices"

	"github.com/whyjp/abbd/internal/model"
)

// Explanation is one unit at a glance. Out and In are the drawn links, the
// ones the script read or a human stated, that leave and reach the unit. They
// also hold the drawn links between a unit inside it and a unit outside it,
// counted as the unit's own the way Compare counts them, so Out's From or
// In's To is then the inside unit. Inferred are a reader's live couplings
// that touch the unit or a unit inside it.
type Explanation struct {
	Unit     model.Unit
	Inside   []Ref
	Out, In  []Link
	Inferred []Link
}

// Explain gathers what the topology holds about one present unit.
func Explain(units []model.Unit, id model.ID) Explanation {
	ex := Explanation{}
	parent := map[model.ID]model.ID{}
	for _, u := range model.Present(units) {
		parent[u.ID] = u.Parent
		switch {
		case u.ID == id:
			ex.Unit = u
		case u.Parent == id:
			ex.Inside = append(ex.Inside, ref(u))
		}
	}
	own := func(x model.ID) bool {
		for seen := map[model.ID]bool{}; x != "" && !seen[x]; x = parent[x] {
			if x == id {
				return true
			}
			seen[x] = true
		}
		return false
	}
	for _, l := range Links(units) {
		switch {
		case l.Origin == model.Inferred:
			if own(l.From) || own(l.To) {
				ex.Inferred = append(ex.Inferred, l)
			}
		case l.From == id, own(l.From) && !own(l.To):
			ex.Out = append(ex.Out, l)
		case l.To == id, own(l.To) && !own(l.From):
			ex.In = append(ex.In, l)
		}
	}
	return ex
}

// strength orders origins by what vouches for them: the code read by the
// script, a human who said so without the code showing it, then a reader's
// guess that rests on a quote.
var strength = map[model.Origin]int{model.Extracted: 2, model.UserStated: 1, model.Inferred: 0}

// Hop is one link of a path. Forward says the path walks it from From to To;
// otherwise the path walks it backwards, from To to From.
type Hop struct {
	Link
	Forward bool
}

// Path is the shortest walk from a to b over the live links, each walked in
// either direction. Among the shortest walks it takes the one whose weakest
// link is strongest, so a guess shows only where nothing firmer connects the
// two. ok is false when no walk exists.
func Path(units []model.Unit, a, b model.ID) (hops []Hop, ok bool) {
	links := Links(units)
	dist := map[model.ID]int{a: 0}
	best := map[model.ID]int{a: strength[model.Extracted] + 1}
	via := map[model.ID]Hop{}
	queue := []model.ID{a}
	for len(queue) > 0 {
		x := queue[0]
		queue = queue[1:]
		for _, l := range links {
			var y model.ID
			switch x {
			case l.From:
				y = l.To
			case l.To:
				y = l.From
			default:
				continue
			}
			d, seen := dist[y]
			if seen && d != dist[x]+1 {
				continue
			}
			w := min(best[x], strength[l.Origin])
			if seen && w <= best[y] {
				continue
			}
			if !seen {
				dist[y] = dist[x] + 1
				queue = append(queue, y)
			}
			best[y], via[y] = w, Hop{Link: l, Forward: l.From == x}
		}
	}
	if _, ok := dist[b]; !ok {
		return nil, false
	}
	for at := b; at != a; {
		h := via[at]
		hops = append(hops, h)
		if h.Forward {
			at = h.From
		} else {
			at = h.To
		}
	}
	slices.Reverse(hops)
	return hops, true
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
