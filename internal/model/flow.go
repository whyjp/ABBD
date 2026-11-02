package model

import (
	"fmt"
	"slices"
	"strings"
)

// FlowKey names a flow: lower-case letters, digits and hyphens, as a
// decision's key. It is the file name under .abbd/flows/.
type FlowKey string

// ParseFlowKey reads a flow's key.
func ParseFlowKey(s string) (FlowKey, error) {
	if !noteKey.MatchString(s) {
		return "", fmt.Errorf("a flow's key is lower-case letters, digits and hyphens, such as sync, not %q", s)
	}
	return FlowKey(s), nil
}

// FlowSteps bounds a flow, as rule 1 bounds a view's boxes: a question that
// takes more steps is two questions.
const FlowSteps = 16

// Step is one moment of a flow as a reader found it in the code. Unit is the
// unit whose code runs it. Its number is its place in the flow, never stored.
// Does, Note and Fails are one line each: what the step does and yields; a
// cache, TTL, per-key or skip rule; what stops or degrades it. Calls are the
// units the step reaches beyond its own, each through a stored relation.
type Step struct {
	Unit     ID         `yaml:"unit"`
	Does     string     `yaml:"does"`
	Note     string     `yaml:"note,omitempty"`
	Fails    string     `yaml:"fails,omitempty"`
	Calls    []ID       `yaml:"calls,omitempty"`
	Evidence []Evidence `yaml:"evidence"`
}

// Validate checks the step on its own: a unit ID, a behaviour, one-line
// texts, calls that name other units once each, and the code it rests on.
func (s Step) Validate() error {
	if _, _, err := ParseID(string(s.Unit)); err != nil {
		return err
	}
	if strings.TrimSpace(s.Does) == "" {
		return fmt.Errorf("%s: a step says what it does", s.Unit)
	}
	for label, t := range map[string]string{"does": s.Does, "note": s.Note, "fails": s.Fails} {
		if strings.ContainsAny(t, "\r\n") {
			return fmt.Errorf("%s: %s is one line", s.Unit, label)
		}
	}
	for i, c := range s.Calls {
		if _, _, err := ParseID(string(c)); err != nil {
			return err
		}
		if c == s.Unit || slices.Contains(s.Calls[:i], c) {
			return fmt.Errorf("%s: calls names %s twice or names the step's own unit", s.Unit, c)
		}
	}
	if len(s.Evidence) == 0 {
		return fmt.Errorf("%s: a step cites the code it rests on", s.Unit)
	}
	return validateEvidence(s.Evidence)
}

// Reach is a step read against the state: the relation kind each call rides,
// and the gaps that keep the step from being true of the state, such as a
// unit that is gone.
type Reach struct {
	Calls map[ID]RelationKind
	Gaps  []string
}

// Against reads the step against the state. The step rests on its unit's
// code: one quote at least lies in the anchors of its unit, a unit inside
// it, or a unit it calls. A call is reached when the
// step's unit, or a unit inside it, holds a live relation to the called unit
// or to a unit inside it, so a chip never draws an edge the graph lacks. The
// kind is the strongest such relation: provides, then consumes, then
// depends_on.
func (s Step) Against(units []Unit) Reach {
	present := map[ID]Unit{}
	for _, u := range Present(units) {
		present[u.ID] = u
	}
	under := func(id, top ID) bool {
		for seen := map[ID]bool{}; id != "" && !seen[id]; id = present[id].Parent {
			if id == top {
				return true
			}
			seen[id] = true
		}
		return false
	}
	r := Reach{Calls: map[ID]RelationKind{}}
	cited := func(top ID) bool {
		for _, u := range present {
			for _, a := range u.Anchors {
				if under(u.ID, top) && slices.ContainsFunc(s.Evidence, func(e Evidence) bool { return a.Holds(e.Path) }) {
					return true
				}
			}
		}
		return false
	}
	if _, ok := present[s.Unit]; !ok {
		r.Gaps = append(r.Gaps, fmt.Sprintf("%s is no longer in the state", s.Unit))
	} else if !cited(s.Unit) && !slices.ContainsFunc(s.Calls, cited) {
		r.Gaps = append(r.Gaps, fmt.Sprintf("none of the step's quotes is in the code of %s, a unit inside it or a unit it calls; cite the lines of that code that run the step", s.Unit))
	}
	rank := map[RelationKind]int{Provides: 3, Consumes: 2, DependsOn: 1}
	for _, c := range s.Calls {
		if _, ok := present[c]; !ok {
			r.Gaps = append(r.Gaps, fmt.Sprintf("%s, which the step calls, is no longer in the state", c))
			continue
		}
		var best RelationKind
		for _, u := range Present(units) {
			if !under(u.ID, s.Unit) {
				continue
			}
			for _, rel := range u.Relations {
				if !rel.Dead && under(rel.To, c) && rank[rel.Kind] > rank[best] {
					best = rel.Kind
				}
			}
		}
		if best == "" {
			r.Gaps = append(r.Gaps, fmt.Sprintf("no live relation joins %s to %s, which the step calls; record the relation first", s.Unit, c))
			continue
		}
		r.Calls[c] = best
	}
	return r
}
