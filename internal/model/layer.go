package model

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
)

// StackLayers bounds a stack's layers and StackUnits the units its layers
// name, as rule 1 bounds a view's boxes: a stack that needs more answers two
// questions.
const (
	StackLayers = 8
	StackUnits  = 16
)

// Layer is one band of a stack as a reader found it in the code: a name, a
// one-line role, and the stored units it holds, each with every unit inside
// it. Its place from 1 at the top is its height, never stored. The code it
// cites shows the role.
type Layer struct {
	Name     string     `yaml:"name"`
	Role     string     `yaml:"role"`
	Units    []ID       `yaml:"units"`
	Evidence []Evidence `yaml:"evidence"`
}

// Kind makes a stack's layers a kind of figure, whose layers hold each unit
// once between them.
func (Layer) Kind() FigureKind[Layer] {
	return FigureKind[Layer]{Noun: "stack", Dir: "stacks", Part: "layer", Parts: "layers", Long: "role", Min: 2, Max: StackLayers, Whole: holdOnce[Layer](StackUnits), Primary: true}
}

// Validate checks the layer on its own: a name and a role, one line each, the
// units it holds, each named once, and the code it rests on.
func (l Layer) Validate() error {
	switch {
	case strings.TrimSpace(l.Name) == "" || strings.ContainsAny(l.Name, "\r\n"):
		return fmt.Errorf("a layer has a name, on one line")
	case strings.TrimSpace(l.Role) == "" || strings.ContainsAny(l.Role, "\r\n"):
		return fmt.Errorf("layer %s says its role, on one line", l.Name)
	case len(l.Units) == 0:
		return fmt.Errorf("layer %s names no unit; a layer holds the stored units it names", l.Name)
	}
	for i, id := range l.Units {
		if _, _, err := ParseID(string(id)); err != nil {
			return err
		}
		if slices.Contains(l.Units[:i], id) {
			return fmt.Errorf("layer %s names %s twice", l.Name, id)
		}
	}
	if len(l.Evidence) == 0 {
		return fmt.Errorf("layer %s cites no code; cite the lines that show its role", l.Name)
	}
	return validateEvidence(l.Evidence)
}

func (l Layer) Label() string               { return l.Name }
func (l Layer) Names() []ID                 { return l.Units }
func (l Layer) Words() []Word               { return []Word{{"name", l.Name}, {"role", l.Role}} }
func (l Layer) Quotes() []Evidence          { return l.Evidence }
func (l Layer) Requote(es []Evidence) Layer { l.Evidence = es; return l }
func (l Layer) Retire() Layer               { return l }

// Gaps reads the layer against the state. Each unit it names is present, and
// one quote at least lies in code that one of them, or a unit inside one,
// anchors more closely than any other unit does, or that a contract one of
// them provides anchors so.
func (l Layer) Gaps(units []Unit) []string {
	st := stateOf(units)
	var gaps []string
	for _, id := range l.Units {
		if _, ok := st[id]; !ok {
			gaps = append(gaps, fmt.Sprintf("%s is no longer in the state", id))
		}
	}
	if len(gaps) == 0 && !st.rests(l.Units, l.Evidence) {
		gaps = append(gaps, "none of the layer's quotes is in the code of its units or a unit inside them; cite the lines that show its role")
	}
	return gaps
}

// holdOnce keeps each unit in one part of a figure whose parts hold units,
// such as a stack's layers. A part holds each unit it names and every unit
// inside it, so a unit named in one part and held by another, through a
// unit that holds it, is held twice. The parts name most units at most, or
// any number when most is 0. A unit moved under another after the figure
// was stored can break the first rule, and the stored figure then reads
// STALE. Each gap names the parts it involves.
func holdOnce[C Claim[C]](most int) func([]Column, []C, []Unit) []Gap {
	return func(_ []Column, parts []C, units []Unit) []Gap {
		k := KindOf[C]()
		st := stateOf(units)
		type named struct {
			id   ID
			part int
		}
		var all []named
		for i, p := range parts {
			for _, id := range p.Names() {
				all = append(all, named{id, i + 1})
			}
		}
		var gaps []Gap
		if most > 0 && len(all) > most {
			every := make([]int, len(parts))
			for i := range every {
				every[i] = i + 1
			}
			gaps = append(gaps, Gap{Text: fmt.Sprintf("a %s's %s name at most %d units, and these name %d; split the question in two", k.Noun, k.Parts, most, len(all)), Parts: every})
		}
		for i, a := range all {
			for _, b := range all[i+1:] {
				inner, outer := a, b
				if !st.under(inner.id, outer.id) {
					inner, outer = b, a
				}
				switch {
				case !st.under(inner.id, outer.id):
				case inner.id == outer.id:
					gaps = append(gaps, Gap{Text: fmt.Sprintf("%s is named by %s %d and by %s %d; a unit sits in one %s", a.id, k.Part, a.part, k.Part, b.part, k.Part), Parts: []int{a.part, b.part}})
				case inner.part == outer.part:
					gaps = append(gaps, Gap{Text: fmt.Sprintf("%s %d names %s and %s, which holds it; name the outer unit alone", k.Part, inner.part, inner.id, outer.id), Parts: []int{inner.part}})
				default:
					gaps = append(gaps, Gap{Text: fmt.Sprintf("%s is held by %s %d and, inside %s, by %s %d; a unit sits in one %s", inner.id, k.Part, inner.part, outer.id, k.Part, outer.part, k.Part), Parts: []int{min(inner.part, outer.part), max(inner.part, outer.part)}})
				}
			}
		}
		return gaps
	}
}

// Holders maps each present unit a part holds to that part's place, from 1,
// such as a layer's from the top: the part that names the unit, or else the
// one that names its nearest ancestor.
func Holders[C Claim[C]](parts []C, units []Unit) map[ID]int {
	named := map[ID]int{}
	for i, p := range parts {
		for _, id := range p.Names() {
			if _, ok := named[id]; !ok {
				named[id] = i + 1
			}
		}
	}
	st := stateOf(units)
	out := map[ID]int{}
	for id := range st {
		for at, seen := id, map[ID]bool{}; at != "" && !seen[at]; at = st[at].Parent {
			if n, ok := named[at]; ok {
				out[id] = n
				break
			}
			seen[at] = true
		}
	}
	return out
}

// Breach is a live stored relation by which a unit uses one higher up a
// stack: From is held by a lower layer than To, so Lower is greater than
// Upper.
type Breach struct {
	From, To     ID
	Kind         RelationKind
	Via          Via
	Origin       Origin
	Lower, Upper int
}

// Breaches are the live relations of the present units by which a unit a
// lower layer holds uses one a higher layer holds: it consumes it or depends
// on it. A provides is no breach, since the lower unit implements a contract
// the higher one homes, which is how a lower layer serves an upper one. A
// relation within one layer, one that skips layers downward, and one with an
// end no layer holds are no breach either. They are sorted by the lower
// layer, then by From, To and Kind.
func Breaches(layers []Layer, units []Unit) []Breach {
	held := Holders(layers, units)
	var out []Breach
	for _, u := range Present(units) {
		lower, ok := held[u.ID]
		if !ok {
			continue
		}
		for _, r := range u.Relations {
			if upper, ok := held[r.To]; ok && !r.Dead && r.Kind.Uses() && upper < lower {
				out = append(out, Breach{From: u.ID, To: r.To, Kind: r.Kind, Via: r.Via, Origin: r.Origin, Lower: lower, Upper: upper})
			}
		}
	}
	slices.SortFunc(out, func(a, b Breach) int {
		return cmp.Or(cmp.Compare(a.Lower, b.Lower), cmp.Compare(a.From, b.From), cmp.Compare(a.To, b.To), cmp.Compare(a.Kind, b.Kind))
	})
	return out
}

// StandsAt is where the overview stands a box no part holds: the first
// part, from 1, that names a unit inside id, and that unit. It is 0 when no
// part names one.
func StandsAt[C Claim[C]](parts []C, units []Unit, id ID) (int, ID) {
	st := stateOf(units)
	for i, p := range parts {
		for _, n := range p.Names() {
			if n != id && st.under(n, id) {
				return i + 1, n
			}
		}
	}
	return 0, ""
}
