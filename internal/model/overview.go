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
// or a unit inside it, to To, or a unit inside it.
func (l Line) Carries(units []Unit) []Carried {
	st := stateOf(units)
	var out []Carried
	for _, id := range slices.Sorted(maps.Keys(st)) {
		if !st.under(id, l.From) {
			continue
		}
		for _, r := range st[id].Relations {
			if !r.Dead && st.under(r.To, l.To) {
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
