package screen

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/whyjp/abbd/internal/model"
)

// Shape is what the structure of a full view says about the code, read off
// the lines the view stands for: the lines it draws and the pairs it counts
// without drawing. It is a report for a reader choosing how to describe the
// view, and no gate. It names no box by its name or kind of word, it moves no
// drawing and it never fails lint; the reader decides what the view is, and
// records it.
type Shape struct {
	// Boxes are the view's parts, Joined those a line touches, and Pairs the
	// pairs of parts a line joins, whichever way it runs.
	Boxes  int `json:"boxes"`
	Joined int `json:"joined"`
	Pairs  int `json:"pairs"`
	// Cyclic are the joined parts that stand on a cycle of the lines.
	Cyclic int `json:"cyclic"`
	// Chain is the most lines one use runs through from a part nothing
	// above uses to the part at the bottom, a group that uses each other
	// round a cycle counting as one part.
	Chain int `json:"chain"`
	// Layers are the parts of each layer candidate, from the top: a part's
	// layer is the longest chain of uses above it. Down are the lines
	// between two layers, every one running down, and Same the lines
	// between parts of one cycle group, which have no order.
	Layers []int `json:"layers,omitempty"`
	Down   int   `json:"down"`
	Same   int   `json:"same"`
	// Stacks are the stored stacks that hold two parts of the view or more
	// in two layers or more, each with the lines of the view that run up its layers.
	Stacks []ShapeStack `json:"stacks,omitempty"`
	// Families are the groups of four parts or more that no line joins and
	// that use or provide the same contract, one group for each contract,
	// by the contract's name.
	Families []ShapeFamily `json:"families,omitempty"`
	// Boundary counts the lines that cross the view's own frame: Contract
	// through a contract chip and Direct to another module, on an inside only.
	// NoProvider are
	// the contracts the view draws loose, with no module providing them.
	Contract   int `json:"contract"`
	Direct     int `json:"direct"`
	NoProvider int `json:"no_provider"`
	// Notes say in words what the numbers suggest for a reader.
	Notes []string `json:"notes,omitempty"`
}

// ShapeStack is a stored stack seen from one view.
type ShapeStack struct {
	Key  string `json:"key"`
	Held int    `json:"held"`
	Up   int    `json:"up"`
}

// ShapeFamily is a group of parts that stand apart and share a contract.
type ShapeFamily struct {
	Contract string `json:"contract"`
	Parts    int    `json:"parts"`
}

const (
	// shapeFamily is the fewest parts that make a family, and shapeChain the
	// shortest chain a layer candidate needs.
	shapeFamily = 4
	shapeChain  = 3
)

// viewShape reads a view's shape. part are the view's boxes, unit gives a
// unit's row, and stacks are the stored stacks.
func viewShape(d Drawing, part map[string]bool, unit map[string]UnitRow, stacks []StackView, inside bool) *Shape {
	kind := func(id string) model.Kind { return model.Kind(unit[id].Kind) }
	if len(part) == 0 {
		return nil
	}
	s := &Shape{Boxes: len(part)}
	edge := map[[2]string]bool{}
	chip := map[string]string{}
	// A chip that stands for a module met without a contract carries that
	// module's ID, so a chip is a contract's only where the unit is one.
	isContract := func(p Port) bool { return p.Contract != "" && kind(p.Contract).IsContract() }
	for _, p := range d.Ports {
		if !isContract(p) {
			continue
		}
		for _, k := range p.Keys {
			chip[k] = p.Contract
		}
	}
	// contractOf is the contract a place stands for, or "" for none.
	contractOf := func(id string) string {
		if kind(id).IsContract() {
			return id
		}
		return chip[id]
	}
	touches := map[string]map[string]bool{} // contract -> parts it joins
	for _, r := range slices.Concat(d.Relations, d.Counted) {
		from, to := r[0], r[1]
		if part[from] && part[to] && from != to {
			edge[[2]string{from, to}] = true
		}
		// A contract is a box inside a module and a chip on a frame; either
		// way the parts that use or provide it touch it.
		if c := contractOf(from); part[to] && !kind(to).IsContract() && c != "" {
			add(touches, c, to)
		} else if c := contractOf(to); part[from] && !kind(from).IsContract() && c != "" {
			add(touches, c, from)
		}
	}
	ids := slices.Sorted(maps.Keys(part))
	pair := map[[2]string]bool{}
	joined := map[string]bool{}
	for e := range edge {
		pair[[2]string{min(e[0], e[1]), max(e[0], e[1])}] = true
		joined[e[0]], joined[e[1]] = true, true
	}
	s.Joined, s.Pairs = len(joined), len(pair)
	comp, ncomp := shapeGroups(ids, edge)
	size := make([]int, ncomp)
	for _, id := range ids {
		size[comp[id]]++
	}
	for id := range joined {
		if size[comp[id]] > 1 {
			s.Cyclic++
		}
	}
	depth := shapeDepths(ncomp, comp, edge)
	for _, dp := range depth {
		s.Chain = max(s.Chain, dp)
	}
	layer := map[string]int{}
	for _, id := range ids {
		if joined[id] {
			layer[id] = depth[comp[id]]
		}
	}
	if s.Chain > 0 {
		s.Layers = make([]int, s.Chain+1)
		for id := range joined {
			s.Layers[layer[id]]++
		}
	}
	for e := range edge {
		if comp[e[0]] == comp[e[1]] {
			s.Same++
		} else {
			s.Down++
		}
	}
	// A stored stack that holds two parts or more says which way it reads.
	for _, st := range stacks {
		held, at := 0, map[int]bool{}
		for _, id := range ids {
			if n := st.held[model.ID(id)]; n > 0 {
				held++
				at[n] = true
			}
		}
		// A stack that puts every part in one layer says nothing of order.
		if held < 2 || len(at) < 2 {
			continue
		}
		up := 0
		for e := range edge {
			a, b := st.held[model.ID(e[0])], st.held[model.ID(e[1])]
			if a > 0 && b > 0 && a > b {
				up++
			}
		}
		s.Stacks = append(s.Stacks, ShapeStack{Key: st.Key, Held: held, Up: up})
	}
	// Families: parts no line joins, one group for each contract they share.
	seen := map[string]bool{}
	for _, c := range slices.Sorted(maps.Keys(touches)) {
		var group []string
		for _, id := range slices.Sorted(maps.Keys(touches[c])) {
			alone := true
			for _, o := range slices.Sorted(maps.Keys(touches[c])) {
				if o != id && pair[[2]string{min(id, o), max(id, o)}] {
					alone = false
				}
			}
			if alone {
				group = append(group, id)
			}
		}
		key := strings.Join(group, ",")
		if len(group) >= shapeFamily && !seen[key] {
			seen[key] = true
			s.Families = append(s.Families, ShapeFamily{Contract: unit[c].Name, Parts: len(group)})
		}
	}
	slices.SortStableFunc(s.Families, func(a, b ShapeFamily) int { return b.Parts - a.Parts })
	for _, p := range d.Ports {
		if !inside {
			break
		}
		n := max(len(p.Rels), 1)
		if isContract(p) {
			s.Contract += n
		} else {
			s.Direct += n
		}
	}
	for _, b := range d.Boxes {
		if strings.Contains(b.Class, "loose") && model.Kind(b.Kind).IsContract() {
			s.NoProvider++
		}
	}
	s.Notes = shapeNotes(s, len(s.Stacks) > 0)
	return s
}

func add(m map[string]map[string]bool, k, v string) {
	if m[k] == nil {
		m[k] = map[string]bool{}
	}
	m[k][v] = true
}

// shapeNotes say what a shape suggests, in words. They are findings, not
// verdicts.
func shapeNotes(s *Shape, stacked bool) []string {
	var out []string
	switch {
	case s.Joined >= 3 && 2*s.Cyclic >= s.Joined:
		out = append(out, fmt.Sprintf("이어진 상자 %d개 가운데 %d개가 순환 위에 있어 층으로 읽히지 않는다. 서로 쓰는 무리가 이 뷰의 모양이다", s.Joined, s.Cyclic))
	case s.Joined >= 3 && s.Chain >= shapeChain && 5*s.Cyclic <= s.Joined && !stacked:
		out = append(out, fmt.Sprintf("사슬이 %d줄이고 순환 위 상자가 적어 층 후보가 된다. 층 이름이 코드에 보이면 stack으로 적을 수 있고, 코드에 없으면 적지 않는다", s.Chain))
	}
	for _, f := range s.Families {
		out = append(out, fmt.Sprintf("%d개 상자가 서로 쓰지 않고 %s 계약을 함께 쓰거나 제공한다. 같은 계약의 구현체인지는 코드를 읽어야 가려진다", f.Parts, f.Contract))
	}
	if s.NoProvider > 0 {
		out = append(out, fmt.Sprintf("제공하는 모듈이 없는 계약이 %d개 그려져 있어, 경계의 약속이 비어 보인다. 코드에 없어서인지 추출이 놓쳐서인지는 코드를 읽어야 가려진다", s.NoProvider))
	}
	return out
}

// shapeGroups numbers the strongly connected groups of ids under edge, so
// parts that use each other round a cycle share a number. The numbers run
// in the order the groups are found.
func shapeGroups(ids []string, edge map[[2]string]bool) (map[string]int, int) {
	next := map[string][]string{}
	for e := range edge {
		next[e[0]] = append(next[e[0]], e[1])
	}
	for _, l := range next {
		slices.Sort(l)
	}
	index, low := map[string]int{}, map[string]int{}
	on := map[string]bool{}
	comp := map[string]int{}
	var stack []string
	n, count := 0, 0
	var walk func(v string)
	walk = func(v string) {
		n++
		index[v], low[v] = n, n
		stack = append(stack, v)
		on[v] = true
		for _, w := range next[v] {
			if index[w] == 0 {
				walk(w)
				low[v] = min(low[v], low[w])
			} else if on[w] {
				low[v] = min(low[v], index[w])
			}
		}
		if low[v] == index[v] {
			for {
				w := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				on[w] = false
				comp[w] = count
				if w == v {
					break
				}
			}
			count++
		}
	}
	for _, id := range ids {
		if index[id] == 0 {
			walk(id)
		}
	}
	return comp, count
}

// shapeDepths gives each group the longest chain of uses above it, counted in
// lines, from a group nothing uses.
func shapeDepths(n int, comp map[string]int, edge map[[2]string]bool) []int {
	next := make([][]int, n)
	for e := range edge {
		if a, b := comp[e[0]], comp[e[1]]; a != b {
			next[b] = append(next[b], a)
		}
	}
	depth := make([]int, n)
	state := make([]int, n)
	var walk func(v int) int
	// depth[v] is the longest chain of uses above v; the groups are a DAG.
	walk = func(v int) int {
		if state[v] == 1 {
			return depth[v]
		}
		state[v] = 1
		for _, w := range next[v] {
			depth[v] = max(depth[v], walk(w)+1)
		}
		return depth[v]
	}
	for v := range n {
		walk(v)
	}
	return depth
}
