// Package confirm is the queue of guesses waiting for a human. An item is a
// why-field or a coupling that only an agent has vouched for.
package confirm

import (
	"fmt"
	"sort"
	"strings"

	"github.com/whyjp/abbd/internal/model"
)

type Item struct {
	// Key names the item on the command line: MOD-0001.purpose or MOD-0001-depends_on->MOD-0002.
	Key   string
	Unit  model.ID
	Value string
	// Design is true when the item touches an intended design unit; those come first.
	Design bool
}

func (it Item) String() string {
	s := it.Key
	if it.Value != "" {
		s += fmt.Sprintf(" %q", it.Value)
	}
	if it.Design {
		s += " (design unit)"
	}
	return s
}

func pending(f *model.Fact) bool {
	return f != nil && f.Origin == model.Inferred && f.Confirmation != model.Confirmed && f.Confirmation != model.Rejected
}

func relationKey(from model.ID, r model.Relation) string {
	return fmt.Sprintf("%s-%s->%s", from, r.Kind, r.To)
}

// Queue lists every pending item of the present units, design units first.
func Queue(units []model.Unit, design map[model.ID]bool) []Item {
	var items []Item
	for _, u := range units {
		if u.Absent {
			continue
		}
		add := func(key, value string, touches bool) {
			items = append(items, Item{Key: key, Unit: u.ID, Value: value, Design: touches})
		}
		if pending(u.Purpose) {
			add(string(u.ID)+".purpose", u.Purpose.Value, design[u.ID])
		}
		if pending(u.Responsibility) {
			add(string(u.ID)+".responsibility", u.Responsibility.Value, design[u.ID])
		}
		for _, r := range u.Relations {
			if !r.Counts() && r.Confirmation != model.Rejected {
				add(relationKey(u.ID, r), string(r.Origin), design[u.ID] || design[r.To])
			}
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Design != items[j].Design {
			return items[i].Design
		}
		return items[i].Key < items[j].Key
	})
	return items
}

// Apply confirms one item. An empty value confirms the guess as written; a
// value replaces it with what the human said.
func Apply(units []model.Unit, key, value string) (model.Unit, error) {
	if id, field, ok := strings.Cut(key, "."); ok {
		for _, u := range units {
			if string(u.ID) != id || u.Absent {
				continue
			}
			f := map[string]**model.Fact{"purpose": &u.Purpose, "responsibility": &u.Responsibility}[field]
			switch {
			case f == nil:
				return u, fmt.Errorf("%s: only purpose and responsibility can be confirmed", key)
			case value != "":
				*f = &model.Fact{Value: value, Origin: model.UserStated, Confirmation: model.Confirmed}
			case *f == nil:
				return u, fmt.Errorf("%s has no value to confirm; give one with --value", key)
			default:
				c := **f
				c.Confirmation = model.Confirmed
				*f = &c
			}
			return u, nil
		}
		return model.Unit{}, fmt.Errorf("no unit %s", id)
	}
	for _, u := range units {
		for i, r := range u.Relations {
			if relationKey(u.ID, r) != key || u.Absent {
				continue
			}
			if value != "" {
				return u, fmt.Errorf("%s: a relation takes no --value", key)
			}
			rels := append([]model.Relation(nil), u.Relations...)
			rels[i].Confirmation = model.Confirmed
			u.Relations = rels
			return u, nil
		}
	}
	return model.Unit{}, fmt.Errorf("no confirmable item %q", key)
}
