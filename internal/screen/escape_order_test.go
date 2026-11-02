package screen

import (
	"strings"
	"testing"

	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/store"
)

// The same units in another order, with each unit's relations reversed, are
// the same data and should draw the same picture.
func TestUnitOrderDoesNotMoveTheDrawing(t *testing.T) {
	units := ripgrep()
	first := build(units)
	rev := make([]model.Unit, len(units))
	for i, u := range units {
		rs := append([]model.Relation(nil), u.Relations...)
		for a, b := 0, len(rs)-1; a < b; a, b = a+1, b-1 {
			rs[a], rs[b] = rs[b], rs[a]
		}
		u.Relations = rs
		rev[len(units)-1-i] = u
	}
	again := build(rev)
	if svg(t, again.Overview) != svg(t, first.Overview) {
		t.Errorf("reordering the units changes the overview SVG")
	}
	for j := range again.Insides {
		if j < len(first.Insides) && svg(t, again.Insides[j].Drawing) != svg(t, first.Insides[j].Drawing) {
			t.Errorf("reordering the units changes the inside of %s", again.Insides[j].ID)
		}
	}
}

// A crafted understanding file or unit name must reach the page as text.
func TestUnderstandingAndNamesAreEscaped(t *testing.T) {
	units := ripgrep()
	units[0].Name.Value = `<img src=x onerror=alert(1)>`
	m := Build(Input{Units: units, Understandings: map[model.ID]store.Understanding{
		"MOD-0001": {Unit: "MOD-0001", Text: "# <script>alert(3)</script>\n- **<b onmouseover=x>** `</code><script>alert(4)</script>`\n[x](javascript:alert(5))"},
	}})
	b := renderAll(t, m)
	html := b.String()
	if !strings.Contains(html, "&lt;script&gt;alert(3)") || !strings.Contains(html, "&lt;img src=x") {
		t.Fatalf("precondition: the crafted text and name should reach the page escaped")
	}
	for _, bad := range []string{"<img src=x", "<script>alert", "<b onmouseover", `href="javascript:`} {
		if strings.Contains(html, bad) {
			t.Errorf("page carries unescaped %q", bad)
		}
	}
}
