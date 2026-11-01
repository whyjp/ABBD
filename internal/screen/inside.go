package screen

import (
	"fmt"
	"slices"

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
	// Unwired names the contracts on the frame no inside unit is known to meet.
	Unwired []string
	// Inferred are the couplings a reader found that touch this module, which
	// the overview does not draw.
	Inferred []Coupling
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

const unitW, unitH = 168, 44

func (o *overview) inside(m model.ID, units []model.Unit) Inside {
	ins := Inside{ID: string(m), Name: o.name(m)}
	var inner []model.Unit
	for _, u := range model.Present(units) {
		if u.Parent == m {
			inner = append(inner, u)
		}
	}
	isInner := map[model.ID]bool{}
	for _, u := range inner {
		isInner[u.ID] = true
	}
	live := func(r model.Relation) bool { return !r.Dead }

	var nodes []lnode
	var edges []ledge
	for _, u := range inner {
		nodes = append(nodes, lnode{id: string(u.ID), w: unitW, h: unitH})
		for _, r := range u.Relations {
			if isInner[r.To] && live(r) {
				edges = append(edges, ledge{string(u.ID), string(r.To)})
			}
		}
	}
	const frameTop = 18
	l := arrange(nodes, edges, frameTop+chipH+40)

	// The frame's ports: the module's own contracts, then anything outside the
	// module an inside unit meets.
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
	for _, c := range o.contracts {
		cls := "design"
		if slices.Contains(o.providers[c], m) {
			addPort(port{id: c, exit: true, label: portLabel(o.name(c)), title: "이 모듈이 제공하는 계약: " + o.name(c), cls: cls})
		}
		if slices.Contains(o.users[c], m) {
			addPort(port{id: c, label: portLabel(o.name(c)), title: "이 모듈이 쓰는 계약: " + o.name(c), cls: cls})
		}
	}
	// outside names where a line to a unit outside the module ends: one of
	// the module's ports, or the module the unit belongs to.
	outside := func(id model.ID, exit bool) (model.ID, bool) {
		if seen[fmt.Sprint(id, true)] {
			return id, true
		}
		if seen[fmt.Sprint(id, false)] && !exit {
			return id, false
		}
		owner := id
		if p := o.units[id].Parent; p != "" {
			owner = p
		}
		if owner == m {
			return "", false
		}
		addPort(port{id: owner, exit: exit, label: map[bool]string{true: "← ", false: "→ "}[exit] + o.name(owner), title: fmt.Sprintf("계약 없이 %s와 닿는 곳", o.name(owner)), cls: "other"})
		return owner, exit
	}
	type line struct {
		from, to model.ID
		exit     bool
		kind     model.RelationKind
		guess    bool
		via      model.Via
		quote    string
		inner    bool
		// incoming is a line from another module into an inside unit.
		incoming bool
	}
	var lines []line
	for _, u := range inner {
		for _, r := range model.SortRelations(u.Relations) {
			if !live(r) || o.units[r.To].Absent {
				continue
			}
			ln := line{from: u.ID, to: r.To, kind: r.Kind, guess: r.Guess(), via: r.Via}
			if len(r.Evidence) > 0 {
				ln.quote = r.Evidence[0].String()
			}
			if isInner[r.To] {
				ln.inner = true
			} else {
				exit := r.Kind == model.Provides || seen[fmt.Sprint(r.To, true)]
				to, ex := outside(r.To, exit)
				if to == "" {
					continue
				}
				ln.to, ln.exit = to, ex
			}
			lines = append(lines, ln)
		}
	}
	for _, u := range model.Present(units) {
		if isInner[u.ID] || u.ID == m || u.Parent == m {
			continue
		}
		for _, r := range u.Relations {
			if isInner[r.To] && live(r) {
				owner := u.ID
				if u.Parent != "" {
					owner = u.Parent
				}
				to, _ := outside(owner, true)
				if to == "" {
					continue
				}
				lines = append(lines, line{from: r.To, to: to, exit: true, incoming: true, kind: r.Kind, guess: r.Guess(), via: r.Via, quote: fmt.Sprintf("%s %s %s", o.name(u.ID), r.Kind, o.name(r.To))})
			}
		}
	}

	center := func(id model.ID) int { p := l.pos[string(id)]; return p.X + p.W/2 }
	var tops, bottoms []attach
	other := map[string][]int{}
	for _, ln := range lines {
		if !ln.inner {
			k := fmt.Sprint(ln.to, ln.exit)
			other[k] = append(other[k], center(ln.from))
		}
	}
	for _, p := range ports {
		k := fmt.Sprint(p.id, p.exit)
		avg := l.width / 2
		if xs := other[k]; len(xs) > 0 {
			avg = 0
			for _, x := range xs {
				avg += x
			}
			avg /= len(xs)
		}
		a := attach{key: k, label: p.label, other: avg, chip: true}
		if p.exit {
			tops = append(tops, a)
		} else {
			bottoms = append(bottoms, a)
		}
	}
	need := func(as []attach) int {
		sum := 16
		for _, a := range as {
			sum += chipWidth(a.label) + 4
		}
		return sum
	}
	width := max(l.width, need(tops), need(bottoms), 360)
	if width > l.width {
		shift := (width - l.width) / 2
		for id, p := range l.pos {
			p.X += shift
			l.pos[id] = p
		}
		l.width = width
	}
	frame := rect{X: 6, Y: frameTop, W: width - 12, H: l.height - frameTop + 40}
	if len(inner) == 0 {
		frame.H = 120
	}
	at := map[string]Port{}
	for k, p := range spread(frame, true, tops) {
		at[k] = p
	}
	for k, p := range spread(frame, false, bottoms) {
		at[k] = p
	}
	d := Drawing{Width: width, Height: frame.Y + frame.H + 18, Frame: &frame}
	wired := map[string]bool{}
	for _, ln := range lines {
		if !ln.inner {
			wired[fmt.Sprint(ln.to, ln.exit)] = true
		}
	}
	for _, p := range ports {
		k := fmt.Sprint(p.id, p.exit)
		pt := at[k]
		pt.Contract, pt.Exit, pt.Class, pt.Title = string(p.id), p.exit, p.cls, p.title
		if !wired[k] {
			pt.Class += " unwired"
			pt.Title += ". 안의 어느 단위가 맡는지 아직 기록이 없습니다"
			if p.cls != "other" {
				ins.Unwired = append(ins.Unwired, o.name(p.id))
			}
		}
		d.Ports = append(d.Ports, pt)
	}
	for _, u := range inner {
		r := l.pos[string(u.ID)]
		b := Box{ID: string(u.ID), Name: u.Name.Value, Kind: string(u.Kind), X: r.X, Y: r.Y, W: r.W, H: r.H, Class: "inner", Sub: kindWord(u.Kind)}
		b.Label = fit(b.Name, r.W-16, 12)
		b.Title = fmt.Sprintf("%s %s", u.ID, u.Name.Value)
		if f := sentence(u); f != nil {
			b.Title += ": " + f.Value
		}
		d.Boxes = append(d.Boxes, b)
		iu := InsideUnit{ID: string(u.ID), Name: u.Name.Value, Kind: kindWord(u.Kind), Anchors: anchors(u)}
		if f := sentence(u); f != nil {
			iu.Sentence, iu.Guess = f.Value, f.Guess()
		}
		ins.Units = append(ins.Units, iu)
	}
	for _, ln := range lines {
		a := l.pos[string(ln.from)]
		var w Wire
		kind := string(ln.kind)
		if ln.guess {
			kind += " guess"
		}
		w.Kind, w.Via = kind, string(ln.via)
		if ln.quote != "" {
			w.Quotes = []string{ln.quote}
		}
		ra := l.rank[string(ln.from)]
		if ln.inner {
			b := l.pos[string(ln.to)]
			rb := l.rank[string(ln.to)]
			from, to := pt{a.X + a.W/2, a.Y + a.H}, pt{b.X + b.W/2, b.Y}
			if rb < ra {
				from, to = pt{a.X + a.W/2, a.Y}, pt{b.X + b.W/2, b.Y + b.H}
			}
			w.D = l.route(from, ra, to, rb)
			w.Title = fmt.Sprintf("%s %s %s", o.name(ln.from), ln.kind, o.name(ln.to))
		} else {
			p := at[fmt.Sprint(ln.to, ln.exit)]
			switch {
			case ln.incoming:
				w.D = l.route(pt{p.X + p.W/2, p.Y + chipH}, -1, pt{a.X + a.W/2, a.Y}, ra)
			case ln.exit:
				w.D = l.route(pt{a.X + a.W/2, a.Y}, ra, pt{p.X + p.W/2, p.Y + chipH}, -1)
			default:
				w.D = l.route(pt{a.X + a.W/2, a.Y + a.H}, ra, pt{p.X + p.W/2, p.Y - chipH}, len(l.ranks))
			}
			w.Title = fmt.Sprintf("%s %s %s", o.name(ln.from), ln.kind, o.name(ln.to))
			if ln.incoming {
				w.Title, w.Quotes = ln.quote, nil
			}
		}
		if ln.via != "" {
			w.Title += " (via " + string(ln.via) + ")"
		}
		if ln.guess {
			w.Title += ", 추정"
		}
		d.Wires = append(d.Wires, w)
	}
	ins.Drawing = d
	ins.Inferred = o.inferred(m, units)
	return ins
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
