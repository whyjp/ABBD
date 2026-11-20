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

// The hubs the common modules are read from are the hubs level L names on
// the overview drawn with every link.
func TestTheHubsOffThePlanAreLevelLs(t *testing.T) {
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
			t.Errorf("%s: the plan reads hubs %v, level L names %v", name, got, want)
		}
	}
}

// A hub whose lines carry what the product does keeps every one of them:
// the overview folds no line into a common module that affects the logic,
// nor into any hub that is no common module, so no box says how many boxes
// use it while some of their lines still stand.
func TestAHubThatIsNoUtilityKeepsItsLines(t *testing.T) {
	t.Parallel()
	units := append(bandUnits(), model.Unit{ID: "DATA-0001", Kind: model.Data, Name: model.Said(model.Script, "registry"), Parent: "MOD-0005"})
	units[0].Relations = append(units[0].Relations, rel("DATA-0001", model.DependsOn))
	m := Build(Input{Units: units})
	if len(m.Common) != 1 || m.Common[0].Utility() {
		t.Fatalf("log should affect the logic: %+v", m.Common)
	}
	into := map[string]bool{}
	for _, r := range m.Overview.ways {
		if slices.Contains(r.ends, "MOD-0005") {
			for _, e := range r.ends {
				into[e] = true
			}
		}
	}
	for _, u := range m.Common[0].Users {
		if !into[u] {
			t.Errorf("the line from %s into log is folded though log affects the logic", u)
		}
	}
	for _, b := range m.Overview.Boxes {
		if b.UsedBy > 0 || b.Ambient {
			t.Errorf("%s folds its lines: %+v", b.Name, b)
		}
	}
	// A hub that is no common module, since it uses as much as it is
	// used, keeps every line too.
	units = bandUnits()
	units[4].Relations = []model.Relation{rel("MOD-0001", model.DependsOn), rel("MOD-0002", model.DependsOn), rel("MOD-0003", model.DependsOn)}
	m = Build(Input{Units: units})
	if len(m.Common) != 0 {
		t.Fatalf("log uses as much as it is used, yet is common: %+v", m.Common)
	}
	for _, b := range m.Overview.Boxes {
		if b.UsedBy > 0 || b.Ambient {
			t.Errorf("%s folds its lines though it is no common module", b.Name)
		}
	}
}

// A utility a line of the record ends on stays a box with that line drawn,
// and so does every other line into it: its lines fold together or not at
// all.
func TestARecordLineOnAUtilityKeepsAllItsLines(t *testing.T) {
	plain := Build(Input{Units: bandUnits()})
	if !slices.ContainsFunc(plain.Overview.Boxes, func(b Box) bool { return b.ID == "MOD-0005" && b.Ambient }) {
		t.Fatal("log is no band without a record, so the case reads nothing")
	}
	var rec store.Overview
	rec.Known = true
	rec.Lines = []store.Part[model.Line]{{Claim: model.Line{From: "MOD-0001", To: "MOD-0005", Word: "logs"}}}
	m := Build(Input{Units: bandUnits(), Overview: rec})
	for _, b := range m.Overview.Boxes {
		if b.UsedBy > 0 || b.Ambient {
			t.Errorf("%s folds its lines though a record line ends on log", b.Name)
		}
	}
	reached := false
	for _, r := range m.Overview.ways {
		reached = reached || slices.Contains(r.ends, "MOD-0005") && slices.Contains(r.ends, "MOD-0001")
	}
	if !reached {
		t.Error("the record's line into log is not drawn")
	}
}

// On every fixture a box folds its lines all at once or not at all: a box
// that says how many boxes use it is a foundation band, says so on its
// last line, names each of those boxes in its tooltip, and no drawn line
// reaches it.
func TestEveryFoldedBoxFoldsAllItsLines(t *testing.T) {
	t.Parallel()
	folded := 0
	// bandUnits is the case where a band surely stands; the fixtures add
	// pstack's bands under a record.
	for _, name := range []string{"bands", "pstack", "ripgrep", "self", "orca"} {
		units := bandUnits()
		if name != "bands" {
			var err error
			if units, err = store.Open(filepath.Join("testdata", name)).LoadUnits(); err != nil {
				t.Fatal(err)
			}
		}
		for _, rec := range []store.Overview{{}, recordOf(units)} {
			m := Build(Input{Units: units, Overview: rec})
			for _, b := range m.Overview.Boxes {
				if b.UsedBy == 0 {
					continue
				}
				folded++
				if !b.Ambient || b.Sub == nil || b.Sub.S != foundationText(b.UsedBy) {
					t.Errorf("%s: %s folds its lines and says %+v, band %v", name, b.Name, b.Sub, b.Ambient)
				}
				c := slices.IndexFunc(m.Common, func(c CommonView) bool { return c.Unit == b.ID })
				if c < 0 || !m.Common[c].Utility() || !m.Common[c].Band {
					t.Errorf("%s: %s folds its lines and is no utility band", name, b.Name)
					continue
				}
				for _, u := range m.Common[c].Users {
					if !strings.Contains(b.Title, nameOf(m, u)) {
						t.Errorf("%s: the band %s does not name its user %s", name, b.Name, nameOf(m, u))
					}
				}
				for _, r := range m.Overview.ways {
					if slices.Contains(r.ends, b.ID) {
						t.Errorf("%s: a line still reaches the band %s: %v", name, b.Name, r.ends)
					}
				}
			}
		}
	}
	if folded < 2 {
		t.Errorf("only %d boxes fold their lines over every case, so the test reads too little", folded)
	}
}

// recordOf is an overview record that chooses the first line of the
// fixture's plain overview, so the fold is read under a record's cut too.
func recordOf(units []model.Unit) store.Overview {
	m := Build(Input{Units: units})
	var rec store.Overview
	for _, r := range m.Overview.ways {
		if len(r.ends) == 2 {
			rec.Known = true
			rec.Lines = []store.Part[model.Line]{{Claim: model.Line{From: model.ID(r.ends[0]), To: model.ID(r.ends[1]), Word: "uses"}}}
			break
		}
	}
	return rec
}
