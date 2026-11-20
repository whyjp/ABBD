package screen

import (
	"cmp"
	"path/filepath"
	"slices"
	"testing"

	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/store"
)

// With a stored overview record that chose lines, the overview draws those
// lines between the top-level boxes they join and counts every other pair on
// its boxes, whether or not the project switched the density cut on. Each
// hidden pair still shows in the focus view of its boxes.
func TestTheOverviewDrawsTheLinesTheRecordChose(t *testing.T) {
	t.Parallel()
	units, err := store.Open(filepath.Join("testdata", "pstack")).LoadUnits()
	if err != nil {
		t.Fatal(err)
	}
	ends := func(d Drawing) map[pair]bool {
		out := map[pair]bool{}
		for _, r := range d.ways {
			if len(r.ends) == 2 {
				out[pairOf(model.ID(r.ends[0]), model.ID(r.ends[1]))] = true
			}
		}
		return out
	}
	plain := ends(drawAll(t, "pstack").Overview)
	if len(plain) < 4 {
		t.Fatalf("the fixture draws %d pairs, too few to choose among", len(plain))
	}
	var all []pair
	for p := range plain {
		all = append(all, p)
	}
	slices.SortFunc(all, func(a, b pair) int { return cmp.Compare(string(a[0])+string(a[1]), string(b[0])+string(b[1])) })
	chosen := map[pair]bool{all[0]: true, all[len(all)-1]: true}
	var rec store.Overview
	rec.Known = true
	for p := range chosen {
		rec.Lines = append(rec.Lines, store.Part[model.Line]{Claim: model.Line{From: p[0], To: p[1], Word: "uses"}})
	}
	m := Build(Input{Units: units, Overview: rec})
	got := ends(m.Overview)
	for p := range chosen {
		if !got[p] {
			t.Errorf("chosen pair %v is not drawn", p)
		}
	}
	more, band := map[string]int{}, map[model.ID]bool{}
	for _, b := range m.Overview.Boxes {
		more[b.ID], band[model.ID(b.ID)] = b.More, b.Ambient
	}
	for p := range plain {
		if chosen[p] {
			continue
		}
		if got[p] {
			t.Errorf("pair %v was not chosen and is drawn", p)
		}
		// A pair into a foundation band adds to no count: the band says
		// how many boxes use it.
		for _, id := range p {
			if more[string(id)] == 0 && !band[p[0]] && !band[p[1]] {
				t.Errorf("box %s hides a pair and counts none", id)
			}
		}
		for i, id := range p {
			other := p[1-i]
			f := slices.IndexFunc(m.Focus, func(f Focus) bool { return f.Unit == string(id) })
			if f < 0 {
				t.Errorf("box %s has no focus view for its hidden pair %v", id, p)
				continue
			}
			if !ends(m.Focus[f].Drawing)[pairOf(id, other)] {
				t.Errorf("focus view of %s does not draw hidden pair %v", id, p)
			}
		}
	}
}

// A record line that names no unit, a unit that is gone, or two boxes no
// relation joins chooses nothing: a record left with no choice draws the
// overview as it would draw without one, and never panics.
func TestARecordWithNoLiveChoiceDrawsAsWithout(t *testing.T) {
	t.Parallel()
	units, err := store.Open(filepath.Join("testdata", "pstack")).LoadUnits()
	if err != nil {
		t.Fatal(err)
	}
	plain := drawAll(t, "pstack").Overview
	drawn := map[pair]bool{}
	var mods []model.ID
	for _, r := range plain.ways {
		if len(r.ends) == 2 {
			drawn[pairOf(model.ID(r.ends[0]), model.ID(r.ends[1]))] = true
		}
	}
	for _, b := range plain.Boxes {
		mods = append(mods, model.ID(b.ID))
	}
	var unjoined [2]model.ID
	for i, a := range mods {
		for _, b := range mods[i+1:] {
			if !drawn[pairOf(a, b)] {
				unjoined = [2]model.ID{a, b}
			}
		}
	}
	if unjoined[0] == "" {
		t.Fatal("the fixture joins every pair of boxes")
	}
	var rec store.Overview
	rec.Known = true
	for _, l := range []model.Line{
		{From: "", To: unjoined[1], Word: "uses"},
		{From: "MOD-9999", To: unjoined[1], Word: "uses"},
		{From: unjoined[0], To: unjoined[1], Word: "uses"},
	} {
		rec.Lines = append(rec.Lines, store.Part[model.Line]{Claim: l})
	}
	got := Build(Input{Units: units, Overview: rec}).Overview
	if len(got.ways) != len(plain.ways) {
		t.Fatalf("a record with no live choice draws %d lines, without one %d", len(got.ways), len(plain.ways))
	}
	for i := range got.ways {
		if got.ways[i].D != plain.ways[i].D {
			t.Errorf("line %v is drawn %s, without the record %s", got.ways[i].ends, got.ways[i].D, plain.ways[i].D)
		}
	}
}

// A record that chooses every pair hides none, so the overview draws as it
// would without the record, its lines where they stand.
func TestARecordThatHidesNothingDrawsAsWithout(t *testing.T) {
	t.Parallel()
	units, err := store.Open(filepath.Join("testdata", "self")).LoadUnits()
	if err != nil {
		t.Fatal(err)
	}
	plain := drawAll(t, "self").Overview
	var rec store.Overview
	rec.Known = true
	for _, r := range plain.ways {
		if len(r.ends) == 2 {
			rec.Lines = append(rec.Lines, store.Part[model.Line]{Claim: model.Line{From: model.ID(r.ends[0]), To: model.ID(r.ends[1]), Word: "uses"}})
		}
	}
	got := Build(Input{Units: units, Overview: rec}).Overview
	if len(got.ways) != len(plain.ways) {
		t.Fatalf("a record of every pair draws %d lines, without one %d", len(got.ways), len(plain.ways))
	}
	for i := range got.ways {
		if got.ways[i].D != plain.ways[i].D {
			t.Errorf("line %v is drawn %s, without the record %s", got.ways[i].ends, got.ways[i].D, plain.ways[i].D)
		}
	}
}

// A project that turns the cut off draws every line, whatever its record
// chose.
func TestTurningTheCutOffDrawsEveryLineDespiteARecord(t *testing.T) {
	t.Parallel()
	units, err := store.Open(filepath.Join("testdata", "pstack")).LoadUnits()
	if err != nil {
		t.Fatal(err)
	}
	plain := drawAll(t, "pstack").Overview
	var rec store.Overview
	rec.Known = true
	for _, r := range plain.ways[:2] {
		rec.Lines = append(rec.Lines, store.Part[model.Line]{Claim: model.Line{From: model.ID(r.ends[0]), To: model.ID(r.ends[1]), Word: "uses"}})
	}
	off := false
	got := Build(Input{Units: units, Overview: rec, Project: store.Project{OverviewCut: &off}}).Overview
	if len(got.ways) != len(plain.ways) {
		t.Fatalf("with the cut off a record draws %d lines, without one %d", len(got.ways), len(plain.ways))
	}
	if cut := Build(Input{Units: units, Overview: rec}).Overview; len(cut.ways) >= len(plain.ways) {
		t.Fatalf("with the cut unset the record draws %d lines of %d, hiding none", len(cut.ways), len(plain.ways))
	}
}

// A record some of whose lines no longer resolve is stale as a whole: the
// overview draws as without it rather than thinned to the lines left.
func TestAPartlyStaleRecordDrawsAsWithout(t *testing.T) {
	t.Parallel()
	units, err := store.Open(filepath.Join("testdata", "pstack")).LoadUnits()
	if err != nil {
		t.Fatal(err)
	}
	plain := drawAll(t, "pstack").Overview
	r := plain.ways[0]
	var rec store.Overview
	rec.Known = true
	for _, l := range []model.Line{
		{From: model.ID(r.ends[0]), To: model.ID(r.ends[1]), Word: "uses"},
		{From: "MOD-9999", To: model.ID(r.ends[1]), Word: "uses"},
	} {
		rec.Lines = append(rec.Lines, store.Part[model.Line]{Claim: l})
	}
	got := Build(Input{Units: units, Overview: rec}).Overview
	if len(got.ways) != len(plain.ways) {
		t.Fatalf("a partly stale record draws %d lines, without one %d", len(got.ways), len(plain.ways))
	}
}

// A record changes which lines the overview draws, not how each box's own
// focus view reads: every focus view draws as it does without the record.
func TestARecordLeavesTheFocusViewsAsTheyAre(t *testing.T) {
	t.Parallel()
	units, err := store.Open(filepath.Join("testdata", "self")).LoadUnits()
	if err != nil {
		t.Fatal(err)
	}
	on := true
	project := store.Project{OverviewCut: &on}
	plain := Build(Input{Units: units, Project: project})
	if len(plain.Focus) == 0 {
		t.Fatal("the fixture draws no focus view without a record")
	}
	var rec store.Overview
	rec.Known = true
	for _, r := range plain.Overview.ways[:3] {
		rec.Lines = append(rec.Lines, store.Part[model.Line]{Claim: model.Line{From: model.ID(r.ends[0]), To: model.ID(r.ends[1]), Word: "uses"}})
	}
	got := Build(Input{Units: units, Overview: rec, Project: project})
	if len(got.Overview.ways) >= len(plain.Overview.ways) {
		t.Fatalf("the record hides nothing: %d lines of %d", len(got.Overview.ways), len(plain.Overview.ways))
	}
	want := map[string][]string{}
	for _, f := range plain.Focus {
		for _, r := range f.Drawing.ways {
			want[f.Unit] = append(want[f.Unit], r.D)
		}
	}
	for _, f := range got.Focus {
		var ds []string
		for _, r := range f.Drawing.ways {
			ds = append(ds, r.D)
		}
		if !slices.Equal(ds, want[f.Unit]) {
			t.Errorf("focus view of %s draws otherwise with the record", f.Unit)
		}
	}
}
