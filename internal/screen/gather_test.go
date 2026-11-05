package screen

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/whyjp/abbd/internal/model"
)

// part builds a part MOD-0010 with boxes and outside users, where meets
// says which boxes each user depends on, and returns its inside view.
func part(t *testing.T, boxes []model.ID, meets map[model.ID][]model.ID) Inside {
	t.Helper()
	var units []model.Unit
	for _, b := range boxes {
		units = append(units, inside(b, "MOD-0010"))
	}
	units = append(units, called("MOD-0010", "core"))
	for u, bs := range meets {
		var rels []model.Relation
		for _, b := range bs {
			rels = append(rels, rel(b, model.DependsOn))
		}
		units = append(units, called(u, "user "+string(u), rels...))
	}
	m := build(units)
	for _, ins := range m.Insides {
		if ins.ID == "MOD-0010" {
			return ins
		}
	}
	t.Fatal("core has no inside view")
	return Inside{}
}

// Modules outside a part that each use several of its boxes with no
// contract are gathered onto one chip per box, so each box takes one line
// from the frame and no line crosses the page to reach it. Every relation
// is still drawn, and each line's tooltip names every relation it stands for.
func TestOutsideUsersGatherOntoOneChipPerBox(t *testing.T) {
	boxes := []model.ID{"MOD-0011", "MOD-0012", "MOD-0013"}
	users := []model.ID{"MOD-0001", "MOD-0002", "MOD-0003", "MOD-0004"}
	meets := map[model.ID][]model.ID{}
	for _, u := range users {
		meets[u] = boxes
	}
	core := part(t, boxes, meets)
	d := core.Drawing

	var others int
	for _, p := range d.Ports {
		if p.Class != "other" {
			continue
		}
		others++
		if !strings.HasPrefix(p.Label, "user MOD-0001") {
			t.Errorf("a gathered chip's label starts with its first user, got %q", p.Label)
		}
		for _, u := range users {
			if !strings.Contains(p.Title, "user "+string(u)) {
				t.Errorf("a gathered chip's tooltip names %s, got %q", u, p.Title)
			}
		}
	}
	if others != len(boxes) {
		t.Fatalf("want one outside chip per box, %d, got %d of %d chips", len(boxes), others, len(d.Ports))
	}

	rels := 0
	var titles []string
	for _, w := range d.Wires {
		rels += len(w.Rels)
		titles = append(titles, w.Title)
	}
	if want := len(users) * len(boxes); rels != want {
		t.Errorf("the wires stand for %d relations, want every one of %d", rels, want)
	}
	all := strings.Join(titles, "\n")
	for _, u := range users {
		for _, b := range d.Boxes {
			if s := fmt.Sprintf("user %s depends_on %s", u, b.Name); !strings.Contains(all, s) {
				t.Errorf("no wire's tooltip says %q", s)
			}
		}
	}

	out, err := json.Marshal(core)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), gatherPort) {
		t.Errorf("the view shows the internal key %q", gatherPort)
	}

	vl, err := lintView(view{d: d, budget: insideWidth})
	if err != nil {
		t.Fatal(err)
	}
	if vl.Totals.Crossings != 0 || vl.Totals.Hard != 0 {
		t.Fatalf("crossings %d, hard %d, want none", vl.Totals.Crossings, vl.Totals.Hard)
	}
}

// A gathered chip that stands for one module links to that module's view;
// one that stands for several links to none.
func TestGatheredChipOfOneModuleLinksToIt(t *testing.T) {
	core := part(t, []model.ID{"MOD-0011", "MOD-0012", "MOD-0013"}, map[model.ID][]model.ID{
		"MOD-0001": {"MOD-0011", "MOD-0012", "MOD-0013"},
		"MOD-0002": {"MOD-0011", "MOD-0012"},
	})
	var one, several int
	for _, p := range core.Drawing.Ports {
		if p.Class != "other" {
			continue
		}
		switch {
		case strings.Contains(p.Title, "user MOD-0002"):
			several++
			if p.Href != "" || p.Contract != "" {
				t.Errorf("a chip for two modules links to %q, contract %q", p.Href, p.Contract)
			}
		default:
			one++
			if p.Href != "#m-MOD-0001" || p.Contract != "MOD-0001" {
				t.Errorf("a chip for MOD-0001 alone links to %q, contract %q", p.Href, p.Contract)
			}
		}
	}
	if one != 1 || several != 2 {
		t.Fatalf("want one chip for MOD-0001 and two for both, got %d and %d", one, several)
	}
}

// A gathered chip's label is cut only between names and stays on one line,
// however wide its letters are.
func TestGatheredLabelCutsAtWholeNames(t *testing.T) {
	names := map[model.ID]string{"A": "결제 처리 모듈", "B": "주문 관리 모듈", "C": "배송 추적 모듈", "D": "internal/screen"}
	name := func(id model.ID) string { return names[id] }
	for _, ids := range [][]model.ID{{"A", "B", "C"}, {"D", "A", "B", "C"}, {"A"}} {
		label := gatheredLabel("← ", ids, name)
		if ls := chipLines(label); len(ls) != 1 {
			t.Errorf("%v: label %q wraps onto %d lines", ids, label, len(ls))
		}
		shown, rest, _ := strings.Cut(strings.TrimPrefix(label, "← "), " +")
		var parts []string
		for _, id := range ids {
			parts = append(parts, lastPart(name(id)))
		}
		k := len(strings.Split(shown, ", "))
		if got := strings.Join(parts[:k], ", "); got != shown {
			t.Errorf("%v: label %q shows %q, want whole names %q", ids, label, shown, got)
		}
		if k < len(ids) && rest != fmt.Sprint(len(ids)-k) {
			t.Errorf("%v: label %q counts %q past %d names", ids, label, rest, k)
		}
	}
}
