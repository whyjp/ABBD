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
	return FigureKind[Layer]{Noun: "stack", Dir: "stacks", Part: "layer", Parts: "layers", Long: "role", Min: 2, Max: StackLayers, Whole: holdOnce}
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

// holdOnce keeps each unit in one layer of a stack. A layer holds each unit
// it names and every unit inside it, so a unit named in one layer and held
// by another, through a unit that holds it, is held twice. The layers name
// StackUnits units at most. A unit moved under another after the stack was
// stored can break the first rule, and the stored stack then reads STALE.
// Each gap names the layers it involves.
func holdOnce(layers []Layer, units []Unit) []Gap {
	st := stateOf(units)
	type named struct {
		id    ID
		layer int
	}
	var all []named
	for i, l := range layers {
		for _, id := range l.Units {
			all = append(all, named{id, i + 1})
		}
	}
	var gaps []Gap
	if len(all) > StackUnits {
		every := make([]int, len(layers))
		for i := range every {
			every[i] = i + 1
		}
		gaps = append(gaps, Gap{fmt.Sprintf("a stack's layers name at most %d units, and these name %d; split the question in two", StackUnits, len(all)), every})
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
				gaps = append(gaps, Gap{fmt.Sprintf("%s is named by layer %d and by layer %d; a unit sits in one layer", a.id, a.layer, b.layer), []int{a.layer, b.layer}})
			case inner.layer == outer.layer:
				gaps = append(gaps, Gap{fmt.Sprintf("layer %d names %s and %s, which holds it; name the outer unit alone", inner.layer, inner.id, outer.id), []int{inner.layer}})
			default:
				gaps = append(gaps, Gap{fmt.Sprintf("%s is held by layer %d and, inside %s, by layer %d; a unit sits in one layer", inner.id, inner.layer, outer.id, outer.layer), []int{min(inner.layer, outer.layer), max(inner.layer, outer.layer)}})
			}
		}
	}
	return gaps
}

// Holders maps each present unit a layer holds to that layer's place, from 1
// at the top: the layer that names the unit, or else the one that names its
// nearest ancestor.
func Holders(layers []Layer, units []Unit) map[ID]int {
	named := map[ID]int{}
	for i, l := range layers {
		for _, id := range l.Units {
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
