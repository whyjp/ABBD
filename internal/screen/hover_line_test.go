package screen

import (
	"encoding/xml"
	"slices"
	"strings"
	"testing"
)

// A pointed-at line is what a reader follows from one shape to the other,
// so the stroke the pointer lights is the whole line: one unbroken path
// whose two ends touch the boxes or chips it joins, painted over every
// other line so no other ink cuts it. A pointed-at box lights each of its
// lines whole the same way, all in the box's colour, so no lit piece wears
// another line's colour. These tests read the drawing as the page draws
// it, in paint order, and point at every piece of ink and every shape.

// pointable is a stroke the pointer can rest on, in paint order.
type pointable struct {
	class, hue string
	parts      [][]fpt
	// half is how far from its centre line the pointer still lands on it.
	half float64
	// painted is its place in paint order.
	painted int
}

func (s pointable) is(c string) bool { return strings.Contains(" "+s.class+" ", " "+c+" ") }

type pointed struct {
	shapes []shape
	hues   map[string]string
	// strokes are the lines and routes, ink the visible lines.
	strokes, ink []pointable
	// lastInk is the paint order of the last visible line.
	lastInk int
}

// lineLights says pointing at a visible line lights that path itself, as
// the page's rules did before every line had a route.
var lineLights = strings.Contains(pageCSS, "path.w:hover")

// pointables reads the strokes and the shapes. A line wears its stroke
// width as page.css sets it; a route is as wide as the line it lights.
func pointables(t *testing.T, d Drawing) pointed {
	t.Helper()
	var b strings.Builder
	if err := site.ExecuteTemplate(&b, "drawing", d); err != nil {
		t.Fatal(err)
	}
	dec := xml.NewDecoder(strings.NewReader(b.String()))
	out := pointed{hues: map[string]string{}}
	var group *shape
	n := 0
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		e, ok := tok.(xml.StartElement)
		if !ok {
			if end, ok := tok.(xml.EndElement); ok && end.Name.Local == "g" && group != nil {
				out.shapes = append(out.shapes, *group)
				group = nil
			}
			continue
		}
		n++
		switch e.Name.Local {
		case "g":
			for _, k := range []string{"box", "port"} {
				if hasClass(e, k) {
					group = &shape{kind: k, class: attr(e, "class"), id: attr(e, "data-id")}
					out.hues[group.class] = attr(e, "data-h")
				}
			}
		case "rect":
			if group != nil && group.r.w == 0 {
				group.r = frect{num(attr(e, "x")), num(attr(e, "y")), num(attr(e, "width")), num(attr(e, "height"))}
			}
		case "path":
			if !hasClass(e, "w") && !hasClass(e, "route") {
				continue
			}
			parts, err := flatten(attr(e, "d"))
			if err != nil {
				t.Fatal(err)
			}
			s := pointable{class: attr(e, "class"), hue: attr(e, "data-h"), parts: parts, painted: n}
			switch {
			case hasClass(e, "w"):
				out.lastInk = n
				out.ink = append(out.ink, s)
				if !lineLights {
					continue
				}
				s.half = 0.75
			default:
				s.half = 1.5
			}
			out.strokes = append(out.strokes, s)
		}
	}
	return out
}

// under is the stroke painted last of those the pointer lands on at p.
func (pd pointed) under(p fpt) *pointable {
	var top *pointable
	for i := range pd.strokes {
		s := &pd.strokes[i]
		for _, part := range s.parts {
			for k := 1; k < len(part); k++ {
				if segDist(p, part[k-1], part[k]) <= s.half {
					top = s
				}
			}
		}
	}
	return top
}

// touches says p lies on the edge of a shape whose classes include one of
// cls, or just outside it where an arrowhead's tip stops.
func (pd pointed) touches(p fpt, cls []string) bool {
	for _, s := range pd.shapes {
		if s.r.grow(2).contains(p, 0) && !s.r.contains(p, 2) && slices.ContainsFunc(cls, s.is) {
			return true
		}
	}
	return false
}

// joined are the classes of the shapes a stroke's lines join: pN for each
// eN it carries.
func joined(class string) []string {
	var out []string
	for _, c := range strings.Fields(class) {
		if len(c) > 1 && c[0] == 'e' && c[1] >= '0' && c[1] <= '9' {
			out = append(out, "p"+c[1:])
		}
	}
	return out
}

// broken says what is wrong with a stroke the pointer lights, or "".
func (pd pointed) broken(s *pointable) string {
	switch {
	case s == nil:
		return "lights nothing"
	case len(s.parts) != 1:
		return "lights a stroke in pieces"
	case !pd.touches(s.parts[0][0], joined(s.class)) || !pd.touches(s.parts[0][len(s.parts[0])-1], joined(s.class)):
		return "lights a stroke that stops short of the boxes or chips it joins"
	case s.painted < pd.lastInk:
		return "lights a stroke other lines are painted over"
	}
	return ""
}

// overhang is how many pieces of bus ink no line's way runs along, so
// pointing there lights nothing: a track drawn past the last line that
// leaves it. Shortening those tracks moves the layout's weight past its
// ceilings, so it stays a known count that may only fall.
const overhang = 7

// Pointing at any piece of ink lights one unbroken stroke between the two
// boxes or chips its line joins, painted over every line.
func TestPointingAtALineLightsItWholeBetweenItsEnds(t *testing.T) {
	total := 0
	why := map[string]int{}
	for _, name := range fixtures {
		for _, v := range views(fixture(t, name)) {
			pd := pointables(t, v.d)
			for _, s := range pd.ink {
				for _, part := range s.parts {
					for k := 1; k < len(part); k++ {
						a, b := part[k-1], part[k]
						if dist(a, b) < 2 {
							continue
						}
						total++
						if w := pd.broken(pd.under(fpt{(a.x + b.x) / 2, (a.y + b.y) / 2})); w != "" {
							why[w]++
						}
					}
				}
			}
		}
	}
	t.Logf("pointed at %d pieces of ink: %v", total, why)
	if why["lights nothing"] > overhang {
		t.Errorf("%d pieces of ink light nothing, more than the %d known", why["lights nothing"], overhang)
	}
	delete(why, "lights nothing")
	if len(why) > 0 {
		t.Errorf("pieces of ink light a stroke that is not one whole line between its ends: %v", why)
	}
}

// Pointing at a box or chip lights every line it has an end on whole,
// between that shape and the line's other end, over every line, and in
// one colour: a box's own, or the warning colour for a line that skips the
// contract boundary; a chip, which wears no unit's colour, leaves each
// line its own. No other ink stays lit to show another colour.
func TestPointingAtAShapeLightsItsLinesWholeInOneColour(t *testing.T) {
	shapes, lines := 0, 0
	why := map[string]int{}
	for _, name := range fixtures {
		for _, v := range views(fixture(t, name)) {
			pd := pointables(t, v.d)
			for _, sh := range pd.shapes {
				own := ""
				for _, c := range strings.Fields(sh.class) {
					if len(c) > 1 && c[0] == 'p' && c[1] >= '0' && c[1] <= '9' {
						own = c
					}
				}
				if own == "" {
					continue
				}
				shapes++
				want := pd.hues[sh.class]
				lit := func(s pointable) bool { return slices.Contains(joined(s.class), own) }
				for _, s := range pd.strokes {
					if !lit(s) {
						continue
					}
					lines++
					if w := pd.broken(&s); w != "" {
						why[w]++
					}
					if c := lightIn(s, want); want != "" && c != want && c != warnKey {
						why["lights a line in another colour than the box's"]++
					}
				}
			}
		}
	}
	t.Logf("pointed at %d shapes, which light %d strokes: %v", shapes, lines, why)
	if len(why) > 0 {
		t.Errorf("shapes light lines that are not whole or not in one colour: %v", why)
	}
}

// lightIn is the colour a stroke takes while the box of colour box is
// pointed at, as hoverCSS sets it.
func lightIn(s pointable, box string) string {
	if s.is("route") && box != "" && !s.is("bypass") && !s.is("mutual") && strings.Contains(hoverCSS, ":not(.bypass,.mutual){--c:") {
		return box
	}
	return s.hue
}
