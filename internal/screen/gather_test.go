package screen

import (
	"strings"
	"testing"

	"github.com/whyjp/abbd/internal/model"
)

// Modules outside a part that each use several of its boxes with no
// contract are gathered onto one chip per box, so each box takes one line
// from the frame and no line crosses the page to reach it.
func TestOutsideUsersGatherOntoOneChipPerBox(t *testing.T) {
	var units []model.Unit
	boxes := []model.ID{"MOD-0011", "MOD-0012", "MOD-0013"}
	for _, b := range boxes {
		units = append(units, inside(b, "MOD-0010"))
	}
	units = append(units, called("MOD-0010", "core"))
	for _, u := range []model.ID{"MOD-0001", "MOD-0002", "MOD-0003", "MOD-0004"} {
		var rels []model.Relation
		for _, b := range boxes {
			rels = append(rels, rel(b, model.DependsOn))
		}
		units = append(units, called(u, "user "+string(u), rels...))
	}
	m := build(units)
	var core *Inside
	for i := range m.Insides {
		if m.Insides[i].ID == "MOD-0010" {
			core = &m.Insides[i]
		}
	}
	if core == nil {
		t.Fatal("core has no inside view")
	}
	var gathered int
	for _, p := range core.Drawing.Ports {
		if strings.HasPrefix(strings.Join(p.Keys, " "), "port|"+gatherPort) {
			gathered++
			if !strings.Contains(p.Title, "user MOD-0001") || !strings.Contains(p.Title, "user MOD-0004") {
				t.Errorf("a gathered chip names every module it stands for, got %q", p.Title)
			}
		}
	}
	if gathered != len(boxes) {
		t.Fatalf("want one gathered chip per box, %d, got %d of %d chips", len(boxes), gathered, len(core.Drawing.Ports))
	}
	vl, err := lintView(view{d: core.Drawing, budget: insideWidth})
	if err != nil {
		t.Fatal(err)
	}
	if vl.Totals.Crossings != 0 || vl.Totals.Hard != 0 {
		t.Fatalf("crossings %d, hard %d, want none", vl.Totals.Crossings, vl.Totals.Hard)
	}
}
