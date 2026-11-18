package screen

import (
	"path/filepath"
	"testing"

	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/store"
)

// On an overview drawn from its stored record, a line against the flow never
// runs flat beside a line with it, besideGap or less apart, but as the other
// lane of a two-lane road: roads never merge the two, so side by side they
// read as one line drawn twice.
func TestARecordedOverviewKeepsBackLinesOffForwardOnes(t *testing.T) {
	t.Parallel()
	for _, name := range fixtures {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			units, err := store.Open(filepath.Join("testdata", name)).LoadUnits()
			if err != nil {
				t.Fatal(err)
			}
			// The record chooses every drawn pair but the last, so it
			// hides one.
			var rec store.Overview
			rec.Known = true
			ways := drawAll(t, name).Overview.ways
			for k, r := range ways {
				if len(r.ends) == 2 && k < len(ways)-1 {
					rec.Lines = append(rec.Lines, store.Part[model.Line]{Claim: model.Line{From: model.ID(r.ends[0]), To: model.ID(r.ends[1]), Word: "uses"}})
				}
			}
			d := Build(Input{Units: units, Overview: rec}).Overview
			for i, a := range d.ways {
				for _, b := range d.ways[i+1:] {
					if a.Back == b.Back {
						continue
					}
					for _, ra := range a.runs {
						for _, rb := range b.runs {
							if ra[0].y != ra[1].y || rb[0].y != rb[1].y {
								continue
							}
							gap := abs(ra[0].y - rb[0].y)
							along := min(max(ra[0].x, ra[1].x), max(rb[0].x, rb[1].x)) - max(min(ra[0].x, ra[1].x), min(rb[0].x, rb[1].x))
							wa, wb := sign(ra[1].x-ra[0].x), sign(rb[1].x-rb[0].x)
							road := gap == twoLaneGap && wa == -wb && sign(ra[0].y-rb[0].y) == wa
							if gap > 0 && gap <= besideGap && along > 0 && !road {
								t.Errorf("%v (back %v) runs %dpx beside %v (back %v), %dpx apart", a.ends, a.Back, along, b.ends, b.Back, gap)
							}
						}
					}
				}
			}
		})
	}
}
