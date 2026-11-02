package screen

import (
	"slices"

	"github.com/whyjp/abbd/internal/model"
)

// Note is one slot of a unit's account as the page shows it: the slot's
// heading, the reader's text, the code it cites, and for a decision the
// reason a human stated and where.
type Note struct {
	Ref, Title, Key string
	Stale           bool
	Changed         []string `json:",omitempty"`
	Blocks          []Block
	Quotes          []string
	Why             *Fact  `json:",omitempty"`
	Source          string `json:",omitempty"`
}

// ContractNotes are a contract's notes, shown in the inside of the module
// that is its home.
type ContractNotes struct {
	ID, Name string
	Notes    []Note
}

// notes is what the page shows of a unit's slots, in slot order. A note
// whose slot abbd no longer knows, or on a unit that takes no slots, shows
// nowhere; status names it.
func notes(in Input, u model.Unit) []Note {
	if !u.Kind.HoldsNotes() {
		return nil
	}
	var out []Note
	for _, n := range in.Notes[u.ID] {
		if !n.Known {
			continue
		}
		v := Note{Ref: n.Ref.String(), Title: n.Ref.Slot.Title, Key: n.Ref.Key, Stale: n.Stale(), Changed: n.Changed, Blocks: blocks(n.Text)}
		for _, e := range n.Evidence {
			v.Quotes = append(v.Quotes, e.String())
		}
		if n.Why != nil {
			v.Why, v.Source = &Fact{Value: n.Why.Value, Badge: Badge(model.UserStated)}, n.Why.Source
		}
		out = append(out, v)
	}
	return out
}

// noteHome is the module whose inside shows a contract's notes: the module
// it sits in when that module provides it, else the provider whose own
// provides relation, its own or one of a unit inside it, rests on the
// strongest origin, so a reader's coupling never moves a contract away from
// the module the code says provides it. With no provider it is the module
// the contract sits in.
func (o *overview) noteHome(c model.Unit) model.ID {
	providers := o.providers[c.ID]
	if slices.Contains(providers, c.Parent) {
		return c.Parent
	}
	score := map[model.ID]int{}
	for id, u := range o.tree.units {
		for _, rel := range u.Relations {
			if rel.To != c.ID || rel.Kind != model.Provides || rel.Dead {
				continue
			}
			for _, p := range o.tree.chain(id) {
				if slices.Contains(providers, p) {
					score[p] = max(score[p], strength[rel.Origin])
				}
			}
		}
	}
	home, best := c.Parent, -1
	for _, p := range providers {
		if score[p] > best {
			home, best = p, score[p]
		}
	}
	return home
}

var strength = map[model.Origin]int{model.Inferred: 1, model.UserStated: 2, model.Extracted: 3}

// placeNotes puts each unit's notes where a reader of the unit looks: a
// module's beside its inside, or under its entry in its parent's inside when
// it has none; a contract's in its home module's inside. A unit no inside
// shows keeps its notes on its row of the unit list.
func (o *overview) placeNotes(in Input, m *Model) {
	shown := map[model.ID]bool{}
	at := map[model.ID]int{}
	for i := range m.Insides {
		at[model.ID(m.Insides[i].ID)] = i
	}
	present := map[model.ID]model.Unit{}
	for _, u := range model.Present(in.Units) {
		present[u.ID] = u
	}
	for i := range m.Insides {
		ins := &m.Insides[i]
		id := model.ID(ins.ID)
		ins.Notes, shown[id] = notes(in, present[id]), true
		for j, iu := range ins.Units {
			if _, own := at[model.ID(iu.ID)]; !own && present[model.ID(iu.ID)].Kind == model.Module {
				ins.Units[j].Notes, shown[model.ID(iu.ID)] = notes(in, present[model.ID(iu.ID)]), true
			}
		}
	}
	for _, u := range model.Present(in.Units) {
		if !u.Kind.IsContract() || len(in.Notes[u.ID]) == 0 {
			continue
		}
		i, ok := at[o.noteHome(u)]
		if ns := notes(in, u); ok && len(ns) > 0 {
			m.Insides[i].Contracts = append(m.Insides[i].Contracts, ContractNotes{ID: string(u.ID), Name: u.Name.Value, Notes: ns})
			shown[u.ID] = true
		}
	}
	for i, row := range m.Units {
		if !shown[model.ID(row.ID)] {
			m.Units[i].Notes = notes(in, present[model.ID(row.ID)])
		}
	}
}
