package screen

import (
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/store"
)

// The hubs the fold reads off the plan are the hubs level L names on the
// overview drawn with every link.
func TestTheFoldFoldsTheHubsLevelLNames(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"pstack", "ripgrep", "self", "orca"} {
		units, err := store.Open(filepath.Join("testdata", name)).LoadUnits()
		if err != nil {
			t.Fatal(err)
		}
		in := Input{Units: units}
		o := planScoped(in.Units, "", nil, tactic{}, layerings(in), walks(in), nil, scopeOf(in))
		hub, _ := o.overviewHubs()
		var got []string
		for _, id := range slices.Sorted(maps.Keys(hub)) {
			got = append(got, o.name(model.ID(id)))
		}
		want := slices.Clone(Levels(drawAll(t, name))[0].Hubs)
		slices.Sort(got)
		slices.Sort(want)
		if !slices.Equal(got, want) {
			t.Errorf("%s: the fold reads hubs %v, level L names %v", name, got, want)
		}
	}
}

// With overview_fold_hubs on, the overview draws no line into a hub from a
// box that only uses it, says on the hub how many boxes use it and names
// them in its tooltip, and keeps every folded pair in Counted, so level L
// reads the same lines and hubs as without the fold.
func TestFoldedHubsSayHowManyBoxesUseThem(t *testing.T) {
	t.Parallel()
	units, err := store.Open(filepath.Join("testdata", "self")).LoadUnits()
	if err != nil {
		t.Fatal(err)
	}
	plain := Build(Input{Units: units})
	folded := Build(Input{Units: units, Project: store.Project{OverviewFoldHubs: true}})
	lp, lf := Levels(plain)[0], Levels(folded)[0]
	if lp.Lines != lf.Lines || !slices.Equal(lp.Hubs, lf.Hubs) {
		t.Errorf("level L reads %d lines and hubs %v folded, %d and %v plain", lf.Lines, lf.Hubs, lp.Lines, lp.Hubs)
	}
	if len(lp.Hubs) == 0 {
		t.Fatal("the fixture's overview has no hub to fold")
	}
	said := 0
	for _, b := range folded.Overview.Boxes {
		if b.UsedBy == 0 {
			continue
		}
		said++
		if b.Sub == nil || b.Sub.S != usedByText(b.UsedBy) || !strings.Contains(b.Title, usedByText(b.UsedBy)+": ") {
			t.Errorf("hub %s says %+v, titled %q", b.Name, b.Sub, b.Title)
		}
	}
	if said == 0 {
		t.Error("no hub says how many boxes use it")
	}
	if len(folded.Overview.ways) >= len(plain.Overview.ways) {
		t.Errorf("the folded overview draws %d routes, the plain one %d", len(folded.Overview.ways), len(plain.Overview.ways))
	}
	if len(folded.Focus) == 0 {
		t.Error("a folded overview gives each box a focus view that draws every line")
	}
}

// Every folded hub says how many boxes use it, on every fixture, and so
// does a utility the overview stands as a foundation band.
func TestEveryFoldedHubSaysItsUsers(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"pstack", "ripgrep", "self", "orca"} {
		units, err := store.Open(filepath.Join("testdata", name)).LoadUnits()
		if err != nil {
			t.Fatal(err)
		}
		m := Build(Input{Units: units, Project: store.Project{OverviewFoldHubs: true}})
		// A band dropped for a word it would take never takes the
		// project's hub fold with it.
		if folds := slices.ContainsFunc(m.Overview.Boxes, func(b Box) bool { return b.UsedBy > 0 && !b.Ambient }); !folds && (name == "pstack" || name == "self") {
			t.Errorf("%s: no hub is folded", name)
		}
		for _, b := range m.Overview.Boxes {
			want := usedByText(b.UsedBy)
			if b.Ambient {
				want = foundationText(b.UsedBy)
			}
			if b.UsedBy > 0 && (b.Sub == nil || b.Sub.S != want) {
				t.Errorf("%s: folded hub %s says %+v", name, b.Name, b.Sub)
			}
		}
	}
}
