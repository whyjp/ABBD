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
// page, the box fill and its own tinted box fill in both themes, and so
// does its climbing tint against the page (rule 11, rule 13).
func TestEveryHueReadsOnBothThemes(t *testing.T) {
	for _, mix := range []string{fmt.Sprintf("var(--c) %d%%, var(--bg)", backMix), fmt.Sprintf("var(--c) %d%%, var(--panel)", tintMix)} {
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
				"tinted fill":   contrast(col, mixed(col, panel, tintMix)),
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
	t.Parallel()
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

// A direct dependency that skips the contract boundary wears the colour
// of the box it leaves, like every other line, so a view where most lines
// are such dependencies still tells its lines apart at rest; its open head
// says what it is. No line or dot wears the warning colour.
func TestABypassWearsItsBoxsColour(t *testing.T) {
	t.Parallel()
	bypasses := 0
	for _, name := range fixtures {
		m := fixture(t, name)
		for _, d := range m.drawings() {
			for _, w := range d.Wires {
				if w.Hue == warnKey {
					t.Errorf("%s: a %s line wears the warning colour", name, w.Kind)
				}
				if warns(w.Kind) {
					bypasses++
					if !w.Joins && w.Head != "" && !strings.HasPrefix(w.Head, "dep-") {
						t.Errorf("%s: a %s line ends in %q, not an open head", name, w.Kind, w.Head)
					}
				}
			}
			for _, j := range d.Dots {
				if j.Hue == warnKey {
					t.Errorf("%s: a %s dot wears the warning colour", name, j.Kind)
				}
			}
		}
	}
	if bypasses == 0 {
		t.Error("no fixture view draws a bypass")
	}
}

// A line wears the colour of the box it leaves. Ink whose lines leave boxes
// of different colours wears one of those colours on a road, the colour of
// its rider that runs furthest on roads, and neutral grey elsewhere (rule 11).
func TestALineWearsItsBoxsColour(t *testing.T) {
	t.Parallel()
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
				want, theirs := "", map[string]bool{}
				for _, n := range w.srcs {
					c := hue[n]
					if c == "" {
						c = hueKey(-1)
					}
					theirs[c] = true
					if want == "" {
						want = c
					} else if c != want {
						want = hueKey(-1)
					}
				}
				if want == hueKey(-1) && len(w.rides) > 0 && theirs[w.Hue] {
					continue
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
	t.Parallel()
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
	t.Parallel()
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

// A chip on a box wears that box's colour, so a module's contracts read as
// its own and never in the accent blue beside it; a frame's chip, on no
// box, keeps the accent.
func TestAChipWearsItsBoxsColour(t *testing.T) {
	t.Parallel()
	chips := 0
	for _, name := range fixtures {
		m := fixture(t, name)
		for _, d := range m.drawings() {
			for _, p := range d.Ports {
				x, y := p.X+p.W/2, p.RectY()+p.H/2
				want := ""
				for _, b := range d.Boxes {
					if b.X <= x && x <= b.X+b.W && b.Y <= y && y <= b.Y+b.H {
						want = b.Hue
					}
				}
				if p.Hue != want {
					t.Errorf("%s: chip %q wears %q, its box %q", name, p.Title, p.Hue, want)
				}
				if p.Hue != "" {
					chips++
				}
			}
		}
	}
	if chips == 0 {
		t.Error("no chip wears a box's colour")
	}
}

// An exit chip on a box is tinted 15% toward its box's colour and written
// in that colour drawn 20% toward the ink, so it reads as the box's without
// the solid pill the exemplars keep for the one hub. Its words are 11px, so
// they stand at least 4.5:1 on that tint in both themes.
func TestAChipsWordsReadOnItsTint(t *testing.T) {
	for _, rule := range []string{"var(--c) 15%, var(--panel)", "var(--c) 80%, var(--ink)"} {
		if !strings.Contains(pageCSS, rule) {
			t.Fatalf("page.css mixes no exit chip colour as %s", rule)
		}
	}
	panel := map[string]string{"light": "#ffffff", "dark": "#201f1c"}
	ink := map[string]string{"light": "#1c1c1a", "dark": "#ecebe6"}
	for _, h := range palette {
		for theme, c := range map[string]string{"light": h.light, "dark": h.dark} {
			col := parseHex(t, c)
			words, tint := mixed(col, parseHex(t, ink[theme]), 80), mixed(col, parseHex(t, panel[theme]), 15)
			if r := contrast(words, tint); r < 4.5 {
				t.Errorf("%s %s: a chip's words read %.2f:1 on its tint, under 4.5:1", h.name, theme, r)
			}
		}
	}
}

// A hub's view whose own lines are under half its ink stays in colour:
// it is not quiet, and each line keeps its box's colour, so it reads at
// rest. The self overview drawn whole and orca's desktop main are such
// views; cut, the self overview's hub owns most of what is left.
func TestABusyHubViewStaysInColour(t *testing.T) {
	t.Parallel()
	cases := map[string]string{"self": "", "orca": "MOD-0042"}
	for name, inside := range cases {
		m := fixture(t, name)
		if inside == "" {
			m = drawAll(t, name)
		}
		d := &m.Overview
		if inside != "" {
			for i := range m.Insides {
				if m.Insides[i].ID == inside {
					d = &m.Insides[i].Drawing
				}
			}
		}
		f := d.Filled()
		if f == nil || f.Role != "hub" {
			t.Fatalf("%s %s: no hub to test", name, inside)
		}
		if own := d.ownLines(); 2*own >= len(d.Wires) {
			t.Fatalf("%s %s: the hub owns %d of %d lines, not under half", name, inside, own, len(d.Wires))
		}
		if d.Quiet {
			t.Errorf("%s %s: a hub owning under half its ink rests the view grey", name, inside)
		}
		hues := map[string]bool{}
		for _, w := range d.Wires {
			if w.Hue != hueKey(-1) {
				hues[w.Hue] = true
			}
		}
		if len(hues) < 2 {
			t.Errorf("%s %s: its lines show %d colours at rest", name, inside, len(hues))
		}
	}
}
