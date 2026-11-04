package screen

import (
	"fmt"
	"html/template"
	"slices"
	"strings"

	"github.com/whyjp/abbd/internal/model"
)

// A box's colour names it among the boxes drawn with it, and a line wears
// the colour of the box it leaves (rule 11). The colours come from one
// table, so the page, the checker and the contrast test read the same
// values.

// swatch is one colour as the page draws it on a light page and on a dark one.
type swatch struct{ name, light, dark string }

// palette holds colours a reader with a colour vision deficiency still
// tells apart by lightness as well as by hue, each at least 3:1 against
// the page and the box fill in both themes, its climbing tint included
// (hues_test.go).
var palette = []swatch{
	{"blue", "#1a62a3", "#6cb0f0"},
	{"vermilion", "#ad410a", "#f29061"},
	{"teal", "#08705e", "#3fc4a3"},
	{"magenta", "#b3337c", "#ee8cc4"},
	{"purple", "#6a4cc4", "#b29df5"},
	{"ochre", "#7d5900", "#d9b040"},
}

// neutral is the colour of ink whose lines leave boxes of different
// colours, such as an arrival trunk several places share, and of a line
// that leaves only a frame's chip and reaches no box.
var neutral = swatch{"neutral", "#5f5b53", "#aca89d"}

// backMix is how much of its own colour a climbing line keeps, mixed with
// the page (rule 13).
const backMix = 70

// hueKey names a palette index the way the page's classes do: its index,
// or n for neutral.
func hueKey(i int) string {
	if i < 0 {
		return "n"
	}
	return fmt.Sprint(i)
}

var hueCSS = func() string {
	vars := func(dark bool) string {
		var b strings.Builder
		for i := -1; i < len(palette); i++ {
			h := neutral
			if i >= 0 {
				h = palette[i]
			}
			c := h.light
			if dark {
				c = h.dark
			}
			fmt.Fprintf(&b, "--h%s: %s; ", hueKey(i), c)
		}
		return b.String()
	}
	var b strings.Builder
	fmt.Fprintf(&b, ":root { %s}\n", vars(false))
	fmt.Fprintf(&b, "@media (prefers-color-scheme: dark) { :root:not([data-theme=\"light\"]) { %s} }\n", vars(true))
	fmt.Fprintf(&b, ":root[data-theme=\"dark\"] { %s}\n", vars(true))
	for i := -1; i < len(palette); i++ {
		fmt.Fprintf(&b, "svg [data-h=\"%s\"] { --c: var(--h%s); }\n", hueKey(i), hueKey(i))
	}
	return b.String()
}()

// colours gives every unit a palette index. Units that share a parent are
// drawn together, in their parent's view and its focus views, so each takes
// the colour its siblings joined to it by a relation use least, and among
// those the colour fewest of all its siblings use, so the colours spread.
// Siblings take their colour in ID order, so a state always draws the same
// colours. A unit keeps its colour in every view that draws it.
func (t tree) colours() map[model.ID]int {
	out := map[model.ID]int{}
	parents := make([]model.ID, 0, len(t.children))
	for p := range t.children {
		parents = append(parents, p)
	}
	slices.Sort(parents)
	for _, p := range parents {
		kids := t.children[p]
		near := map[model.ID]map[model.ID]bool{}
		for _, c := range kids {
			near[c] = map[model.ID]bool{}
		}
		for _, c := range kids {
			for _, s := range t.subtree(c) {
				for _, r := range t.units[s].Relations {
					if _, live := t.units[r.To]; !live || r.Dead {
						continue
					}
					if b, ok := t.within(r.To, p); ok && b != c && near[b] != nil {
						near[c][b], near[b][c] = true, true
					}
				}
			}
		}
		used := make([]int, len(palette))
		for _, c := range kids {
			clash := make([]int, len(palette))
			for b := range near[c] {
				if i, ok := out[b]; ok {
					clash[i]++
				}
			}
			best := 0
			for i := range palette {
				if clash[i] < clash[best] || clash[i] == clash[best] && used[i] < used[best] {
					best = i
				}
			}
			out[c] = best
			used[best]++
		}
	}
	return out
}

// head draws the arrowhead a marker key names: its shape, then its colour.
func head(key string) template.HTML {
	shape, h, _ := strings.Cut(key, "-")
	c := "var(--h" + h + ")"
	switch shape {
	case "dep":
		return template.HTML(`<path d="M1 1 L9 5 L1 9" style="fill: none; stroke: ` + c + `; stroke-width: 1.6"/>`)
	case "impl":
		return template.HTML(`<path d="M1 1 L9 5 L1 9 z" style="fill: var(--bg); stroke: ` + c + `; stroke-width: 1.4"/>`)
	}
	return template.HTML(`<path d="M0 0 L10 5 L0 10 z" style="fill: ` + c + `"/>`)
}
