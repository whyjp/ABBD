package screen

import (
	"slices"
	"testing"

	"github.com/whyjp/abbd/internal/model"
)

func derivedOf(paths ...[]string) layering {
	o := &overview{units: map[model.ID]model.Unit{}}
	for i, ps := range paths {
		u := model.Unit{ID: model.FormatID(model.Module, i+1), Kind: model.Module}
		for _, p := range ps {
			u.Anchors = append(u.Anchors, model.Anchor{Path: p})
		}
		o.units[u.ID] = u
		o.modules = append(o.modules, u.ID)
	}
	l, _ := o.derive(nil)
	return l
}

// With no stored stack, the overview groups its modules by the top
// directory their code sits in: directories that share a stem before a
// hyphen are one group, titled by the stem, a module whose code spans
// stems or sits at the root stands in none, and a directory of one module
// is no group.
func TestTheOverviewGroupsModulesByTopDirectory(t *testing.T) {
	t.Parallel()
	l := derivedOf(
		[]string{"crates/tauri", "crates/tauri/Cargo.toml"},
		[]string{"crates/tauri-utils/src/lib.rs"},
		[]string{"libobs", "libobs/obs.c"},
		[]string{"libobs-winrt/winrt.cpp"},
		[]string{"libobs-d3d11", "libobs-opengl", "libobs/graphics/device.h"},
		[]string{"frontend/main.cpp"},
		[]string{"ARCHITECTURE.md"},
		[]string{"crates/a", "plugins/b"},
	)
	got := map[string][]model.ID{}
	for i, n := range l.names {
		got[n] = l.units[i]
	}
	want := map[string][]model.ID{"crates/": {"MOD-0001", "MOD-0002"}, "libobs*": {"MOD-0003", "MOD-0004", "MOD-0005"}}
	if len(got) != len(want) || !l.derived {
		t.Fatalf("groups %v, want %v", got, want)
	}
	for n, ids := range want {
		if !slices.Equal(got[n], ids) {
			t.Errorf("group %q holds %v, want %v", n, got[n], ids)
		}
	}
}

// Where every module sits under one directory, the groups are the
// directories below it.
func TestTheOverviewGroupsBelowADirectoryAllShare(t *testing.T) {
	t.Parallel()
	l := derivedOf([]string{"src/core/a.go"}, []string{"src/core/b"}, []string{"src/cli/c.go"}, []string{"src/cli/d.go"}, []string{"src/web/e.go"})
	if !slices.Equal(l.names, []string{"src/cli/", "src/core/"}) && !slices.Equal(l.names, []string{"src/core/", "src/cli/"}) {
		t.Errorf("groups %v, want src/core/ and src/cli/", l.names)
	}
}

// A group that wraps onto a second row bands every box when nothing else
// stands among them, though its first row alone would end the run.
func TestAWrappedGroupSitsTogether(t *testing.T) {
	t.Parallel()
	d := Drawing{Width: 1000, Height: 600, Boxes: []Box{
		{ID: "a", X: 40, Y: 40, W: 200, H: 60}, {ID: "b", X: 300, Y: 40, W: 200, H: 60},
		{ID: "c", X: 40, Y: 200, W: 200, H: 60}, {ID: "d", X: 300, Y: 200, W: 200, H: 60},
		{ID: "e", X: 700, Y: 400, W: 200, H: 60},
	}}
	if got := d.together([]string{"a", "b", "c", "d"}); len(got) != 4 {
		t.Errorf("together gives %v, want all four", got)
	}
}
