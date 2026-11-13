// Package topology answers questions about the stored topology: one unit at a
// glance, the shortest path between two units, what a change to some units
// reaches, and how the topology differs between two revisions. It reads
// units and never writes them.
package topology

import (
	"fmt"
	"maps"
	"slices"

	"github.com/whyjp/abbd/internal/model"
)

// Link is one live relation between present units, with its origin and, for
// a reader's coupling, the code it rests on.
type Link struct {
	model.Edge
	Origin model.Origin
	Via    model.Via
	// Phrase is the note that names what crosses the link, with its own
	// writer and evidence, while it is live or drifted.
	Phrase   *model.PhraseNote
	Evidence []model.Evidence
	// Entries are the keys of the entries of To the link narrows to.
	Entries []string
	// Drifted says the link counts on evidence the code no longer holds as
	// quoted; a reader must refresh or retract it.
	Drifted bool
	// When is the condition the link holds under, empty when it always does.
	When string
}

// Label says where the link comes from and how it arises, then what crosses
// it and whose words those are: " (extracted; phrase "Cypher", inferred)".
func (l Link) Label() string {
	s := origins[l.Origin]
	if l.Via != "" {
		s += ", via " + string(l.Via)
	}
	switch {
	case model.Scope(l.When):
		s += ", scope " + l.When
	case l.When != "":
		s += ", when " + l.When
	}
	if l.Drifted {
		s += ", drifted"
	}
	if p := l.PhraseLabel(); p != "" {
		s += "; " + p
	}
	return " (" + s + ")"
}

// PhraseLabel says what crosses the link, by its note's own origin and
// status, or "" when no note names it.
func (l Link) PhraseLabel() string {
	n := l.Phrase
	if n == nil {
		return ""
	}
	s := fmt.Sprintf("phrase %q, %s", n.Text, origins[n.Writer.Origin()])
	if n.Status() == model.Drifted {
		s += ", drifted"
	}
	return s
}

var origins = map[model.Origin]string{model.Extracted: "extracted", model.UserStated: "stated", model.Inferred: "inferred"}

// Links lists every live relation between present units: what the script
// read, what a human stated, and a reader's couplings while their evidence lives.
func Links(units []model.Unit) []Link {
	present := map[model.ID]bool{}
	for _, u := range units {
		present[u.ID] = !u.Absent
	}
	var out []Link
	for _, u := range model.Present(units) {
		for _, r := range model.SortRelations(u.Relations) {
			if present[r.To] && !r.Dead {
				l := Link{Edge: model.Edge{From: u.ID, To: r.To, Kind: r.Kind}, Origin: r.Origin, Via: r.Via, Evidence: r.Evidence, Entries: r.Entries, Drifted: r.Drifted(), When: r.When}
				if n, ok := u.Note(r.Kind, r.To); ok {
					l.Phrase = &n
				}
				out = append(out, l)
			}
		}
	}
	return out
}

// Item is one impacted unit and why.
type Item struct {
	Unit model.ID
	Why  string
}

// Impact is the neighbourhood of a change for an agent to read: Direct closely,
// Far briefly.
type Impact struct {
	Direct, Far []Item
}

// ImpactOf follows the links from the seeds: whatever depends on or consumes
// an impacted unit, the providers of an impacted contract, and the contracts a
// seed provides. A provider is impacted because it implements the contract,
// but what it does elsewhere is not followed. A unit that depends on or
// consumes an impacted one passes the change on to what uses it, not to the
// other contracts it provides, which the change does not travel through. A
// unit inside a seed module, at any depth, is a seed too. What a seed itself
// depends on or consumes is impacted without being followed, because the
// change most likely breaks where they meet.
//
// Direct are the seeds, the units that share a link with a seed, and the
// consumers of a contract that does. Far is the rest.
func ImpactOf(units []model.Unit, seeds []model.ID) Impact {
	links := Links(units)
	all := slices.Clone(seeds)
	for grew := true; grew; {
		grew = false
		for _, u := range model.Present(units) {
			if slices.Contains(all, u.Parent) && !slices.Contains(all, u.ID) {
				all, grew = append(all, u.ID), true
			}
		}
	}
	var items []Item
	seen := map[model.ID]bool{}
	var queue []model.ID
	add := func(id model.ID, why string, follow bool) {
		if seen[id] {
			return
		}
		seen[id] = true
		items = append(items, Item{Unit: id, Why: why})
		if follow {
			queue = append(queue, id)
		}
	}
	for _, s := range seeds {
		add(s, "seed", true)
	}
	for _, id := range all[len(seeds):] {
		add(id, "inside a seed", true)
	}
	for len(queue) > 0 {
		u := queue[0]
		queue = queue[1:]
		for _, l := range links {
			switch {
			case l.To == u && (l.Kind == model.DependsOn || l.Kind == model.Consumes):
				add(l.From, fmt.Sprintf("%s %s%s", l.Kind, u, l.Label()), true)
			case l.To == u && l.Kind == model.Provides && isContract(u):
				add(l.From, "provides "+string(u)+l.Label(), false)
			case l.From == u && l.Kind == model.Provides && slices.Contains(all, u):
				add(l.To, "provided by "+string(u)+l.Label(), true)
			}
		}
	}
	for _, l := range links {
		if slices.Contains(all, l.From) && (l.Kind == model.DependsOn || l.Kind == model.Consumes) {
			add(l.To, fmt.Sprintf("%s by %s%s", map[model.RelationKind]string{model.DependsOn: "depended on", model.Consumes: "consumed"}[l.Kind], l.From, l.Label()), false)
		}
	}

	near := map[model.ID]bool{}
	for _, s := range all {
		near[s] = true
	}
	for _, l := range links {
		if slices.Contains(all, l.From) {
			near[l.To] = true
		}
		if slices.Contains(all, l.To) {
			near[l.From] = true
		}
	}
	reached := maps.Clone(near)
	for _, l := range links {
		if (l.Kind == model.DependsOn || l.Kind == model.Consumes) && isContract(l.To) && near[l.To] {
			reached[l.From] = true
		}
	}
	var im Impact
	for _, it := range items {
		if reached[it.Unit] {
			im.Direct = append(im.Direct, it)
		} else {
			im.Far = append(im.Far, it)
		}
	}
	return im
}

func isContract(id model.ID) bool {
	k, _, err := model.ParseID(string(id))
	return err == nil && k.IsContract()
}
