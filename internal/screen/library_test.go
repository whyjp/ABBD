package screen

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/whyjp/abbd/internal/model"
)

// libraryUnits are boxes modules, the first users of them using the
// libraries in order: module i uses library i mod len(libs).
func libraryUnits(boxes, users int, libs ...string) []model.Unit {
	var out []model.Unit
	for i := range boxes {
		id := model.ID(fmt.Sprintf("MOD-%04d", i+1))
		u := called(id, fmt.Sprintf("m%d", i+1))
		if i+1 < boxes {
			u.Relations = append(u.Relations, rel(model.ID(fmt.Sprintf("MOD-%04d", i+2)), model.DependsOn))
		}
		if i < users {
			r := rel(model.ID(fmt.Sprintf("EXT-%04d", i%len(libs)+1)), model.DependsOn)
			r.Via = "import"
			u.Relations = append(u.Relations, r)
		}
		out = append(out, u)
	}
	for i, name := range libs {
		out = append(out, called(model.ID(fmt.Sprintf("EXT-%04d", i+1)), name))
	}
	return out
}

func libraryBoxes(d Drawing) []Box {
	var out []Box
	for _, b := range d.Boxes {
		if b.Kind == string(model.External) {
			out = append(out, b)
		}
	}
	return out
}

// libraryWires are the drawn lines that reach a library, by the stored
// relations each stands for.
func libraryWires(d Drawing) []string {
	var out []string
	for _, w := range d.Wires {
		for _, r := range w.Rels {
			if strings.Contains(r, ".EXT-") && !slices.Contains(out, r) {
				out = append(out, r)
			}
		}
	}
	slices.Sort(out)
	return out
}

// Each library the overview has room for is one chip apart from the
// product's boxes, with one plain line from each module that uses it and a
// line of words that says how many boxes use it.
func TestTheOverviewDrawsEachLibraryAsOneChip(t *testing.T) {
	t.Parallel()
	m := Build(Input{Units: libraryUnits(8, 3, "serde", "tokio")})
	libs := libraryBoxes(m.Overview)
	if len(libs) != 2 {
		t.Fatalf("the overview draws %d library chips, want 2: %+v", len(libs), libs)
	}
	for _, b := range libs {
		if !strings.Contains(b.Class, "external") || strings.Contains(b.Class, "twin") {
			t.Errorf("library %s is drawn as %q, not as an external chip", b.Name, b.Class)
		}
	}
	if !strings.Contains(libs[0].Title, "2개 상자가 사용") {
		t.Errorf("serde's chip says %q, want how many boxes use it", libs[0].Title)
	}
	want := []string{"MOD-0001.depends_on.EXT-0001", "MOD-0002.depends_on.EXT-0002", "MOD-0003.depends_on.EXT-0001"}
	if got := libraryWires(m.Overview); !slices.Equal(got, want) {
		t.Errorf("lines to libraries stand for %q, want %q", got, want)
	}
	for _, w := range m.Overview.Wires {
		if slices.ContainsFunc(w.Rels, func(r string) bool { return strings.Contains(r, ".EXT-") }) && w.Plan {
			t.Errorf("a line to a library is dotted: %+v", w)
		}
	}
}

// Past a quarter of the boxes, every library folds into one chip, and each
// module that uses any keeps one line to it that still stands for each of
// its stored relations; the units list names every user of each library.
func TestManyLibrariesFoldIntoOneChip(t *testing.T) {
	t.Parallel()
	m := Build(Input{Units: libraryUnits(8, 4, "a", "b", "c")})
	libs := libraryBoxes(m.Overview)
	if len(libs) != 1 || libs[0].ID != string(libraryFold) {
		t.Fatalf("the overview draws %+v, want the one fold chip", libs)
	}
	if !strings.Contains(libs[0].Name, "3개") || !strings.Contains(libs[0].Title, "a, b, c") || !strings.Contains(libs[0].Title, "4개 상자가 사용") {
		t.Errorf("the fold chip reads %q / %q", libs[0].Name, libs[0].Title)
	}
	want := []string{"MOD-0001.depends_on.EXT-0001", "MOD-0002.depends_on.EXT-0002", "MOD-0003.depends_on.EXT-0003", "MOD-0004.depends_on.EXT-0001"}
	if got := libraryWires(m.Overview); !slices.Equal(got, want) {
		t.Errorf("lines to the fold stand for %q, want %q", got, want)
	}
	for _, r := range m.Units {
		if r.ID == "EXT-0001" && !slices.Equal(r.UsedBy, []string{"m1", "m4"}) {
			t.Errorf("the list says a is used by %q, want m1 and m4", r.UsedBy)
		}
	}
}

// A module's inside view draws its own parts, not the libraries they use.
func TestAnInsideViewLeavesTheLibrariesOut(t *testing.T) {
	t.Parallel()
	us := []model.Unit{
		called("MOD-0001", "app"),
		inside("MOD-0002", "MOD-0001", rel("MOD-0003", model.DependsOn), rel("EXT-0001", model.DependsOn)),
		inside("MOD-0003", "MOD-0001"),
		called("EXT-0001", "serde"),
	}
	m := Build(Input{Units: us})
	for _, in := range m.Insides {
		for _, p := range in.Drawing.Ports {
			if strings.Contains(p.Title, "serde") {
				t.Errorf("the inside of %s draws the library as a port: %+v", in.ID, p)
			}
		}
	}
}
