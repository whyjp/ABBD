package screen

import (
	"fmt"
	"slices"
	"strings"

	"github.com/whyjp/abbd/internal/model"
)

// Inside is the second level: one module's units and how they connect, with
// the module's contracts on the frame and a line from the unit that
// implements or uses each one out to it.
type Inside struct {
	ID, Name string
	Drawing  Drawing
	// Units lists the inside units with the one sentence each is responsible for.
	Units []InsideUnit
	// Understanding is what pstack's how wrote about the module.
	Understanding *Understanding
	// Unwired are the module's contracts no inside unit is known to meet,
	// grouped by a shared first word. The drawing leaves them off the frame.
	Unwired []Group
	// Up is the view one level above; Trail names the levels from the top.
	Up    string
	Trail []string
	// Inferred are the couplings a reader found that touch this module, which
	// the overview does not draw.
	Inferred []Coupling
	// Focus are the views derived from this one when it is dense, one per
	// box (focus.go). graph.json leaves them out: they hold nothing the
	// stored graph does not.
	Focus []Focus `json:"-"`
}

// Group is a run of names that share a first word, shown once as Head.
type Group struct {
	Head  string `json:",omitempty"`
	Names []string
}

// Coupling is one INFERRED relation as a human checks it: its ends, how it
// arises, and the code it rests on.
type Coupling struct {
	From, To, Kind, Via string
	Quotes              []string
}

type InsideUnit struct {
	ID, Name, Kind string
	Sentence       string
	// Guess is true when only a reader vouches for the sentence.
	Guess   bool
	Anchors []string
}

const (
	unitMinW, unitMaxW = 140, 220
	frameTop           = 18
)

// line is one relation the inside may draw: between two inside units, or
// between an inside unit and a port on the frame.
type line struct {
	from, to model.ID
	exit     bool
	kind     model.RelationKind
	origin   string
	via      model.Via
	quote    string
	inner    bool
	// incoming is a line from another module into an inside unit.
	incoming bool
	// rels are the stored relations the line stands for.
	rels []string
}

func (ln line) String() string {
	return fmt.Sprintf("%s %s %s", ln.from, ln.kind, ln.to)
}

// inside draws m's view. With a focus, it draws the focus view of that
// child instead: the child, the children it meets, and only its own lines.
func (o *overview) inside(m model.ID, units []model.Unit, focus model.ID, seat map[string]int) Inside {
	t := o.tree
	ins := Inside{ID: string(m), Name: o.name(m), Up: "#overview"}
	if p := t.units[m].Parent; p != "" {
		ins.Up = "#m-" + string(p)
	}
	for _, id := range t.chain(m) {
		ins.Trail = append(ins.Trail, o.name(id))
	}
	var inner []model.Unit
	for _, id := range t.children[m] {
		inner = append(inner, t.units[id])
	}
	isContract := func(id model.ID) bool { return t.units[id].Kind.IsContract() }

	// The frame's ports: the module's own contracts, then anything outside the
	// module an inside unit meets, as the largest unit holding it that does
	// not hold this module.
	type port struct {
		id    model.ID
		exit  bool
		label string
		title string
		cls   string
	}
	var ports []port
	seen := map[string]bool{}
	addPort := func(p port) {
		k := fmt.Sprint(p.id, p.exit)
		if !seen[k] {
			seen[k] = true
			ports = append(ports, p)
		}
	}
	contract := func(c model.ID, exit bool) {
		if exit {
			addPort(port{id: c, exit: true, label: portLabel(o.name(c)), title: "이 모듈이 제공하는 계약: " + o.name(c), cls: "design"})
		} else {
			addPort(port{id: c, label: portLabel(o.name(c)), title: "이 모듈이 쓰는 계약: " + o.name(c), cls: "design"})
		}
	}
	for _, c := range o.contracts {
		if slices.Contains(o.providers[c], m) {
			contract(c, true)
		}
		if slices.Contains(o.users[c], m) {
			contract(c, false)
		}
	}
	for _, s := range t.sources(m) {
		for _, r := range t.units[s].Relations {
			if _, ok := t.units[r.To]; ok && !r.Dead && isContract(r.To) && r.Kind != model.DependsOn {
				if _, in := t.within(r.To, m); !in {
					contract(r.To, r.Kind == model.Provides)
				}
			}
		}
	}
	// outside is the port a line to id outside the module ends at. A line
	// in, or one that provides a contract, meets the port on top; a line out
	// that uses something meets its port below, even when the module also
	// provides that contract, rule 13.
	outside := func(id model.ID, exit bool) (model.ID, bool) {
		o2 := t.outside(id, m)
		if o2 == "" {
			return "", false
		}
		if isContract(o2) {
			contract(o2, exit)
			return o2, exit
		}
		addPort(port{id: o2, exit: exit, label: map[bool]string{true: "← ", false: "→ "}[exit] + lastPart(o.name(o2)), title: fmt.Sprintf("계약 없이 %s와 닿는 곳", o.name(o2)), cls: "other"})
		return o2, exit
	}
	var lines []line
	index := map[string]int{}
	add := func(ln line, rel string) {
		k := fmt.Sprint(ln.from, ln.to, ln.kind, ln.exit, ln.inner, ln.incoming, ln.origin)
		if i, ok := index[k]; ok {
			lines[i].rels = append(lines[i].rels, rel)
			return
		}
		index[k] = len(lines)
		ln.rels = []string{rel}
		lines = append(lines, ln)
	}
	for _, x := range inner {
		for _, s := range t.sources(x.ID) {
			for _, r := range model.SortRelations(t.units[s].Relations) {
				if _, ok := t.units[r.To]; !ok || r.Dead {
					continue
				}
				ln := line{from: x.ID, to: r.To, kind: r.Kind, origin: Badge(r.Origin), via: r.Via}
				if len(r.Evidence) > 0 {
					ln.quote = r.Evidence[0].String()
				}
				if to, ok := t.within(r.To, m); ok {
					if to == x.ID {
						continue
					}
					ln.to, ln.inner = to, true
				} else {
					exit := r.Kind == model.Provides
					to, ex := outside(r.To, exit)
					if to == "" {
						continue
					}
					ln.to, ln.exit = to, ex
				}
				add(ln, key(s, r.Kind, r.To))
			}
		}
	}
	var ids []model.ID
	for id := range t.units {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	for _, id := range ids {
		u := t.units[id]
		if _, in := t.within(id, m); in || id == m {
			continue
		}
		for _, r := range model.SortRelations(u.Relations) {
			to, ok := t.within(r.To, m)
			if !ok || r.Dead {
				continue
			}
			from, _ := outside(id, true)
			if from == "" {
				continue
			}
			add(line{from: to, to: from, exit: true, incoming: true, kind: r.Kind, origin: Badge(r.Origin), via: r.Via, quote: fmt.Sprintf("%s %s %s", o.name(id), r.Kind, o.name(r.To))}, key(id, r.Kind, r.To))
		}
	}

	if focus != "" {
		lines = slices.DeleteFunc(lines, func(ln line) bool { return ln.from != focus && !(ln.inner && ln.to == focus) })
		calls := map[model.ID]bool{}
		for _, ln := range lines {
			if ln.inner && ln.to == focus {
				calls[ln.from] = true
			}
		}
		ring := map[model.ID]bool{focus: true}
		for i, ln := range lines {
			if !ln.inner {
				continue
			}
			if ln.from == focus && calls[ln.to] {
				tw := twin(ln.to)
				if !ring[tw] {
					u := t.units[ln.to]
					u.ID = tw
					inner = append(inner, u)
				}
				lines[i].to = tw
			}
			ring[lines[i].from], ring[lines[i].to] = true, true
		}
		inner = slices.DeleteFunc(inner, func(u model.Unit) bool { return !ring[u.ID] })
	}
	var nodes []lnode
	var edges []ledge
	names := map[model.ID][]string{}
	for _, u := range inner {
		// A focus view names its boxes by their last path segment, as
		// frame ports are; the tooltip keeps the whole name.
		label := u.Name.Value
		if focus != "" {
			label = lastPart(label)
		}
		ls, w := lines2(label)
		names[u.ID] = ls
		nodes = append(nodes, lnode{id: string(u.ID), w: w, h: 26 + len(ls)*nameLine + 2})
	}
	// The frame's top and bottom take part in the layering, so a unit that
	// meets a port on top rises toward it and one that uses a port below
	// sinks toward it.
	for _, ln := range lines {
		switch {
		case ln.inner:
			edges = append(edges, ledge{string(ln.from), string(ln.to)})
		case ln.exit:
			edges = append(edges, ledge{frameTopNode, string(ln.from)})
		default:
			edges = append(edges, ledge{string(ln.from), frameBottomNode})
		}
	}
	nodes = append(nodes, lnode{id: frameTopNode}, lnode{id: frameBottomNode})
	// A focus view stands on its own, with no text beside it, so it takes
	// the overview's width.
	budget := insideBudget
	if focus != "" {
		budget = overviewBudget
	}
	l := arrange(nodes, edges, budget-12, seatOf(focus, seat))

	// A wired port sits on the side its lines flow from, rule 10: a contract
	// the module provides and a caller from outside on top, what the module
	// uses below. A side that cannot hold its chips hands the least
	// committed ones to the other side.
	wired := map[string][]model.ID{}
	for _, ln := range lines {
		if !ln.inner {
			k := fmt.Sprint(ln.to, ln.exit)
			wired[k] = append(wired[k], ln.from)
		}
	}
	below := map[string]bool{}
	lean := map[string]int{}
	var tops, bottoms []attach
	for _, p := range ports {
		k := fmt.Sprint(p.id, p.exit)
		us := wired[k]
		if len(us) == 0 {
			continue
		}
		x, r := 0, 0
		for _, u := range us {
			edge := frameTopNode
			if !p.exit {
				edge = frameBottomNode
			}
			x += l.toward(string(u), edge)
			r += l.row[string(u)]
		}
		lean[k] = 2*r - (len(l.rows)-1)*len(us)
		below[k] = !p.exit
		a := attach{key: k, label: p.label, other: x / len(us)}
		if below[k] {
			bottoms = append(bottoms, a)
		} else {
			tops = append(tops, a)
		}
	}
	need := func(as []attach) int {
		labels := []string{}
		for _, a := range as {
			labels = append(labels, a.label)
		}
		return chipsWidth(labels) + 12
	}
	// least is the port whose units sit nearest the other side: the lowest
	// of a crowded top, the highest of a crowded bottom.
	least := func(as []attach, down int) int {
		best := -1
		for i, a := range as {
			if best < 0 || down*lean[a.key] > down*lean[as[best].key] || lean[a.key] == lean[as[best].key] && a.key < as[best].key {
				best = i
			}
		}
		return best
	}
	for need(tops) > budget || need(bottoms) > budget {
		from, to, down := &tops, &bottoms, 1
		if need(bottoms) > budget {
			from, to, down = &bottoms, &tops, -1
		}
		i := least(*from, down)
		a := (*from)[i]
		if need(append(slices.Clone(*to), a)) > budget {
			break
		}
		*from = slices.Delete(*from, i, i+1)
		*to = append(*to, a)
		below[a.key] = !below[a.key]
	}
	// The frame never grows past the budget, rule 3: chips that still do
	// not fit fold into one "+N" chip that names them all.
	width := max(l.width, min(budget, max(need(tops), need(bottoms))), 360)
	if width > l.width {
		shift := (width - l.width) / 2
		for id, p := range l.pos {
			p.X += shift
			l.pos[id] = p
		}
		for k := range l.pass {
			l.pass[k] += shift
		}
		for _, as := range [][]attach{tops, bottoms} {
			for i := range as {
				as[i].other += shift
			}
		}
		l.width = width
	}
	frame := rect{X: 6, Y: frameTop, W: width - 12}
	at := map[string]Port{}
	for k, p := range settle(spread(frame, false, tops, 1), tops, frame) {
		at[k] = p
	}
	for k, p := range settle(spread(frame, true, bottoms, 1), bottoms, frame) {
		at[k] = p
	}
	l.top = frameTop + chipH + 3
	l.minGap, l.endGap = 32, 24
	unitEnd := func(id model.ID) end {
		r := l.pos[string(id)]
		return end{node: string(id), row: l.row[string(id)], lo: r.X + 8, hi: r.X + r.W - 8, key: string(id)}
	}
	portEnd := func(k string) end {
		p := at[k]
		row := -1
		if below[k] {
			row = len(l.rows)
		}
		return end{row: row, lo: p.X + 4, hi: p.X + p.W - 4, key: "port|" + k}
	}
	var reqs []wireReq
	for _, ln := range lines {
		r := wireReq{from: unitEnd(ln.from), kind: string(ln.kind)}
		switch {
		case ln.inner:
			r.to = unitEnd(ln.to)
		case ln.incoming:
			r.from, r.to = portEnd(fmt.Sprint(ln.to, ln.exit)), unitEnd(ln.from)
		default:
			r.to = portEnd(fmt.Sprint(ln.to, ln.exit))
		}
		reqs = append(reqs, r)
	}
	paths := l.route(reqs)

	frame.H = l.gapTop[len(l.rows)] + l.gapH[len(l.rows)] - frame.Y
	if len(bottoms) > 0 {
		frame.H += chipH + 3
	}
	if len(inner) == 0 {
		frame.H = max(frame.H, 80)
	}
	d := Drawing{Width: width, Height: frame.Y + frame.H + 18, Frame: &frame, Dots: junctions(pieces(paths), joinPoints(paths)), Relations: wanted(reqs)}
	var unwired []port
	for _, p := range ports {
		k := fmt.Sprint(p.id, p.exit)
		if len(wired[k]) == 0 {
			if p.cls != "other" {
				unwired = append(unwired, p)
			}
			continue
		}
		pt := at[k]
		if below[k] {
			pt.Y = frame.Y + frame.H
		}
		pt.Contract, pt.Exit, pt.Class, pt.Title, pt.Keys = string(p.id), p.exit, p.cls, p.title, []string{"port|" + k}
		pt.Href = viewOf(t, p.id)
		if j := slices.IndexFunc(d.Ports, func(q Port) bool { return q.X == pt.X && q.Y == pt.Y }); j >= 0 {
			q := &d.Ports[j]
			q.Title += "; " + pt.Title
			q.Contract, q.Href = "", ""
			q.Keys = append(q.Keys, pt.Keys...)
			continue
		}
		d.Ports = append(d.Ports, pt)
	}
	if focus == "" {
		ins.Unwired = groups(unwired, func(p port) string { return o.name(p.id) })
	}
	for _, u := range inner {
		r := l.pos[string(u.ID)]
		b := Box{ID: string(u.ID), Name: u.Name.Value, Kind: string(u.Kind), X: r.X, Y: r.Y, W: r.W, H: r.H, Class: "inner"}
		if len(t.children[u.ID]) > 0 {
			b.Href, b.Class = "#m-"+string(u.ID), "inner has"
		}
		if focus != "" && u.ID != focus {
			b.Href = "#" + focusID(m, base(u.ID))
		}
		ls := names[u.ID]
		for i, s := range ls {
			b.Lines = append(b.Lines, Text{S: s, X: r.X + 8, Y: r.Y + 19 + i*nameLine})
		}
		b.Sub = &Text{S: o.kindWord(u), X: r.X + 8, Y: r.Y + 19 + len(ls)*nameLine + 1}
		if base(u.ID) != u.ID {
			b.Sub.S += " · 위에도 있음"
			b.Class += " twin"
		}
		b.Title = fmt.Sprintf("%s %s", base(u.ID), u.Name.Value)
		if f := sentence(u); f != nil {
			b.Title += ": " + f.Value
		}
		if base(u.ID) != u.ID {
			b.Title += "\n" + twinNote
		}
		d.Boxes = append(d.Boxes, b)
		if base(u.ID) != u.ID {
			continue
		}
		iu := InsideUnit{ID: string(u.ID), Name: u.Name.Value, Kind: o.kindWord(u), Anchors: anchors(u)}
		if f := sentence(u); f != nil {
			iu.Sentence, iu.Guess = f.Value, f.Guess()
		}
		ins.Units = append(ins.Units, iu)
	}
	for _, p := range paths {
		var w Wire
		var titles []string
		origins := map[string]bool{}
		for j, i := range p.reqs {
			ln := lines[i]
			title := fmt.Sprintf("%s %s %s", o.name(ln.from), ln.kind, o.name(ln.to))
			if ln.incoming {
				title = ln.quote
			} else if ln.quote != "" {
				w.Quotes = append(w.Quotes, ln.quote)
			}
			if ln.via != "" {
				title += " (via " + string(ln.via) + ")"
				w.Via = string(ln.via)
			}
			if ln.origin != "extracted" {
				title += ", " + labels[ln.origin]
			}
			if s := p.starts[j]; s != noStart {
				if ln.origin != "extracted" {
					w.Marks = append(w.Marks, Mark{X: s.x, Y: s.y, Origin: ln.origin})
				}
				origins[ln.origin] = true
				w.Origin = ln.origin
			}
			titles = append(titles, title)
			w.Rels = append(w.Rels, ln.rels...)
		}
		ln := lines[p.reqs[0]]
		w.D, w.Kind, w.Title = p.D, string(ln.kind), strings.Join(titles, "\n")
		if len(origins) > 1 {
			w.Origin = "mixed"
		}
		if len(p.reqs) > 1 {
			w.Via = ""
		}
		if p.count > 1 {
			w.Count, w.LX, w.LY = p.count, p.label.x, p.label.y
		}
		w.Net, w.Joins, w.Back = p.net, p.joins, p.back
		d.Wires = append(d.Wires, w)
	}
	ins.Drawing = d
	if focus == "" {
		ins.Inferred = o.inferred(m, units)
	}
	return ins
}

// kindWord names a unit's kind under its name; a folded cycle says so.
func (o *overview) kindWord(u model.Unit) string {
	if o.tree.folds[base(u.ID)] {
		return "순환 묶음"
	}
	return kindWord(u.Kind)
}

// lastPart drops the leading path segments of a module's name, such as
// "internal/" or a package scope; the port's tooltip keeps the full name.
func lastPart(name string) string {
	if i := strings.LastIndex(name, "/"); i >= 0 && i < len(name)-1 && !strings.Contains(name, " ") {
		return name[i+1:]
	}
	return name
}

// lines2 sets an inside unit's name.
func lines2(name string) ([]string, int) { return setName(name, 13, 20, unitMinW, unitMaxW) }

// groups gathers names that share a first word, such as every "abbd"
// command, so a long run of them reads as one head and its rest.
func groups[T any](items []T, name func(T) string) []Group {
	byHead := map[string][]string{}
	var heads []string
	for _, it := range items {
		n := name(it)
		head, rest, ok := strings.Cut(n, " ")
		if !ok || rest == "" {
			head, rest = "", n
		}
		if _, ok := byHead[head]; !ok {
			heads = append(heads, head)
		}
		byHead[head] = append(byHead[head], rest)
	}
	var out []Group
	var single []string
	for _, h := range heads {
		if h == "" || len(byHead[h]) < 3 {
			for _, r := range byHead[h] {
				single = append(single, strings.TrimSpace(h+" "+r))
			}
			continue
		}
		out = append(out, Group{Head: h, Names: byHead[h]})
	}
	if len(single) > 0 {
		out = append(out, Group{Names: single})
	}
	return out
}

// inferred lists the live INFERRED relations with an end in module m or in a
// unit it contains.
func (o *overview) inferred(m model.ID, units []model.Unit) []Coupling {
	in := func(id model.ID) bool { return id == m || o.units[id].Parent == m }
	var out []Coupling
	for _, u := range model.Present(units) {
		for _, r := range model.SortRelations(u.Relations) {
			if !r.Guess() || r.Dead || o.units[r.To].Absent || !(in(u.ID) || in(r.To)) {
				continue
			}
			c := Coupling{From: o.name(u.ID), To: o.name(r.To), Kind: string(r.Kind), Via: string(r.Via)}
			for _, e := range r.Evidence {
				c.Quotes = append(c.Quotes, e.String())
			}
			out = append(out, c)
		}
	}
	return out
}

// sentence is the one thing a unit is responsible for: its responsibility,
// else its purpose.
func sentence(u model.Unit) *model.Fact {
	if u.Responsibility != nil && u.Responsibility.Value != "" {
		return u.Responsibility
	}
	if u.Purpose != nil && u.Purpose.Value != "" {
		return u.Purpose
	}
	return nil
}
