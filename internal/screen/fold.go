package screen

import "slices"

// Two boxes that use each other by relations of one kind are a mutual
// pair: a line from a to b and one from b to a. Drawn apart they
// read as one line drawn twice, the "11" a hand drawing never draws. The
// router's fold action merges each such pair into one line with a head at each
// end, on buses and roads of its own, so the shared ink joins no third place
// (rule 18). Each line keeps its relations, its word and its route: the
// folded line lights both when a reader points at it.
//
// Only a pair whose two lines both end on the boxes themselves folds. A
// line that ends on a chip, a pair of two kinds or of two dottings, and a
// pair with more than one line either way stay apart: one stroke would
// show there a relation that is not stored, or lose a dotting.

// foldable are the mutual pairs among reqs, as [kept, rider]: the
// rider folds into the kept line, which runs the way the eye reads, down
// the rows, or right within a row.
func foldable(reqs []wireReq, before func(a, b end) bool) [][2]int {
	bare := func(e end) bool { return e.node != "" && (e.key == e.node || e.key == "plain|"+e.node) }
	type way struct{ from, to, kind string }
	n := map[way][]int{}
	for i, r := range reqs {
		n[way{place(r.from), place(r.to), r.kind}] = append(n[way{place(r.from), place(r.to), r.kind}], i)
	}
	var out [][2]int
	for i, r := range reqs {
		if !bare(r.from) || !bare(r.to) || r.from.node == r.to.node || r.kind == kindMutual || r.kind == kindBypass || r.lead {
			continue
		}
		there, back := n[way{place(r.from), place(r.to), r.kind}], n[way{place(r.to), place(r.from), r.kind}]
		if len(there) != 1 || len(back) != 1 {
			continue
		}
		j := back[0]
		if q := reqs[j]; j < i || q.plan != r.plan || q.lead {
			continue
		}
		if before(r.to, r.from) {
			out = append(out, [2]int{j, i})
		} else {
			out = append(out, [2]int{i, j})
		}
	}
	slices.SortFunc(out, func(a, b [2]int) int { return a[0] - b[0] })
	return out
}

// readFirst says a stands where a reader comes to it before b: in a row
// above, or left of it in one row.
func readFirst(a, b end) bool {
	if a.row != b.row {
		return a.row < b.row
	}
	return a.lo+a.hi < b.lo+b.hi
}

// folding maps a view's lines to the fewer lines a fold lays: kept is,
// for each line laid, the line it stands for, and rider holds, for each of
// the view's lines, the line it folds into, or -1.
type folding struct {
	kept  []int
	rider []int
	// at is where each of the view's lines stands among those laid, a
	// rider at the line it folds into.
	at []int
}

// newFolding folds the riders into their kept lines.
func newFolding(reqs []wireReq, pairs [][2]int) folding {
	f := folding{rider: make([]int, len(reqs)), at: make([]int, len(reqs))}
	for i := range f.rider {
		f.rider[i] = -1
	}
	for _, p := range pairs {
		f.rider[p[1]] = p[0]
	}
	for i := range reqs {
		if f.rider[i] < 0 {
			f.at[i] = len(f.kept)
			f.kept = append(f.kept, i)
		}
	}
	for i, k := range f.rider {
		if k >= 0 {
			f.at[i] = f.at[k]
		}
	}
	return f
}

// reqs are the lines a fold lays: each kept line, reading both ways where a
// rider folds into it.
func (f folding) reqs(reqs []wireReq) []wireReq {
	out := make([]wireReq, len(f.kept))
	for k, i := range f.kept {
		out[k] = reqs[i]
	}
	for _, k := range f.rider {
		if k >= 0 {
			out[f.at[k]].both = true
		}
	}
	return out
}

// in maps the view's lines is to the lines laid, each once.
func (f folding) in(is []int) []int {
	var out []int
	for _, i := range is {
		if k := f.at[i]; !slices.Contains(out, k) {
			out = append(out, k)
		}
	}
	return out
}

// out maps lines laid back to the view's lines, a folded line to its kept
// line and its rider.
func (f folding) out(ks []int) []int {
	var out []int
	for _, k := range ks {
		i := f.kept[k]
		out = append(out, i)
		if j := slices.Index(f.rider, i); j >= 0 {
			out = append(out, j)
		}
	}
	return out
}

// opts are o for the lines laid.
func (f folding) opts(o layOpts) layOpts {
	if o.Bent == nil {
		return o
	}
	bent := map[int]bool{}
	for i, ok := range o.Bent {
		if ok {
			bent[f.at[i]] = true
		}
	}
	o.Bent = bent
	return o
}

// held are the tracks held, by the lines laid.
func (f folding) held(h map[[3]int]int) map[[3]int]int {
	if h == nil {
		return nil
	}
	out := map[[3]int]int{}
	for k, t := range h {
		if f.rider[k[1]] < 0 {
			out[[3]int{k[0], f.at[k[1]], k[2]}] = t
		}
	}
	return out
}

// unfold is r, laid for the lines laid, as the view's lines read it: a
// rider runs its kept line's way backward, and rides every path the kept
// line rides, with no start of its own there, as the folded line's ends
// are both the kept line's.
func (f folding) unfold(r routing) routing {
	n := len(f.rider)
	ways, back := make([][]pt, n), make([]bool, n)
	for k, i := range f.kept {
		ways[i], back[i] = r.ways[k], r.back[k]
	}
	for i, k := range f.rider {
		if k >= 0 {
			ways[i] = slices.Clone(ways[k])
			slices.Reverse(ways[i])
		}
	}
	drawn := slices.Clone(r.drawn)
	for d := range drawn {
		p := &drawn[d]
		var starts []pt
		for s, k := range p.reqs {
			starts = append(starts, p.starts[s])
			if f.rides(f.kept[k]) {
				starts = append(starts, noStart)
			}
		}
		p.reqs, p.counted, p.starts = f.out(p.reqs), f.out(p.counted), starts
	}
	tracks := map[[3]int]int{}
	for k, t := range r.tracks {
		tracks[[3]int{k[0], f.kept[k[1]], k[2]}] = t
	}
	r.ways, r.back, r.drawn, r.bent, r.tracks = ways, back, drawn, f.out(r.bent), tracks
	r.rider = f.rider
	return r
}

// rides says some rider folds into line i.
func (f folding) rides(i int) bool { return slices.Contains(f.rider, i) }

// fold is r, unfolded, as the lines laid read it again, and the folding,
// or r and false where no line of it rides another.
func (r routing) fold() (routing, folding, bool) {
	if !slices.ContainsFunc(r.rider, func(k int) bool { return k >= 0 }) {
		return r, folding{}, false
	}
	var pairs [][2]int
	for i, k := range r.rider {
		if k >= 0 {
			pairs = append(pairs, [2]int{k, i})
		}
	}
	f := newFolding(make([]wireReq, len(r.rider)), pairs)
	out := r
	out.ways, out.back = make([][]pt, len(f.kept)), make([]bool, len(f.kept))
	for k, i := range f.kept {
		out.ways[k], out.back[k] = r.ways[i], r.back[i]
	}
	out.drawn = slices.Clone(r.drawn)
	for d := range out.drawn {
		p := &out.drawn[d]
		var starts []pt
		for s, i := range p.reqs {
			if f.rider[i] < 0 {
				starts = append(starts, p.starts[s])
			}
		}
		p.reqs, p.counted, p.starts = f.in(p.reqs), f.in(p.counted), starts
	}
	out.tracks = map[[3]int]int{}
	for k, t := range r.tracks {
		if f.rider[k[1]] < 0 {
			out.tracks[[3]int{k[0], f.at[k[1]], k[2]}] = t
		}
	}
	out.bent, out.rider = f.in(r.bent), nil
	return out, f, true
}

// riding says line i of r folds into another line.
func (r routing) riding(i int) bool { return r.rider != nil && r.rider[i] >= 0 }

// foldsAny says some two routes of d join two boxes opposite ways by one
// kind, so drawing d with its pairs folded may draw something else.
func foldsAny(d Drawing) bool {
	box := map[string]bool{}
	for _, b := range d.Boxes {
		box[b.ID] = true
	}
	seen := map[[3]string]bool{}
	for _, r := range d.Routes {
		if len(r.ends) != 2 || !box[r.ends[0]] || !box[r.ends[1]] {
			continue
		}
		if seen[[3]string{r.ends[1], r.ends[0], r.Kind}] {
			return true
		}
		seen[[3]string{r.ends[0], r.ends[1], r.Kind}] = true
	}
	return false
}

// The tiers of folding a view is drawn in (tactic.pairs): none, every mutual
// pair, or only the lone ones (lone).
const (
	foldNone = iota
	foldAll
	foldLone
)

// lone are the pairs whose two lines share no drawn path with a third line
// in paths: folding such a pair takes nothing off another line's bus, so it
// adds no stub beside one.
func lone(pairs [][2]int, paths []drawn) [][2]int {
	var out [][2]int
	for _, p := range pairs {
		if !slices.ContainsFunc(paths, func(d drawn) bool {
			return (slices.Contains(d.reqs, p[0]) || slices.Contains(d.reqs, p[1])) && slices.ContainsFunc(d.reqs, func(i int) bool { return i != p[0] && i != p[1] })
		}) {
			out = append(out, p)
		}
	}
	return out
}
