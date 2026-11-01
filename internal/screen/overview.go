package screen

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/whyjp/abbd/internal/model"
)

// Drawing is one SVG picture: the overview of modules, or one module's inside.
type Drawing struct {
	Width, Height int
	Boxes         []Box
	Ports         []Port
	Wires         []Wire
	// Frame is the module's own boundary on its inside drawing.
	Frame *rect `json:",omitempty"`
}

// Box is a module on the overview, or a unit inside a module.
type Box struct {
	ID, Name, Label, Kind, Class, Title string
	// Sub is a second line: the kind of an inside unit.
	Sub     string `json:",omitempty"`
	X, Y, W int
	H       int
	// Href opens the module's inside.
	Href string `json:",omitempty"`
}

// Port is a contract on a module's boundary: an exit on top for one it
// provides, an entry below for one it uses. Another module, met without a
// contract, is a port on an inside drawing's frame.
type Port struct {
	Contract, Label, Class, Title string
	Exit                          bool
	X, Y, W                       int
}

// Wire is one line. Kind says what it stands for, and the page draws each
// kind its own way: use through a contract, a dependency that bypasses every
// contract, and, inside a module, a coupling only a reader found.
type Wire struct {
	D, Kind, Title string
	Via            string   `json:",omitempty"`
	Quotes         []string `json:",omitempty"`
}

const (
	boxH, chipH = 60, 17
	chipPx      = 11
)

func kindWord(k model.Kind) string {
	return map[model.Kind]string{
		model.Module: "모듈", model.Interface: "인터페이스", model.Event: "이벤트",
		model.Data: "데이터", model.Config: "설정", model.Feature: "기능",
	}[k]
}

// portLabel drops a trailing parenthesis, which names where the contract
// lives, and a leading declaration keyword; the box the port sits on and the
// port's shape already say both, and a chip has little room.
func portLabel(name string) string {
	if i := strings.LastIndex(name, " ("); i > 0 && strings.HasSuffix(name, ")") {
		name = name[:i]
	}
	if word, rest, ok := strings.Cut(name, " "); ok && rest != "" {
		switch word {
		case "trait", "interface", "struct", "enum", "type", "class", "protocol":
			name = rest
		}
	}
	return name
}

func chipWidth(label string) int { return min(130, textWidth(label, chipPx)+12) }

// overview is what the overview's layout may depend on: the top-level units
// and the relations the script read or a human stated between them. A
// module's inside or a reader's guess never moves a box.
type overview struct {
	units     map[model.ID]model.Unit
	design    []model.ID
	relations []model.Edge
	modules   []model.ID
	contracts []model.ID
	providers map[model.ID][]model.ID
	users     map[model.ID][]model.ID
	layout    layout
	boxes     map[model.ID]Box
}

// top lists the top-level units, those with no parent, and the drawn
// relations between them.
func top(units []model.Unit) ([]model.ID, []model.Edge) {
	var ids []model.ID
	var rels []model.Edge
	present := map[model.ID]bool{}
	for _, u := range model.Present(units) {
		if u.Parent == "" {
			ids = append(ids, u.ID)
			present[u.ID] = true
		}
	}
	for _, u := range model.Present(units) {
		for _, r := range u.Relations {
			if present[u.ID] && present[r.To] && r.Counts() {
				rels = append(rels, model.Edge{From: u.ID, To: r.To, Kind: r.Kind})
			}
		}
	}
	sort.Slice(rels, func(i, j int) bool { return rels[i].String() < rels[j].String() })
	return ids, rels
}

func planOverview(units []model.Unit) *overview {
	o := &overview{units: map[model.ID]model.Unit{}, providers: map[model.ID][]model.ID{}, users: map[model.ID][]model.ID{}, boxes: map[model.ID]Box{}}
	for _, u := range units {
		o.units[u.ID] = u
	}
	o.design, o.relations = top(units)
	slices.Sort(o.design)
	isModule := map[model.ID]bool{}
	for _, id := range o.design {
		k, _, _ := model.ParseID(string(id))
		switch {
		case k == model.Module:
			o.modules = append(o.modules, id)
			isModule[id] = true
		case k.IsContract():
			o.contracts = append(o.contracts, id)
		}
	}
	add := func(m map[model.ID][]model.ID, k, v model.ID) {
		if !slices.Contains(m[k], v) {
			m[k] = append(m[k], v)
		}
	}
	for _, c := range o.contracts {
		if p := o.units[c].Parent; isModule[p] {
			add(o.providers, c, p)
		}
	}
	for _, e := range o.relations {
		if !isModule[e.From] || !slices.Contains(o.contracts, e.To) {
			continue
		}
		if e.Kind == model.Provides {
			add(o.providers, e.To, e.From)
		} else {
			add(o.users, e.To, e.From)
		}
	}
	for _, c := range o.contracts {
		slices.Sort(o.providers[c])
		slices.Sort(o.users[c])
		o.users[c] = slices.DeleteFunc(o.users[c], func(u model.ID) bool { return slices.Contains(o.providers[c], u) })
	}

	exits, entries := map[model.ID][]model.ID{}, map[model.ID][]model.ID{}
	var edges []ledge
	for _, c := range o.contracts {
		for _, p := range o.providers[c] {
			exits[p] = append(exits[p], c)
		}
		for _, u := range o.users[c] {
			entries[u] = append(entries[u], c)
			if len(o.providers[c]) == 0 {
				edges = append(edges, ledge{string(u), string(c)})
			}
			for _, p := range o.providers[c] {
				edges = append(edges, ledge{string(u), string(p)})
			}
		}
	}
	for _, e := range o.relations {
		if e.Kind == model.DependsOn && isModule[e.From] && isModule[e.To] {
			edges = append(edges, ledge{string(e.From), string(e.To)})
		}
	}
	var nodes []lnode
	for _, m := range o.modules {
		name := o.units[m].Name.Value
		if name == "" {
			name = string(m)
		}
		w := max(150, min(260, textWidth(name, 13)+28))
		for _, side := range [][]model.ID{exits[m], entries[m]} {
			sum := 8
			for _, c := range side {
				sum += chipWidth(portLabel(o.name(c))) + 4
			}
			w = max(w, min(400, sum))
		}
		nodes = append(nodes, lnode{id: string(m), w: w, h: boxH})
	}
	for _, c := range o.contracts {
		if len(o.providers[c]) == 0 {
			nodes = append(nodes, lnode{id: string(c), w: chipWidth(portLabel(o.name(c))) + 16, h: chipH + 12})
		}
	}
	o.layout = arrange(nodes, edges, padY+14)
	return o
}

func (o *overview) name(id model.ID) string {
	if u, ok := o.units[id]; ok && u.Name.Value != "" {
		return u.Name.Value
	}
	return string(id)
}

// side places what attaches to one edge of a box: the labelled contract
// chips first, in the order of what they lead to, then plain anchors.
type attach struct {
	key   string
	label string
	other int
	chip  bool
}

func spread(r rect, top bool, items []attach) map[string]Port {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].chip != items[j].chip {
			return items[i].chip
		}
		if items[i].other != items[j].other {
			return items[i].other < items[j].other
		}
		return items[i].key < items[j].key
	})
	out := map[string]Port{}
	y := r.Y
	if !top {
		y = r.Y + r.H
	}
	var chips, plain []attach
	for _, a := range items {
		if a.chip {
			chips = append(chips, a)
		} else {
			plain = append(plain, a)
		}
	}
	// Chips on one side that share a first word, such as "abbd" on every CLI
	// command, show without it.
	if len(chips) > 1 {
		first, _, _ := strings.Cut(chips[0].label, " ")
		shared := first != ""
		for _, c := range chips {
			w, rest, ok := strings.Cut(c.label, " ")
			shared = shared && ok && w == first && rest != ""
		}
		if shared {
			for i := range chips {
				chips[i].label = strings.TrimPrefix(chips[i].label, first+" ")
			}
		}
	}
	// A side with more chips than fit readably keeps the first ones and
	// folds the rest into one "+N" chip; their lines all meet it.
	const least = 64
	fold := 0
	need := 0
	for _, c := range chips {
		need += min(chipWidth(c.label), least) + 4
	}
	if fits := max(1, (r.W-8)/(least+4)); need > r.W-8 && len(chips) > fits {
		fold = len(chips) - (fits - 1)
	}
	shown := chips[:len(chips)-fold]
	labels := make([]string, len(shown))
	for i, c := range shown {
		labels[i] = c.label
	}
	if fold > 0 {
		labels = append(labels, fmt.Sprintf("+%d", fold))
	}
	room := r.W - 8 - 4*len(labels)
	want := 0
	for _, l := range labels {
		want += chipWidth(l)
	}
	x := r.X + 4
	for i, l := range labels {
		w := chipWidth(l)
		if want > room && want > 0 {
			w = max(24, w*room/want)
		}
		x += 4
		p := Port{X: x, Y: y, W: w, Label: fit(l, w-8, chipPx)}
		if i < len(shown) {
			out[shown[i].key] = p
		} else {
			for _, c := range chips[len(shown):] {
				out[c.key] = p
			}
		}
		x += w
	}
	for i, a := range plain {
		px := r.X + r.W*(2*i+1)/(2*len(plain))
		out[a.key] = Port{X: px, Y: y}
	}
	return out
}

func (o *overview) draw(href func(model.ID) string) Drawing {
	l := &o.layout
	d := Drawing{Width: max(l.width, 320), Height: l.height + padY + 4}
	center := func(id model.ID) int {
		p := l.pos[string(id)]
		return p.X + p.W/2
	}
	isModule := func(id model.ID) bool { return slices.Contains(o.modules, id) }

	type wireSpec struct {
		kind, title    string
		from, to       model.ID
		fromKey, toKey string
	}
	var specs []wireSpec
	top, bottom := map[model.ID][]attach{}, map[model.ID][]attach{}
	chipKey := func(box, c model.ID, exit bool) string { return fmt.Sprintf("%s|%s|%v", box, c, exit) }

	for _, c := range o.contracts {
		for _, p := range o.providers[c] {
			other := 0
			for _, u := range o.users[c] {
				other += center(u)
			}
			if n := len(o.users[c]); n > 0 {
				other /= n
			}
			top[p] = append(top[p], attach{key: chipKey(p, c, true), label: portLabel(o.name(c)), other: other, chip: true})
		}
		for _, u := range o.users[c] {
			other := 0
			for _, p := range o.providers[c] {
				other += center(p)
			}
			if n := len(o.providers[c]); n > 0 {
				other /= n
			} else {
				other = center(c)
			}
			bottom[u] = append(bottom[u], attach{key: chipKey(u, c, false), label: portLabel(o.name(c)), other: other, chip: true})
			targets := o.providers[c]
			if len(targets) == 0 {
				targets = []model.ID{c}
			}
			for _, p := range targets {
				title := fmt.Sprintf("%s → %s: %s 계약을 거쳐 씁니다", o.name(u), o.name(p), o.name(c))
				toKey := chipKey(p, c, true)
				if p == c {
					toKey = string(c)
				}
				specs = append(specs, wireSpec{kind: "contract", title: title, from: u, to: p, fromKey: chipKey(u, c, false), toKey: toKey})
			}
		}
	}
	covered := func(a, b model.ID) bool {
		for _, c := range o.contracts {
			if slices.Contains(o.users[c], a) && slices.Contains(o.providers[c], b) {
				return true
			}
		}
		return false
	}
	for _, e := range o.relations {
		if e.Kind != model.DependsOn || !isModule(e.From) || !isModule(e.To) || covered(e.From, e.To) {
			continue
		}
		k := fmt.Sprintf("bypass>%s", e)
		s := wireSpec{kind: "bypass", title: fmt.Sprintf("%s → %s: 계약을 거치지 않는 직접 의존", o.name(e.From), o.name(e.To)), from: e.From, to: e.To, fromKey: "a|" + k, toKey: "b|" + k}
		bottom[e.From] = append(bottom[e.From], attach{key: s.fromKey, other: center(e.To)})
		top[e.To] = append(top[e.To], attach{key: s.toKey, other: center(e.From)})
		specs = append(specs, s)
	}

	at := map[string]Port{}
	for _, m := range o.modules {
		r := l.pos[string(m)]
		for k, p := range spread(r, true, top[m]) {
			at[k] = p
		}
		for k, p := range spread(r, false, bottom[m]) {
			at[k] = p
		}
	}
	for _, c := range o.contracts {
		if r, ok := l.pos[string(c)]; ok {
			at[string(c)] = Port{X: r.X + r.W/2, Y: r.Y}
		}
	}

	for _, m := range o.modules {
		r := l.pos[string(m)]
		u := o.units[m]
		b := Box{ID: string(m), Name: o.name(m), Kind: string(u.Kind), X: r.X, Y: r.Y, W: r.W, H: r.H, Class: "design", Href: href(m)}
		b.Label = fit(b.Name, r.W-20, 13)
		b.Title = fmt.Sprintf("%s %s. 누르면 이 모듈의 안을 봅니다", m, b.Name)
		d.Boxes = append(d.Boxes, b)
	}
	// A folded "+N" chip stands for several contracts at one place: it is
	// drawn once, names them all, and is marked when any of them is.
	folded := map[[3]int]int{}
	addPort := func(pt Port, what, name string) {
		k := [3]int{pt.X, pt.Y, map[bool]int{true: 1}[pt.Exit]}
		if i, ok := folded[k]; ok {
			q := &d.Ports[i]
			q.Title += ", " + name
			q.Contract = ""
			return
		}
		folded[k] = len(d.Ports)
		pt.Title = what + ": " + name
		d.Ports = append(d.Ports, pt)
	}
	for _, c := range o.contracts {
		if r, ok := l.pos[string(c)]; ok {
			d.Boxes = append(d.Boxes, Box{ID: string(c), Name: o.name(c), Label: fit(portLabel(o.name(c)), r.W-12, chipPx), Kind: string(o.units[c].Kind), Class: "loose design", X: r.X, Y: r.Y, W: r.W, H: r.H, Title: o.name(c) + ": 그림 안에 제공하는 모듈이 없는 계약"})
		}
		for _, p := range o.providers[c] {
			pt := at[chipKey(p, c, true)]
			pt.Contract, pt.Exit, pt.Class = string(c), true, "design"
			addPort(pt, fmt.Sprintf("%s가 제공하는 계약", o.name(p)), o.name(c))
		}
		for _, u := range o.users[c] {
			pt := at[chipKey(u, c, false)]
			pt.Contract, pt.Class = string(c), "design"
			addPort(pt, fmt.Sprintf("%s가 쓰는 계약", o.name(u)), o.name(c))
		}
	}
	// One line per user and provider, however many contracts it carries:
	// it leaves between the user's chips and arrives between the provider's.
	var merged []wireSpec
	keys := map[[2]model.ID][][2]string{}
	at2 := map[[2]model.ID]int{}
	for _, s := range specs {
		pair := [2]model.ID{s.from, s.to}
		if s.kind != "contract" {
			merged = append(merged, s)
			continue
		}
		if i, ok := at2[pair]; ok {
			merged[i].title += "; " + s.title
		} else {
			at2[pair] = len(merged)
			merged = append(merged, s)
		}
		keys[pair] = append(keys[pair], [2]string{s.fromKey, s.toKey})
	}
	middle := func(ks []string) (int, int) {
		lo, hi, y := 1<<30, -1<<30, 0
		for _, k := range ks {
			p := at[k]
			lo, hi, y = min(lo, p.X+p.W/2), max(hi, p.X+p.W/2), p.Y
		}
		return (lo + hi) / 2, y
	}
	for _, s := range merged {
		var fx, fy, tx, ty int
		if ks, ok := keys[[2]model.ID{s.from, s.to}]; ok && s.kind == "contract" {
			var fs, ts []string
			for _, k := range ks {
				fs, ts = append(fs, k[0]), append(ts, k[1])
			}
			fx, fy = middle(fs)
			tx, ty = middle(ts)
		} else {
			f, t := at[s.fromKey], at[s.toKey]
			fx, fy, tx, ty = f.X+f.W/2, f.Y, t.X+t.W/2, t.Y
		}
		ra, rb := l.rank[string(s.from)], l.rank[string(s.to)]
		if rb <= ra {
			// The used box sits level with or above its user: leave from the
			// top of the user and arrive at the bottom of the used box.
			if pos, ok := l.pos[string(s.to)]; ok && rb < ra {
				ty = pos.Y + pos.H
				fy = l.pos[string(s.from)].Y
			}
		}
		w := Wire{D: l.route(pt{fx, fy}, ra, pt{tx, ty}, rb), Kind: s.kind, Title: s.title}
		d.Wires = append(d.Wires, w)
	}
	return d
}
