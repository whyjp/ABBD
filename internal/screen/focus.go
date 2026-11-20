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

// A box that both uses the focus and is used by it is drawn once where its
// two lines fold into one and the view reads no worse (insideFocus). Else it
// is drawn twice: once above among its callers and once below among what
// it uses, so every line of a focus view reads down. Its second box is a
// twin, a unit of the drawing only.
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
	// The focus keeps the libraries its module uses, each with its line.
	var libraries []model.ID
	for _, e := range o.relations {
		if e.From == f && e.Kind == model.DependsOn && slices.Contains(o.libraries, e.To) {
			libraries = append(libraries, e.To)
			relations = append(relations, e)
		}
	}
	o.libraries = libraries
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

// overviewFocus derives a focus view per module of a dense overview, or of
// one that counts lines it does not draw.
func (o *overview) overviewFocus(units []model.Unit, full Drawing, hides bool) []Focus {
	all := hides || dense(full)
	var out []Focus
	for _, m := range o.modules {
		// The overview draws no library, so a module that uses one keeps its
		// focus view even where the overview is sparse: it is where the
		// library is drawn.
		if !all && !o.usesLibrary(m) {
			continue
		}
		f := planScoped(units, m, centres(full), tactic{roads: true}, nil, nil, nil, o.scope)
		f.hooks = o.hooks
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
		t := tactic{gather: full.gathered}
		ins, _ := o.inside(scope, units, id, centres(full.Drawing), t, nil)
		// Drawn again with room for its counts, where some count would get
		// it, the view keeps that if choose's checks pass it against the
		// first drawing and lint reads it no worse.
		if ins.roomy {
			ct := t
			ct.counted = true
			if alt, ok := o.inside(scope, units, id, centres(full.Drawing), ct, nil); ok {
				var c, ac cost
				c.judge(ins.Drawing)
				ac.judge(alt.Drawing)
				b := newGates()
				b.admit(tactic{}, c)
				if b.admit(ct, ac) && !c.beats(ac) {
					ins, t = alt, ct
				}
			}
		}
		t.roads = true
		if alt, ok := o.inside(scope, units, id, centres(full.Drawing), t, nil); ok {
			ins = alt
		}
		// Where two lines are a true reverse pair, the view is drawn again
		// with each pair folded into one line, kept where that reads no worse.
		for _, tier := range []int{foldAll, foldLone} {
			if !foldsAny(ins.Drawing) {
				break
			}
			ft := t
			ft.pairs = tier
			if folded, ok := o.inside(scope, units, id, centres(full.Drawing), ft, nil); ok {
				var c, fc cost
				c.judge(ins.Drawing)
				fc.judge(folded.Drawing)
				if foldsNoWorse(fc, c) {
					ins, t = folded, ft
					break
				}
			}
		}
		// Where the view draws a box twice that makes a true reverse pair
		// with the focus, it is drawn again with that box once and its pair
		// folded into one line: with each such box whose only lines are that
		// pair standing beside the focus, then without; then each of those
		// with the folded lines of one hub sharing its roads (tactic.fan);
		// then all four with such boxes seated in the ring row next to the
		// focus where that row wraps (tactic.near). The first that folds
		// every such pair and reads no worse is kept. Where none does, as
		// about a hub that many boxes use both ways, the box stays drawn
		// twice: a known debt until opposite lines at a hub keep two lanes.
		if hasTwin(ins.Drawing) {
			var c cost
			c.judge(ins.Drawing)
			pt := t
			pt.paired, pt.pairs = true, foldAll
			bt := pt
			bt.beside = true
			bf, pf := bt, pt
			bf.fan, pf.fan = true, true
			var near []tactic
			for _, at := range []tactic{bt, pt, bf, pf} {
				at.near = true
				near = append(near, at)
			}
			for _, at := range append([]tactic{bt, pt, bf, pf}, near...) {
				if alt, ok := o.inside(scope, units, id, centres(full.Drawing), at, nil); ok && foldsAll(alt.Drawing) {
					var ac cost
					ac.judge(alt.Drawing)
					if foldsNoWorse(ac, c) {
						ins, t = alt, at
						break
					}
				}
			}
		}
		// Where some line climbs, the view is drawn again with such lines on
		// roads of their own, and that is kept where sharing roads reads worse.
		if slices.ContainsFunc(ins.Drawing.Routes, func(r Route) bool { return r.Back }) {
			at := t
			at.apart = true
			if apart, ok := o.inside(scope, units, id, centres(full.Drawing), at, nil); ok {
				var c, ac cost
				c.judge(ins.Drawing)
				ac.judge(apart.Drawing)
				if !sharedOrApart(c, ac) {
					ins = apart
				}
			}
		}
		d := ins.Drawing
		out = append(out, o.focus(scope, "#m-"+string(scope), full.Name+" 내부", id, d))
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

// usesLibrary says the module depends on an external library.
func (o *overview) usesLibrary(m model.ID) bool {
	return slices.ContainsFunc(o.relations, func(e model.Edge) bool {
		return e.From == m && e.Kind == model.DependsOn && o.units[e.To].Kind == model.External
	})
}

// hasTwin says d draws some box twice.
func hasTwin(d Drawing) bool {
	return slices.ContainsFunc(d.Boxes, func(b Box) bool { return strings.HasSuffix(b.ID, twinMark) })
}

// foldsAll says d draws every two routes that join two boxes opposite ways
// by one kind as one line with a head at each end.
func foldsAll(d Drawing) bool {
	box := map[string]bool{}
	for _, b := range d.Boxes {
		box[b.ID] = true
	}
	way := map[[3]string]bool{}
	for _, r := range d.Routes {
		if len(r.ends) == 2 && box[r.ends[0]] && box[r.ends[1]] {
			way[[3]string{r.ends[0], r.ends[1], r.Kind}] = true
		}
	}
	for _, r := range d.Routes {
		if len(r.ends) == 2 && box[r.ends[0]] && box[r.ends[1]] && way[[3]string{r.ends[1], r.ends[0], r.Kind}] && !r.Both {
			return false
		}
	}
	return true
}
