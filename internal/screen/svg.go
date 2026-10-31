package screen

import (
	"fmt"
	"slices"
	"sort"

	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/topology"
)

// The map's layout is a function of each unit's ID alone: its kind picks the
// column and its number the slot. IDs are never reused, so a node never moves
// between runs, a removed node leaves its slot empty, and a new one takes a
// fresh slot. A column exists once its kind has issued an ID, from the high
// water mark that never goes down.
var columns = []struct {
	kind model.Kind
	span int
}{
	{model.Feature, 1}, {model.Module, 2}, {model.Interface, 1}, {model.Event, 1}, {model.Data, 1}, {model.Config, 1},
}

const (
	colW, nodeW, nodeH, rowH = 176, 152, 46, 74
	pad, header              = 16, 46
)

type Map struct {
	Width, Height int
	Columns       []Label
	Nodes         []Node
	Edges         []Line
}

type Label struct {
	X    int
	Text string
}

type Node struct {
	ID, Name, Kind, Class, Title string
	X, Y, R                      int
	// Guess marks a unit whose purpose or responsibility only an agent vouches for.
	Guess bool
}

type Line struct {
	D, Class, Title string
}

type slot struct{ col, row int }

func Draw(units []model.Unit, in *topology.Intended, items []topology.Item, highWater map[model.Kind]int) Map {
	top := map[model.Kind]int{}
	for k, n := range highWater {
		top[k] = n
	}
	for _, u := range units {
		if _, n, err := model.ParseID(string(u.ID)); err == nil && n > top[u.Kind] {
			top[u.Kind] = n
		}
	}
	base := map[model.Kind]int{}
	var m Map
	cols, rows := 0, 0
	for _, c := range columns {
		if top[c.kind] == 0 {
			continue
		}
		base[c.kind] = cols
		m.Columns = append(m.Columns, Label{X: pad + cols*colW, Text: kindLabel(c.kind)})
		cols += c.span
		rows = max(rows, (top[c.kind]+c.span-1)/c.span)
	}
	span := map[model.Kind]int{}
	for _, c := range columns {
		span[c.kind] = c.span
	}
	place := func(id model.ID) (slot, bool) {
		k, n, err := model.ParseID(string(id))
		if _, ok := base[k]; err != nil || !ok {
			return slot{}, false
		}
		return slot{col: base[k] + (n-1)%span[k], row: (n - 1) / span[k]}, true
	}
	m.Width = pad*2 + max(cols, 1)*colW - (colW - nodeW)
	m.Height = header + max(rows, 1)*rowH + pad

	design := map[model.ID]bool{}
	var intendedEdges []topology.Edge
	if in != nil {
		for _, u := range in.Units {
			design[u.ID] = true
		}
		intendedEdges = in.Relations
	}
	effect := map[string]topology.Effect{}
	for _, it := range items {
		effect[it.Key()] = it.Effect
	}

	pos := map[model.ID]slot{}
	present := map[model.ID]bool{}
	for _, u := range units {
		s, ok := place(u.ID)
		if !ok || (u.Absent && !design[u.ID]) {
			continue
		}
		pos[u.ID] = s
		present[u.ID] = !u.Absent
		n := Node{ID: string(u.ID), Name: u.Name.Value, Kind: string(u.Kind), X: pad + s.col*colW, Y: header + s.row*rowH, R: radius(u.Kind)}
		n.Guess = u.Purpose.Guess() || u.Responsibility.Guess()
		switch {
		case u.Absent:
			n.Class, n.Title = "removed", "intended, gone from the code"
		case effect["contract-changed:"+string(u.ID)] != "":
			n.Class, n.Title = "changed", "contract signature changed"
		case effect["unit-added:"+string(u.ID)] != "":
			n.Class, n.Title = "added", "new below the line: a promotion candidate"
		case design[u.ID]:
			n.Class, n.Title = "design", "design unit"
		default:
			n.Class, n.Title = "below", "below the line"
		}
		if n.Guess {
			n.Title += "; purpose inferred, unconfirmed"
		}
		m.Nodes = append(m.Nodes, n)
	}
	sort.Slice(m.Nodes, func(i, j int) bool { return m.Nodes[i].ID < m.Nodes[j].ID })

	actual := map[topology.Edge]bool{}
	var guesses []topology.Edge
	for _, u := range units {
		if u.Absent {
			continue
		}
		for _, r := range u.Relations {
			e := topology.Edge{From: u.ID, To: r.To, Kind: r.Kind}
			switch {
			case !present[r.To]:
			case r.Counts():
				actual[e] = true
			case r.Guess() || (r.Origin == model.Adopted && r.Confirmation != model.Rejected):
				guesses = append(guesses, e)
			}
		}
	}
	add := func(e topology.Edge, class, title string) {
		a, okA := pos[e.From]
		b, okB := pos[e.To]
		if okA && okB {
			m.Edges = append(m.Edges, Line{D: route(a, b), Class: class, Title: e.String() + ": " + title})
		}
	}
	for _, e := range intendedEdges {
		if actual[e] {
			add(e, "kept", "intended")
		} else {
			add(e, "removed", "intended, gone from the code")
		}
	}
	var fresh []topology.Edge
	for e := range actual {
		if !slices.Contains(intendedEdges, e) {
			fresh = append(fresh, e)
		}
	}
	sort.Slice(fresh, func(i, j int) bool { return fresh[i].String() < fresh[j].String() })
	for _, e := range fresh {
		switch effect["new-relation:"+e.String()] {
		case topology.Blocked:
			add(e, "blocked", "new, crosses the intended boundary")
		case topology.Realign:
			add(e, "realign", "new, touches a design unit")
		default:
			add(e, "below", "below the line")
		}
	}
	for _, e := range guesses {
		add(e, "guess", "inferred, unconfirmed")
	}
	return m
}

func route(a, b slot) string {
	ax, ay := pad+a.col*colW, header+a.row*rowH+nodeH/2
	bx, by := pad+b.col*colW, header+b.row*rowH+nodeH/2
	// An edge that skips a column on the same row would run under the nodes
	// between, so it arcs over them.
	lift := 0
	if a.row == b.row && abs(a.col-b.col) > 1 {
		lift = rowH / 2
	}
	switch {
	case a.col < b.col:
		sx, ex := ax+nodeW, bx
		dx := max(28, (ex-sx)/2)
		return fmt.Sprintf("M%d %d C%d %d %d %d %d %d", sx, ay, sx+dx, ay-lift, ex-dx, by-lift, ex, by)
	case a.col > b.col:
		sx, ex := ax, bx+nodeW
		dx := max(28, (sx-ex)/2)
		return fmt.Sprintf("M%d %d C%d %d %d %d %d %d", sx, ay, sx-dx, ay-lift, ex+dx, by-lift, ex, by)
	}
	sx := ax + nodeW
	bulge := 18 + abs(by-ay)/4
	return fmt.Sprintf("M%d %d C%d %d %d %d %d %d", sx, ay, sx+bulge, ay, sx+bulge, by, sx, by)
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func radius(k model.Kind) int {
	switch {
	case k.IsContract():
		return nodeH / 2
	case k == model.Feature:
		return 12
	}
	return 4
}

func kindLabel(k model.Kind) string {
	return map[model.Kind]string{
		model.Feature: "Features", model.Module: "Modules", model.Interface: "Interfaces",
		model.Event: "Events", model.Data: "Data", model.Config: "Config",
	}[k]
}
