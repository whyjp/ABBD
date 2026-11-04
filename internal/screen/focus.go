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
// partial graph of the full one, cut from the full view's own lines: it
// stores nothing, and the full view stays. Every line of the full
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
// drawing only.
const twinMark = "↓"

const twinNote = "서로 쓰는 상자라 위와 아래에 한 번씩 그렸습니다"

func twin(id model.ID) model.ID { return id + twinMark }

// base is the unit a box stands for, a twin's included.
func base(id model.ID) model.ID { return model.ID(strings.TrimSuffix(string(id), twinMark)) }

// around cuts the planned overview down to the focus view of f: f, the
// lines of the overview that have f at one end, and the boxes at their other
// ends. It filters the overview's own lines rather than finding them again,
// so the focus view is a partial graph of the overview. A module on both
// sides of f gets a twin below.
func (o *overview) around(f model.ID) {
	callers := map[model.ID]bool{}
	for _, k := range o.pairs {
		if k[1] == f {
			callers[k[0]] = true
		}
	}
	isModule := func(id model.ID) bool { return slices.Contains(o.modules, id) }
	direct := func(e model.Edge) bool { return e.Kind == model.DependsOn && isModule(e.From) && isModule(e.To) }
	for _, e := range o.relations {
		if direct(e) && e.To == f {
			callers[e.From] = true
		}
	}
	below := func(id model.ID) model.ID {
		if callers[id] {
			return o.addTwin(id)
		}
		return id
	}
	ring := map[model.ID]bool{f: true}
	var pairs [][2]model.ID
	carry := map[[2]model.ID][]model.ID{}
	for _, k := range o.pairs {
		if k[0] != f && k[1] != f {
			continue
		}
		cut := k
		if k[0] == f && isModule(k[1]) {
			cut[1] = below(k[1])
		}
		pairs, carry[cut] = append(pairs, cut), o.carry[k]
		ring[cut[0]], ring[cut[1]] = true, true
	}
	var relations []model.Edge
	for _, e := range o.relations {
		if !direct(e) || e.From != f && e.To != f {
			continue
		}
		cut := e
		if e.From == f {
			cut.To = below(e.To)
		}
		o.rels[cut.String()] = o.rels[e.String()]
		relations = append(relations, cut)
		ring[cut.From], ring[cut.To] = true, true
	}
	o.pairs, o.carry, o.relations = pairs, carry, relations
	o.unreached = map[model.ID][]model.ID{f: o.unreached[f]}
	var modules, contracts []model.ID
	for id := range ring {
		if o.units[id].Kind.IsContract() {
			contracts = append(contracts, id)
		} else {
			modules = append(modules, id)
		}
	}
	slices.Sort(modules)
	slices.Sort(contracts)
	o.modules, o.contracts = modules, contracts
}

// addTwin adds the twin of id, which stands for id's stored relations. It
// reads the overview's lines, so around calls it before cutting them.
func (o *overview) addTwin(id model.ID) model.ID {
	u := o.units[id]
	u.ID = twin(id)
	if _, ok := o.units[u.ID]; !ok {
		o.units[u.ID], o.tree.units[u.ID] = u, u
		for _, e := range o.relations {
			if e.From == id {
				t := e
				t.From = u.ID
				o.rels[t.String()] = o.rels[e.String()]
			}
		}
	}
	return u.ID
}

// overviewFocus derives a focus view per module of a dense overview.
func (o *overview) overviewFocus(units []model.Unit, full Drawing) []Focus {
	if !dense(full) {
		return nil
	}
	var out []Focus
	for _, m := range o.modules {
		f := planOverview(units, m, centres(full), tactic{})
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
		d := o.inside(scope, units, id, centres(full.Drawing), tactic{}).Drawing
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

// centres gives the x of every box's centre in a full view, a twin's
// included as its unit's.
func centres(d Drawing) map[string]int {
	at := map[string]int{}
	for _, b := range d.Boxes {
		at[b.ID] = b.X + b.W/2
		at[string(twin(model.ID(b.ID)))] = b.X + b.W/2
	}
	return at
}

// seatOf is how a focus view on focus keeps its full view's map, nil for a
// full view.
func seatOf(focus model.ID, at map[string]int) *focusSeat {
	if focus == "" {
		return nil
	}
	return &focusSeat{focus: string(focus), at: at}
}

// linkFocus gives a copy of a full view whose boxes that open nothing else
// open their focus views.
func linkFocus(d Drawing, fs []Focus) Drawing {
	d.Boxes = slices.Clone(d.Boxes)
	for i, b := range d.Boxes {
		if j := slices.IndexFunc(fs, func(f Focus) bool { return f.Unit == b.ID }); j >= 0 && b.Href == "" {
			d.Boxes[i].Href = "#" + fs[j].ID
		}
	}
	return d
}
