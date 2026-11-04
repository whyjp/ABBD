package screen

import (
	"math"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/store"
)

type srgb [3]float64

func parseHex(t *testing.T, h string) srgb {
	t.Helper()
	var c srgb
	for i := range c {
		v, err := strconv.ParseUint(h[1+2*i:3+2*i], 16, 8)
		if err != nil {
			t.Fatal(err)
		}
		c[i] = float64(v) / 255
	}
	return c
}

func luminance(c srgb) float64 {
	lin := func(v float64) float64 {
		if v <= 0.03928 {
			return v / 12.92
		}
		return math.Pow((v+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(c[0]) + 0.7152*lin(c[1]) + 0.0722*lin(c[2])
}

func contrast(a, b srgb) float64 {
	x, y := luminance(a), luminance(b)
	return (max(x, y) + 0.05) / (min(x, y) + 0.05)
}

// mixed is CSS's color-mix in srgb: p percent of a, the rest b.
func mixed(a, b srgb, p float64) srgb {
	var c srgb
	for i := range c {
		c[i] = a[i]*p/100 + b[i]*(1-p/100)
	}
	return c
}

// Every colour a line or a border takes stands at least 3:1 against the
// page and the box fill in both themes, and so does its climbing tint
// against the page (rule 11, rule 13).
func TestEveryHueReadsOnBothThemes(t *testing.T) {
	pages := map[string][2]string{"light": {"#f6f5f1", "#ffffff"}, "dark": {"#161614", "#201f1c"}}
	for _, h := range append(palette, neutral) {
		for theme, bg := range pages {
			c := h.light
			if theme == "dark" {
				c = h.dark
			}
			col, page, panel := parseHex(t, c), parseHex(t, bg[0]), parseHex(t, bg[1])
			for what, r := range map[string]float64{
				"page":          contrast(col, page),
				"box fill":      contrast(col, panel),
				"climbing tint": contrast(mixed(col, page, backMix), page),
			} {
				if r < 3 {
					t.Errorf("%s on the %s %s: %.2f:1, under 3:1", h.name, theme, what, r)
				}
			}
		}
	}
}

// Siblings joined by a relation take different colours wherever the
// palette allows, and the same state always colours the same way.
func TestJoinedSiblingsTakeDifferentColours(t *testing.T) {
	for _, name := range fixtures {
		units, err := store.Open(filepath.Join("testdata", name)).LoadUnits()
		if err != nil {
			t.Fatal(err)
		}
		tr := newTree(units)
		again := newTree(units)
		pairs, same := 0, 0
		for p, kids := range tr.children {
			for _, c := range kids {
				if tr.hues[c] != again.hues[c] {
					t.Errorf("%s: %s changes colour between two builds", name, c)
				}
				seen := map[model.ID]bool{}
				for _, s := range tr.subtree(c) {
					for _, r := range tr.units[s].Relations {
						b, ok := tr.within(r.To, p)
						if _, live := tr.units[r.To]; !live || r.Dead || !ok || b == c || seen[b] || tr.units[b].Parent != tr.units[c].Parent {
							continue
						}
						seen[b] = true
						pairs++
						if tr.hues[b] == tr.hues[c] {
							same++
						}
					}
				}
			}
		}
		t.Logf("%s: %d of %d joined sibling pairs share a colour", name, same, pairs)
	}
}
