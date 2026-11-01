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
	// Via says how a semantic coupling arises; Quote is the code it rests on.
	Via   string `json:",omitempty"`
	Quote string `json:",omitempty"`
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
			n.Class, n.Title = "removed", "의도했으나 코드에서 사라짐"
		case effect["contract-changed:"+string(u.ID)] != "":
			n.Class, n.Title = "changed", "계약 시그니처 바뀜"
		case effect["unit-added:"+string(u.ID)] != "":
			n.Class, n.Title = "added", "선 아래 새 단위: 승격 후보"
		case design[u.ID]:
			n.Class, n.Title = "design", "설계 단위"
		default:
			n.Class, n.Title = "below", "선 아래"
		}
		if n.Guess {
			n.Title += "; purpose inferred, unconfirmed"
		}
		m.Nodes = append(m.Nodes, n)
	}
	sort.Slice(m.Nodes, func(i, j int) bool { return m.Nodes[i].ID < m.Nodes[j].ID })

	actual := map[topology.Edge]bool{}
	var guesses []model.Relation
	var from []model.ID
	for _, u := range units {
		if u.Absent {
			continue
		}
		for _, r := range u.Relations {
			e := topology.Edge{From: u.ID, To: r.To, Kind: r.Kind}
			switch {
			case !present[r.To] || r.Dead:
			case r.Counts():
				actual[e] = true
			case r.Guess() || (r.Origin == model.Adopted && r.Confirmation != model.Rejected):
				guesses, from = append(guesses, r), append(from, u.ID)
			}
		}
	}
	line := func(e topology.Edge, class, title string) *Line {
		a, okA := pos[e.From]
		b, okB := pos[e.To]
		if !okA || !okB {
			return nil
		}
		m.Edges = append(m.Edges, Line{D: route(a, b), Class: class, Title: e.String() + ": " + title})
		return &m.Edges[len(m.Edges)-1]
	}
	add := func(e topology.Edge, class, title string) { line(e, class, title) }
	for _, e := range intendedEdges {
		if actual[e] {
			add(e, "kept", "의도")
		} else {
			add(e, "removed", "의도했으나 코드에서 사라짐")
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
			add(e, "blocked", "새 결합, 의도한 경계를 넘음")
		case topology.Realign:
			add(e, "realign", "새 결합, 설계 단위에 닿음")
		default:
			add(e, "below", "선 아래")
		}
	}
	for i, r := range guesses {
		e := topology.Edge{From: from[i], To: r.To, Kind: r.Kind}
		title := "추정, 미확인"
		if r.Via != "" {
			title += ", via " + string(r.Via)
		}
		if len(r.Evidence) > 0 {
			title += ": " + r.Evidence[0].String()
		}
		if l := line(e, "guess", title); l != nil && len(r.Evidence) > 0 {
			l.Via, l.Quote = string(r.Via), r.Evidence[0].String()
		}
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
		model.Feature: "기능", model.Module: "모듈", model.Interface: "인터페이스",
		model.Event: "이벤트", model.Data: "데이터", model.Config: "설정",
	}[k]
}
