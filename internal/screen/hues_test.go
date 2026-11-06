package screen

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/whyjp/abbd/internal/model"
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
	for _, mix := range []string{fmt.Sprintf("var(--c) %d%%, var(--bg)", backMix), fmt.Sprintf("var(--c) %d%%, var(--panel)", borderMix)} {
		if !strings.Contains(pageCSS, mix) {
			t.Errorf("page.css mixes no colour as %s", mix)
		}
	}
	pages := map[string][2]string{"light": {"#f6f5f1", "#ffffff"}, "dark": {"#161614", "#201f1c"}}
	for _, h := range inks() {
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
				"border tint":   contrast(mixed(col, panel, borderMix), page),
			} {
				if r < 3 {
					t.Errorf("%s on the %s %s: %.2f:1, under 3:1", h.name, theme, what, r)
				}
			}
		}
	}
}

// cvd are Machado, Oliveira and Fernandes' (2009) matrices for a full
// protanopia and deuteranopia, which act on linear sRGB.
var cvd = map[string][3][3]float64{
	"normal vision": {{1, 0, 0}, {0, 1, 0}, {0, 0, 1}},
	"protanopia":    {{0.152286, 1.052583, -0.204868}, {0.114503, 0.786281, 0.099216}, {-0.003882, -0.048116, 1.051998}},
	"deuteranopia":  {{0.367322, 0.860646, -0.227968}, {0.280085, 0.672501, 0.047413}, {-0.011820, 0.042940, 0.968881}},
}

// lab is the CIELAB colour a reader with the given vision sees for c.
func lab(c srgb, m [3][3]float64) [3]float64 {
	var l [3]float64
	for i := range l {
		for j := range c {
			v := c[j]
			if v <= 0.04045 {
				v /= 12.92
			} else {
				v = math.Pow((v+0.055)/1.055, 2.4)
			}
			l[i] += m[i][j] * v
		}
		l[i] = min(1, max(0, l[i]))
	}
	x := (0.4124*l[0] + 0.3576*l[1] + 0.1805*l[2]) / 0.95047
	y := 0.2126*l[0] + 0.7152*l[1] + 0.0722*l[2]
	z := (0.0193*l[0] + 0.1192*l[1] + 0.9505*l[2]) / 1.08883
	f := func(t float64) float64 {
		if t > 0.008856 {
			return math.Cbrt(t)
		}
		return 7.787*t + 16.0/116
	}
	return [3]float64{116*f(y) - 16, 500 * (f(x) - f(y)), 200 * (f(y) - f(z))}
}

// The warning colour stays apart from every other colour a line takes, for
// a reader with normal vision and for one with either common colour vision
// deficiency, in both themes: at least 30 apart in CIELAB, where 2.3 is
// the least difference a reader notices (rule 11).
func TestTheWarningStandsApart(t *testing.T) {
	for _, h := range append(slices.Clone(palette), neutral) {
		for _, theme := range []string{"light", "dark"} {
			a, b := h.light, warning.light
			if theme == "dark" {
				a, b = h.dark, warning.dark
			}
			for vision, m := range cvd {
				p, q := lab(parseHex(t, a), m), lab(parseHex(t, b), m)
				if d := math.Hypot(math.Hypot(p[0]-q[0], p[1]-q[1]), p[2]-q[2]); d < 30 {
					t.Errorf("%s is %.1f from the warning colour on the %s page, with %s", h.name, d, theme, vision)
				}
			}
		}
	}
}

// The palette's colours and neutral stay apart from one another for a
// reader with normal vision, in both themes: at least 25 apart in CIELAB
// (rule 11).
func TestTheHuesStandApart(t *testing.T) {
	all := append(slices.Clone(palette), neutral)
	for i, a := range all {
		for _, b := range all[i+1:] {
			for _, theme := range []string{"light", "dark"} {
				x, y := a.light, b.light
				if theme == "dark" {
					x, y = a.dark, b.dark
				}
				p, q := lab(parseHex(t, x), cvd["normal vision"]), lab(parseHex(t, y), cvd["normal vision"])
				if d := math.Hypot(math.Hypot(p[0]-q[0], p[1]-q[1]), p[2]-q[2]); d < 25 {
					t.Errorf("%s and %s are %.1f apart on the %s page", a.name, b.name, d, theme)
				}
			}
		}
	}
}

// sameHue is how many pairs of boxes some view joins by a drawn line,
// through a contract's chip or not, that wear one colour, and how many
// such pairs there are. Each pair counts once in each view that draws it.
func sameHue(m Model) (same, pairs int) {
	for _, d := range m.drawings() {
		hue := map[string]string{}
		for _, b := range d.Boxes {
			if b.Hue != "" {
				hue[b.ID] = b.Hue
			}
		}
		seen := map[[2]string]bool{}
		for _, r := range d.Relations {
			a, b := r[0], r[1]
			if a > b {
				a, b = b, a
			}
			if hue[a] == "" || hue[b] == "" || base(model.ID(a)) == base(model.ID(b)) || seen[[2]string{a, b}] {
				continue
			}
			seen[[2]string{a, b}] = true
			pairs++
			if hue[a] == hue[b] {
				same++
			}
		}
	}
	return same, pairs
}

// sameHueCeiling is how many joined pairs of boxes may share a colour
// across the fixtures. None do with six colours. With five, 26 of 1167
// did, in Orca's and pstack's dense insides.
const sameHueCeiling = 0

// Boxes a drawn line joins wear different colours, a unit wears one colour
// in every view that draws it, and the same state always colours the same
// way (rule 11).
func TestJoinedBoxesWearDifferentColours(t *testing.T) {
	total, all := 0, 0
	for _, name := range fixtures {
		m := fixture(t, name)
		again := fixture(t, name)
		hue := map[model.ID]string{}
		for _, d := range m.drawings() {
			for _, b := range d.Boxes {
				if b.Hue == "" {
					continue
				}
				id := base(model.ID(b.ID))
				if h, ok := hue[id]; ok && h != b.Hue {
					t.Errorf("%s: %s wears %s in one view and %s in another", name, id, h, b.Hue)
				}
				hue[id] = b.Hue
			}
		}
		for _, d := range again.drawings() {
			for _, b := range d.Boxes {
				if b.Hue != "" && b.Hue != hue[base(model.ID(b.ID))] {
					t.Errorf("%s: %s changes colour between two builds", name, b.ID)
				}
			}
		}
		same, pairs := sameHue(m)
		t.Logf("%s: %d of %d joined box pairs share a colour", name, same, pairs)
		total += same
		all += pairs
	}
	t.Logf("%d of %d joined box pairs share a colour across the fixtures", total, all)
	if total > sameHueCeiling {
		t.Errorf("%d joined box pairs share a colour, ceiling %d", total, sameHueCeiling)
	}
}

// A direct dependency that skips the contract boundary is drawn in the
// warning colour, and so is a dot on its ink. No other line or dot is
// (rule 11).
func TestOnlyABypassWearsTheWarning(t *testing.T) {
	for _, name := range fixtures {
		m := fixture(t, name)
		for _, d := range m.drawings() {
			if d.Quiet {
				continue
			}
			for _, w := range d.Wires {
				if warns(w.Kind) != (w.Hue == warnKey) {
					t.Errorf("%s: a %s line wears %q", name, w.Kind, w.Hue)
				}
			}
			for _, j := range d.Dots {
				if warns(j.Kind) != (j.Hue == warnKey) {
					t.Errorf("%s: a %s dot wears %q", name, j.Kind, j.Hue)
				}
			}
		}
	}
}

// A line wears the colour of the box it leaves, or neutral grey where its
// lines leave boxes of different colours (rule 11).
func TestALineWearsItsBoxsColour(t *testing.T) {
	for _, name := range fixtures {
		m := fixture(t, name)
		for _, d := range m.drawings() {
			if d.Quiet {
				continue
			}
			hue := map[string]string{}
			for _, b := range d.Boxes {
				hue[b.ID] = b.Hue
			}
			for _, w := range d.Wires {
				if warns(w.Kind) {
					continue
				}
				want := ""
				for _, n := range w.srcs {
					c := hue[n]
					if c == "" {
						c = hueKey(-1)
					}
					if want == "" {
						want = c
					} else if c != want {
						want = hueKey(-1)
					}
				}
				if want != "" && w.Hue != want {
					t.Errorf("%s: a %s line wears %q, its boxes %q", name, w.Kind, w.Hue, want)
				}
			}
		}
	}
}

// A view that draws a lead draws every other line grey, its dots too, so
// the lead is its one coloured way (lead.go).
func TestALeadQuietsEveryOtherLine(t *testing.T) {
	leads := 0
	for _, name := range fixtures {
		m := fixture(t, name)
		for _, d := range m.drawings() {
			if d.Lead == nil {
				continue
			}
			leads++
			for _, w := range d.Wires {
				if w.Hue != hueKey(-1) {
					t.Errorf("%s: a %s line beside a lead wears %q", name, w.Kind, w.Hue)
				}
			}
			if !d.Quiet {
				t.Errorf("%s: a view with a lead is not drawn quiet", name)
			}
			for _, j := range d.Dots {
				if j.Hue != hueKey(-1) {
					t.Errorf("%s: a dot beside a lead wears %q", name, j.Hue)
				}
			}
		}
	}
	if leads == 0 {
		t.Error("no fixture view draws a lead")
	}
}

// A view with a hub and no lead rests every line grey but the hub's own,
// which wear the hub's colour (dress.go).
func TestAHubKeepsOnlyItsOwnLinesInColour(t *testing.T) {
	hubs := 0
	for _, name := range fixtures {
		m := fixture(t, name)
		for _, d := range m.drawings() {
			f := d.Filled()
			if d.Lead != nil || f == nil || f.Role != "hub" || !d.Quiet {
				continue
			}
			hubs++
			for _, w := range d.Wires {
				own := true
				for _, k := range classNumbers(w.Lit, "r") {
					own = own && slices.Contains(d.Routes[k].ends, f.ID)
				}
				own = own && len(classNumbers(w.Lit, "r")) > 0
				want := map[bool]string{true: f.Hue, false: hueKey(-1)}[own]
				if w.Hue != want {
					t.Errorf("%s: a %s line wears %q, want %q", name, w.Kind, w.Hue, want)
				}
			}
		}
	}
	if hubs == 0 {
		t.Error("no fixture view draws a hub")
	}
}
