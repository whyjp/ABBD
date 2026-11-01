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
	// Dots mark where lines split or merge.
	Dots []Dot `json:",omitempty"`
	// Frame is the module's own boundary on its inside drawing.
	Frame *rect `json:",omitempty"`
}

// Box is a module on the overview, or a unit inside a module.
type Box struct {
	ID, Name, Kind, Class, Title string
	// Lines are the name as set in the box, one or two lines.
	Lines []Text
	// Sub is a last line: the kind of an inside unit.
	Sub     *Text `json:",omitempty"`
	X, Y, W int
	H       int
	// Href opens the module's inside.
	Href string `json:",omitempty"`
}

// Text is one line of a label and where it starts; Mid centres it on X.
type Text struct {
	S    string
	X, Y int
	Mid  bool `json:",omitempty"`
}

// Port is a contract on a module's boundary: an exit for one it provides, an
// entry for one it uses. Another module, met without a contract, is a port
// on an inside drawing's frame. Below says the chip sits on a bottom edge.
type Port struct {
	Contract, Label, Class, Title string
	Exit                          bool
	Below                         bool `json:",omitempty"`
	X, Y, W                       int
	// Href opens the view of the unit a frame port stands for.
	Href string `json:",omitempty"`
	// Rels are the stored relations the chip stands for.
	Rels []string `json:",omitempty"`
	// Key names a frame port as the lines that join it do.
	Key string `json:"-"`
}

// Wire is one line, or a bundle of lines into one place. Kind says what it
// stands for, and the page draws each kind its own way. Origin says where
// the relation came from; the page marks a reader's or a human's relation
// with a small circle at its start, not with the line's dash.
type Wire struct {
	D, Kind, Title string
	Origin         string `json:",omitempty"`
	// Joins says the path ends on a bundle's trunk, not at a box, so it
	// carries no arrowhead.
	Joins bool `json:",omitempty"`
	// Back says the line runs against the reading direction: up, or left
	// within a row. The page draws it dashed.
	Back bool `json:",omitempty"`
	// Both says the path starts at a box that the line also reaches: two
	// modules that use each other. The page draws a head at its start too.
	Both   bool     `json:",omitempty"`
	Via    string   `json:",omitempty"`
	Quotes []string `json:",omitempty"`
	Marks  []Mark   `json:",omitempty"`
	// Rels are the stored relations the line stands for.
	Rels []string `json:",omitempty"`
	// Ends name what each line it draws joins, for the checker.
	Ends [][2]string `json:"-"`
	// Count is how many lines a bundle carries, drawn at LX, LY. Net groups
	// the paths of one bundle drawn as several paths.
	Count  int `json:",omitempty"`
	Net    int `json:",omitempty"`
	LX, LY int `json:",omitempty"`
}

// Mark is where a line from a reader or a human leaves its unit.
type Mark struct {
	X, Y   int
	Origin string
}

const (
	boxH, chipH = 60, 17
	// chipRow is how far each further row of chips sits inside a box. A
	// module whose chips need more than chipWrap wraps them into at most
	// chipRowsMax rows instead of widening, so lines can pass beside it.
	chipRow     = 20
	chipWrap    = 480
	chipRowsMax = 2
	chipPx      = 11
	nameLine    = 16
	// chipMax and boxMax are the hard widths past which a label is cut and
	// its full text left to the tooltip, rule 4.
	chipMax = 220
	boxMax  = 280
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

func chipWidth(label string) int { return min(chipMax, textWidth(label, chipPx)+12) }

// shorten drops a first word every label on one side shares, such as "abbd"
// on every CLI command.
func shorten(labels []string) []string {
	if len(labels) < 2 {
		return labels
	}
	first, _, _ := strings.Cut(labels[0], " ")
	for _, l := range labels {
		w, rest, ok := strings.Cut(l, " ")
		if !ok || w != first || rest == "" {
			return labels
		}
	}
	out := make([]string, len(labels))
	for i, l := range labels {
		out[i] = strings.TrimPrefix(l, first+" ")
	}
	return out
}

func chipsWidth(labels []string) int { return chipRowWidth(shorten(labels)) }

// overview is what the overview's layout may depend on: the top-level units
// and the live relations between them, a group's members' relations drawn as
// the group's. A reader's coupling is drawn too, because sync checks its
// quote every time; a dead one, whose quote is gone, is not. A module's
// inside never moves a box.
type overview struct {
	tree      tree
	units     map[model.ID]model.Unit
	design    []model.ID
	relations []model.Edge
	modules   []model.ID
	contracts []model.ID
	providers map[model.ID][]model.ID
	users     map[model.ID][]model.ID
	layout    layout
	names     map[model.ID][]string
	// chipRows are how many rows of chips each module's top and bottom hold.
	chipRows map[model.ID][2]int
	// carry are the contracts each drawn contract line carries, by the
	// user and the provider it joins, in the order lines are drawn.
	carry map[[2]model.ID][]model.ID
	pairs [][2]model.ID
	// chips sit on each module's top, where lines arrive at what it
	// provides, and on its bottom, where lines leave for what it uses.
	chips map[model.ID][2][]chip
	// rels are the stored relations each drawn edge stands for, by edge,
	// and origins say where each stored relation came from.
	rels    map[string][]string
	origins map[string]model.Origin
}

// top lists the top-level units, those with no parent, and the drawn
// relations between them, with the stored relations each one stands for. A
// relation from anywhere inside a top-level unit to another one crosses the
// boundary the overview shows, so it is drawn as that unit's.
func top(t tree) ([]model.ID, []model.Edge, map[string][]string, map[string]model.Origin) {
	ids := slices.Clone(t.children[""])
	var rels []model.Edge
	via := map[string][]string{}
	origins := map[string]model.Origin{}
	for _, id := range ids {
		for _, s := range t.subtree(id) {
			for _, r := range t.units[s].Relations {
				to, ok := t.within(r.To, "")
				if _, live := t.units[r.To]; !live || !ok || to == id || r.Dead {
					continue
				}
				e := model.Edge{From: id, To: to, Kind: r.Kind}
				if len(via[e.String()]) == 0 {
					rels = append(rels, e)
				}
				via[e.String()] = append(via[e.String()], key(s, r.Kind, r.To))
				origins[key(s, r.Kind, r.To)] = r.Origin
			}
		}
	}
	sort.Slice(rels, func(i, j int) bool { return rels[i].String() < rels[j].String() })
	return ids, rels, via, origins
}

// origin is what a drawn edge rests on: the code when the script read any
// relation it stands for, else a human's word, else a reader's reading.
func (o *overview) origin(rels []string) string {
	best := model.Inferred
	for _, r := range rels {
		switch o.origins[r] {
		case model.Extracted:
			return Badge(model.Extracted)
		case model.UserStated:
			best = model.UserStated
		}
	}
	return Badge(best)
}

// chip is one contract chip on a module's edge. On the overview a chip
// stands for the contracts one drawn line carries there, so a module that
// provides twenty commands to one caller shows one chip, not twenty, and its
// box keeps its size (rule 21). The contracts no line in the view reaches
// fold into one "+N" chip.
type chip struct {
	key, label, title string
	contracts         []model.ID
	rels              []string
}

func chipKey(at model.ID, cs []model.ID, exit bool) string { return fmt.Sprint(at, exit, cs) }

// chipFor names a chip after its one contract, or counts its contracts.
func (o *overview) chipFor(at model.ID, cs []model.ID, exit bool) chip {
	var names []string
	for _, c := range cs {
		names = append(names, o.name(c))
	}
	ch := chip{key: chipKey(at, cs, exit), contracts: cs, label: fmt.Sprintf("계약 %d개", len(cs)), title: strings.Join(names, ", ")}
	if len(cs) == 1 {
		ch.label = portLabel(names[0])
	}
	for _, c := range cs {
		ch.rels = append(ch.rels, o.between(at, c)...)
	}
	return ch
}

func planOverview(units []model.Unit) *overview {
	o := &overview{tree: newTree(units), units: map[model.ID]model.Unit{}, providers: map[model.ID][]model.ID{}, users: map[model.ID][]model.ID{}, names: map[model.ID][]string{}, chipRows: map[model.ID][2]int{}, carry: map[[2]model.ID][]model.ID{}, chips: map[model.ID][2][]chip{}}
	for _, u := range units {
		o.units[u.ID] = u
	}
	for id := range o.tree.folds {
		o.units[id] = o.tree.units[id]
	}
	o.design, o.relations, o.rels, o.origins = top(o.tree)
	slices.Sort(o.design)
	isModule := map[model.ID]bool{}
	for _, id := range o.design {
		k, _, _ := model.ParseID(string(id))
		switch {
		case k == model.Module || k == model.Feature:
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

	var edges []ledge
	for _, c := range o.contracts {
		targets := o.providers[c]
		if len(targets) == 0 {
			targets = []model.ID{c}
		}
		for _, u := range o.users[c] {
			for _, p := range targets {
				k := [2]model.ID{u, p}
				if len(o.carry[k]) == 0 {
					o.pairs = append(o.pairs, k)
				}
				o.carry[k] = append(o.carry[k], c)
				edges = append(edges, ledge{string(u), string(p)})
			}
		}
	}
	put := func(m model.ID, side int, ch chip) {
		cs := o.chips[m]
		if !slices.ContainsFunc(cs[side], func(q chip) bool { return q.key == ch.key }) {
			cs[side] = append(cs[side], ch)
			o.chips[m] = cs
		}
	}
	for _, k := range o.pairs {
		put(k[0], 1, o.chipFor(k[0], o.carry[k], false))
		if isModule[k[1]] {
			put(k[1], 0, o.chipFor(k[1], o.carry[k], true))
		}
	}
	unreached := map[model.ID][]model.ID{}
	for _, c := range o.contracts {
		if len(o.users[c]) == 0 {
			for _, p := range o.providers[c] {
				unreached[p] = append(unreached[p], c)
			}
		}
	}
	for _, m := range o.modules {
		if cs := unreached[m]; len(cs) > 0 {
			ch := o.chipFor(m, cs, true)
			ch.key, ch.label = fmt.Sprint(m, "+"), fmt.Sprintf("+%d 계약", len(cs))
			if len(cs) == 1 {
				ch.label = "+1 계약"
			}
			put(m, 0, ch)
		}
	}
	labels := func(cs []chip) []string {
		var out []string
		for _, c := range cs {
			out = append(out, c.label)
		}
		return out
	}
	for _, e := range o.relations {
		if e.Kind == model.DependsOn && isModule[e.From] && isModule[e.To] {
			edges = append(edges, ledge{string(e.From), string(e.To)})
		}
	}
	var nodes []lnode
	for _, m := range o.modules {
		ls, w := setName(o.name(m), 13, 28, 150, boxMax)
		o.names[m] = ls
		exits, entries := labels(o.chips[m][0]), labels(o.chips[m][1])
		w = max(w, min(chipWrap, max(chipsWidth(exits), chipsWidth(entries))))
		rows := [2]int{max(1, chipRows(exits, w, chipRowsMax)), max(1, chipRows(entries, w, chipRowsMax))}
		o.chipRows[m] = rows
		nodes = append(nodes, lnode{id: string(m), w: w, h: boxH + (len(ls)-1)*nameLine + (rows[0]+rows[1]-2)*chipRow})
	}
	for _, c := range o.contracts {
		if len(o.providers[c]) == 0 {
			ls, w := setName(portLabel(o.name(c)), chipPx, 16, 40, chipMax+16)
			o.names[c] = ls
			nodes = append(nodes, lnode{id: string(c), w: w, h: chipH + 12 + (len(ls)-1)*14})
		}
	}
	o.layout = arrange(nodes, edges, overviewBudget)
	o.layout.top = padY
	return o
}

func (o *overview) name(id model.ID) string {
	if u, ok := o.units[id]; ok && u.Name.Value != "" {
		return u.Name.Value
	}
	return string(id)
}

// attach is what sits on one edge of a box: a labelled contract chip, in
// the order of what it leads to.
type attach struct {
	key   string
	label string
	other int
}

// spread sets the chips on one edge of r, left to right in the order of
// what they lead to, in at most rows rows: a full row starts the next one
// further inside the box. The last row keeps what fits and folds the rest
// into one "+N" chip.
func spread(r rect, below bool, items []attach, rows int) map[string]Port {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].other != items[j].other {
			return items[i].other < items[j].other
		}
		return items[i].key < items[j].key
	})
	labels := make([]string, len(items))
	for i, a := range items {
		labels[i] = a.label
	}
	labels = shorten(labels)
	out := map[string]Port{}
	for row, start := 0, 0; start < len(labels); row++ {
		end := len(labels)
		if row < rows-1 {
			end = rowEnd(labels, start, r.W)
		}
		shown := end
		for shown > start+1 && chipRowWidth(labels[start:shown])+chipWidth(fmt.Sprintf("+%d", end-shown)) > r.W && chipRowWidth(labels[start:end]) > r.W {
			shown--
		}
		y := r.Y + row*chipRow
		if below {
			y = r.Y + r.H - row*chipRow
		}
		x := r.X + 4
		for i := start; i < end; i++ {
			l := labels[i]
			if i >= shown {
				l = fmt.Sprintf("+%d", end-shown)
			}
			w := chipWidth(l)
			out[items[i].key] = Port{X: x + 4, Y: y, W: w, Label: fit(l, w-12, chipPx), Below: below}
			if i < shown {
				x += w + 4
			}
		}
		start = end
	}
	return out
}

// rowEnd is where a row of chips that starts at labels[start] ends, at w
// wide: it always holds one.
func rowEnd(labels []string, start, w int) int {
	end := start + 1
	for end < len(labels) && chipRowWidth(labels[start:end+1]) <= w {
		end++
	}
	return end
}

// chipRows is how many rows spread sets labels in at w wide.
func chipRows(labels []string, w, rows int) int {
	labels = shorten(labels)
	n := 0
	for start := 0; start < len(labels) && n < rows; n++ {
		start = rowEnd(labels, start, w)
	}
	return n
}

// chipRowWidth is how wide one row of chips is, labels already shortened.
func chipRowWidth(labels []string) int {
	sum := 8
	for _, l := range labels {
		sum += chipWidth(l) + 4
	}
	return sum
}

func (o *overview) draw(href func(model.ID) string) Drawing {
	l := &o.layout
	center := func(id model.ID) int {
		p := l.pos[string(id)]
		return p.X + p.W/2
	}
	isModule := func(id model.ID) bool { return slices.Contains(o.modules, id) }

	type wireSpec struct {
		kind, title    string
		rels           []string
		from, to       model.ID
		fromKey, toKey string
	}
	var specs []wireSpec
	tops, bottoms := map[model.ID][]attach{}, map[model.ID][]attach{}
	// One line per user and provider, however many contracts it carries:
	// it leaves the user's chip for those contracts and reaches the
	// provider's.
	toward := map[string][]int{}
	for _, k := range o.pairs {
		u, p := k[0], k[1]
		in, out := chipKey(u, o.carry[k], false), string(p)
		toward[in] = append(toward[in], center(p))
		if isModule(p) {
			out = chipKey(p, o.carry[k], true)
			toward[out] = append(toward[out], center(u))
		}
		var titles, rels []string
		for _, c := range o.carry[k] {
			titles = append(titles, fmt.Sprintf("%s → %s: %s 계약을 거쳐 씁니다", o.name(u), o.name(p), o.name(c)))
			rels = append(rels, o.between(u, c)...)
		}
		specs = append(specs, wireSpec{kind: "contract", title: strings.Join(titles, "; "), from: u, to: p, fromKey: in, toKey: out, rels: rels})
	}
	// Chips sit in the order of what their lines lead to; the chip of
	// contracts no line reaches sits last.
	for _, m := range o.modules {
		for side, cs := range o.chips[m] {
			for _, ch := range cs {
				other := 1 << 20
				if xs := toward[ch.key]; len(xs) > 0 {
					other = 0
					for _, x := range xs {
						other += x
					}
					other /= len(xs)
				}
				a := attach{key: ch.key, label: ch.label, other: other}
				if side == 0 {
					tops[m] = append(tops[m], a)
				} else {
					bottoms[m] = append(bottoms[m], a)
				}
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
	// A dependency a contract line already carries is drawn by that line.
	carried := map[[2]model.ID][]string{}
	for _, e := range o.relations {
		if e.Kind == model.DependsOn && isModule(e.From) && isModule(e.To) && covered(e.From, e.To) {
			carried[[2]model.ID{e.From, e.To}] = append(carried[[2]model.ID{e.From, e.To}], o.rels[e.String()]...)
		}
	}
	for _, e := range o.relations {
		if e.Kind != model.DependsOn || !isModule(e.From) || !isModule(e.To) || covered(e.From, e.To) {
			continue
		}
		specs = append(specs, wireSpec{kind: "bypass", title: fmt.Sprintf("%s → %s: 계약을 거치지 않는 직접 의존", o.name(e.From), o.name(e.To)), from: e.From, to: e.To, rels: o.rels[e.String()]})
	}

	chips := func() map[string]Port {
		at := map[string]Port{}
		for _, m := range o.modules {
			r := l.pos[string(m)]
			for k, p := range spread(r, false, tops[m], chipRowsMax) {
				at[k] = p
			}
			for k, p := range spread(r, true, bottoms[m], chipRowsMax) {
				at[k] = p
			}
		}
		return at
	}
	at := chips()

	var merged []wireSpec
	for _, s := range specs {
		if s.kind == "contract" {
			s.rels = append(s.rels, carried[[2]model.ID{s.from, s.to}]...)
		}
		merged = append(merged, s)
	}
	// Two modules that use each other directly are joined by one line with
	// a head at each end, drawn the way the eye reads, instead of two lines
	// of which one would climb.
	before := func(a, b model.ID) bool {
		if ra, rb := l.row[string(a)], l.row[string(b)]; ra != rb {
			return ra < rb
		}
		return l.pos[string(a)].X < l.pos[string(b)].X
	}
	bypasses := map[[2]model.ID]int{}
	for i, s := range merged {
		if s.kind == "bypass" {
			bypasses[[2]model.ID{s.from, s.to}] = i
		}
	}
	gone := map[int]bool{}
	for i, s := range merged {
		j, ok := bypasses[[2]model.ID{s.to, s.from}]
		if s.kind != "bypass" || !ok || gone[i] || gone[j] {
			continue
		}
		keep, drop := i, j
		if !before(s.from, s.to) {
			keep, drop = j, i
		}
		merged[keep].kind = "mutual"
		merged[keep].title += "\n" + merged[drop].title
		merged[keep].rels = append(merged[keep].rels, merged[drop].rels...)
		gone[drop] = true
	}
	kept := merged[:0:0]
	for i, s := range merged {
		if !gone[i] {
			kept = append(kept, s)
		}
	}
	merged = kept
	span := func(ks []string) (int, int) {
		lo, hi := 1<<30, -1<<30
		for _, k := range ks {
			p := at[k]
			lo, hi = min(lo, p.X+3), max(hi, p.X+p.W-3)
		}
		return lo, hi
	}
	whole := func(id model.ID, key string) end {
		r := l.pos[string(id)]
		return end{node: string(id), row: l.row[string(id)], lo: r.X + 10, hi: r.X + r.W - 10, key: key}
	}
	var reqs []wireReq
	for _, s := range merged {
		from, to := whole(s.from, "plain|"+string(s.from)), whole(s.to, "plain|"+string(s.to))
		if s.fromKey != "" {
			from.lo, from.hi = span([]string{s.fromKey})
			from.key = s.fromKey
		}
		if s.toKey != "" && s.toKey != string(s.to) {
			to.lo, to.hi = span([]string{s.toKey})
			to.key = s.toKey
		}
		reqs = append(reqs, wireReq{from: from, to: to, kind: s.kind})
	}
	paths := l.route(reqs)
	at = chips()

	d := Drawing{Width: max(l.width, 320), Height: l.height + 4, Dots: junctions(pieces(paths), joinPoints(paths))}
	for _, m := range o.modules {
		r := l.pos[string(m)]
		u := o.units[m]
		b := Box{ID: string(m), Name: o.name(m), Kind: string(u.Kind), X: r.X, Y: r.Y, W: r.W, H: r.H, Class: "design"}
		ls := o.names[m]
		rows := o.chipRows[m]
		y := r.Y + (r.H+(rows[0]-rows[1])*chipRow)/2 + 5 - (len(ls)-1)*nameLine/2
		for i, s := range ls {
			b.Lines = append(b.Lines, Text{S: s, X: r.X + r.W/2, Y: y + i*nameLine, Mid: true})
		}
		b.Title = fmt.Sprintf("%s %s", m, b.Name)
		if viewOf(o.tree, m) != "" {
			b.Href = href(m)
			b.Title += ". 누르면 이 모듈의 안을 봅니다"
		}
		d.Boxes = append(d.Boxes, b)
	}
	// A folded "+N" chip stands for several contracts at one place: it is
	// drawn once and names them all.
	folded := map[[3]int]int{}
	addPort := func(pt Port, what, name string) {
		k := [3]int{pt.X, pt.Y, map[bool]int{true: 1}[pt.Exit]}
		if i, ok := folded[k]; ok {
			q := &d.Ports[i]
			q.Title += ", " + name
			q.Contract = ""
			q.Rels = append(q.Rels, pt.Rels...)
			return
		}
		folded[k] = len(d.Ports)
		pt.Title = what + ": " + name
		d.Ports = append(d.Ports, pt)
	}
	for _, c := range o.contracts {
		if r, ok := l.pos[string(c)]; ok {
			b := Box{ID: string(c), Name: o.name(c), Kind: string(o.units[c].Kind), Class: "loose design", X: r.X, Y: r.Y, W: r.W, H: r.H, Title: o.name(c) + ": 그림 안에 제공하는 모듈이 없는 계약"}
			ls := o.names[c]
			for i, s := range ls {
				b.Lines = append(b.Lines, Text{S: s, X: r.X + r.W/2, Y: r.Y + r.H/2 + 4 - (len(ls)-1)*7 + i*14, Mid: true})
			}
			d.Boxes = append(d.Boxes, b)
		}
	}
	for _, m := range o.modules {
		for side, cs := range o.chips[m] {
			for _, ch := range cs {
				pt := at[ch.key]
				pt.Exit, pt.Class, pt.Rels = side == 0, "design", ch.rels
				if len(ch.contracts) == 1 {
					pt.Contract = string(ch.contracts[0])
				}
				what := "쓰는"
				if pt.Exit {
					what = "제공하는"
				}
				addPort(pt, fmt.Sprintf("%s가 %s 계약", o.name(m), what), ch.title)
			}
		}
	}
	for _, p := range paths {
		var titles []string
		for _, i := range p.reqs {
			titles = append(titles, merged[i].title)
		}
		w := Wire{D: p.D, Kind: merged[p.reqs[0]].kind}
		w.Both = w.Kind == "mutual" && p.starts[0] != noStart
		origins := map[string]bool{}
		for j, i := range p.reqs {
			w.Rels = append(w.Rels, merged[i].rels...)
			og := o.origin(merged[i].rels)
			if og != Badge(model.Extracted) {
				titles[j] += ", " + labels[og]
			}
			if p.starts[j] == noStart {
				continue
			}
			origins[og] = true
			w.Origin = og
			if og != Badge(model.Extracted) {
				w.Marks = append(w.Marks, Mark{X: p.starts[j].x, Y: p.starts[j].y, Origin: og})
			}
		}
		if len(origins) > 1 {
			w.Origin = "mixed"
		}
		w.Title = strings.Join(titles, "\n")
		if p.count > 1 {
			w.Count, w.LX, w.LY = p.count, p.label.x, p.label.y
		}
		w.Net, w.Joins, w.Back, w.Ends = p.net, p.joins, p.back, p.ends
		d.Wires = append(d.Wires, w)
	}
	return d
}

// viewOf is the link to the view that shows id's inside, when it has one.
func viewOf(t tree, id model.ID) string {
	if len(t.children[id]) > 0 || t.units[id].Kind == model.Module {
		return "#m-" + string(id)
	}
	return ""
}

// between is every stored relation an edge from a to b stands for.
func (o *overview) between(a, b model.ID) []string {
	var out []string
	for _, k := range []model.RelationKind{model.Consumes, model.DependsOn, model.Provides} {
		out = append(out, o.rels[model.Edge{From: a, To: b, Kind: k}.String()]...)
	}
	return out
}
