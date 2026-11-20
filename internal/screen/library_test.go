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

// The overview draws no library: no chip, no line, no fold chip. The
// libraries stay in the units list with every user named.
func TestTheOverviewDrawsNoLibrary(t *testing.T) {
	t.Parallel()
	want := map[int][]string{2: {"m1", "m3"}, 3: {"m1", "m4"}}
	for _, libs := range [][]string{{"serde", "tokio"}, {"a", "b", "c"}} {
		m := Build(Input{Units: libraryUnits(8, 4, libs...)})
		if got := libraryBoxes(m.Overview); len(got) != 0 {
			t.Errorf("the overview draws library chips %+v", got)
		}
		if got := libraryWires(m.Overview); len(got) != 0 {
			t.Errorf("the overview draws lines to libraries %q", got)
		}
		for _, r := range m.Units {
			if r.ID == "EXT-0001" && !slices.Equal(r.UsedBy, want[len(libs)]) {
				t.Errorf("the list says %s is used by %q, want %q", libs[0], r.UsedBy, want[len(libs)])
			}
		}
	}
}

// A focus view keeps the libraries its module uses, each one chip with one
// plain line that stands for the stored relation.
func TestAFocusViewDrawsItsLibraries(t *testing.T) {
	t.Parallel()
	m := Build(Input{Units: libraryUnits(8, 3, "serde", "tokio")})
	var f Drawing
	for _, v := range m.Focus {
		if v.Unit == "MOD-0001" {
			f = v.Drawing
		}
		if v.Unit == "MOD-0008" {
			t.Errorf("a module that uses no library has a focus view of a sparse overview")
		}
	}
	libs := libraryBoxes(f)
	if len(libs) != 1 || !strings.Contains(libs[0].Class, "external") {
		t.Fatalf("m1's focus draws %+v, want the one library chip it uses", libs)
	}
	if got, want := libraryWires(f), []string{"MOD-0001.depends_on.EXT-0001"}; !slices.Equal(got, want) {
		t.Errorf("lines to libraries stand for %q, want %q", got, want)
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

// A focus view weighs its module's libraries against its own boxes: a
// module that uses one library shows that library.
func TestAFocusViewFoldsByItsOwnBoxes(t *testing.T) {
	t.Parallel()
	us := libraryUnits(8, 4, "a", "b", "c")
	if o := planOverview(us, "", nil, tactic{}, nil, nil, nil); len(o.libraries) != 0 {
		t.Fatalf("the overview keeps %v, want no library", o.libraries)
	}
	if o := planOverview(us, "MOD-0001", nil, tactic{}, nil, nil, nil); !slices.Equal(o.libraries, []model.ID{"EXT-0001"}) {
		t.Errorf("m1's focus keeps %v, want the one library it uses", o.libraries)
	}
}

// The legend says how a library is drawn only where there is one.
func TestTheLegendNamesLibrariesOnlyWhereThereAreSome(t *testing.T) {
	t.Parallel()
	with, without := Build(Input{Units: libraryUnits(8, 3, "serde")}), Build(Input{Units: libraryUnits(8, 0, "serde")[:8]})
	if !with.HasLibraries() || without.HasLibraries() {
		t.Errorf("HasLibraries is %v with a library and %v without", with.HasLibraries(), without.HasLibraries())
	}
}
