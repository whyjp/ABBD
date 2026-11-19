package screen

import (
	"testing"

	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/store"
)

// A box the overview record marks ambient stands, with overview_ambient
// on, as a foundation band under every other box and as wide as they span,
// with none of its lines drawn and its last line saying how many boxes use
// it; its pairs stay counted, so level L reads the same lines. With the
// switch off the mark changes nothing.
func TestAnAmbientBoxStandsAsAFoundationBand(t *testing.T) {
	units := []model.Unit{
		called("MOD-0001", "api", rel("MOD-0002", model.DependsOn), rel("MOD-0005", model.DependsOn)),
		called("MOD-0002", "core", rel("MOD-0003", model.DependsOn), rel("MOD-0005", model.DependsOn)),
		called("MOD-0003", "store", rel("MOD-0005", model.DependsOn)),
		called("MOD-0004", "cli", rel("MOD-0001", model.DependsOn), rel("MOD-0005", model.DependsOn)),
		called("MOD-0005", "log"),
	}
	units[4].Anchors = []model.Anchor{{Path: "log.go"}}
	var rec store.Overview
	rec.Known = true
	rec.Ambient = []store.Part[model.AmbientMark]{{Claim: model.AmbientMark{Unit: "MOD-0005", As: model.AmbientGlobal, Evidence: []model.Evidence{{Path: "log.go", Quote: "var Log"}}}}}
	off := Build(Input{Units: units, Overview: rec})
	on := Build(Input{Units: units, Overview: rec, Project: store.Project{OverviewAmbient: true}})
	if Levels(on)[0].Lines != Levels(off)[0].Lines {
		t.Errorf("level L reads %d lines with the band, %d without", Levels(on)[0].Lines, Levels(off)[0].Lines)
	}
	var band *Box
	lowest, left, right := 0, 1<<30, 0
	for i, b := range on.Overview.Boxes {
		if b.Ambient {
			band = &on.Overview.Boxes[i]
			continue
		}
		lowest, left, right = max(lowest, b.Y+b.H), min(left, b.X), max(right, b.X+b.W)
	}
	if band == nil || band.ID != "MOD-0005" {
		t.Fatalf("no band for the ambient log: %+v", on.Overview.Boxes)
	}
	if band.Y <= lowest || band.X != left || band.X+band.W != max(right, left+boxMax) {
		t.Errorf("the band stands at %d,%d %dpx wide; the boxes end at %d and span %d..%d", band.X, band.Y, band.W, lowest, left, right)
	}
	if band.Sub == nil || band.Sub.S != foundationText(4) {
		t.Errorf("the band says %+v (box %+v)", band.Sub, *band)
	}
	for _, r := range on.Overview.ways {
		for _, e := range r.ends {
			if e == "MOD-0005" {
				t.Errorf("a route still ends on the band: %+v", r.ends)
			}
		}
	}
	for _, b := range off.Overview.Boxes {
		if b.Ambient {
			t.Errorf("with the switch off, %s stands as a band", b.Name)
		}
	}
}

// A box a line of the record ends on stays a box, whatever its mark, so
// the record's word is never lost to the band.
func TestARecordedLineKeepsAnAmbientBoxABox(t *testing.T) {
	units := []model.Unit{
		called("MOD-0001", "api", rel("MOD-0002", model.DependsOn), rel("MOD-0005", model.DependsOn)),
		called("MOD-0002", "core", rel("MOD-0003", model.DependsOn), rel("MOD-0005", model.DependsOn)),
		called("MOD-0003", "store", rel("MOD-0005", model.DependsOn)),
		called("MOD-0004", "cli", rel("MOD-0001", model.DependsOn), rel("MOD-0005", model.DependsOn)),
		called("MOD-0005", "log"),
	}
	units[4].Anchors = []model.Anchor{{Path: "log.go"}}
	var rec store.Overview
	rec.Known = true
	rec.Ambient = []store.Part[model.AmbientMark]{{Claim: model.AmbientMark{Unit: "MOD-0005", As: model.AmbientGlobal, Evidence: []model.Evidence{{Path: "log.go", Quote: "var Log"}}}}}
	rec.Lines = []store.Part[model.Line]{{Claim: model.Line{From: "MOD-0001", To: "MOD-0005", Word: "logs"}}}
	m := Build(Input{Units: units, Overview: rec, Project: store.Project{OverviewAmbient: true}})
	for _, b := range m.Overview.Boxes {
		if b.Ambient {
			t.Errorf("%s stands as a band though the record draws a line to it", b.Name)
		}
	}
}
