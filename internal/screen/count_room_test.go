package screen

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"slices"
	"strings"
	"testing"
)

// countMargin is how far a ×N keeps from every line but its own bus's, as a
// label keeps labelRoom from every mark.
const countMargin = 2.0

// countsCrowded lists each ×N of a drawing whose glyphs come within
// countMargin of another line's ink, or that any line, its own included,
// runs through.
func countsCrowded(svg string) []string {
	type inkPath struct {
		rels map[string]bool
		pts  [][]fpt
	}
	type count struct {
		at   fpt
		rels map[string]bool
		s    string
	}
	relsOf := func(class string) map[string]bool {
		out := map[string]bool{}
		for _, c := range strings.Fields(class) {
			if len(c) > 1 && c[0] == 'r' && c[1] >= '0' && c[1] <= '9' {
				out[c] = true
			}
		}
		return out
	}
	var paths []inkPath
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
			case e.Name.Local == "path" && hasClass(e, "w"):
				parts, err := flatten(attr(e, "d"))
				if err == nil {
					paths = append(paths, inkPath{relsOf(attr(e, "class")), parts})
				}
			case e.Name.Local == "text" && hasClass(e, "count"):
				counts = append(counts, count{at: fpt{num(attr(e, "x")), num(attr(e, "y"))}, rels: relsOf(attr(e, "class"))})
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
		for _, p := range paths {
			own := false
			for k := range p.rels {
				own = own || c.rels[k]
			}
			room := countMargin
			if own {
				room = 0
			}
			hit := false
			for _, part := range p.pts {
				for i := 1; i < len(part) && !hit; i++ {
					d := rectSegDist(r, part[i-1], part[i])
					hit = d < room || d == 0
				}
			}
			if hit {
				out = append(out, fmt.Sprintf("%s at %v", c.s, c.at))
				break
			}
		}
	}
	return out
}

// fixtureCountsCrowded is every crowded ×N of every fixture's views.
func fixtureCountsCrowded(t *testing.T) map[string][]string {
	t.Helper()
	out := map[string][]string{}
	for _, name := range fixtures {
		for _, v := range views(fixture(t, name)) {
			var b bytes.Buffer
			if err := site.ExecuteTemplate(&b, "drawing", v.d); err != nil {
				t.Fatal(err)
			}
			for _, c := range countsCrowded(b.String()) {
				out[name] = append(out[name], v.name+": "+c)
			}
		}
	}
	return out
}

// Every ×N stands clear of the lines beside its stub or drop, rule 22,
// save these, whose slots have no room to spare without costing the
// drawing more than lint weighs a crowded count. A count that comes clear
// fails too, so its entry goes.
func TestCountsStandClearOfTheNextLine(t *testing.T) {
	t.Parallel()
	left := map[string][]string{
		"orca": {"MOD-0042 focus MOD-0052 desktop-main/ipc: ×12 at {667 285}"},
	}
	got := fixtureCountsCrowded(t)
	for _, name := range fixtures {
		if !slices.Equal(got[name], left[name]) {
			t.Errorf("%s: crowded counts %q, want %q", name, got[name], left[name])
		}
	}
}
