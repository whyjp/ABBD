// Package parts proposes the responsibilities of one directory of code. Its
// subdirectories are parts already; its own files join the part that alone
// uses them, a family of files that implement one type is one part, and the
// files most parts use are the shared layer. It reads source text only, with
// no parser per language, so the same rule runs on any repository, and it is
// deterministic, so an extraction script can call it.
package parts

import (
	"cmp"
	"slices"
	"strings"
)

// File is one source file. Node is what the proposal places: the file
// itself, or the subdirectory of the directory that holds it.
type File struct {
	Path string
	Node string
	Text string
}

// Link is how one file uses another: the names it takes from it and the
// first line that shows it.
type Link struct {
	From, To string
	Names    []string
	Line     int
	Quote    string
}

// Part is one proposed responsibility.
type Part struct {
	Name string
	// Symbol is the declaration the name comes from, or empty when the name
	// is a file or directory stem.
	Symbol string
	Nodes  []string
	// Links are the uses between the part's own nodes that hold it together.
	Links []Link
	// Shared marks the layer most other parts use.
	Shared bool
	// Family is the type every node of the part implements, constructs or
	// extends, when that is what holds the part together.
	Family string
}

// Result is a directory's proposal. Loose are the files of the directory
// that use no other node and that no other node uses; they belong to the
// module itself, not to a part.
type Result struct {
	Parts []Part
	Loose []string
	Links []Link
}

// group is a set of nodes the proposal has joined so far.
type group struct {
	nodes  []string
	dir    bool
	size   int
	family string
	shared bool
}

// Propose places the nodes of one directory in parts.
func Propose(files []File) Result {
	srcs := make([]*source, len(files))
	node := map[string]string{}
	for i, f := range files {
		srcs[i] = load(f)
		n := f.Node
		if n == "" {
			n = f.Path
		}
		srcs[i].node = n
		node[f.Path] = n
	}
	slices.SortFunc(srcs, func(a, b *source) int { return cmp.Compare(a.path, b.path) })
	links := linksOf(srcs)

	at := map[string]*group{}
	var order []string
	for _, s := range srcs {
		g := at[s.node]
		if g == nil {
			g = &group{nodes: []string{s.node}, dir: s.node != s.path}
			at[s.node] = g
			order = append(order, s.node)
		}
		g.size += s.size()
	}
	slices.Sort(order)
	stem := map[string]string{}
	for _, s := range srcs {
		if s.node == s.path {
			stem[s.node] = s.stem
		}
	}
	join := func(from, into *group) {
		if from == into {
			return
		}
		into.nodes = append(into.nodes, from.nodes...)
		slices.Sort(into.nodes)
		into.size += from.size
		into.dir = into.dir || from.dir
		for _, n := range from.nodes {
			at[n] = into
		}
	}
	groups := func() []*group {
		var out []*group
		seen := map[*group]bool{}
		for _, n := range order {
			if g := at[n]; !seen[g] {
				seen[g] = true
				out = append(out, g)
			}
		}
		return out
	}
	users := func(g *group) []*group {
		var out []*group
		for _, l := range links {
			a, b := at[node[l.From]], at[node[l.To]]
			if b == g && a != g && !slices.Contains(out, a) {
				out = append(out, a)
			}
		}
		return out
	}
	uses := func(g *group) []*group {
		var out []*group
		for _, l := range links {
			a, b := at[node[l.From]], at[node[l.To]]
			if a == g && b != g && !slices.Contains(out, b) {
				out = append(out, b)
			}
		}
		return out
	}

	// Files named as one family, such as context and context_appengine.
	for i, a := range order {
		for _, b := range order[i+1:] {
			if stem[a] != "" && stem[b] != "" && prefix(stem[a]) == prefix(stem[b]) {
				join(at[b], at[a])
			}
		}
	}
	// A helper only one part uses sits inside that part, unless it is the
	// larger of the two, which makes it a responsibility of its own.
	for changed := true; changed; {
		changed = false
		for _, g := range groups() {
			if g.dir {
				continue
			}
			if us := users(g); len(us) == 1 && g.size <= us[0].size {
				join(g, us[0])
				changed = true
				break
			}
		}
	}
	// Files that each implement, construct or extend one type another node
	// declares are one family, such as every middleware or every exporter.
	fams := families(srcs)
	names := make([]string, 0, len(fams))
	for t := range fams {
		names = append(names, t)
	}
	slices.SortFunc(names, func(a, b string) int {
		return cmp.Or(len(fams[b])-len(fams[a]), cmp.Compare(a, b))
	})
	for _, t := range names {
		var members []*group
		for _, i := range fams[t] {
			g := at[srcs[i].node]
			if !g.dir && len(g.nodes) == 1 && !slices.Contains(members, g) {
				members = append(members, g)
			}
		}
		if len(members) < 2 {
			continue
		}
		for _, g := range members[1:] {
			join(g, members[0])
		}
		members[0].family = t
	}
	median := fileMedian(srcs)
	single := func(g *group) bool { return !g.dir && len(g.nodes) == 1 && g.family == "" }
	// strongest is the other group g takes the most names from, or that
	// takes the most from g.
	strongest := func(g *group, from bool) *group {
		n := map[*group]int{}
		for _, l := range links {
			a, b := at[node[l.From]], at[node[l.To]]
			if !from {
				a, b = b, a
			}
			if a == g && b != g {
				n[b] += len(l.Names)
			}
		}
		var best *group
		for _, h := range groups() {
			if n[h] > n[best] {
				best = h
			}
		}
		return best
	}
	// A small file no other file of the directory uses, such as an entry
	// point, a facade or a few methods added to a type, joins the part it
	// takes the most names from.
	for _, g := range groups() {
		if single(g) && g.size*2 <= median && len(users(g)) == 0 {
			if h := strongest(g, true); h != nil {
				join(g, h)
			}
		}
	}
	// The single files several parts use, no larger than most files, are
	// the shared layer.
	var shared []*group
	for _, g := range groups() {
		if single(g) && len(users(g)) >= 3 && g.size <= median {
			shared = append(shared, g)
		}
	}
	for _, g := range shared {
		join(g, shared[0])
		shared[0].shared = true
	}
	// A small file left on its own joins the part that uses most of it.
	for _, g := range groups() {
		if single(g) && !g.shared && g.size*2 <= median {
			if h := strongest(g, false); h != nil {
				join(g, h)
			}
		}
	}

	var res Result
	res.Links = links
	for _, g := range groups() {
		if !g.dir && len(g.nodes) == 1 && len(users(g)) == 0 && len(uses(g)) == 0 {
			res.Loose = append(res.Loose, g.nodes[0])
			continue
		}
		res.Parts = append(res.Parts, describe(g, srcs, links, node))
	}
	stripBrand(res.Parts, srcs)
	return res
}

// fileMedian is the median size of the directory's own files.
func fileMedian(srcs []*source) int {
	var s []int
	for _, x := range srcs {
		if x.node == x.path {
			s = append(s, x.size())
		}
	}
	if len(s) == 0 {
		return 0
	}
	slices.Sort(s)
	return s[len(s)/2]
}

// describe names a group by the type its nodes declare that the most other
// files use, or that its family implements, or by its largest node's stem.
func describe(g *group, srcs []*source, links []Link, node map[string]string) Part {
	p := Part{Nodes: g.nodes, Shared: g.shared, Family: g.family}
	in := map[string]bool{}
	for _, n := range g.nodes {
		in[n] = true
	}
	users := map[string]map[string]bool{}
	for _, l := range links {
		a, b := node[l.From], node[l.To]
		if in[a] && in[b] && a != b {
			p.Links = append(p.Links, l)
		}
		// The part is named by what the rest of the directory knows it by.
		if !in[b] || in[a] {
			continue
		}
		for _, nm := range l.Names {
			nm = strings.TrimSuffix(nm, "()")
			if users[nm] == nil {
				users[nm] = map[string]bool{}
			}
			users[nm][l.From] = true
		}
	}
	best, score := "", 0
	for _, s := range srcs {
		if !in[s.node] || s.node != s.path {
			continue
		}
		for _, d := range s.decls {
			if !d.isType && s.methods[d.name] == 0 {
				continue
			}
			if sc := len(users[d.name]) + s.methods[d.name]; sc > score || sc == score && sc > 0 && d.name < best {
				best, score = d.name, sc
			}
		}
	}
	switch {
	case g.shared && len(g.nodes) > 1:
		p.Name = "shared layer"
	case g.family != "" && len(g.nodes) > 1:
		p.Symbol, p.Name = g.family, words(g.family)+" family"
	case best != "" && !g.dir:
		p.Symbol, p.Name = best, words(best)
	default:
		p.Name = words(strings.Trim(largestStem(g, srcs), "_."))
	}
	return p
}

// largestStem is the stem of the group's directory, or of its largest file.
func largestStem(g *group, srcs []*source) string {
	size := map[string]int{}
	for _, s := range srcs {
		size[s.node] += s.size()
	}
	big := g.nodes[0]
	for _, n := range g.nodes {
		if size[n] > size[big] {
			big = n
		}
	}
	for _, n := range g.nodes {
		if strings.HasSuffix(n, "/") || isDirNode(n, srcs) {
			big = n
			break
		}
	}
	base := big[strings.LastIndex(strings.TrimSuffix(big, "/"), "/")+1:]
	if i := strings.LastIndex(base, "."); i > 0 {
		base = base[:i]
	}
	return strings.TrimSuffix(base, "/")
}

func isDirNode(n string, srcs []*source) bool {
	for _, s := range srcs {
		if s.node == n && s.path != n {
			return true
		}
	}
	return false
}
