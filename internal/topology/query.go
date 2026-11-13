package topology

import "github.com/whyjp/abbd/internal/model"

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
	for _, u := range model.Present(units) {
		switch {
		case u.ID == id:
			ex.Unit = u
		case u.Parent == id:
			ex.Inside = append(ex.Inside, ref(u))
		}
	}
	ps := newParents(units)
	own := func(x model.ID) bool { return ps.within(x, id) }
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
