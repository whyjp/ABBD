package screen

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/store"
)

// fixtures are real states: this repository's own .abbd, ripgrep read by a
// pstack reader, Orca, and pstack's own skill tree. Each holds only the unit files the drawings read.
var fixtures = []string{"self", "ripgrep", "orca", "pstack"}

func fixture(t *testing.T, name string) Model {
	t.Helper()
	units, err := store.Open(filepath.Join("testdata", name)).LoadUnits()
	if err != nil {
		t.Fatal(err)
	}
	return Build(Input{Units: units})
}

func measured(t *testing.T, d Drawing) Metrics {
	t.Helper()
	var b bytes.Buffer
	if err := site.ExecuteTemplate(&b, "drawing", d); err != nil {
		t.Fatal(err)
	}
	s, err := readSVG(b.String())
	if err != nil {
		t.Fatal(err)
	}
	return measure(s)
}

// Every drawing of every real state keeps docs/blueprint-svg.md's rules.
// With ABBD_BLUEPRINTS set to a directory, it also writes each fixture's page
// there, one folder of pages per fixture, for a person to look at.
func TestEveryRealDrawingIsLegible(t *testing.T) {
	for _, name := range fixtures {
		m := fixture(t, name)
		if dir := os.Getenv("ABBD_BLUEPRINTS"); dir != "" {
			pages, err := Pages(m)
			if err != nil {
				t.Fatal(err)
			}
			os.MkdirAll(filepath.Join(dir, name), 0o755)
			for f, b := range pages {
				if err := os.WriteFile(filepath.Join(dir, name, f), b, 0o644); err != nil {
					t.Fatal(err)
				}
			}
		}
		for _, v := range views(m) {
			got := measured(t, v.d)
			bad := got.violations(v.budget)
			ceiling, owed := debt[name+" "+v.name]
			switch {
			case owed && len(bad) == 0:
				t.Errorf("%s %s now keeps every rule; take it off the debt list", name, v.name)
			case owed:
				for _, over := range got.over(ceiling) {
					t.Errorf("%s %s got worse than its recorded debt: %s", name, v.name, over)
				}
			default:
				for _, b := range bad {
					t.Errorf("%s %s: rule %s", name, v.name, b)
				}
			}
			t.Logf("%s %s: %s", name, v.name, got)
		}
	}
}

// debt are the views of the real states that do not keep every rule yet,
// with what they measure now. They may only improve. The Orca insides are
// dense cyclic import graphs among siblings. The renderer draws no box the
// state lacks, so their climbing lines stay (docs/blueprint-svg.md, "Known
// gaps"). The rest are near misses listed there one by one.
var debt = map[string]Metrics{
	"self MOD-0030 internal/model":           {Back: 3},
	"self MOD-0047 internal/topology":        {Back: 2},
	"orca MOD-0007 mobile app (Expo)":        {Back: 11},
	"orca MOD-0042 desktop main process":     {Crossings: 58, Back: 9},
	"orca MOD-0056 Orca runtime (RPC host)":  {Back: 7},
	"orca MOD-0072 desktop renderer UI":      {Back: 6},
	"orca MOD-0081 shared types and schemas": {Boxes: 22, Back: 6, MaxDetour: 136},
	// A focus view (focus.go) of the same state, where lines of two kinds
	// between the same boxes run side by side.
	"pstack MOD-0001 focus FEAT-0008 docs/harness/claude-code.md": {Braid: 708},
}

// over names each measure that went past its recorded debt. A measure the
// debt leaves at zero is held to the rules instead.
func (m Metrics) over(d Metrics) []string {
	var out []string
	for _, c := range []struct {
		name      string
		got, owed int
	}{
		{"boxes", m.Boxes, d.Boxes}, {"width", m.Width, d.Width}, {"channel", m.MaxChannel, d.MaxChannel},
		{"too-close pairs", m.TooClose, d.TooClose}, {"crossings", m.Crossings, d.Crossings}, {"back lines", m.Back, d.Back}, {"split arrivals", m.SplitIn, d.SplitIn}, {"cut labels", m.Truncated, d.Truncated},
		{"braid", m.Braid, d.Braid}, {"longest detour", m.MaxDetour, d.MaxDetour},
	} {
		if c.owed > 0 && c.got > c.owed {
			out = append(out, fmt.Sprintf("%s %d, recorded %d", c.name, c.got, c.owed))
		}
	}
	held := m
	held.Boxes, held.Width, held.MaxChannel, held.TooClose, held.Crossings, held.Back, held.SplitIn, held.Truncated, held.Braid, held.MaxDetour = 0, 0, 0, 0, 0, 0, 0, 0, 0, 0
	if d.Boxes == 0 {
		held.Boxes = m.Boxes
	}
	if d.Width == 0 {
		held.Width = m.Width
	}
	if d.MaxChannel == 0 {
		held.MaxChannel = m.MaxChannel
	}
	if d.TooClose == 0 {
		held.TooClose = m.TooClose
	}
	if d.Crossings == 0 {
		held.Crossings = m.Crossings
	}
	if d.Back == 0 {
		held.Back = m.Back
	}
	if d.SplitIn == 0 {
		held.SplitIn = m.SplitIn
	}
	if d.Truncated == 0 {
		held.Truncated = m.Truncated
	}
	if d.Braid == 0 {
		held.Braid = m.Braid
	}
	if d.MaxDetour == 0 {
		held.MaxDetour = m.MaxDetour
	}
	width := insideWidth
	if strings.HasSuffix(fmt.Sprint(d), "overview") {
		width = overviewWidth
	}
	return append(out, held.violations(max(width, overviewWidth))...)
}

// No relation is dropped: every live relation between present units is
// drawn in some view, as a line, a bundle or a contract chip, or listed as
// a reader's coupling where the overview leaves it out.
func TestEveryRelationIsInSomeView(t *testing.T) {
	for _, name := range fixtures {
		units, err := store.Open(filepath.Join("testdata", name)).LoadUnits()
		if err != nil {
			t.Fatal(err)
		}
		m := Build(Input{Units: units})
		shown := map[string]bool{}
		for _, v := range views(m) {
			for _, w := range v.d.Wires {
				for _, r := range w.Rels {
					shown[r] = true
				}
			}
			for _, p := range v.d.Ports {
				for _, r := range p.Rels {
					shown[r] = true
				}
			}
		}
		present := map[model.ID]model.Unit{}
		for _, u := range model.Present(units) {
			present[u.ID] = u
		}
		for _, c := range m.Links {
			shown[c.From+"|"+c.Kind+"|"+c.To] = true
		}
		for _, ins := range m.Insides {
			for _, c := range ins.Inferred {
				shown[c.From+"|"+c.Kind+"|"+c.To] = true
			}
		}
		missing := 0
		for _, u := range present {
			for _, r := range u.Relations {
				if _, ok := present[r.To]; !ok || r.Dead {
					continue
				}
				if shown[key(u.ID, r.Kind, r.To)] || shown[name4(present, u.ID)+"|"+string(r.Kind)+"|"+name4(present, r.To)] {
					continue
				}
				missing++
				if missing <= 5 {
					t.Errorf("%s: %s is in no view", name, key(u.ID, r.Kind, r.To))
				}
			}
		}
		if missing > 5 {
			t.Errorf("%s: %d relations in no view", name, missing)
		}
	}
}

func name4(units map[model.ID]model.Unit, id model.ID) string {
	if u, ok := units[id]; ok && u.Name.Value != "" {
		return u.Name.Value
	}
	return string(id)
}
