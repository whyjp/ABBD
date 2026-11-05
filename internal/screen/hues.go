package screen

import (
	"fmt"
	"html/template"
	"maps"
	"slices"
	"strings"

	"github.com/whyjp/abbd/internal/model"
)

// A box's colour names it among the boxes drawn with it, and a line wears
// the colour of the box it leaves, except a line that skips the contract
// boundary, which wears the warning colour (rule 11). The colours come
// from one table, so the page, the checker and the contrast test read the
// same values.

// swatch is one colour as the page draws it on a light page and on a dark one.
type swatch struct{ name, light, dark string }

// palette holds six colours, each at least 3:1 against the page and the box
// fill in both themes, its climbing tint included, at least 25 apart from
// one another and from neutral, and at least 30 from the warning colour for
// a reader with either common colour vision deficiency too (hues_test.go).
// Orange, ochre and green are left out, since that reader sees them as the
// warning.
var palette = []swatch{
	{"blue", "#1a62a3", "#6cb0f0"},
	{"teal", "#08705e", "#3fc4a3"},
	{"magenta", "#b3337c", "#ee8cc4"},
	{"purple", "#6a4cc4", "#b29df5"},
	{"cyan", "#00708a", "#4cc3df"},
	{"orchid", "#af2caf", "#d454d4"},
}

// neutral is the colour of ink whose lines leave boxes of different
// colours, such as an arrival trunk several places share, and of a line
// that leaves only a frame's chip and reaches no box.
var neutral = swatch{"neutral", "#5f5b53", "#aca89d"}

// warning is the colour of a direct dependency that skips the contract
// boundary the overview shows, the one line whose colour says what it is.
var warning = swatch{"orange", "#b93e0a", "#fb923c"}

// warnKey names the warning colour the way the page's classes do.
const warnKey = "b"

// backMix is how much of its own colour a climbing line keeps, mixed with
// the page (rule 13), and borderMix how much of its colour a box's border
// keeps, mixed with the box fill, so the bar beside it reads first.
const backMix, borderMix = 70, 80

// hueKey names a palette index the way the page's classes do: its index,
// or n for neutral.
func hueKey(i int) string {
	if i < 0 {
		return "n"
	}
	return fmt.Sprint(i)
}

// inks are every colour a line or a border takes, by the key the page
// names it with.
func inks() map[string]swatch {
	out := map[string]swatch{hueKey(-1): neutral, warnKey: warning}
	for i, h := range palette {
		out[hueKey(i)] = h
	}
	return out
}

var hueCSS = func() string {
	keys := slices.Sorted(maps.Keys(inks()))
	vars := func(dark bool) string {
		var b strings.Builder
		for _, k := range keys {
			c := inks()[k].light
			if dark {
				c = inks()[k].dark
			}
			fmt.Fprintf(&b, "--h%s: %s; ", k, c)
		}
		return b.String()
	}
	var b strings.Builder
	fmt.Fprintf(&b, ":root { %s}\n", vars(false))
	fmt.Fprintf(&b, "@media (prefers-color-scheme: dark) { :root:not([data-theme=\"light\"]) { %s} }\n", vars(true))
	fmt.Fprintf(&b, ":root[data-theme=\"dark\"] { %s}\n", vars(true))
	for _, k := range keys {
		fmt.Fprintf(&b, "svg [data-h=\"%s\"] { --c: var(--h%s); }\n", k, k)
	}
	return b.String()
}()

// drawings are every view of the model, the focus views included.
func (m *Model) drawings() []*Drawing {
	out := []*Drawing{&m.Overview}
	for i := range m.Focus {
		out = append(out, &m.Focus[i].Drawing)
	}
	for i := range m.Insides {
		out = append(out, &m.Insides[i].Drawing)
		for j := range m.Insides[i].Focus {
			out = append(out, &m.Insides[i].Focus[j].Drawing)
		}
	}
	return out
}

// coloured says a box takes a unit's colour: a contract drawn as a box
// because no module provides it in the view stands for no unit's colour.
func coloured(b Box) bool { return !strings.Contains(" "+b.Class+" ", " loose ") }

// colours gives every unit drawn as a box one palette index for every view.
// Two units are joined when some view draws a line between their boxes,
// through a contract's chip or not, and joined units take different
// colours wherever the palette allows. The units are coloured most
// constrained first: the one whose joined units already hold the most
// colours, then the one joined to the most, then by ID, so a state always
// draws the same colours. Each takes the colour fewest of its joined units
// hold, and among those the colour its siblings use least, so the colours
// spread over the boxes one view draws.
func colours(ds []*Drawing, t tree) map[model.ID]int {
	near := map[model.ID]map[model.ID]bool{}
	for _, d := range ds {
		boxes := map[string]bool{}
		for _, b := range d.Boxes {
			if coloured(b) {
				boxes[b.ID] = true
				if near[base(model.ID(b.ID))] == nil {
					near[base(model.ID(b.ID))] = map[model.ID]bool{}
				}
			}
		}
		for _, r := range d.Relations {
			a, b := base(model.ID(r[0])), base(model.ID(r[1]))
			if boxes[r[0]] && boxes[r[1]] && a != b {
				near[a][b], near[b][a] = true, true
			}
		}
	}
	ids := make([]model.ID, 0, len(near))
	for id := range near {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	out := map[model.ID]int{}
	used := map[model.ID][]int{}
	for range ids {
		var next model.ID
		sat, deg := -1, -1
		for _, id := range ids {
			if _, done := out[id]; done {
				continue
			}
			held := map[int]bool{}
			for b := range near[id] {
				if i, ok := out[b]; ok {
					held[i] = true
				}
			}
			if len(held) > sat || len(held) == sat && len(near[id]) > deg {
				next, sat, deg = id, len(held), len(near[id])
			}
		}
		clash := make([]int, len(palette))
		for b := range near[next] {
			if i, ok := out[b]; ok {
				clash[i]++
			}
		}
		p := t.units[next].Parent
		if used[p] == nil {
			used[p] = make([]int, len(palette))
		}
		best := 0
		for i := range palette {
			if clash[i] < clash[best] || clash[i] == clash[best] && used[p][i] < used[p][best] {
				best = i
			}
		}
		out[next] = best
		used[p][best]++
	}
	return out
}

// head draws the arrowhead a marker key names: its shape, then its colour.
func head(key string) template.HTML {
	shape, h, _ := strings.Cut(key, "-")
	return headIn(shape, "var(--h"+h+")")
}

// routeHead draws a route's arrowhead in the colour of the route it ends,
// which a pointed-at box sets, so the head always matches its line.
func routeHead(shape string) template.HTML { return headIn(shape, "context-stroke") }

func headIn(shape, c string) template.HTML {
	switch shape {
	case "dep":
		return template.HTML(`<path d="M1 1 L9 5 L1 9" style="fill: none; stroke: ` + c + `; stroke-width: 1.6"/>`)
	case "impl":
		return template.HTML(`<path d="M1 1 L9 5 L1 9 z" style="fill: var(--bg); stroke: ` + c + `; stroke-width: 1.4"/>`)
	}
	return template.HTML(`<path d="M0 0 L10 5 L0 10 z" style="fill: ` + c + `"/>`)
}
