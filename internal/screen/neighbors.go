package screen

import "slices"

// Neighbor is one box of a full view with the boxes it uses and the boxes
// that use it, read off every line the view stands for: those it draws and
// the pairs it counts without drawing. A capture or the text of the page then
// holds the neighbors a box's focus view shows, which stays hidden until a
// reader opens it. Boxes outside the view, the libraries and contracts the
// view draws loose, are left to the focus view.
type Neighbor struct {
	Near
	Uses, UsedBy []Near
}

// Near is a box by name, linked to its focus view where the view has one, and
// where a relation between it and the box it is listed under has a phrase that
// names the symbols carrying it, those (symbols.go).
type Near struct {
	Name, Href string
	Carries    []Carry `json:",omitempty"`
}

// neighbors lists the view's boxes in the order it draws them, each with the
// boxes it uses and those that use it in name order, and leaves out a box no
// line touches. It reads parts the way the level does. href gives the focus
// view of a box, by its ID, where there is one.
func (d *Drawing) neighbors(at where, href map[string]string, carries map[[2]string][]Carry) []Neighbor {
	part := partsOf(*d, at)
	ref := map[string]Near{}
	for _, b := range d.Boxes {
		name := b.Name
		if name == "" {
			name = b.ID
		}
		ref[b.ID] = Near{Name: name, Href: href[b.ID]}
	}
	// A box that provides a contract is no user of it, so a provides line
	// joins no neighbors.
	provides := map[[2]string]bool{}
	for _, w := range d.ways {
		if w.Kind == kindProvides && len(w.ends) == 2 {
			provides[[2]string{w.ends[0], w.ends[1]}] = true
			provides[[2]string{w.ends[1], w.ends[0]}] = true
		}
	}
	uses, usedBy := map[string]map[string]bool{}, map[string]map[string]bool{}
	for _, r := range slices.Concat(d.Relations, d.Counted) {
		from, to := r[0], r[1]
		if !part[from] || !part[to] || from == to || provides[r] {
			continue
		}
		add(uses, from, to)
		add(usedBy, to, from)
	}
	// list is the boxes ids, each with the symbols of the relations between it
	// and box at, the user first when users is true.
	list := func(at string, ids map[string]bool, users bool) []Near {
		out := make([]Near, 0, len(ids))
		for id := range ids {
			n := ref[id]
			if users {
				n.Carries = carries[[2]string{id, at}]
			} else {
				n.Carries = carries[[2]string{at, id}]
			}
			out = append(out, n)
		}
		slices.SortFunc(out, func(a, b Near) int {
			if a.Name != b.Name {
				return cmpString(a.Name, b.Name)
			}
			return cmpString(a.Href, b.Href)
		})
		return out
	}
	var out []Neighbor
	for _, b := range d.Boxes {
		if !part[b.ID] || len(uses[b.ID])+len(usedBy[b.ID]) == 0 {
			continue
		}
		out = append(out, Neighbor{Near: ref[b.ID], Uses: list(b.ID, uses[b.ID], false), UsedBy: list(b.ID, usedBy[b.ID], true)})
	}
	return out
}

func cmpString(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}
