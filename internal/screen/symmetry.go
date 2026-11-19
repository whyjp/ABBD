package screen

import (
	"cmp"
	"slices"
)

// A reader recalls a drawing better when what belongs together lines up:
// boxes of one row centred on one line, and a box standing over the middle
// of the boxes it uses in the row below (Marriott et al. 2012, Purchase et
// al. 2010). lint reads both on each drawing, as diagnostics.

// symmetry is what lint reads of a drawing's alignment.
type symmetry struct {
	// rowsOff is the rows of two boxes or more whose centres do not stand
	// on one horizontal line.
	rowsOff int
	// parents is the boxes that use two boxes or more of the row right
	// below, and offCentre those of them whose centre stands off the middle
	// of those boxes by more than a quarter of the span their centres cover,
	// so the measure scales with each parent's own span.
	parents, offCentre int
}

func symmetryOf(d Drawing) symmetry {
	var s symmetry
	row, rows := drawnRows(d.Boxes)
	at := map[string]Box{}
	byRow := make([][]Box, rows)
	for _, b := range d.Boxes {
		at[b.ID] = b
		byRow[row[b.ID]] = append(byRow[row[b.ID]], b)
	}
	for _, r := range byRow {
		for _, b := range r[min(1, len(r)):] {
			if 2*b.Y+b.H != 2*r[0].Y+r[0].H {
				s.rowsOff++
				break
			}
		}
	}
	kids := map[string][]int{}
	for _, u := range d.uses() {
		p, ok1 := at[u.from]
		c, ok2 := at[u.to]
		if ok1 && ok2 && row[c.ID] == row[p.ID]+1 && !slices.Contains(kids[p.ID], 2*c.X+c.W) {
			kids[p.ID] = append(kids[p.ID], 2*c.X+c.W)
		}
	}
	ids := make([]string, 0, len(kids))
	for id := range kids {
		ids = append(ids, id)
	}
	slices.SortFunc(ids, cmp.Compare)
	for _, id := range ids {
		cs := kids[id]
		if len(cs) < 2 {
			continue
		}
		s.parents++
		lo, hi := slices.Min(cs), slices.Max(cs)
		p := at[id]
		if off := abs(2*p.X + p.W - (lo+hi)/2); 4*off > hi-lo {
			s.offCentre++
		}
	}
	return s
}
