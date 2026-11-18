package screen

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/store"
)

// record is an overview record of the pipeline: its zones and its lines.
func record(zones []model.Zone, lines ...model.Line) store.Overview {
	o := store.Overview{Known: true}
	for _, z := range zones {
		o.Zones = append(o.Zones, store.Part[model.Zone]{Claim: z})
	}
	for _, l := range lines {
		o.Lines = append(o.Lines, store.Part[model.Line]{Claim: l})
	}
	return o
}

func pipelineRecord(zones ...model.Zone) store.Overview {
	return record(zones,
		model.Line{From: "MOD-0006", To: "MOD-0001", Word: "loads"},
		model.Line{From: "MOD-0001", To: "MOD-0003", Word: "configures"},
		model.Line{From: "MOD-0004", To: "MOD-0005", Word: "hands on"})
}

var events = model.Zone{Name: "events", Units: []model.ID{"MOD-0003", "MOD-0004", "MOD-0005"}}

// A record that shows the primary flow and a zone composes the overview:
// the stages stand left to right in order, numbered on their boxes, the
// zone stands as a band, the box no line reaches stands on the shared
// base, and only a drawn line between neighbouring stages is bold.
func TestARecordWithAFlowAndAZoneComposesTheOverview(t *testing.T) {
	in := Input{Units: pipeline(), Figures: pipelineStages(), Overview: pipelineRecord(events)}
	d := Build(in).Overview
	if d.Lead == nil || len(d.Lead.Hops) != 5 {
		t.Fatalf("the lead numbers %v, want the five stages", d.Lead)
	}
	bs := boxes(d)
	order := []string{"MOD-0001", "MOD-0002", "MOD-0003", "MOD-0004", "MOD-0005"}
	for i, h := range d.Lead.Hops {
		b := bs[order[i]]
		if h.N != i+1 || h.X != b.X || h.Y != b.Y {
			t.Errorf("stage %d's number stands at %d,%d, want on %s at %d,%d", h.N, h.X, h.Y, b.Name, b.X, b.Y)
		}
		if i > 0 && (b.Y != bs[order[0]].Y || b.X <= bs[order[i-1]].X) {
			t.Errorf("%s does not stand right of the stage before it on the axis", b.Name)
		}
		if bold := h.D != ""; bold != (i == 4) {
			t.Errorf("stage %d's line is bold %v; only transforms to sinks has a drawn line", h.N, bold)
		}
	}
	titles := map[string]Zone{}
	for _, z := range d.Zones {
		titles[z.Title] = z
	}
	if _, ok := titles["events"]; !ok {
		t.Errorf("the record's zone has no band; bands are %v", d.Zones)
	}
	base, ok := titles["공용 기반"]
	f := bs["MOD-0007"]
	if !ok || f.X < base.X || f.X+f.W > base.X+base.W || f.Y < base.Y {
		t.Errorf("foundation, which every stage uses with no line drawn, is not on the shared base: %v", base)
	}
	if got := FlowGaps(in); len(got) != 3 {
		t.Errorf("the flow's gaps are %q, want stages 1-2, 2-3 and 3-4", got)
	}
}

// An overview that is not composed draws as it did: a record with zones but
// a flow of one stage draws the same overview as without its zones.
func TestARecordWithoutTwoStagesDrawsAsBefore(t *testing.T) {
	one := primaryStack(band("config", "MOD-0001"))
	with := Build(Input{Units: pipeline(), Figures: one, Overview: pipelineRecord(events)}).Overview
	without := Build(Input{Units: pipeline(), Figures: one, Overview: pipelineRecord()}).Overview
	a, _ := json.Marshal(with)
	b, _ := json.Marshal(without)
	if string(a) != string(b) {
		t.Error("a record's zones changed an overview that is not composed")
	}
	if FlowGaps(Input{Units: pipeline(), Figures: one, Overview: pipelineRecord(events)}) != nil {
		t.Error("an overview that is not composed reports flow gaps")
	}
	if slices.ContainsFunc(with.Zones, func(z Zone) bool { return z.Title == "공용 기반" }) {
		t.Error("an overview that is not composed draws a shared base")
	}
}
