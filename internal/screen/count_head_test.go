package screen

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/whyjp/abbd/internal/store"
)

var headAt = regexp.MustCompile(`translate\((-?[\d.]+) (-?[\d.]+)\) rotate\((-?\d+)\)`)

// countsByHeads lists each ×N of a drawing whose glyphs come within 6px of
// an arrowhead's chevron, its own line's included: 7.2px deep, 3.6px to
// each side of its tip.
func countsByHeads(svg string) []string {
	type count struct {
		at fpt
		s  string
	}
	var chevrons [][2]fpt
	var counts []count
	var cur *count
	dec := xml.NewDecoder(strings.NewReader(svg))
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch e := tok.(type) {
		case xml.StartElement:
			switch {
			case e.Name.Local == "use" && strings.HasPrefix(attr(e, "href"), "#t-"):
				m := headAt.FindStringSubmatch(attr(e, "transform"))
				if m == nil {
					continue
				}
				tip, back := fpt{num(m[1]), num(m[2])}, 7.2
				if m[3] == "-90" {
					back = -7.2
				}
				for _, side := range []float64{-3.6, 3.6} {
					chevrons = append(chevrons, [2]fpt{tip, {tip.x + side, tip.y - back}})
				}
			case e.Name.Local == "text" && hasClass(e, "count"):
				counts = append(counts, count{at: fpt{num(attr(e, "x")), num(attr(e, "y"))}})
				cur = &counts[len(counts)-1]
			}
		case xml.CharData:
			if cur != nil {
				cur.s += string(e)
			}
		case xml.EndElement:
			if e.Name.Local == "text" {
				cur = nil
			}
		}
	}
	var out []string
	for _, c := range counts {
		r := frect{c.at.x, c.at.y - 9, float64(textWidth(c.s, 11)), 11}
		for _, ch := range chevrons {
			if rectSegDist(r, ch[0], ch[1]) < 6 {
				out = append(out, fmt.Sprintf("%s at %v", c.s, c.at))
				break
			}
		}
	}
	return out
}

// Every ×N keeps 6px off every arrowhead, so a count beside a drop stands
// up the drop from the head it ends in, save these, whose stretch leaves no
// such room clear of the other lines. A count that comes clear fails too,
// so its entry goes.
func TestCountsKeepOffArrowheads(t *testing.T) {
	left := map[string][]string{
		"orca": {"MOD-0072 desktop renderer UI: ×5 at {661 214}"},
		"pstack": {"overview: ×2 at {66 104}", "overview: ×2 at {294 104}", "overview: ×5 at {607 104}",
			"overview: ×5 at {451 280}", "overview: ×3 at {204 456}"},
	}
	for _, name := range fixtures {
		units, err := store.Open(filepath.Join("testdata", name)).LoadUnits()
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, v := range views(Build(Input{Units: units})) {
			var b bytes.Buffer
			if err := site.ExecuteTemplate(&b, "drawing", v.d); err != nil {
				t.Fatal(err)
			}
			for _, c := range countsByHeads(b.String()) {
				got = append(got, v.name+": "+c)
			}
		}
		if !slices.Equal(got, left[name]) {
			t.Errorf("%s: counts by arrowheads %q, want %q", name, got, left[name])
		}
	}
}
