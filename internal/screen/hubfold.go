package screen

import (
	"fmt"
	"maps"
	"slices"

	"github.com/whyjp/abbd/internal/model"
)

// A shared layer most of the overview's boxes use, such as shared types or
// a logger, draws a line from each of them, and those lines say little
// beyond "most boxes use it". With overview_fold_hubs on, the overview folds
// the lines into each hub level L names (levels.go) into one line on the hub
// box, "N개 상자가 사용", where N is how many boxes use it. The fold is the
// drawing's alone: the folded pairs stay in Counted, so L, the elements'
// range and every count read them, the hub's tooltip names its users, and
// its focus view draws every line it has. What is lost at rest is which
// boxes do not use the hub.
//
// A pair stays drawn where the hub uses the box back, since a folded pair
// would hide that way; where the record gives the pair a word; where the
// lead a drawing of every line would find rides it; and every line of a hub
// that is a stage of the primary flow stays drawn, since the flow reads
// through it.

// overviewHubs are the hubs level L would name on the overview drawn with
// every link, read from the plan before any line is routed, and the boxes
// that use each one.
func (o *overview) overviewHubs() (map[string]bool, map[string]map[string]bool) {
	part := map[string]bool{}
	var boxes []Box
	for _, m := range o.modules {
		part[string(m)] = true
		boxes = append(boxes, Box{ID: string(m)})
	}
	users := map[string]map[string]bool{}
	note := func(from, to model.ID) {
		if !part[string(from)] || !part[string(to)] || from == to {
			return
		}
		if users[string(to)] == nil {
			users[string(to)] = map[string]bool{}
		}
		users[string(to)][string(from)] = true
	}
	for _, k := range o.pairs {
		note(k[0], k[1])
	}
	for _, e := range o.relations {
		if e.Kind == model.DependsOn {
			note(e.From, e.To)
		}
	}
	return hubs(boxes, part, users), users
}

func isModuleOf(o *overview, id model.ID) bool { return slices.Contains(o.modules, id) }

// foldHubs folds the overview's lines into its hubs on top of the cut c,
// nil for every link drawn, when hubs is on, and every line of each box
// the overview record marks ambient, which then stands as a foundation band
// under the boxes (band). It is c itself when no line folds.
func (o *overview) foldHubs(c *cut, flows []flowWalk, hubs bool, ambient map[model.ID]bool) *cut {
	hub, users := o.overviewHubs()
	if !hubs {
		hub = nil
	}
	if len(hub) == 0 && len(ambient) == 0 {
		return c
	}
	box := func(u model.ID) string {
		if ch := o.tree.chain(u); len(ch) > 0 && isModuleOf(o, ch[0]) {
			return string(ch[0])
		}
		return ""
	}
	stage := o.flow.stagesOn(box)
	ls, _, _, uses := o.links()
	lead := o.leadPairs(uses, flows)
	var said map[pair]string
	if c != nil {
		said = c.said
	}
	folded := map[pair]bool{}
	fold := map[model.ID][]model.ID{}
	usersOf := func(h model.ID) []model.ID {
		var out []model.ID
		for _, u := range slices.Sorted(maps.Keys(users[string(h)])) {
			out = append(out, model.ID(u))
		}
		return out
	}
	// A stage of the primary flow, a box a line of the record ends on and a
	// box the lead rides through are read through their lines, so each
	// stays a box whatever its mark.
	read := map[model.ID]bool{}
	for _, l := range ls {
		if lead[l] || said[l] != "" {
			read[l[0]], read[l[1]] = true, true
		}
	}
	band := map[model.ID]bool{}
	bandUses := map[model.ID][]model.ID{}
	for _, m := range o.modules {
		if ambient[m] && stage[string(m)] == 0 && !read[m] {
			band[m], fold[m] = true, usersOf(m)
			for _, t := range o.modules {
				if users[string(t)][string(m)] {
					bandUses[m] = append(bandUses[m], t)
				}
			}
		}
	}
	for _, l := range ls {
		if band[l[0]] || band[l[1]] {
			folded[l] = true
			continue
		}
		for _, ends := range [][2]model.ID{{l[0], l[1]}, {l[1], l[0]}} {
			from, h := ends[0], ends[1]
			if !hub[string(h)] || stage[string(h)] > 0 || !users[string(h)][string(from)] || users[string(from)][string(h)] || lead[l] || said[l] != "" {
				continue
			}
			folded[l] = true
			fold[h] = usersOf(h)
		}
	}
	if len(folded) == 0 && len(band) == 0 {
		return c
	}
	keep := map[pair]bool{}
	for _, l := range ls {
		if c == nil || c.keep[l] {
			keep[l] = !folded[l]
		}
	}
	out := counting(ls, func() map[pair]bool {
		// Counting counts what is neither kept nor folded on its boxes.
		k := maps.Clone(keep)
		for l := range folded {
			k[l] = true
		}
		return k
	}())
	out.keep, out.fold, out.band, out.bandUses = keep, fold, band, bandUses
	if c != nil {
		out.said = c.said
	}
	return out
}

// usedByText is what a folded hub's last line says.
func usedByText(n int) string { return fmt.Sprintf("%d개 상자가 사용", n) }

// bandGap is the room between the boxes and a foundation band, and between
// two bands.
const bandGap = 24

// bandRects are where the overview's foundation bands stand: under every
// box the layout placed and below the gap under its last row, each as wide
// as those boxes span and at least boxMax wide, one under another in the
// order of the modules.
func (o *overview) bandRects(l *layout) map[model.ID]rect {
	if len(o.band) == 0 {
		return nil
	}
	left, right, bottom := 1<<30, 0, padY
	for id, r := range l.pos {
		if isChip(id) {
			continue
		}
		left, right, bottom = min(left, r.X), max(right, r.X+r.W), max(bottom, r.Y+r.H)
	}
	if right <= left {
		left, right = padX, max(l.width, minWidth)-padX
	}
	out := map[model.ID]rect{}
	// Lines entering a bottom-row box from below take tracks in the gap
	// under the last row, which the layout's height holds.
	y := max(bottom+bandGap, l.height)
	for _, m := range o.modules {
		if o.band[m] {
			h := boxH + (len(o.names[m])-1)*nameLine
			out[m] = rect{X: left, Y: y, W: max(right-left, boxMax), H: h}
			y += h + bandGap/2
		}
	}
	return out
}

// foundationText is what a foundation band's last line says.
func foundationText(n int) string {
	if n == 0 {
		return "기반"
	}
	return "기반 · " + usedByText(n)
}
