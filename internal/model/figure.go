package model

import (
	"fmt"
	"slices"
)

// FigureKey names a figure: lower-case letters, digits and hyphens, as a
// decision's key. It is the file name under its kind's directory.
type FigureKey string

// ParseFigureKey reads the key of a figure of kind C.
func ParseFigureKey[C Claim[C]](s string) (FigureKey, error) {
	if !noteKey.MatchString(s) {
		return "", fmt.Errorf("a %s's key is lower-case letters, digits and hyphens, not %q", KindOf[C]().Noun, s)
	}
	return FigureKey(s), nil
}

// FigureKind is what sets one kind of figure apart; the store and the
// command do the rest for every kind. Noun names a figure of the kind and
// the command that stores one, Dir the directory under .abbd that holds
// them. Part names one claim and Parts the frontmatter key that lists them.
// Min and Max bound how many claims a figure has. Whole says what keeps the
// claims, taken together, from being true of the state: a draft with any is
// refused, and a stored figure with any reads STALE. It is nil when each
// claim stands alone.
type FigureKind[C any] struct {
	Noun, Dir, Part, Parts string
	Min, Max               int
	Whole                  func(claims []C, units []Unit) []string
}

// Claim is one part of a figure as a reader or a human found it in the code.
// It names stored units and rests on quoted code, and stores neither.
type Claim[C any] interface {
	// Kind is read on the zero value.
	Kind() FigureKind[C]
	// Validate checks the claim on its own, before the state.
	Validate() error
	// Label is what status names the claim by, beside its place.
	Label() string
	// Names are the units the claim names.
	Names() []ID
	// Words are the lines the claim says, each one line, in a fixed order.
	Words() []Word
	Quotes() []Evidence
	// Requote is the same claim citing es.
	Requote(es []Evidence) C
	// Gaps are what keep the claim from being true of the state.
	Gaps(units []Unit) []string
}

// Word is one line a claim says, by the field that holds it.
type Word struct{ Field, Text string }

// KindOf is the kind of the claims C.
func KindOf[C Claim[C]]() FigureKind[C] {
	var c C
	return c.Kind()
}

// state is the present units by ID, which claims are read against.
type state map[ID]Unit

func stateOf(units []Unit) state {
	st := state{}
	for _, u := range Present(units) {
		st[u.ID] = u
	}
	return st
}

// under reports whether id is top or a unit inside it.
func (st state) under(id, top ID) bool {
	for seen := map[ID]bool{}; id != "" && !seen[id]; id = st[id].Parent {
		if id == top {
			return true
		}
		seen[id] = true
	}
	return false
}

// within reports whether id is one of tops or a unit inside one.
func (st state) within(id ID, tops []ID) bool {
	return slices.ContainsFunc(tops, func(top ID) bool { return st.under(id, top) })
}

// rests reports whether one quote at least lies in code that one of tops, or
// a unit inside it, anchors more closely than any other unit does, or that a
// contract one of them provides anchors so. A file two units share lends a
// quote only to the unit whose anchor names it best, and a root anchor of .
// only the files no closer anchor names.
func (st state) rests(tops []ID, quotes []Evidence) bool {
	provides := func(c ID) bool {
		for _, u := range st {
			if st.within(u.ID, tops) && slices.ContainsFunc(u.Relations, func(rel Relation) bool { return !rel.Dead && rel.Kind == Provides && rel.To == c }) {
				return true
			}
		}
		return false
	}
	held := func(e Evidence) bool {
		best, owners := 0, []ID(nil)
		for _, u := range st {
			for _, a := range u.Anchors {
				switch c := a.Closeness(e); {
				case c < 0 || c < best:
				case c > best || owners == nil:
					best, owners = c, []ID{u.ID}
				default:
					owners = append(owners, u.ID)
				}
			}
		}
		return slices.ContainsFunc(owners, func(id ID) bool { return st.within(id, tops) || provides(id) })
	}
	return slices.ContainsFunc(quotes, held)
}
