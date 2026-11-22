package screen

import "slices"

// Neighbor is one box of a full view with the boxes it uses and the boxes
// that use it, by name, read off every line the view stands for: those it
// draws and the pairs it counts without drawing. A capture or the text of
// the page then holds what a box's focus view shows, which stays hidden
// until a reader opens it.
type Neighbor struct {
	Name         string
	Uses, UsedBy []string
}

// neighbors lists the view's boxes in the order it draws them, each with the
// boxes it uses and those that use it, by name in name order, and leaves out
// a box no line touches. It reads parts the way the level does.
func (d *Drawing) neighbors(at where) []Neighbor {
	part := partsOf(*d, at)
	name := map[string]string{}
	for _, b := range d.Boxes {
		name[b.ID] = b.Name
	}
	uses, usedBy := map[string]map[string]bool{}, map[string]map[string]bool{}
	for _, r := range slices.Concat(d.Relations, d.Counted) {
		from, to := r[0], r[1]
		if !part[from] || !part[to] || from == to {
			continue
		}
		add(uses, from, name[to])
		add(usedBy, to, name[from])
	}
	var out []Neighbor
	for _, b := range d.Boxes {
		if !part[b.ID] || len(uses[b.ID])+len(usedBy[b.ID]) == 0 {
			continue
		}
		out = append(out, Neighbor{Name: b.Name, Uses: nameList(uses[b.ID]), UsedBy: nameList(usedBy[b.ID])})
	}
	return out
}

func nameList(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}
