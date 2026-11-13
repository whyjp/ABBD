package model

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"strings"
)

// The overview is the summary a person reads first, drawn the way a person
// sketches a system by hand: one primary flow in numbered stages, named
// zones, and a few chosen lines, each with a word. The primary flow is the
// stack marked primary (FigureKind.Primary), its layers the stages. The
// zones and the lines are the overview's own record, chosen by a reader
// from the code and the stored relations; abbd stores neither as a unit or
// a relation. OverviewZones bounds the zones and OverviewLines the chosen
// lines, as rule 1 bounds a view's boxes.
const (
	OverviewZones = 8
	OverviewLines = 20
)

// Zone is one named region of the overview: the stored units it holds, each
// with every unit inside it, and the code that shows they belong together.
// Its place from 1 is its number, never stored.
type Zone struct {
	Name     string     `yaml:"name"`
	Units    []ID       `yaml:"units"`
	Evidence []Evidence `yaml:"evidence"`
}

// Kind makes the overview's zones a kind of figure part, whose zones hold
// each unit once between them, each under a name of its own.
func (Zone) Kind() FigureKind[Zone] {
	return FigureKind[Zone]{Noun: "overview", Part: "zone", Parts: "zones", Max: OverviewZones, Whole: zonesOnce}
}

// zonesOnce keeps each unit in one zone, and each name on one zone, since
// the name is the label the overview draws.
func zonesOnce(cols []Column, zones []Zone, units []Unit) []Gap {
	gaps := holdOnce[Zone](0)(cols, zones, units)
	for i, a := range zones {
		for j, b := range zones[i+1:] {
			if a.Name == b.Name {
				gaps = append(gaps, Gap{Text: fmt.Sprintf("zones %d and %d are both named %s; give each zone a name of its own", i+1, i+j+2, a.Name), Parts: []int{i + 1, i + j + 2}})
			}
		}
	}
	return gaps
}

func (z Zone) Requote(es []Evidence) Zone { z.Evidence = es; return z }
func (z Zone) Retire() Zone               { return z }

// Validate checks the zone on its own: a name that fits on the drawing as
// a phrase does, the units it holds, each named once, and the code it rests
// on.
func (z Zone) Validate() error {
	if strings.TrimSpace(z.Name) == "" {
		return fmt.Errorf("a zone has a name, on one line")
	}
	if err := ValidatePhrase(z.Name, ""); err != nil {
		return fmt.Errorf("zone %s: %w", z.Name, err)
	}
	if len(z.Units) == 0 {
		return fmt.Errorf("zone %s names no unit; a zone holds the stored units it names", z.Name)
	}
	for i, id := range z.Units {
		if _, _, err := ParseID(string(id)); err != nil {
			return err
		}
		if slices.Contains(z.Units[:i], id) {
			return fmt.Errorf("zone %s names %s twice", z.Name, id)
		}
	}
	if len(z.Evidence) == 0 {
		return fmt.Errorf("zone %s cites no code; cite the lines that show its units belong together", z.Name)
	}
	return validateEvidence(z.Evidence)
}

func (z Zone) Label() string      { return z.Name }
func (z Zone) Names() []ID        { return z.Units }
func (z Zone) Words() []Word      { return []Word{{"name", z.Name}} }
func (z Zone) Quotes() []Evidence { return z.Evidence }

// Gaps reads the zone against the state, as a stack's layer is read: each
// unit it names is present and is a box the overview draws, and one quote
// at least lies in their code.
func (z Zone) Gaps(units []Unit) []string {
	st := stateOf(units)
	var gaps []string
	for _, id := range z.Units {
		if _, ok := st[id]; !ok {
			gaps = append(gaps, fmt.Sprintf("%s is no longer in the state", id))
		} else if box := st.box(id); box != id {
			gaps = append(gaps, fmt.Sprintf("%s sits inside %s, which the overview draws as one box; name %s", id, box, box))
		}
	}
	if len(gaps) == 0 && !st.rests(z.Units, z.Evidence) {
		gaps = append(gaps, fmt.Sprintf("none of %s's quotes is in the code of its units or a unit inside them; cite the lines that show they belong together", z.Name))
	}
	return gaps
}

// Line is one line the overview draws, chosen by a reader: from a unit to
// another, with the word it carries. It draws the stored relations that run
// from From, or a unit inside it, to To, or a unit inside it, in that
// direction, and invents none.
type Line struct {
	From     ID         `yaml:"from"`
	To       ID         `yaml:"to"`
	Word     string     `yaml:"word"`
	Evidence []Evidence `yaml:"evidence"`
}

// Kind makes the overview's chosen lines a kind of figure part, each drawn
// once.
func (Line) Kind() FigureKind[Line] {
	return FigureKind[Line]{Noun: "overview", Part: "line", Parts: "lines", Max: OverviewLines, Whole: drawOnce}
}

// Validate checks the line on its own: two different units, a word that
// fits beside a line as a phrase does, and the code it rests on.
func (l Line) Validate() error {
	for _, id := range []ID{l.From, l.To} {
		if _, _, err := ParseID(string(id)); err != nil {
			return err
		}
	}
	switch {
	case l.From == l.To:
		return fmt.Errorf("line %s to %s joins a unit to itself", l.From, l.To)
	case l.Word == "":
		return fmt.Errorf("line %s to %s says no word; name what crosses it", l.From, l.To)
	case len(l.Evidence) == 0:
		return fmt.Errorf("line %s to %s cites no code; cite the lines that show what crosses it", l.From, l.To)
	}
	if err := ValidatePhrase(l.Word, ""); err != nil {
		return fmt.Errorf("line %s to %s: %w", l.From, l.To, err)
	}
	return validateEvidence(l.Evidence)
}

func (l Line) Label() string              { return string(l.From) + " " + string(l.To) }
func (l Line) Names() []ID                { return []ID{l.From, l.To} }
func (l Line) Words() []Word              { return []Word{{"word", l.Word}} }
func (l Line) Quotes() []Evidence         { return l.Evidence }
func (l Line) Requote(es []Evidence) Line { l.Evidence = es; return l }
func (l Line) Retire() Line               { return l }

// Carried is one live stored relation a chosen line draws.
type Carried struct {
	From, To ID
	Kind     RelationKind
	Via      Via
}

// Carries are the live relations the line draws, in order: each from From,
// or a unit inside it, to To, or a unit inside it, or to a contract the
// overview draws as a chip on To because To, or a unit inside it,
// provides it (reaches).
func (l Line) Carries(units []Unit) []Carried {
	st := stateOf(units)
	var out []Carried
	for _, id := range slices.Sorted(maps.Keys(st)) {
		if !st.under(id, l.From) {
			continue
		}
		for _, r := range st[id].Relations {
			if !r.Dead && st.reaches(r, l.To) {
				out = append(out, Carried{id, r.To, r.Kind, r.Via})
			}
		}
	}
	slices.SortFunc(out, func(a, b Carried) int {
		return cmp.Or(cmp.Compare(a.From, b.From), cmp.Compare(a.To, b.To), cmp.Compare(a.Kind, b.Kind), cmp.Compare(a.Via, b.Via))
	})
	return out
}

// Gaps reads the line against the state: both ends are present and sit in
// two boxes the overview draws, so neither holds the other, a live relation
// runs from one to the other in the line's direction, and one
// quote at least lies in the code of an end, or in a file the evidence of a
// relation under the line cites, where what crosses it is written.
func (l Line) Gaps(units []Unit) []string {
	st := stateOf(units)
	var gaps []string
	for _, id := range []ID{l.From, l.To} {
		if _, ok := st[id]; !ok {
			gaps = append(gaps, fmt.Sprintf("%s is no longer in the state", id))
		}
	}
	if len(gaps) > 0 {
		return gaps
	}
	if box := st.box(l.From); box == st.box(l.To) {
		inner, outer := l.From, l.To
		if st.under(outer, inner) {
			inner, outer = outer, inner
		}
		if !st.under(inner, outer) {
			return []string{fmt.Sprintf("%s and %s both sit inside %s, so the line runs inside one box; join units the overview draws in two boxes", l.From, l.To, box)}
		}
		return []string{fmt.Sprintf("%s sits inside %s, so the line runs inside one box; join units the overview draws in two boxes", inner, outer)}
	}
	var crossed []string
	for _, c := range l.Carries(units) {
		for _, r := range st[c.From].Relations {
			if !r.Dead && r.To == c.To && r.Kind == c.Kind {
				for _, e := range r.Evidence {
					crossed = append(crossed, e.Path)
				}
			}
		}
	}
	if len(l.Carries(units)) == 0 {
		gap := fmt.Sprintf("no live relation runs from %s, or a unit inside it, to %s, or a unit inside it; a line draws a stored relation, in its direction", l.From, l.To)
		if len(Line{From: l.To, To: l.From}.Carries(units)) > 0 {
			gap += fmt.Sprintf(", and the relations run from %s to %s", l.To, l.From)
		}
		gaps = append(gaps, gap+"; choose a line a stored relation runs under, or drop it")
	}
	if !st.rests([]ID{l.From, l.To}, l.Evidence) && !slices.ContainsFunc(l.Evidence, func(e Evidence) bool { return slices.Contains(crossed, e.Path) }) {
		gaps = append(gaps, "none of the line's quotes is in the code of its ends or a unit inside them; cite the lines that show what crosses it")
	}
	return gaps
}

// drawOnce keeps each pair of ends to one line in each direction.
func drawOnce(_ []Column, lines []Line, _ []Unit) []Gap {
	var gaps []Gap
	for i, a := range lines {
		for j, b := range lines[i+1:] {
			if a.From == b.From && a.To == b.To {
				gaps = append(gaps, Gap{Text: fmt.Sprintf("lines %d and %d both run from %s to %s; draw it once", i+1, i+j+2, a.From, a.To), Parts: []int{i + 1, i + j + 2}})
			}
		}
	}
	return gaps
}

// providers are the boxes of the units that provide c, a contract at the
// top level, in order: the boxes the overview sets c on as a chip, and
// where a line to c lands. They are none when c is no such contract.
func (st state) providers(c ID) []ID {
	if st.box(c) != c || !st[c].Kind.IsContract() {
		return nil
	}
	var out []ID
	for _, id := range slices.Sorted(maps.Keys(st)) {
		for _, r := range st[id].Relations {
			if !r.Dead && r.Kind == Provides && r.To == c && !slices.Contains(out, st.box(id)) {
				out = append(out, st.box(id))
			}
		}
	}
	return out
}

// reaches reports whether r, a relation of a unit, lands on to: its end is
// to or a unit inside it, or its end is a contract at the top level that to,
// or a unit inside it, provides, which the overview draws as a chip on the
// provider's box with the line running there.
func (st state) reaches(r Relation, to ID) bool {
	if st.under(r.To, to) {
		return true
	}
	if r.Kind == Provides {
		return false
	}
	for _, p := range st.providers(r.To) {
		if st.under(p, to) {
			return true
		}
	}
	return false
}

// Boxes are the units the overview draws as boxes, in ID order: each
// present unit whose parent is not present, but a contract a unit provides,
// which the overview draws as a chip on its provider's box.
func Boxes(units []Unit) []ID {
	st := stateOf(units)
	var out []ID
	for _, id := range slices.Sorted(maps.Keys(st)) {
		if st.box(id) == id && len(st.providers(id)) == 0 {
			out = append(out, id)
		}
	}
	return out
}

// Carry is how much a relation tells a person about why two boxes are
// tied, the order a draft offers the overview's lines in.
type Carry int

const (
	// Runs is work or data that moves at run time: a posts_to, a call, a
	// file or table read or written, an implementation provided or used
	// through a trait or a callback, or a relation that names no via.
	Runs Carry = iota
	// Wires is a tie one side sets up for the other: config, a convention,
	// a load, a spawn, a link, a macro or a build feature.
	Wires
	// Imports is a plain import.
	Imports
)

var carryWords = [...]string{Runs: "runs", Wires: "wires", Imports: "imports"}

func (c Carry) String() string { return carryWords[c] }

// CarryOf is how much a relation of kind k through via carries.
func CarryOf(k RelationKind, via Via) Carry {
	switch via {
	case "import":
		return Imports
	case "", "call", "data", "trait", "callback", "queue", "channel", "signal", "proxy":
		return Runs
	}
	if k == PostsTo {
		return Runs
	}
	return Wires
}

// Tie is a kind and a via of relation and how many live relations under a
// candidate line have them.
type Tie struct {
	Kind RelationKind
	Via  Via
	N    int
}

// LineCandidate is a pair of overview boxes some live relation joins, in
// its direction, as a draft offers it: the relations under it by kind and
// via, strongest first, and the first quote of its strongest relation that
// has one, a line a reader can cite or start from. Quote is empty when no
// relation under it quotes code.
type LineCandidate struct {
	From, To ID
	Ties     []Tie
	Quote    Evidence
}

// Carry is the strongest carry among the relations under the line.
func (c LineCandidate) Carry() Carry { return CarryOf(c.Ties[0].Kind, c.Ties[0].Via) }

// Relations counts the relations under the line, and those of carry k
// alone when k is given.
func (c LineCandidate) Relations(k ...Carry) int {
	n := 0
	for _, t := range c.Ties {
		if len(k) == 0 || CarryOf(t.Kind, t.Via) == k[0] {
			n += t.N
		}
	}
	return n
}

// LineCandidates are the lines the overview could draw, one per pair of
// boxes a live relation joins in its direction, a use of a contract drawn as
// a chip joining the user's box to each provider's, the strongest first: by
// the carry of its strongest relation, then by how many relations of that
// carry it holds, then by how many it holds in all, then by its ends.
func LineCandidates(units []Unit) []LineCandidate {
	st := stateOf(units)
	type rel struct {
		Relation
		from ID
	}
	by := map[[2]ID][]rel{}
	for _, id := range slices.Sorted(maps.Keys(st)) {
		for _, r := range st[id].Relations {
			if _, ok := st[r.To]; !ok || r.Dead {
				continue
			}
			a, ends := st.box(id), []ID{st.box(r.To)}
			if ps := st.providers(r.To); len(ps) > 0 {
				if r.Kind == Provides {
					continue
				}
				ends = ps
			}
			for _, b := range ends {
				if a != b {
					by[[2]ID{a, b}] = append(by[[2]ID{a, b}], rel{r, id})
				}
			}
		}
	}
	var out []LineCandidate
	for pair, rels := range by {
		slices.SortStableFunc(rels, func(a, b rel) int {
			return cmp.Or(cmp.Compare(CarryOf(a.Kind, a.Via), CarryOf(b.Kind, b.Via)), cmp.Compare(a.from, b.from), cmp.Compare(a.To, b.To), cmp.Compare(a.Kind, b.Kind), cmp.Compare(a.Via, b.Via))
		})
		c := LineCandidate{From: pair[0], To: pair[1]}
		for _, r := range rels {
			i := slices.IndexFunc(c.Ties, func(t Tie) bool { return t.Kind == r.Kind && t.Via == r.Via })
			if i < 0 {
				c.Ties = append(c.Ties, Tie{Kind: r.Kind, Via: r.Via})
				i = len(c.Ties) - 1
			}
			c.Ties[i].N++
			if c.Quote.Path != "" {
				continue
			}
			for _, e := range r.Evidence {
				if e.Status != Dead && e.Quote != "" {
					c.Quote = Evidence{Path: e.Path, Lines: e.Lines, Symbol: e.Symbol, Quote: e.Quote}
					break
				}
			}
		}
		slices.SortStableFunc(c.Ties, func(a, b Tie) int {
			return cmp.Or(cmp.Compare(CarryOf(a.Kind, a.Via), CarryOf(b.Kind, b.Via)), cmp.Compare(b.N, a.N), cmp.Compare(a.Kind, b.Kind), cmp.Compare(a.Via, b.Via))
		})
		out = append(out, c)
	}
	slices.SortFunc(out, func(a, b LineCandidate) int {
		return cmp.Or(cmp.Compare(a.Carry(), b.Carry()), cmp.Compare(b.Relations(a.Carry()), a.Relations(a.Carry())), cmp.Compare(b.Relations(), a.Relations()), cmp.Compare(a.From, b.From), cmp.Compare(a.To, b.To))
	})
	return out
}
