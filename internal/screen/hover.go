package screen

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/whyjp/abbd/internal/model"
)

// A dense view is a braid however it is laid out, so the page lets a reader
// follow one box. While the pointer rests on a box or a frame chip, or a
// box that is a link has the keyboard's focus, every line with no end there
// fades, and so does every box and chip no relation joins to it. The page holds no
// script, so this is plain CSS. Each drawing numbers its shapes. A shape
// carries pN for its own number and nN for itself and each shape a relation
// of the view joins it to. A line or a dot carries eN for each shape at an
// end of a line it carries. One fixed set of rules (hoverCSS) then serves
// every drawing.

// hoverMax is how many shapes a drawing may hold and still take part. A
// twin counts with its unit, and a chip that stands for several ports
// counts once. A drawing with more takes no part at all, since a shape the
// rules cannot name would fade whatever was pointed at. The fixtures'
// largest drawing holds 35, and a test fails when a real drawing holds
// more than this.
const hoverMax = 48

func (d *Drawing) hover() {
	at := map[string]int{}
	n := 0
	number := func(keys ...string) {
		if len(keys) == 0 {
			return
		}
		if _, ok := at[keys[0]]; ok {
			return
		}
		for _, k := range keys {
			at[k] = n
		}
		n++
	}
	for _, b := range d.Boxes {
		number(string(base(model.ID(b.ID))))
	}
	for _, p := range d.Ports {
		number(p.Keys...)
	}
	if n > hoverMax {
		return
	}
	num := func(id string) (int, bool) {
		j, ok := at[string(base(model.ID(id)))]
		return j, ok
	}
	near := map[int][]int{}
	join := func(a, b int) {
		if !slices.Contains(near[a], b) {
			near[a] = append(near[a], b)
		}
	}
	for _, r := range d.Relations {
		a, ok1 := num(r[0])
		b, ok2 := num(r[1])
		if ok1 && ok2 {
			join(a, b)
			join(b, a)
		}
	}
	classes := func(ends []string) []string {
		var is []int
		for _, e := range ends {
			if j, ok := num(e); ok && !slices.Contains(is, j) {
				is = append(is, j)
			}
		}
		slices.Sort(is)
		var cls []string
		for _, j := range is {
			cls = append(cls, fmt.Sprint("e", j))
		}
		return cls
	}
	for i := range d.Wires {
		d.Wires[i].Lit = strings.Join(classes(d.Wires[i].ends), " ")
	}
	for i := range d.Routes {
		d.Routes[i].Lit = strings.Join(classes(d.Routes[i].ends), " ")
	}
	for i := range d.Dots {
		dot := pt{d.Dots[i].X, d.Dots[i].Y}
		var ends []string
		for _, w := range d.Wires {
			if hue(w.Kind) == d.Dots[i].Kind && through(w.D, dot) {
				ends = append(ends, w.ends...)
			}
		}
		d.Dots[i].Lit = strings.Join(classes(ends), " ")
	}
	lit := func(id string) string {
		j, ok := num(id)
		if !ok {
			return ""
		}
		ks := append([]int{j}, near[j]...)
		slices.Sort(ks)
		ks = slices.Compact(ks)
		cls := []string{fmt.Sprint("p", j)}
		for _, k := range ks {
			cls = append(cls, fmt.Sprint("n", k))
		}
		return strings.Join(cls, " ")
	}
	for i := range d.Boxes {
		d.Boxes[i].Lit = lit(d.Boxes[i].ID)
	}
	for i := range d.Ports {
		if len(d.Ports[i].Keys) > 0 {
			d.Ports[i].Lit = lit(d.Ports[i].Keys[0])
		}
	}
}

// route adds each request's route along its way. title says what the
// request's line stands for.
func (d *Drawing) route(r routing, reqs []wireReq, title func(int) string) {
	for i, way := range r.ways {
		src := reqs[i].from.node
		if src == "" {
			src = reqs[i].to.node
		}
		d.Routes = append(d.Routes, Route{D: polyline(way), Kind: reqs[i].kind, Title: title(i), Back: r.back[i], Both: reqs[i].kind == "mutual", src: src, ends: endsOf(reqs, []int{i})})
	}
}

// through says the path d, written as polyline writes it, passes p.
func through(d string, p pt) bool {
	fs := strings.Fields(d)
	var last pt
	for i := 0; i+1 < len(fs); i += 2 {
		x, _ := strconv.Atoi(fs[i][1:])
		y, _ := strconv.Atoi(fs[i+1])
		q := pt{x, y}
		if fs[i][0] == 'L' && on(p, last, q) {
			return true
		}
		last = q
	}
	return false
}

// hoverCSS are the rules that light what a pointed-at shape or line joins.
// A lit line shows as its route, whole and above the ink, and all the ink
// fades, so no piece of another line's colour shows through. A pointed-at
// box colours every line it lights its own colour, except a line that skips
// the contract boundary, which keeps the warning colour; a pointed-at line
// or chip keeps the line's own. Hover takes part only where a pointer can
// rest, so a tap on a phone does not leave a view faded, and focus only
// when it came from the keyboard, so a click that opens a focus view does
// not leave V faded on the way back.
var hoverCSS = func() string {
	var b strings.Builder
	keys := slices.Sorted(maps.Keys(inks()))
	rules := func(on string) {
		for j := range hoverMax {
			a := fmt.Sprintf(on, j, "")
			fmt.Fprintf(&b, "svg:has(%s) :is(.w,.junction){opacity:.1}\n", a)
			fmt.Fprintf(&b, "svg:has(%s) :is(.count,.lab,.origin):not(.e%d){opacity:.1}\n", a, j)
			fmt.Fprintf(&b, "svg:has(%s) :is(.box,.port[class*=\" n\"]):not(.n%d){opacity:.3}\n", a, j)
			fmt.Fprintf(&b, "svg:has(%s) .route.e%d{opacity:1}\n", a, j)
			for _, k := range keys {
				box := fmt.Sprintf(on, j, fmt.Sprintf("[data-h=%q]", k))
				fmt.Fprintf(&b, "svg:has(%s) .route.e%d:not(.bypass,.mutual){--c:var(--h%s)}\n", box, j, k)
			}
		}
	}
	b.WriteString("@media (hover:hover){\n")
	rules(".p%d%s:hover")
	b.WriteString("svg:has(.route:hover) :is(.w,.count,.lab,.origin,.junction){opacity:.1}\n")
	b.WriteString("svg:has(.route:hover) :is(.box,.port[class*=\" n\"]){opacity:.3}\n")
	for j := range hoverMax {
		fmt.Fprintf(&b, "svg:has(.route.e%d:hover) .p%d{opacity:1}\n", j, j)
	}
	b.WriteString("}\n")
	rules(":focus-visible>.p%d%s")
	return b.String()
}()
