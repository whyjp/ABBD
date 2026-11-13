package model

import (
	"fmt"
	"maps"
	"path"
	"slices"
	"strings"
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
// Min and Max bound how many claims a figure has, and MinColumns and
// MaxColumns how many columns its head names: none for a kind without a
// head. Long is the word field a store warns on past 80 characters, or ""
// for every word. Whole says what keeps the claims, taken together with the
// head, from being true of the state: a draft with any gap is refused, and a
// stored figure with any reads STALE. It is nil when each claim stands
// alone. Renamed is true for a kind whose word fields are the column names
// of the figure's own head, which a draft may rename, so a claim's words
// compare by their text alone, whatever field holds them. Primary is true
// for a kind one figure of which may be marked the repository's primary
// flow, its claims the flow's stages in order.
type FigureKind[C any] struct {
	Noun, Dir, Part, Parts, Long string
	Min, Max                     int
	MinColumns, MaxColumns       int
	Whole                        func(cols []Column, claims []C, units []Unit) []Gap
	Renamed                      bool
	Primary                      bool
}

// Gap is one thing that keeps a figure's claims, taken together, from being
// true of the state, with the places, from 1, of the claims it involves, and
// the names of the columns of its head it involves.
type Gap struct {
	Text    string
	Parts   []int
	Columns []string
}

func (g Gap) String() string { return g.Text }

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
	// Retire is the claim as a retired part keeps it: what it said, on which
	// code, about its own units only, so a retired part is forgotten once
	// one of those units leaves the state and not when a unit it reached
	// beyond them does.
	Retire() C
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

// box is the outermost present unit that holds id, or id itself: the box
// the overview draws it in. A unit whose parent is not present sits at the
// top level, as the overview draws it.
func (st state) box(id ID) ID {
	for seen := map[ID]bool{}; !seen[id]; seen[id] = true {
		p := st[id].Parent
		if _, ok := st[p]; !ok {
			break
		}
		id = p
	}
	return id
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

// strongest is the strongest live relation by which from, or a unit inside
// it, reaches to or a unit inside it: provides, then consumes, then
// depends_on. ok is false when none does.
func (st state) strongest(from, to ID) (rel Relation, ok bool) {
	rank := map[RelationKind]int{Provides: 3, Consumes: 2, DependsOn: 1}
	for _, id := range slices.Sorted(maps.Keys(st)) {
		if !st.under(id, from) {
			continue
		}
		for _, r := range st[id].Relations {
			if !r.Dead && st.under(r.To, to) && rank[r.Kind] > rank[rel.Kind] {
				rel = r
			}
		}
	}
	return rel, rel.Kind != ""
}

// Home is the directory a unit's code sits in: the deepest directory that
// holds every file it is anchored to, "." when they share none.
func Home(u Unit) string {
	var dirs []string
	for _, a := range u.Anchors {
		p := strings.TrimSuffix(a.Path, "/")
		if path.Ext(p) != "" || strings.Contains(path.Base(p), ".") {
			p = path.Dir(p)
		}
		dirs = append(dirs, p)
	}
	if len(dirs) == 0 {
		return "."
	}
	common := strings.Split(dirs[0], "/")
	for _, d := range dirs[1:] {
		parts := strings.Split(d, "/")
		n := 0
		for n < len(common) && n < len(parts) && common[n] == parts[n] {
			n++
		}
		common = common[:n]
	}
	if len(common) == 0 {
		return "."
	}
	return strings.Join(common, "/")
}
