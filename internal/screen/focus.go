package screen

import (
	"slices"
	"strings"

	"github.com/whyjp/abbd/internal/model"
)

// A dense view, one whose boxes use each other many times over, stays a
// tangle however it is laid out. So each of its boxes also gets a focus
// view: that box, the boxes it meets as a ring around it, and only the lines
// it has, laid out on their own. Callers sit above it and what it uses below.
// Its lines out of the view's scope stay on the frame, lifted to the largest
// unit that holds their end, as in the full view. The focus view is a
// partial graph of the full one, cut from the stored graph by the same
// rules: it stores nothing, and the full view stays. Every line of the full
// view has a box at one end, so it is drawn in that box's focus view too.

// Focus is one focus view: Unit and its ring, cut from the view at Of.
type Focus struct {
	// ID is the page anchor.
	ID, Unit, Name string
	// Of is the anchor of the full view it is cut from, OfName that view's
	// title, and In the focused unit's own inside, when it has one.
	Of, OfName, In string
	Drawing        Drawing
	// Ring counts the boxes it meets.
	Ring int
}

// dense says a view draws more than two lines per box on average: enough
// that a reader cannot follow one box's lines through the others'.
func dense(d Drawing) bool { return len(d.Boxes) > 2 && len(d.Relations) > 2*len(d.Boxes) }

// focusID is the anchor of unit's focus view in the view of scope, the
// overview when scope is empty.
func focusID(scope, unit model.ID) string {
	if scope == "" {
		scope = "overview"
	}
	return "f-" + string(scope) + "--" + string(unit)
}

// A box that both uses the focus and is used by it is drawn twice: once
// above among its callers and once below among what it uses, so every
// line of a focus view reads down. Its second box is a twin, a unit of the
// drawing only, as a folded cycle is.
const twinMark = "↓"

const twinNote = "서로 쓰는 상자라 위와 아래에 한 번씩 그렸습니다"

func twin(id model.ID) model.ID { return id + twinMark }

// base is the unit a box stands for, a twin's included.
func base(id model.ID) model.ID { return model.ID(strings.TrimSuffix(string(id), twinMark)) }

// around cuts the overview down to the focus view of f: f, the edges at f,
// and through a contract f provides or uses, the modules on the contract's
// other side. A module on both sides of f gets a twin below.
func (o *overview) around(f model.ID) {
	isContract := func(id model.ID) bool { return o.units[id].Kind.IsContract() }
	provides := map[model.ID]bool{}
	for _, e := range o.relations {
		if e.From == f && isContract(e.To) {
			provides[e.To] = provides[e.To] || e.Kind == model.Provides
		}
	}
	var kept []model.Edge
	for _, e := range o.relations {
		mine, ok := provides[e.To]
		if e.From == f || e.To == f || ok && (e.Kind == model.Provides) != mine {
			kept = append(kept, e)
		}
	}
	callers := map[model.ID]bool{}
	for _, e := range kept {
		if e.To == f || e.From != f && provides[e.To] {
			callers[e.From] = true
		}
	}
	ids := []model.ID{f}
	for i, e := range kept {
		switch {
		case e.From == f && !isContract(e.To) && callers[e.To]:
			kept[i].To = o.addTwin(e.To)
		case e.From != f && e.Kind == model.Provides && !provides[e.To] && callers[e.From]:
			kept[i].From = o.addTwin(e.From)
		}
		o.rels[kept[i].String()] = o.rels[e.String()]
		for _, id := range []model.ID{kept[i].From, kept[i].To} {
			if !slices.Contains(ids, id) {
				ids = append(ids, id)
			}
		}
	}
	slices.Sort(ids)
	o.design, o.relations = ids, kept
}

func (o *overview) addTwin(id model.ID) model.ID {
	u := o.units[id]
	u.ID = twin(id)
	o.units[u.ID], o.tree.units[u.ID] = u, u
	return u.ID
}

// overviewFocus derives a focus view per module of a dense overview.
func (o *overview) overviewFocus(units []model.Unit, full Drawing) []Focus {
	if !dense(full) {
		return nil
	}
	var out []Focus
	for _, m := range o.modules {
		f := planOverview(units, m)
		d := f.draw(func(id model.ID) string {
			if id == m {
				return viewOf(o.tree, id)
			}
			return "#" + focusID("", base(id))
		})
		out = append(out, o.focus("", "#overview", "모듈과 계약", m, d))
	}
	return out
}

// insideFocus derives a focus view per box of a dense inside.
func (o *overview) insideFocus(scope model.ID, units []model.Unit, full Inside) []Focus {
	if !dense(full.Drawing) {
		return nil
	}
	var out []Focus
	for _, b := range full.Drawing.Boxes {
		id := model.ID(b.ID)
		d := o.inside(scope, units, id).Drawing
		out = append(out, o.focus(scope, "#m-"+string(scope), full.Name+"의 안", id, d))
	}
	return out
}

func (o *overview) focus(scope model.ID, of, ofName string, unit model.ID, d Drawing) Focus {
	for i := range d.Boxes {
		if d.Boxes[i].ID == string(unit) {
			d.Boxes[i].Class += " focus"
		}
	}
	return Focus{
		ID: focusID(scope, unit), Unit: string(unit), Name: o.name(unit),
		Of: of, OfName: ofName, In: viewOf(o.tree, unit), Drawing: d, Ring: len(d.Boxes) - 1,
	}
}

// linkFocus makes each box of a full view that opens nothing else open its
// focus view.
func linkFocus(d *Drawing, fs []Focus) {
	for i, b := range d.Boxes {
		if j := slices.IndexFunc(fs, func(f Focus) bool { return f.Unit == b.ID }); j >= 0 && b.Href == "" {
			d.Boxes[i].Href = "#" + fs[j].ID
		}
	}
}
