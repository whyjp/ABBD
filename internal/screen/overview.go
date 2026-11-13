package screen

import (
	"fmt"
	"html/template"
	"maps"
	"slices"
	"sort"
	"strings"
	"unicode/utf8"

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
	// Relations are the pairs of places the view's lines stand for, as it
	// requested them before routing. The checker holds the ink to them, so
	// a relation the routing drops is lost; one the view never requests is
	// not caught there.
	Relations [][2]string `json:"-"`
	// Counted are the pairs of places whose lines the view counts on their
	// boxes instead of drawing (primary.go), and countedRels the stored
	// relations those lines stand for.
	Counted     [][2]string `json:"-"`
	countedRels []string
	// Heads are the markers the drawing's lines end in.
	Heads []string `json:"-"`
	// Routes are each line's whole way, one per line the view requests.
	Routes []Route `json:"-"`
	// numbers are the shapes' numbers by place, as hover gave them.
	numbers map[string]int
	// ways are every line's route, kept when hover leaves Routes empty,
	// and Lead the way the view draws bold over them (lead.go).
	ways []Route
	Lead *Lead `json:",omitempty"`
	// Quiet says the view rests its lines grey but for its lead's or its
	// hub's own (lead.go, dress.go).
	Quiet bool `json:"-"`
	// Zones are the bands behind the boxes of the groupings the project
	// shows (zone.go).
	Zones []Zone `json:",omitempty"`
	// Card is the key the drawing keeps in a free corner (card.go).
	Card *Card `json:",omitempty"`
	// roads says its lines ride roads (confluence.go).
	roads bool
}

// Roads says the drawing's lines ride roads, so the page marks it for
// the checkers that read the SVG back.
func (d Drawing) Roads() bool { return d.roads }

// Route is one line's whole way from the shape it leaves to the shape it
// reaches, over every bus it rides, drawn unseen above the ink. While a
// reader points at a piece of ink, each route that runs along it shows in
// its line's colour, so every line there lights whole however many paths
// draw its ink. A route takes no pointer itself, so the ink keeps its title.
type Route struct {
	D, Kind    string
	Back, Both bool
	Hue, Lit   string
	// Vars name, for each box at an end, the colour the route takes while
	// that box is pointed at: --kN for the box numbered N (hover.go).
	Vars template.CSS
	// Tips place the arrowheads, of shape Head, at the route's ends.
	Head string
	Tips []string
	// src is the place whose colour the line wears, ends where it joins,
	// at the numbers of the shapes there, and runs its straight pieces.
	src  string
	ends []string
	at   []int
	runs [][2]pt
	// rels are the stored relations the line stands for.
	rels []string
}

// Box is a module on the overview, or a unit inside a module.
type Box struct {
	ID, Name, Kind, Class, Title string
	// Lines are the name as set in the box, one or two lines.
	Lines []Text
	// Sub is a last line: the kind of an inside unit.
	Sub *Text `json:",omitempty"`
	// Gist is a module's short phrase of what it does, set on its last line
	// in place of its kind (gist).
	Gist string `json:"-"`
	// Artifact is the build artifact a module is, said on its last line.
	Artifact string `json:",omitempty"`
	X, Y, W  int
	H        int
	// Href opens the module's inside.
	Href string `json:",omitempty"`
	// Hue is the palette key of the box's border and its tint (rule 11),
	// empty for a box that stands for no unit's colour.
	Hue string `json:"-"`
	// Lit are the classes that light the box while a reader points at it
	// or at a box it shares a line with (hover.go).
	Lit string `json:"-"`
	// Role is "entry" for the box a view's lead starts from and "hub" for
	// one most lines touch; either is drawn filled (dress.go).
	Role string `json:",omitempty"`
	// More is how many of its links the overview counts on the box instead
	// of drawing, and MoreHref the focus view that draws them all.
	More     int    `json:",omitempty"`
	MoreHref string `json:",omitempty"`
}

// MoreW is how wide the pill counting a box's undrawn links is, MoreX and
// MoreY where it starts, straddling the box's right edge halfway down, off
// the chips on its top and bottom, and MoreMid its middle.
func (b Box) MoreW() int   { return 14 + 7*len(fmt.Sprint(b.More)) }
func (b Box) MoreX() int   { return b.X + b.W - b.MoreW()/2 }
func (b Box) MoreY() int   { return b.Y + b.H/2 - 8 }
func (b Box) MoreMid() int { return b.MoreX() + b.MoreW()/2 }

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
	// H is the chip's height, the same for every chip in its row, and Lines
	// its label as wrapped to fit chipMax (chipLines).
	H     int      `json:",omitempty"`
	Lines []string `json:"-"`
	// Href opens the view of the unit a frame port stands for.
	Href string `json:",omitempty"`
	// Rels are the stored relations the chip stands for.
	Rels []string `json:",omitempty"`
	// Keys name a frame port as the lines that join it do: one chip that
	// stands for several ports carries every one's key.
	Keys []string `json:"-"`
	Lit  string   `json:"-"`
	// Folded says the edge folds this port into its row's "+N" chip.
	Folded bool `json:"-"`
	// OnFill says the chip sits on a filled box, where it takes an outline
	// of the panel colour so it still reads as a chip, and Hue is the
	// colour of the box it sits on, which it wears (dress.go). A frame's
	// chip sits on no box and keeps the accent.
	OnFill bool   `json:"-"`
	Hue    string `json:"-"`
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
	// Count is how many lines a bundle carries, drawn at LX, LY. Net groups
	// the paths of one bundle drawn as several paths.
	Count  int `json:",omitempty"`
	Net    int `json:",omitempty"`
	LX, LY int `json:",omitempty"`
	// ends are the places the line's requests join, and Lit the classes
	// that light it while a reader points at one of them (hover.go).
	ends []string
	// rides are, on a road (confluence.go), the requests whose routes ride
	// it, by route number: what pointing at it lights.
	rides []int
	// ink is the ink the path joins (inkOf).
	ink string
	Lit string `json:"-"`
	// Gaps are where the line is cut because another passes over it, and
	// Mask names the mask that cuts them (rule 24).
	Gaps []Gap  `json:"-"`
	Mask string `json:"-"`
	// Label is the word that says how the line's relations arise, set
	// along its own ink (rule 25).
	Label *Label `json:",omitempty"`
	// Hue is the palette key the line is drawn in and Head the marker at
	// its end, its shape for its kind and its colour (rule 11).
	Hue  string `json:"-"`
	Head string `json:"-"`
	// Plan says every relation the line stands for arises only in design,
	// with no call or data at run time, so it is drawn dotted (dress.go).
	Plan bool `json:"-"`
	// srcs are the places each of the line's relations leaves, which give
	// it its colour.
	srcs []string
	// lines are the relations of each line the label speaks for: on the
	// path that shows a bus's count, the lines it counts, else the path's
	// own.
	lines [][]string
	// owed is the count a bus gave up for the word wire by's label says
	// for it, put back if that label is dropped later (unlabelKin).
	owed *owed
}

type owed struct{ count, lx, ly, by int }

// carry gives w the relations of the lines its path p draws and of those
// its count counts, each relation once, however many requests merged it.
func (w *Wire) carry(p drawn, rels func(int) []string) {
	of := func(is []int) (out [][]string) {
		for _, i := range is {
			var line []string
			for _, r := range rels(i) {
				if !slices.Contains(line, r) {
					line = append(line, r)
				}
			}
			out = append(out, line)
		}
		return out
	}
	w.lines = of(p.reqs)
	for _, line := range w.lines {
		for _, r := range line {
			if !slices.Contains(w.Rels, r) {
				w.Rels = append(w.Rels, r)
			}
		}
	}
	if p.count > 1 {
		w.lines = of(p.counted)
	}
}

// Mark is where a line from a reader or a human leaves its unit.
type Mark struct {
	X, Y   int
	Origin string
}

const (
	boxH, chipH = 60, 17
	// chipLine is how much taller each further line makes a wrapped chip.
	chipLine = 13
	// chipRow is how far each further row of chips sits inside a box. A
	// module whose chips need more than chipWrap wraps them into at most
	// chipRowsMax rows instead of widening, so lines can pass beside it.
	chipRow     = 20
	chipWrap    = 480
	chipRowsMax = 2
	chipPx      = 11
	nameLine    = 16
	// boxMax, and chipMax for a contract drawn as a box of its own, are the
	// hard widths past which a name is cut and its full text left to the
	// tooltip, rule 8. A chip is never cut: a label wider than chipMax wraps
	// onto more lines.
	chipMax = 220
	boxMax  = 280
)

func kindWord(k model.Kind) string {
	return map[model.Kind]string{
		model.Module: "모듈", model.Interface: "인터페이스", model.Event: "이벤트",
		model.Data: "데이터", model.Config: "설정", model.Feature: "기능", model.Exec: "실행",
	}[k]
}

// boxTag is the line under a box's name: the build artifact a module is, or
// how an execution context runs, such as "TICK · 1".
func boxTag(u model.Unit) string {
	if r, ok := u.Ran(); ok {
		return r.Words()
	}
	return model.Words(u.Built())
}

// gist is the first clause of a unit's sentence, so a box says what it does
// as the exemplars do: the longest run of its words that fits room and
// ends where a phrase does, before a small word or at the clause's end. A
// cut never lands before "to" or "that", which carry what the verb needs,
// and a clause that opens on an article names a thing rather than what it
// does; either, or a clause that leaves no run of two words or more, gives
// no gist. The full sentence stays in the box's tooltip.
func gist(u model.Unit, room int) string {
	f := sentence(u)
	if f == nil {
		return ""
	}
	s := strings.TrimPrefix(strings.TrimSpace(f.Value), "so ")
	for _, sep := range []string{", ", ": ", "; ", ". ", " (", " —"} {
		s, _, _ = strings.Cut(s, sep)
	}
	words := strings.Fields(strings.TrimRight(s, "."))
	if len(words) == 0 || slices.Contains([]string{"a", "an", "the"}, strings.ToLower(words[0])) {
		return ""
	}
	small := func(w string) bool { return slices.Contains(smallWords, strings.ToLower(w)) }
	binds := func(w string) bool { w = strings.ToLower(w); return w == "to" || w == "that" }
	for n := len(words); n >= 2; n-- {
		run := strings.Join(words[:n], " ")
		if (n == len(words) || small(words[n]) && !binds(words[n])) && !small(words[n-1]) && monoWidth(run) <= room {
			return run
		}
	}
	return ""
}

// monoWidth is how wide a last line is set in its monospaced face, 10px
// with spacing: under 7px a narrow character and 11px a wide one. subWidth
// measures capitals in the name's face and so leaves a gist half the room.
func monoWidth(s string) int {
	w := 0
	for _, r := range s {
		w += 7
		if r >= 0x1100 {
			w += 4
		}
	}
	return w
}

var smallWords = []string{"a", "an", "the", "to", "of", "for", "on", "in", "into", "onto", "with", "and", "or", "by", "from", "at", "as", "that", "its", "their", "about", "over", "through", "without", "when", "so", "per", "across", "between"}

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

// chipLines sets a chip's label on lines at most chipMax wide, broken after
// a separator or before a space, and a word too long for a line where the
// line is full, so a long name grows the chip down and never off its edge.
func chipLines(label string) []string {
	room := chipMax - 12
	if textWidth(label, chipPx) <= room {
		return []string{label}
	}
	var words []string
	start := 0
	for i, r := range label {
		switch {
		case r == ' ' && i > start:
			words, start = append(words, label[start:i]), i
		case strings.ContainsRune("/-_.,", r):
			words, start = append(words, label[start:i+1]), i+1
		}
	}
	words = append(words, label[start:])
	var lines []string
	line := ""
	for _, w := range words {
		if line != "" && textWidth(line+w, chipPx) > room {
			lines, line = append(lines, strings.TrimSpace(line)), ""
			w = strings.TrimLeft(w, " ")
		}
		for textWidth(w, chipPx) > room {
			_, cut := utf8.DecodeRuneInString(w)
			for i := range w {
				if i > cut && textWidth(w[:i], chipPx) <= room {
					cut = i
				}
			}
			lines, w = append(lines, w[:cut]), w[cut:]
		}
		line += w
	}
	if line = strings.TrimSpace(line); line != "" {
		lines = append(lines, line)
	}
	return lines
}

func chipWidth(label string) int {
	w := 0
	for _, l := range chipLines(label) {
		w = max(w, textWidth(l, chipPx))
	}
	return w + 12
}

// chipHeight is how tall the chip for label is.
func chipHeight(label string) int { return chipH + (len(chipLines(label))-1)*chipLine }

// RectY is where the chip's rect starts: below its edge on top of a box or
// frame, above it on the bottom.
func (p Port) RectY() int {
	if p.Below {
		return p.Y - p.H - 3
	}
	return p.Y + 3
}

// LineY is where the chip's line i starts, the lines centred in the chip.
func (p Port) LineY(i int) int {
	return p.RectY() + (p.H-chipH-(len(p.Lines)-1)*chipLine)/2 + i*chipLine
}

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
	relations []model.Edge
	modules   []model.ID
	contracts []model.ID
	providers map[model.ID][]model.ID
	users     map[model.ID][]model.ID
	layout    layout
	names     map[model.ID][]string
	// chipBands are how tall the rows of chips on each module's top and
	// bottom are (chipBand).
	chipBands map[model.ID][2]int
	// carry are the contracts each drawn contract line carries, by the
	// user and the provider it joins, in the order lines are drawn.
	carry map[[2]model.ID][]model.ID
	pairs [][2]model.ID
	// unreached are the contracts each module provides that no module uses.
	unreached map[model.ID][]model.ID
	// chips sit on each module's top, where lines arrive at what it
	// provides, and on its bottom, where lines leave for what it uses.
	chips map[model.ID][2][]chip
	// rels are the stored relations each drawn edge stands for, by edge,
	// and origins say where each stored relation came from.
	rels    map[string][]string
	origins map[string]model.Origin
	// read is what the routing draw last made costs a reader (costOf).
	read cost
	// more are the links each box counts instead of drawing (primary.go),
	// counted the pairs of places they join and countedRels the stored
	// relations they stand for.
	more        map[model.ID]int
	counted     [][2]string
	countedRels []string
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
// A count label keeps its words together with a no-break space, so the
// first word that shorten drops from a row of contract names stays.
func (o *overview) chipFor(at model.ID, cs []model.ID, exit bool) chip {
	var names []string
	for _, c := range cs {
		names = append(names, o.name(c))
	}
	ch := chip{key: chipKey(at, cs, exit), contracts: cs, label: fmt.Sprintf("계약\u00a0%d개", len(cs)), title: strings.Join(names, ", ")}
	if len(cs) == 1 {
		ch.label = portLabel(names[0])
	}
	for _, c := range cs {
		ch.rels = append(ch.rels, o.between(at, c)...)
	}
	return ch
}

// planOverview lays out the overview. With a focus, it plans the whole
// overview, then keeps only that module's part of it and lays that out: the
// focus view (focus.go), its ring ordered by at, the x of each box in the
// full view.
// A stored stack among stacks stands the boxes of each of its tiers together in
// the stack's order (tier) on the whole overview, not on a focus view, and
// within a tier the boxes the stored flows run through in their steps' order.
func planOverview(units []model.Unit, focus model.ID, at map[string]int, t tactic, stacks []layering, flows []flowWalk, c *cut) *overview {
	o := &overview{tree: newTree(units), units: map[model.ID]model.Unit{}, providers: map[model.ID][]model.ID{}, users: map[model.ID][]model.ID{}, names: map[model.ID][]string{}, chipBands: map[model.ID][2]int{}, carry: map[[2]model.ID][]model.ID{}, chips: map[model.ID][2][]chip{}}
	for _, u := range units {
		o.units[u.ID] = u
	}
	design, relations, rels, origins := top(o.tree)
	o.relations, o.rels, o.origins = relations, rels, origins
	slices.Sort(design)
	isModule := map[model.ID]bool{}
	for _, id := range design {
		k := o.units[id].Kind
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
			}
		}
	}
	o.unreached = map[model.ID][]model.ID{}
	for _, c := range o.contracts {
		if len(o.users[c]) == 0 {
			for _, p := range o.providers[c] {
				o.unreached[p] = append(o.unreached[p], c)
			}
		}
	}
	if focus != "" {
		o.around(focus)
		for _, id := range o.modules {
			isModule[id] = true
		}
	} else if c != nil {
		o.pairs = slices.DeleteFunc(o.pairs, func(k [2]model.ID) bool {
			if c.keep[pairOf(k[0], k[1])] {
				return false
			}
			o.counted = append(o.counted, [2]string{string(k[0]), string(k[1])})
			for _, ct := range o.carry[k] {
				o.countedRels = append(o.countedRels, o.between(k[0], ct)...)
				o.countedRels = append(o.countedRels, o.between(k[1], ct)...)
			}
			return true
		})
		o.relations = slices.DeleteFunc(slices.Clone(o.relations), func(e model.Edge) bool {
			if e.Kind != model.DependsOn || !isModule[e.From] || !isModule[e.To] || c.keep[pairOf(e.From, e.To)] {
				return false
			}
			o.counted = append(o.counted, [2]string{string(e.From), string(e.To)})
			o.countedRels = append(o.countedRels, o.rels[e.String()]...)
			return true
		})
		o.more = c.more
	}
	var edges []ledge
	for _, k := range o.pairs {
		edges = append(edges, ledge{string(k[0]), string(k[1])})
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
	for _, m := range o.modules {
		if cs := o.unreached[m]; len(cs) > 0 {
			ch := o.chipFor(m, cs, true)
			ch.key, ch.label = fmt.Sprint(m, "+"), fmt.Sprintf("+%d\u00a0계약", len(cs))
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
	tiers, steps := map[string]int{}, map[string]int{}
	if focus == "" {
		box := func(u model.ID) string {
			if c := o.tree.chain(u); len(c) > 0 && isModule[c[0]] {
				return string(c[0])
			}
			return ""
		}
		for i, g := range stackOf(stacks, box, func(ids []string) []string { return ids }) {
			for _, id := range g.ids {
				tiers[id] = i + 1
			}
		}
		for _, f := range flows {
			for _, u := range f.units {
				if b := box(u); b != "" && steps[b] == 0 {
					steps[b] = len(steps) + 1
				}
			}
		}
	}
	var nodes []lnode
	for _, m := range o.modules {
		ls, w := setName(o.name(m), 13, 28, 150, boxMax)
		o.names[m] = ls
		exits, entries := labels(o.chips[m][0]), labels(o.chips[m][1])
		w = max(w, min(chipWrap, max(chipsWidth(exits), chipsWidth(entries))))
		bands := [2]int{chipBand(exits, w, chipRowsMax), chipBand(entries, w, chipRowsMax)}
		o.chipBands[m] = bands
		nodes = append(nodes, lnode{id: string(m), w: w, h: boxH + (len(ls)-1)*nameLine + bands[0] + bands[1] - 2*chipRow, tier: tiers[string(m)], step: steps[string(m)]})
	}
	for _, c := range o.contracts {
		if len(o.providers[c]) == 0 {
			ls, w := setName(portLabel(o.name(c)), chipPx, 16, 40, chipMax+16)
			o.names[c] = ls
			nodes = append(nodes, lnode{id: string(c), w: w, h: chipH + 12 + (len(ls)-1)*14})
		}
	}
	o.layout = arrange(nodes, edges, overviewBudget, seatOf(focus, at), t)
	o.layout.top = padY
	return o
}

func (o *overview) name(id model.ID) string {
	if u, ok := o.units[base(id)]; ok && u.Name.Value != "" {
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
	// one says every line of the chip runs at other, so settle may stand
	// the chip off centre to keep it over that x.
	one bool
	// runs are the x each of its lines runs at toward the edge.
	runs []int
	// to is the least ID of the places its lines lead to. Chips that lead
	// to one x stand in its order, not in the order of the contracts they
	// carry, so a chip keeps its place when its line gains a contract.
	to string
}

// reorder moves chips of an edge settle set out to other places in their
// order while that leaves fewer chips over none of their lines, then less
// distance from them, and says whether it moved any. On a crowded edge the
// order of the mean x can push a chip with one line far off it while a
// chip whose lines spread wide stands where that line runs.
func reorder(ps map[string]Port, items []attach, frame rect) (map[string]Port, bool) {
	miss := func(ps map[string]Port) [2]int {
		var m [2]int
		for _, a := range items {
			p := ps[a.key]
			lo, hi := p.X+4, p.X+p.W-4
			least := 1 << 20
			for _, x := range a.runs {
				least = min(least, max(0, x-hi, lo-x))
			}
			if least > 0 {
				m[0]++
			}
			m[1] += least
		}
		return m
	}
	less := func(a, b [2]int) bool { return a[0] < b[0] || a[0] == b[0] && a[1] < b[1] }
	best, moved := miss(ps), false
	for again := true; again; {
		again = false
		for i := range items {
			for j := range items {
				if i == j {
					continue
				}
				try := slices.Insert(slices.Delete(slices.Clone(items), i, i+1), j, items[i])
				if q := settle(maps.Clone(ps), try, frame); less(miss(q), best) {
					best, ps, again, moved = miss(q), q, true, true
					copy(items, try)
				}
			}
		}
	}
	return ps, moved
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
		if items[i].to != items[j].to {
			return items[i].to < items[j].to
		}
		return items[i].key < items[j].key
	})
	labels := make([]string, len(items))
	for i, a := range items {
		labels[i] = a.label
	}
	labels = shorten(labels)
	out := map[string]Port{}
	off := 0
	for row, start := 0, 0; start < len(labels); row++ {
		end := len(labels)
		if row < rows-1 {
			end = rowEnd(labels, start, r.W)
		}
		shown := end
		for shown > start+1 && chipRowWidth(labels[start:shown])+chipWidth(fmt.Sprintf("+%d", end-shown)) > r.W && chipRowWidth(labels[start:end]) > r.W {
			shown--
		}
		y := r.Y + off
		if below {
			y = r.Y + r.H - off
		}
		h := rowHeight(labels[start:end])
		x := r.X + 4
		for i := start; i < end; i++ {
			l := labels[i]
			if i >= shown {
				l = fmt.Sprintf("+%d", end-shown)
			}
			w := chipWidth(l)
			out[items[i].key] = Port{X: x + 4, Y: y, W: w, H: h, Label: l, Lines: chipLines(l), Below: below, Folded: i >= shown}
			if i < shown {
				x += w + 4
			}
		}
		off += h + 3
		start = end
	}
	return out
}

// rowHeight is how tall a row of chips is: as tall as its tallest chip.
func rowHeight(labels []string) int {
	h := chipH
	for _, l := range labels {
		h = max(h, chipHeight(l))
	}
	return h
}

// settle moves each chip of a frame's or a box's edge, set out by spread
// in the order of what it leads to, over the x its lines run at, so they
// drop straight from it. The chips keep their order and gaps inside the
// edge; an edge whose chips wrap into two rows or fold into "+N" stays as
// spread set it.
func settle(ps map[string]Port, items []attach, frame rect) map[string]Port {
	xs, ys := map[int]bool{}, map[int]bool{}
	for _, p := range ps {
		xs[p.X], ys[p.Y] = true, true
	}
	if len(xs) < len(items) || len(ys) > 1 {
		return ps
	}
	at := make([]int, len(items))
	edge := frame.X + 8
	// A chip pushed so far that its line no longer meets it moves back
	// until the line meets its first lane's room, and the chips before it
	// give way while their own lines still meet them, so crowded chips
	// stand off centre rather than bend their lines.
	w := func(i int) int { return ps[items[i].key].W }
	for i, a := range items {
		at[i] = max(a.other-w(i)/2, edge)
		if hi := a.other - 4; a.one && at[i] > hi && hi >= frame.X+8 {
			at[i] = hi
			for j := i - 1; j >= 0 && at[j]+w(j)+4 > at[j+1]; j-- {
				at[j] = max(at[j+1]-4-w(j), items[j].other-w(j)+4, frame.X+8)
			}
			for j := 1; j <= i; j++ {
				at[j] = max(at[j], at[j-1]+w(j-1)+4)
			}
		}
		edge = at[i] + w(i) + 4
	}
	edge = frame.X + frame.W - 4
	for i := len(items) - 1; i >= 0; i-- {
		p := ps[items[i].key]
		at[i] = min(at[i], edge-p.W)
		edge = at[i] - 4
	}
	for i, a := range items {
		p := ps[a.key]
		p.X = at[i]
		ps[a.key] = p
	}
	return ps
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

// chipBand is how tall the rows spread sets labels in at w wide are, gaps
// included, and one row's height when there are none.
func chipBand(labels []string, w, rows int) int {
	labels = shorten(labels)
	band := 0
	for n, start := 0, 0; start < len(labels); n++ {
		end := len(labels)
		if n < rows-1 {
			end = rowEnd(labels, start, w)
		}
		band += rowHeight(labels[start:end]) + 3
		start = end
	}
	return max(band, chipRow)
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
	toward, to := map[string][]int{}, map[string]string{}
	lead := func(key string, x int, place model.ID) {
		if xs := toward[key]; len(xs) == 0 || string(place) < to[key] {
			to[key] = string(place)
		}
		toward[key] = append(toward[key], x)
	}
	for _, k := range o.pairs {
		u, p := k[0], k[1]
		in, out := chipKey(u, o.carry[k], false), string(p)
		lead(in, l.toward(string(u), string(p)), p)
		if isModule(p) {
			out = chipKey(p, o.carry[k], true)
			lead(out, l.toward(string(p), string(u)), u)
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
				a := attach{key: ch.key, label: ch.label, other: other, to: to[ch.key]}
				if side == 0 {
					tops[m] = append(tops[m], a)
				} else {
					bottoms[m] = append(bottoms[m], a)
				}
			}
		}
	}
	covered := func(a, b model.ID) bool { return len(o.carry[[2]model.ID{a, b}]) > 0 }
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
			for k, p := range settle(spread(r, false, tops[m], chipRowsMax), tops[m], r) {
				at[k] = p
			}
			for k, p := range settle(spread(r, true, bottoms[m], chipRowsMax), bottoms[m], r) {
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
		reqs = append(reqs, wireReq{from: from, to: to, kind: s.kind, plan: o.tree.planOnly(s.rels)})
	}
	routed := l.route(reqs)
	o.read = costOf(routed, reqs)
	paths := routed.drawn
	at = chips()

	d := Drawing{Width: max(l.width, minWidth), Height: l.height + 4, Dots: faint(junctions(pieces(paths), joinPoints(paths)), paths), Relations: wanted(reqs), Counted: o.counted, countedRels: o.countedRels, roads: l.roads}
	for _, m := range o.modules {
		r := l.pos[string(m)]
		u := o.units[m]
		b := Box{ID: string(m), Name: o.name(m), Kind: string(u.Kind), Gist: gist(u, r.W-16), Artifact: boxTag(u), X: r.X, Y: r.Y, W: r.W, H: r.H, Class: "design"}
		ls := o.names[m]
		bands := o.chipBands[m]
		y := r.Y + (r.H+bands[0]-bands[1])/2 + 5 - (len(ls)-1)*nameLine/2
		for i, s := range ls {
			b.Lines = append(b.Lines, Text{S: s, X: r.X + r.W/2, Y: y + i*nameLine, Mid: true})
		}
		b.Title = fmt.Sprintf("%s %s", base(m), b.Name)
		if b.Artifact != "" {
			b.Title += " (" + b.Artifact + ")"
		}
		if f := sentence(u); f != nil {
			b.Title += ": " + f.Value
		}
		if n := o.more[m]; n > 0 {
			b.More, b.MoreHref = n, "#"+focusID("", m)
		}
		if base(m) != m {
			b.Title += "\n" + twinNote
			b.Class += " twin"
		}
		if h := href(m); h != "" {
			b.Href = h
			if strings.HasPrefix(h, "#m-") {
				b.Title += ". 누르면 이 모듈의 안을 봅니다"
			} else {
				b.Title += ". 누르면 이 모듈을 중심으로 봅니다"
			}
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
		w := Wire{D: p.D, Kind: p.kind}
		w.Both = w.Kind == "mutual" && slices.ContainsFunc(p.starts, func(s pt) bool { return s != noStart })
		origins := map[string]bool{}
		for j, i := range p.reqs {
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
		w.carry(p, func(i int) []string { return merged[i].rels })
		if p.count > 1 {
			w.Count, w.LX, w.LY = p.count, p.label.x, p.label.y
		}
		w.Net, w.Joins, w.Back = p.net, p.joins, p.back
		w.ends = endsOf(reqs, p.reqs)
		if l.roads {
			w.rides = p.reqs
		}
		w.ink = inkOf(w.Kind, p.road)
		d.Wires = append(d.Wires, w)
	}
	d.route(routed, reqs, func(i int) []string { return merged[i].rels })
	d.mark(paths, reqs, o.tree)
	d.hover()
	return d
}

// viewOf is the link to the view that shows id's inside, when it has one.
func viewOf(t tree, id model.ID) string {
	if u := t.units[id]; len(t.children[id]) > 0 || u.Parent == "" && u.Kind == model.Module {
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
