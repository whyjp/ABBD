package screen

import (
	"fmt"
	"hash/fnv"
	"slices"

	"github.com/whyjp/abbd/internal/model"
)

// Marks are what the page adds to routed ink so a reader can read it: a gap
// where one line passes over another (rule 24), and a word along a line that
// says how its relations arise (rule 25).

const (
	// gapHalf is how far a gap reaches each side of the line that passes
	// over it, the over line's stroke included, and gapEnd how far a
	// crossing must stand from the ends of both pieces to be gapped: closer
	// in, it reads as a line that ends on the other.
	gapHalf, gapEnd = 4, 6
)

// Gap is where a horizontal stretch of a line is cut because a vertical
// line crosses over it there.
type Gap struct{ X, Y int }

// seg is one straight stretch of a wire's ink.
type seg struct {
	a, b pt
	wire int
}

func (s seg) flat() bool { return s.a.y == s.b.y }

// lo and hi are the stretch's extent along its own direction.
func (s seg) lo() int {
	if s.flat() {
		return min(s.a.x, s.b.x)
	}
	return min(s.a.y, s.b.y)
}

func (s seg) hi() int {
	if s.flat() {
		return max(s.a.x, s.b.x)
	}
	return max(s.a.y, s.b.y)
}

func (s seg) bounds() box {
	return box{min(s.a.x, s.b.x), min(s.a.y, s.b.y), abs(s.a.x - s.b.x), abs(s.a.y - s.b.y)}
}

func segsOf(paths []drawn) []seg {
	var out []seg
	for i, p := range paths {
		for _, part := range p.parts {
			part = straight(part)
			for k := 1; k < len(part); k++ {
				if part[k-1] != part[k] {
					out = append(out, seg{part[k-1], part[k], i})
				}
			}
		}
	}
	return out
}

// mark adds the marks to d, whose wires were built from paths in order.
func (d *Drawing) mark(paths []drawn, t tree) {
	segs := segsOf(paths)
	d.gaps(segs)
	d.labels(segs, t)
}

// gaps cuts every horizontal piece where a vertical piece of another line
// crosses it away from both pieces' ends and no dot joins them. The
// horizontal line is always the one broken, so the choice never depends on
// the order the lines are drawn in.
func (d *Drawing) gaps(segs []seg) {
	dot := map[pt]bool{}
	for _, j := range d.Dots {
		dot[pt{j.X, j.Y}] = true
	}
	var hs, vs []seg
	for _, s := range segs {
		if s.flat() {
			hs = append(hs, s)
		} else {
			vs = append(vs, s)
		}
	}
	inside := func(v, lo, hi, room int) bool { return lo+room <= v && v <= hi-room }
	for _, h := range hs {
		for _, v := range vs {
			x, y := v.a.x, h.a.y
			if v.wire == h.wire || dot[pt{x, y}] || !inside(x, h.lo(), h.hi(), gapEnd) || !inside(y, v.lo(), v.hi(), gapEnd) {
				continue
			}
			// The hole would cut the horizontal line's own vertical ink.
			own := slices.ContainsFunc(vs, func(o seg) bool {
				return o.wire == h.wire && abs(o.a.x-x) <= gapHalf && inside(y, o.lo(), o.hi(), -gapHalf)
			})
			w := &d.Wires[h.wire]
			if g := (Gap{x, y}); !own && !slices.Contains(w.Gaps, g) {
				w.Gaps = append(w.Gaps, g)
			}
		}
	}
	// Mask ids are unique to the drawing's ink, so two drawings on one page
	// never share one unless they draw the same lines.
	f := fnv.New32a()
	fmt.Fprint(f, d.Width, d.Height)
	for _, w := range d.Wires {
		fmt.Fprint(f, w.D)
	}
	for i := range d.Wires {
		w := &d.Wires[i]
		if len(w.Gaps) == 0 {
			continue
		}
		slices.SortFunc(w.Gaps, func(a, b Gap) int {
			if a.Y != b.Y {
				return a.Y - b.Y
			}
			return a.X - b.X
		})
		w.Mask = fmt.Sprintf("x%x-%d", f.Sum32(), i)
	}
}

// viaWords are the words a line's label shows for how its relations arise.
var viaWords = map[model.Via]string{
	"call": "호출", "data": "데이터", "convention": "관례", "config": "설정",
	"import": "임포트", "trait": "트레이트", "callback": "콜백", "macro": "매크로",
	"build-feature": "빌드 기능",
}

const (
	// labelPx is a label's font size. labelPad is how far a label keeps
	// from its stretch's ends, where an arrowhead, an origin mark or a turn
	// stands, and labelOff how far it stands off its line.
	labelPx, labelPad, labelOff = 10, 10, 3
	// labelMax is the widest a label may be. A longer word is left to the
	// tooltip.
	labelMax = 60
)

// Label is a word set along one straight stretch of a line. Up says the
// stretch is vertical and the word reads upward along it.
type Label struct {
	S    string
	X, Y int
	Up   bool `json:",omitempty"`
}

// via is the word for how every relation a wire stands for arises, or ""
// when they arise in different ways or the store does not say.
func via(t tree, rels []string) string {
	word := ""
	for i, r := range rels {
		w := viaWords[t.vias[r]]
		if w == "" || i > 0 && w != word {
			return ""
		}
		word = w
	}
	return word
}

func hit(a, b box) bool {
	return a.x < b.x+b.w && b.x < a.x+a.w && a.y < b.y+b.h && b.y < a.y+a.h
}

func grow(b box, n int) box { return box{b.x - n, b.y - n, b.w + 2*n, b.h + 2*n} }

// labels sets each line's word on a straight stretch of its own ink long
// enough to hold it, beside the line and clear of every box, chip, count,
// other label and other line. A bus whose lines all say the same word is
// labelled once, anywhere along it. Otherwise a line is labelled only where
// its ink is its own, such as the drop into its box, since a word on a
// shared trunk would name lines it does not stand for. A line with no room
// keeps its word in its tooltip only.
func (d *Drawing) labels(segs []seg, t tree) {
	var taken []box
	for _, b := range d.Boxes {
		taken = append(taken, box{b.X, b.Y, b.W, b.H})
	}
	for _, p := range d.Ports {
		taken = append(taken, box{p.X, p.RectY(), p.W, p.H})
	}
	if f := d.Frame; f != nil {
		taken = append(taken, box{f.X, f.Y, f.W, 0}, box{f.X, f.Y + f.H, f.W, 0}, box{f.X, f.Y, 0, f.H}, box{f.X + f.W, f.Y, 0, f.H})
	}
	for _, w := range d.Wires {
		if w.Count > 1 {
			taken = append(taken, box{w.LX, w.LY - 9, textWidth(fmt.Sprintf("×%d", w.Count), 11) + 2, 11})
		}
	}
	words := make([]string, len(d.Wires))
	for i, w := range d.Wires {
		words[i] = via(t, w.Rels)
	}
	// whole says every path of a net says one word, so the net is labelled
	// as one line.
	whole := map[int]bool{}
	for i, w := range d.Wires {
		if w.Net == 0 {
			continue
		}
		if _, seen := whole[w.Net]; !seen {
			whole[w.Net] = true
		}
		whole[w.Net] = whole[w.Net] && words[i] == words[slices.IndexFunc(d.Wires, func(o Wire) bool { return o.Net == w.Net })]
	}
	mate := func(i, j int) bool {
		n := d.Wires[i].Net
		return i == j || n != 0 && whole[n] && d.Wires[j].Net == n
	}
	done := map[int]bool{}
	for i := range d.Wires {
		s, n := words[i], d.Wires[i].Net
		if s == "" || n != 0 && whole[n] && done[n] {
			continue
		}
		tw := textWidth(s, labelPx)
		if tw > labelMax {
			continue
		}
		var mine []seg
		for _, g := range segs {
			if mate(i, g.wire) {
				mine = append(mine, g)
			}
		}
		slices.SortStableFunc(mine, func(a, b seg) int { return (b.hi() - b.lo()) - (a.hi() - a.lo()) })
		// clear says the label's box lb, set beside g between lo and hi,
		// keeps off everything else, and no other line's ink runs along g
		// there.
		clear := func(g seg, lb box, lo, hi int) bool {
			for _, o := range taken {
				if hit(lb, grow(o, 2)) {
					return false
				}
			}
			for _, o := range segs {
				if o == g {
					continue
				}
				if hit(lb, grow(o.bounds(), 1)) {
					return false
				}
				along := o.flat() == g.flat() && (g.flat() && o.a.y == g.a.y || !g.flat() && o.a.x == g.a.x)
				if along && !mate(i, o.wire) && max(o.lo(), lo) < min(o.hi(), hi) {
					return false
				}
			}
			return true
		}
		var got *Label
		var at box
	search:
		for _, g := range mine {
			lo, hi := g.lo()+labelPad+tw/2, g.hi()-labelPad-tw/2
			if lo > hi {
				continue
			}
			mid := (lo + hi) / 2
			for off := 0; mid-off >= lo || mid+off <= hi; off += 4 {
				for _, c := range []int{mid - off, mid + off} {
					if c < lo || c > hi {
						continue
					}
					type option struct {
						lb box
						l  Label
					}
					var opts []option
					if g.flat() {
						y := g.a.y
						opts = []option{
							{box{c - tw/2, y - labelOff - 9, tw, 9}, Label{s, c, y - labelOff - 1, false}},
							{box{c - tw/2, y + labelOff, tw, 9}, Label{s, c, y + labelOff + 8, false}},
						}
					} else {
						x := g.a.x
						opts = []option{
							{box{x - labelOff - 9, c - tw/2, 9, tw}, Label{s, x - labelOff - 1, c, true}},
							{box{x + labelOff, c - tw/2, 9, tw}, Label{s, x + labelOff + 8, c, true}},
						}
					}
					for _, o := range opts {
						if clear(g, o.lb, c-tw/2, c+tw/2) {
							got, at = &o.l, o.lb
							break search
						}
					}
				}
			}
		}
		if got == nil {
			continue
		}
		d.Wires[i].Label = got
		taken = append(taken, at)
		if n != 0 && whole[n] {
			done[n] = true
		}
	}
}
