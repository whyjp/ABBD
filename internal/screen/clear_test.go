package screen

import (
	"math"
	"path/filepath"
	"slices"
	"testing"

	"github.com/whyjp/abbd/internal/store"
)

// Every hop's number keeps inkGap from every line that is not its own
// hop's, from every arrowhead and chip, and off every box but the two its
// line joins, whose edge it may sit on clear of their words, so it never
// reads as sitting on another line or a head (user case 21).
func TestABadgeKeepsClearOfOtherInk(t *testing.T) {
	badges, bad := 0, 0
	for name, m := range clearModels(t) {
		for _, d := range m.drawings() {
			if d.Lead == nil {
				continue
			}
			for _, h := range d.Lead.Hops {
				badges++
				c := pt{h.X, h.Y}
				need := float64(h.R + inkGap)
				own := d.ways[h.way].runs
				near := ""
				for _, w := range d.Wires {
					rs := runs(w.D)
					for _, r := range rs {
						along := false
						for _, o := range own {
							along = along || overlap(r, o) > 0
						}
						if !along && stretchDist(c, r[0], r[1]) < need+1 {
							near += " a line"
						}
					}
					if w.Head != "" && !w.Joins && len(rs) > 0 && boxDist(c, tipBox(rs[len(rs)-1][1], rs[len(rs)-1][0])) < need-1 {
						near += " an arrowhead"
					}
				}
				for _, p := range d.Ports {
					if boxDist(c, box{p.X, p.RectY(), p.W, p.H}) < need {
						near += " a chip"
					}
				}
				ends := d.ways[h.way].ends
				for _, b := range d.Boxes {
					if slices.Contains(ends, b.ID) {
						if d.onWords(c, h.R) {
							near += " its box's words"
						}
						continue
					}
					if boxDist(c, box{b.X, b.Y, b.W, b.H}) < float64(h.R) {
						near += " a box"
					}
				}
				if near != "" {
					bad++
					t.Errorf("%s: hop %d's number at %v sits within %v of %s", name, h.N, c, need, near)
				}
			}
		}
	}
	t.Logf("%d badges, %d too close to other ink", badges, bad)
	if badges == 0 {
		t.Error("no fixture view draws a badge")
	}
}

// A band's title keeps inkGap from every line, arrowhead, dot, chip, label
// and count on every fixture. This repository's own overview is logged, not held: three
// buses run through the whole room above internal/screen there, which is
// the routing's to clear.
func TestAZoneTitleKeepsClearOfInk(t *testing.T) {
	titles, bad := 0, 0
	for name, m := range clearModels(t) {
		for _, d := range m.drawings() {
			for _, z := range d.Zones {
				titles++
				lines := z.Lines
				if len(lines) == 0 {
					lines = []string{z.Label()}
				}
				w := 0
				for _, l := range lines {
					w = max(w, zoneWidth(l))
				}
				area := box{z.TX - 2, z.TY - 10, w + 4, 13 + (len(lines)-1)*zoneLine}
				if !d.clearOfInk(grow(area, inkGap), nil) || d.crossed(grow(area, inkGap)) > 0 {
					t.Logf("%s: band %q's title meets ink", name, z.Label())
					if name != "repository" {
						bad++
					}
				}
			}
		}
	}
	t.Logf("%d titles, %d meet ink", titles, bad)
	if bad > 0 {
		t.Errorf("%d of %d band titles meet ink", bad, titles)
	}
}

// clearModels are the fixtures and this repository's own state, with its
// stored flows and stacks, which the fixtures lack.
func clearModels(t *testing.T) map[string]*Model {
	t.Helper()
	out := map[string]*Model{}
	for _, name := range fixtures {
		m := fixture(t, name)
		out[name] = &m
	}
	in, err := Load(store.Open(filepath.Join("..", "..")))
	if err != nil {
		t.Fatal(err)
	}
	m := Build(in)
	out["repository"] = &m
	return out
}

// A lead's numbers stand badgeApart from each other on every fixture and
// this repository's own overview, whose lead doubles back through
// internal/store and once stacked three numbers within 95x25px.
func TestALeadsNumbersStandApart(t *testing.T) {
	pairs := 0
	for name, m := range clearModels(t) {
		for _, d := range m.drawings() {
			if d.Lead == nil {
				continue
			}
			hs := d.Lead.Hops
			for i := range hs {
				for j := i + 1; j < len(hs); j++ {
					pairs++
					if gap := math.Hypot(float64(hs[i].X-hs[j].X), float64(hs[i].Y-hs[j].Y)); gap < badgeApart {
						t.Errorf("%s: numbers %d and %d stand %.0fpx apart, under %d", name, hs[i].N, hs[j].N, gap, badgeApart)
					}
				}
			}
		}
	}
	if pairs == 0 {
		t.Error("no view draws two numbers")
	}
}

// Every band holds two boxes at least, but the outside contracts' band,
// which names a lone outside box as outside, and stored layers' bands are
// numbered 1, 2, 3 in the order they are drawn.
func TestABandHoldsTwoBoxesAndNumbersRunOn(t *testing.T) {
	for name, m := range clearModels(t) {
		for _, d := range m.drawings() {
			n := 0
			for _, z := range d.Zones {
				if z.N > 0 {
					n++
					if z.N != n {
						t.Errorf("%s: band %q is numbered %d, want %d", name, z.Title, z.N, n)
					}
				}
				if z.Title == "외부 계약" {
					continue
				}
				held := 0
				for _, b := range d.Boxes {
					if b.X >= z.X && b.Y >= z.Y && b.X+b.W <= z.X+z.W && b.Y+b.H <= z.Y+z.H {
						held++
					}
				}
				if held < 2 {
					t.Errorf("%s: band %q holds %d box", name, z.Label(), held)
				}
			}
		}
	}
}
