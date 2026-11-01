package screen

import (
	"bytes"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/whyjp/abbd/internal/model"
)

// marks are what a drawing draws, each kind of mark on its own: the pairs its
// lines join (user to provider on the overview, from to on an inside), the
// stored relations its lines stand for, those its chips stand for, and its
// boxes. A twin counts as the unit it stands for.
type marks struct{ pairs, wires, chips, boxes map[string]bool }

func drawnIn(d Drawing) marks {
	b := func(s string) string { return string(base(model.ID(s))) }
	out := marks{map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}}
	for _, r := range d.Relations {
		out.pairs[b(r[0])+" -> "+b(r[1])] = true
	}
	for _, w := range d.Wires {
		for _, r := range w.Rels {
			out.wires[r] = true
		}
	}
	for _, p := range d.Ports {
		for _, r := range p.Rels {
			out.chips[r] = true
		}
	}
	for _, x := range d.Boxes {
		out.boxes[b(x.ID)] = true
	}
	return out
}

// A dense view is read through its focus views: each focus view is a partial
// graph of its view, drawing no line, chip or box the view does not, and
// together they draw every line and chip of the view. Each unit box of the
// view has one.
func TestFocusViewsArePartialGraphsOfTheirView(t *testing.T) {
	type full struct {
		name  string
		d     Drawing
		focus []Focus
	}
	seen := 0
	for _, name := range fixtures {
		m := fixture(t, name)
		all := []full{{name + " overview", m.Overview, m.Focus}}
		for _, ins := range m.Insides {
			all = append(all, full{name + " " + ins.ID, ins.Drawing, ins.Focus})
		}
		for _, v := range all {
			if len(v.focus) == 0 {
				if dense(v.d) {
					t.Errorf("%s is dense and has no focus views", v.name)
				}
				continue
			}
			seen++
			whole := drawnIn(v.d)
			union := marks{map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}}
			for _, f := range v.focus {
				part := drawnIn(f.Drawing)
				for _, c := range []struct {
					what       string
					part, w, u map[string]bool
				}{
					{"line", part.pairs, whole.pairs, union.pairs},
					{"wire relation", part.wires, whole.wires, union.wires},
					{"chip relation", part.chips, whole.chips, union.chips},
					{"box", part.boxes, whole.boxes, union.boxes},
				} {
					for k := range c.part {
						c.u[k] = true
						if !c.w[k] {
							t.Errorf("%s: %s draws %s %s, which the view does not", v.name, f.ID, c.what, k)
						}
					}
				}
			}
			for _, c := range []struct {
				what string
				w, u map[string]bool
			}{{"line", whole.pairs, union.pairs}, {"wire relation", whole.wires, union.wires}, {"chip relation", whole.chips, union.chips}} {
				for k := range c.w {
					if !c.u[k] {
						t.Errorf("%s: %s %s is in no focus view", v.name, c.what, k)
					}
				}
			}
			// A contract no module provides is drawn as a box of its own; its
			// lines are its users', in their focus views.
			for _, b := range v.d.Boxes {
				if !strings.Contains(b.Class, "loose") && !slices.ContainsFunc(v.focus, func(f Focus) bool { return f.Unit == b.ID }) {
					t.Errorf("%s: box %s has no focus view", v.name, b.ID)
				}
			}
		}
	}
	if seen == 0 {
		t.Fatal("no fixture view is dense")
	}
}

// A unit that both uses the focus and is used by it is drawn twice, above
// and below, so no line of the pair climbs; the twin opens the unit's own
// focus view.
func TestAUnitOnBothSidesOfTheFocusIsDrawnTwice(t *testing.T) {
	m := fixture(t, "pstack")
	for _, f := range m.Focus {
		var twins []Box
		for _, b := range f.Drawing.Boxes {
			if strings.HasSuffix(b.ID, twinMark) {
				twins = append(twins, b)
			}
		}
		for _, tw := range twins {
			i := slices.IndexFunc(f.Drawing.Boxes, func(b Box) bool { return b.ID == strings.TrimSuffix(tw.ID, twinMark) })
			if i < 0 {
				t.Fatalf("%s: twin %s without its unit", f.ID, tw.ID)
			}
			if f.Drawing.Boxes[i].Y >= tw.Y {
				t.Errorf("%s: %s is not drawn above its twin", f.ID, tw.ID)
			}
			if tw.Href != "#"+focusID("", base(model.ID(tw.ID))) {
				t.Errorf("%s: twin %s opens %q", f.ID, tw.ID, tw.Href)
			}
		}
		if len(twins) > 0 {
			return
		}
	}
	t.Fatal("no focus view of pstack's overview has a twin")
}

// Focus views are part of the page, and the page is the same bytes for the
// same data.
func TestFocusViewsAreDeterministic(t *testing.T) {
	page := func() []byte {
		var b bytes.Buffer
		if err := Render(&b, fixture(t, "orca")); err != nil {
			t.Fatal(err)
		}
		return b.Bytes()
	}
	first := page()
	if !bytes.Contains(first, []byte(`id="f-MOD-0042--MOD-0052"`)) {
		t.Fatal("the page has no focus view of desktop-main/ipc")
	}
	for i := 0; i < 3; i++ {
		if !bytes.Equal(page(), first) {
			t.Fatal("rendering the same data twice gave two pages")
		}
	}
}

// Every link on the page opens a section the page holds.
func TestEveryLinkOpensASection(t *testing.T) {
	href, id := regexp.MustCompile(`href="#([^"]+)"`), regexp.MustCompile(` id="([^"]+)"`)
	for _, name := range fixtures {
		var b bytes.Buffer
		if err := Render(&b, fixture(t, name)); err != nil {
			t.Fatal(err)
		}
		ids := map[string]bool{}
		for _, m := range id.FindAllStringSubmatch(b.String(), -1) {
			ids[m[1]] = true
		}
		broken := 0
		for _, m := range href.FindAllStringSubmatch(b.String(), -1) {
			if !ids[m[1]] {
				broken++
				if broken <= 3 {
					t.Errorf("%s: a link opens #%s, which the page does not hold", name, m[1])
				}
			}
		}
	}
}

// A focus view keeps the reader's map of its view: the focus stands at the
// middle of the drawing, and the boxes of each row stand left to right in
// the order they stand in the view.
func TestFocusViewsKeepTheMapOfTheirView(t *testing.T) {
	for _, name := range fixtures {
		m := fixture(t, name)
		all := map[string][]Focus{"overview": m.Focus}
		full := map[string]Drawing{"overview": m.Overview}
		for _, ins := range m.Insides {
			all[ins.ID], full[ins.ID] = ins.Focus, ins.Drawing
		}
		for v, fs := range all {
			at := centres(full[v])
			for _, f := range fs {
				rows := map[int][]Box{}
				for _, b := range f.Drawing.Boxes {
					if b.ID == f.Unit {
						if mid := b.X + b.W/2; abs(2*mid-f.Drawing.Width) > 4 {
							t.Errorf("%s %s: the focus stands at %d in a drawing %d wide", name, f.ID, mid, f.Drawing.Width)
						}
					}
					rows[b.Y] = append(rows[b.Y], b)
				}
				for _, row := range rows {
					slices.SortFunc(row, func(a, b Box) int { return a.X - b.X })
					for i := 1; i < len(row); i++ {
						if at[row[i-1].ID] > at[row[i].ID] {
							t.Errorf("%s %s: %s stands left of %s, which the view has the other way", name, f.ID, row[i-1].ID, row[i].ID)
						}
					}
				}
			}
		}
	}
}
