package screen

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/store"
)

// recordedFixture builds the Orca fixture with an overview record whose
// zones name the boxes the overview draws: the zones are given as lists of
// box indexes into the overview's boxes in drawing order, and a part that
// stands cites the first anchor of each of its units.
func recordedFixture(t *testing.T, zones map[string][]int, stale ...string) Model {
	t.Helper()
	st := store.Open(filepath.Join("testdata", "orca"))
	units, err := st.LoadUnits()
	if err != nil {
		t.Fatal(err)
	}
	p, err := st.LoadProject()
	if err != nil {
		t.Fatal(err)
	}
	base := Build(Input{Units: units, Project: p})
	by := map[model.ID]model.Unit{}
	for _, u := range units {
		by[u.ID] = u
	}
	var parts []store.Part[model.Zone]
	for _, name := range slices.Sorted(mapsKeys(zones)) {
		z := model.Zone{Name: name}
		for _, i := range zones[name] {
			id := model.ID(base.Overview.Boxes[i].ID)
			z.Units = append(z.Units, id)
			// A group's quote lies in the code of a unit inside it.
			for _, u := range units {
				if u.Parent == id && len(u.Anchors) > 0 && z.Evidence == nil {
					z.Evidence = []model.Evidence{{Path: u.Anchors[0].Path, Lines: "1", Quote: "x"}}
				}
			}
		}
		part := store.Part[model.Zone]{Claim: z}
		if slices.Contains(stale, name) {
			part.Changed = []string{z.Evidence[0].Path}
		}
		parts = append(parts, part)
	}
	return Build(Input{Units: units, Project: p, Overview: store.Overview{Known: true, Zones: parts}})
}

func mapsKeys(m map[string][]int) func(func(string) bool) {
	return func(yield func(string) bool) {
		for k := range m {
			if !yield(k) {
				return
			}
		}
	}
}

func titles(d *Drawing) []string {
	var out []string
	for _, z := range d.Zones {
		out = append(out, z.Title)
	}
	return out
}

// Where the overview record holds no zone that stands, the overview draws
// its bands as it always has: the same bands, to the pixel.
func TestAnOverviewWithoutRecordedZonesKeepsItsBands(t *testing.T) {
	t.Parallel()
	plain := fixture(t, "orca")
	none := recordedFixture(t, nil)
	if !slices.Equal(titles(&plain.Overview), titles(&none.Overview)) || len(plain.Overview.Zones) != len(none.Overview.Zones) {
		t.Fatalf("bands %v, want %v", titles(&none.Overview), titles(&plain.Overview))
	}
	for i, z := range plain.Overview.Zones {
		if o := none.Overview.Zones[i]; z.X != o.X || z.Y != o.Y || z.W != o.W || z.H != o.H || z.TX != o.TX || z.TY != o.TY || z.Hue != o.Hue {
			t.Errorf("band %d moved: %+v, want %+v", i, none.Overview.Zones[i], z)
		}
	}
}

// A recorded zone is drawn under its own name, in place of the bands of
// layers and folders, and its band is never numbered. Boxes it does not name
// stand in no band, and every band of one zone wears one colour.
func TestARecordedZoneIsDrawnUnderItsNameAndOneColour(t *testing.T) {
	t.Parallel()
	plain := fixture(t, "orca")
	n := len(plain.Overview.Boxes)
	if n < 4 {
		t.Skip("the fixture's overview has too few boxes")
	}
	all := map[string][]int{"Alpha": {}, "Beta": {}}
	for i := range plain.Overview.Boxes {
		if i%2 == 0 {
			all["Alpha"] = append(all["Alpha"], i)
		} else {
			all["Beta"] = append(all["Beta"], i)
		}
	}
	m := recordedFixture(t, all)
	hue := map[string]string{}
	pieces := map[string]int{}
	for _, z := range m.Overview.Zones {
		if z.N != 0 {
			t.Errorf("band %q is numbered %d", z.Title, z.N)
		}
		switch z.Title {
		case "Alpha", "Beta":
			pieces[z.Title]++
			if h, ok := hue[z.Title]; ok && h != z.Hue {
				t.Errorf("zone %s wears %s and %s", z.Title, h, z.Hue)
			}
			hue[z.Title] = z.Hue
		}
	}
	if pieces["Alpha"]+pieces["Beta"] == 0 {
		t.Fatalf("no recorded zone is drawn: %v", titles(&m.Overview))
	}
	// Every box a zone names stands in a band of that zone.
	for name, idx := range all {
		for _, i := range idx {
			b := plain.Overview.Boxes[i]
			in := slices.ContainsFunc(m.Overview.Zones, func(z Zone) bool {
				return z.Title == name && b.X >= z.X && b.Y >= z.Y && b.X+b.W <= z.X+z.W && b.Y+b.H <= z.Y+z.H
			})
			if !in {
				t.Errorf("box %s of zone %s stands in none of its bands", b.ID, name)
			}
		}
	}
	for _, title := range titles(&m.Overview) {
		if title != "Alpha" && title != "Beta" && title != "외부 계약" && title != "외부 라이브러리" {
			t.Errorf("band %q is drawn beside the recorded zones", title)
		}
	}
}

// A zone the code has moved under is STALE and draws nothing; the bands
// are those the overview would draw without the record.
func TestAStaleRecordedZoneIsNotDrawn(t *testing.T) {
	t.Parallel()
	plain := fixture(t, "orca")
	if len(plain.Overview.Boxes) < 2 {
		t.Skip("the fixture's overview has too few boxes")
	}
	// The same zone, not stale, is drawn: the only difference is the stamp.
	if live := recordedFixture(t, map[string][]int{"Stale": {0, 1}}); !slices.Contains(titles(&live.Overview), "Stale") {
		t.Fatalf("the zone is not drawn when it stands: %v", titles(&live.Overview))
	}
	m := recordedFixture(t, map[string][]int{"Stale": {0, 1}}, "Stale")
	if slices.Contains(titles(&m.Overview), "Stale") {
		t.Errorf("a STALE zone is drawn: %v", titles(&m.Overview))
	}
	if !slices.Equal(titles(&m.Overview), titles(&plain.Overview)) {
		t.Errorf("bands %v, want those without the record %v", titles(&m.Overview), titles(&plain.Overview))
	}
}

// Two zones that name units one drawn box shows share it: the box belongs
// to the zone stored first, and the second zone draws no band for it, so no
// two bands ask for one box.
func TestABoxTwoZonesNameBelongsToTheFirst(t *testing.T) {
	t.Parallel()
	plain := fixture(t, "orca")
	if len(plain.Overview.Boxes) < 2 {
		t.Skip("the fixture's overview has too few boxes")
	}
	m := recordedFixture(t, map[string][]int{"Alpha": {0, 1}, "Beta": {1}})
	if !slices.Contains(titles(&m.Overview), "Alpha") {
		t.Fatalf("the first zone is not drawn: %v", titles(&m.Overview))
	}
	if slices.Contains(titles(&m.Overview), "Beta") {
		t.Errorf("a second zone draws a band round a box the first holds: %v", titles(&m.Overview))
	}
}
