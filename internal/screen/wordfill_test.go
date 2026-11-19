package screen

import (
	"path/filepath"
	"testing"

	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/store"
)

// The word fill counts the routes labelled counts, each by one source, and
// on an overview drawn from a record it gives the record's words to the
// pairs the record chose, counting those it labels.
func TestTheWordFillSaysWhereEachWordComesFrom(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"pstack", "ripgrep", "self"} {
		m := drawAll(t, name)
		for _, d := range append([]Drawing{m.Overview}, insideDrawings(m)...) {
			f := wordFill(d)
			if f.Labelled != labelled(d) || f.Record+f.Phrase+f.Via != f.Labelled || f.Routes != len(d.ways) {
				t.Errorf("%s: fill %+v does not add up to %d labelled of %d routes", name, f, labelled(d), len(d.ways))
			}
			if f.Record != 0 || f.RecordPairs != 0 {
				t.Errorf("%s: a view with no record gives the record %+v", name, f)
			}
		}
	}
	units, err := store.Open(filepath.Join("testdata", "pstack")).LoadUnits()
	if err != nil {
		t.Fatal(err)
	}
	var first *Route
	plain := drawAll(t, "pstack").Overview
	for i, r := range plain.ways {
		if len(r.ends) == 2 {
			first = &plain.ways[i]
			break
		}
	}
	if first == nil {
		t.Fatal("the fixture's overview draws no route between two boxes")
	}
	var rec store.Overview
	rec.Known = true
	rec.Lines = []store.Part[model.Line]{{Claim: model.Line{From: model.ID(first.ends[0]), To: model.ID(first.ends[1]), Word: "orders"}}}
	f := wordFill(Build(Input{Units: units, Overview: rec}).Overview)
	if f.RecordPairs != 1 || f.RecordSaid > 1 || f.Record < f.RecordSaid {
		t.Errorf("a record of one worded pair fills %+v", f)
	}
}

func insideDrawings(m Model) []Drawing {
	var out []Drawing
	for _, ins := range m.Insides {
		out = append(out, ins.Drawing)
	}
	return out
}
