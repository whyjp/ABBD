package screen

import (
	"fmt"
	"math"
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
// end of a line it carries. Each line's route carries rK for its own number,
// and each piece of ink rK for every route that runs along it. One fixed set
// of rules (hoverCSS) then serves every drawing.

// hoverMax is how many shapes a drawing may hold and still take part. A
// twin counts with its unit, and a chip that stands for several ports
// counts once. A drawing with more takes no part at all, since a shape the
// rules cannot name would fade whatever was pointed at. The fixtures'
// largest drawing holds 35, and a test fails when a real drawing holds
// more than this.
const hoverMax = 48

// routeMax is how many lines a drawing may route and still take part,
// since each line has its own rule that lights it. A drawing past it keeps
// its lines on roads but lights none of them, so every join its roads
// draw reads as a coupling (J3). The fixtures' largest drawing routes 79
// and vector's vector-lib 98; each rule adds about 50 bytes to a page.
// A test fails when a real drawing routes more.
const routeMax = 160

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
	d.ways = d.Routes
	if n > hoverMax || len(d.Routes) > routeMax {
		d.Routes = nil
		return
	}
	d.numbers = at
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
	nums := func(ends []string) []int {
		var is []int
		for _, e := range ends {
			if j, ok := num(e); ok && !slices.Contains(is, j) {
				is = append(is, j)
			}
		}
		slices.Sort(is)
		return is
	}
	classes := func(is, ks []int) string {
		var cls []string
		for _, j := range is {
			cls = append(cls, fmt.Sprint("e", j))
		}
		for _, k := range ks {
			cls = append(cls, fmt.Sprint("r", k))
		}
		return strings.Join(cls, " ")
	}
	for k := range d.Routes {
		r := &d.Routes[k]
		r.at = nums(r.ends)
		r.Lit = classes(r.at, []int{k})
	}
	// A piece of ink lights every route of its kind that runs along it, and
	// keeps lit the shapes at their ends: ink of another kind on the same
	// track is another line.
	for i := range d.Wires {
		w := &d.Wires[i]
		ink := runs(w.D)
		is := nums(w.ends)
		var ks []int
		for k := range d.Routes {
			// A road lights the routes that ride it, whatever their kind;
			// other ink the routes of its kind that run along it.
			if rides := w.rides != nil; rides && slices.Contains(w.rides, k) || !rides && d.Routes[k].Kind == w.Kind && alongside(ink, d.Routes[k].runs) {
				ks = append(ks, k)
				is = append(is, d.Routes[k].at...)
			}
		}
		slices.Sort(is)
		w.Lit = classes(slices.Compact(is), ks)
	}
	for i := range d.Dots {
		dot := pt{d.Dots[i].X, d.Dots[i].Y}
		var ends []string
		for _, w := range d.Wires {
			if w.inkKey() == d.Dots[i].Kind && through(w.D, dot) {
				ends = append(ends, w.ends...)
			}
		}
		d.Dots[i].Lit = classes(nums(ends), nil)
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
		p := &d.Ports[i]
		if len(p.Keys) > 0 {
			p.Lit = lit(p.Keys[0])
			continue
		}
		// A contract chip on a box is part of it: it dims and lights with
		// its box, and pointing at it points at the box.
		x, y := p.X+p.W/2, p.RectY()+p.H/2
		for _, b := range d.Boxes {
			if b.X <= x && x <= b.X+b.W && b.Y <= y && y <= b.Y+b.H {
				p.Lit = b.Lit
			}
		}
	}
}

// route adds each request's route along its way, with an arrowhead at its
// end, and at its start too when the two boxes use each other.
func (d *Drawing) route(r routing, reqs []wireReq, rels func(int) []string) {
	for i, way := range r.ways {
		src := reqs[i].from.node
		if src == "" {
			src = reqs[i].to.node
		}
		ps := straight(way)
		rt := Route{D: polyline(ps), Kind: reqs[i].kind, Back: r.back[i], Both: reqs[i].kind == "mutual", Head: shapes[reqs[i].kind], src: src, ends: endsOf(reqs, []int{i}), rels: rels(i)}
		rt.runs = runs(rt.D)
		if rt.Head == "" {
			rt.Head = "use"
		}
		if k := len(ps); k >= 2 {
			rt.Tips = append(rt.Tips, tip(ps[k-1], ps[k-2]))
			if rt.Both {
				rt.Tips = append(rt.Tips, tip(ps[0], ps[1]))
			}
		}
		d.Routes = append(d.Routes, rt)
	}
}

// tip places an arrowhead as a marker would: its point on p, its back
// toward from.
func tip(p, from pt) string {
	return fmt.Sprintf("translate(%d %d) rotate(%g)", p.x, p.y, math.Atan2(float64(p.y-from.y), float64(p.x-from.x))*180/math.Pi)
}

// runs are the straight pieces of a path written as polyline writes it.
func runs(d string) [][2]pt {
	fs := strings.Fields(d)
	var out [][2]pt
	var last pt
	for i := 0; i+1 < len(fs); i += 2 {
		x, _ := strconv.Atoi(fs[i][1:])
		y, _ := strconv.Atoi(fs[i+1])
		q := pt{x, y}
		if fs[i][0] == 'L' {
			out = append(out, [2]pt{last, q})
		}
		last = q
	}
	return out
}

// alongside says some piece of a runs along some piece of b for a length,
// not only across it.
func alongside(a, b [][2]pt) bool {
	for _, p := range a {
		for _, q := range b {
			if overlap(p, q) > 0 {
				return true
			}
		}
	}
	return false
}

// overlap is how far the pieces p and q run along each other. Lines run
// only across and down.
func overlap(p, q [2]pt) int {
	switch {
	case p[0].y == p[1].y && q[0].y == q[1].y && p[0].y == q[0].y:
		return min(max(p[0].x, p[1].x), max(q[0].x, q[1].x)) - max(min(p[0].x, p[1].x), min(q[0].x, q[1].x))
	case p[0].x == p[1].x && q[0].x == q[1].x && p[0].x == q[0].x:
		return min(max(p[0].y, p[1].y), max(q[0].y, q[1].y)) - max(min(p[0].y, p[1].y), min(q[0].y, q[1].y))
	}
	return 0
}

// through says the path d, written as polyline writes it, passes p.
func through(d string, p pt) bool {
	return slices.ContainsFunc(runs(d), func(r [2]pt) bool { return on(p, r[0], r[1]) })
}

// hoverCSS are the rules that light what a pointed-at shape or line joins.
// A lit line shows as its route, whole and above the ink, and all the ink
// fades, so no piece of another line's colour shows through. A pointed-at
// shape lights each line with an end there in the colour the line's --kN
// names for it (paint): a box's own, whatever the line's kind, which its
// dash and arrowhead still show. Pointing at ink lights every line that
// runs along it in its own colour, with its marks and the shapes at its
// ends, and the ink keeps its title. Every rule takes part only where a pointer can rest, so
// a tap on a phone does not leave a view faded, and focus only when it came
// from the keyboard, so a click that opens a focus view does not leave V
// faded on the way back.
var hoverCSS = func() string {
	var b strings.Builder
	b.WriteString("@media (hover:hover){\n")
	shape := `:is(.box,.port)[class*=" n"]`
	pointed := fmt.Sprintf("svg:has(.route):has(%s:hover,:focus-visible>%s,.w:hover)", shape, shape)
	fmt.Fprintf(&b, "%s :where(.w,.junction,.count,.lab,.origin,.hop,.badge){opacity:.1}\n", pointed)
	fmt.Fprintf(&b, "%s :where(.box,.port[class*=\" n\"]){opacity:.3}\n", pointed)
	for j := range hoverMax {
		on := fmt.Sprintf("svg:has(.p%d:hover,:focus-visible>.p%d)", j, j)
		fmt.Fprintf(&b, "%s :is(.box,.port).n%d,%s :is(.count,.lab,.origin,.badge).e%d,svg:has(.w.e%d:hover) .p%d{opacity:1}\n", on, j, on, j, j, j)
		fmt.Fprintf(&b, "%s .route.e%d{opacity:1;--c:var(--k%d)}\n", on, j, j)
	}
	for k := range routeMax {
		fmt.Fprintf(&b, "svg:has(.w.r%d:hover) .r%d:not(.w){opacity:1}\n", k, k)
	}
	b.WriteString("}\n")
	return b.String()
}()
