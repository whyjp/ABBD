package screen

import (
	"fmt"
	"maps"
	"slices"

	"github.com/whyjp/abbd/internal/model"
)

// A shared layer most of the overview's boxes use, such as shared types or
// a logger, draws a line from each of them, and those lines say little
// beyond "most boxes use it". The overview folds those lines only for a
// common module the classification calls a utility (common.go), and then
// all of them at once: the module stands as a foundation band under the
// boxes, its last line says how many boxes use it, its tooltip names them
// and the words their relations carry, and its focus view and the common
// modules' page draw and list every line. A box whose lines carry what the
// product does, a common module that affects the logic or any other hub,
// keeps every line it has, since folding them would drop what they mean.
// The folded pairs stay in Counted, so level L, the elements' range and
// every count read them.

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

// foldBands folds every line of each box in band, the utilities the
// overview stands as foundation bands, on top of the cut c, nil for every
// link drawn. A stage of the primary flow, a box a line of the record ends
// on and a box the lead rides through stay boxes, since a reader reads
// through their lines. It is c itself when no box is left to fold.
func (o *overview) foldBands(c *cut, ambient map[model.ID]bool) *cut {
	_, users := o.overviewHubs()
	box := func(u model.ID) string {
		if ch := o.tree.chain(u); len(ch) > 0 && isModuleOf(o, ch[0]) {
			return string(ch[0])
		}
		return ""
	}
	stage := o.flow.stagesOn(box)
	ls, _, _, uses := o.links()
	lead := o.leadPairs(uses, o.walks)
	var said map[pair]string
	if c != nil {
		said = c.said
	}
	read := map[model.ID]bool{}
	for _, l := range ls {
		if lead[l] || said[l] != "" {
			read[l[0]], read[l[1]] = true, true
		}
	}
	band := map[model.ID]bool{}
	fold := map[model.ID][]model.ID{}
	bandUses := map[model.ID][]model.ID{}
	for _, m := range o.modules {
		if !ambient[m] || stage[string(m)] > 0 || read[m] {
			continue
		}
		band[m] = true
		for _, u := range slices.Sorted(maps.Keys(users[string(m)])) {
			fold[m] = append(fold[m], model.ID(u))
		}
		for _, t := range o.modules {
			if users[string(t)][string(m)] {
				bandUses[m] = append(bandUses[m], t)
			}
		}
	}
	if len(band) == 0 {
		return c
	}
	folded := map[pair]bool{}
	for _, l := range ls {
		folded[l] = band[l[0]] || band[l[1]]
	}
	keep := map[pair]bool{}
	for _, l := range ls {
		if c == nil || c.keep[l] {
			keep[l] = !folded[l]
		}
	}
	out := counting(ls, func() map[pair]bool {
		// Counting counts what is neither kept nor folded as left out.
		k := maps.Clone(keep)
		for l, f := range folded {
			if f {
				k[l] = true
			}
		}
		return k
	}())
	out.keep, out.fold, out.band, out.bandUses = keep, fold, band, bandUses
	if c != nil {
		out.said, out.static, out.aside = c.said, c.static, c.aside
	}
	return out
}

// usedByText says how many boxes use a foundation band.
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
