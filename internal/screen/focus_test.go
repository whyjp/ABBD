package screen

import (
	"bytes"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/whyjp/abbd/internal/model"
)

// rels are the stored relations a drawing's lines and chips stand for.
func rels(d Drawing) map[string]bool {
	out := map[string]bool{}
	for _, w := range d.Wires {
		for _, r := range w.Rels {
			out[r] = true
		}
	}
	for _, p := range d.Ports {
		for _, r := range p.Rels {
			out[r] = true
		}
	}
	return out
}

// A dense view is read through its focus views, so together they draw every
// relation the full view draws, and each unit box of the full view has one.
func TestFocusViewsDrawEveryRelationOfTheirView(t *testing.T) {
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
			got := map[string]bool{}
			for _, f := range v.focus {
				for r := range rels(f.Drawing) {
					got[r] = true
				}
			}
			for r := range rels(v.d) {
				if !got[r] {
					t.Errorf("%s: %s is in no focus view", v.name, r)
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
