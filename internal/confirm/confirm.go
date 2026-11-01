// Package confirm is the queue of guesses waiting for a human. An item is a
// why-field or a coupling that only an agent has vouched for.
package confirm

import (
	"fmt"
	"sort"
	"strings"

	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/topology"
)

type Item struct {
	// Key names the item on the command line: MOD-0001.purpose or MOD-0001.depends_on.MOD-0002.
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

func relationKey(from model.ID, r model.Relation) string {
	return topology.Edge{From: from, To: r.To, Kind: r.Kind}.String()
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
		if u.Purpose.Guess() {
			add(string(u.ID)+".purpose", u.Purpose.Value, design[u.ID])
		}
		if u.Responsibility.Guess() {
			add(string(u.ID)+".responsibility", u.Responsibility.Value, design[u.ID])
		}
		for _, r := range u.Relations {
			if !r.Counts() && !r.Dead && r.Confirmation != model.Rejected {
				add(relationKey(u.ID, r), relationValue(r), design[u.ID] || design[r.To])
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

// relationValue is what a human judges a guessed coupling by: how it arises
// and the code it rests on.
func relationValue(r model.Relation) string {
	s := string(r.Origin)
	if r.Via != "" {
		s += " via " + string(r.Via)
	}
	if len(r.Evidence) > 0 {
		s += ": " + r.Evidence[0].String()
	}
	return s
}

// Apply confirms one item. An empty value confirms the guess as written; a
// value replaces it with what the human said.
func Apply(units []model.Unit, key, value string) (model.Unit, error) {
	return decide(units, key, func(f **model.Fact) error {
		switch {
		case value != "":
			*f = &model.Fact{Value: value, Origin: model.UserStated, Confirmation: model.Confirmed}
		case *f == nil:
			return fmt.Errorf("%s has no value to confirm; give one with --value", key)
		default:
			c := **f
			c.Confirmation = model.Confirmed
			*f = &c
		}
		return nil
	}, func(r *model.Relation) error {
		if value != "" {
			return fmt.Errorf("%s: a relation takes no --value", key)
		}
		r.Confirmation = model.Confirmed
		return nil
	})
}

// Reject records that a guess is wrong. A why-field takes the value the human
// gives instead and keeps the rejected guess beside it; a rejected coupling
// stays on record and never counts.
func Reject(units []model.Unit, key, value string) (model.Unit, error) {
	return decide(units, key, func(f **model.Fact) error {
		switch {
		case !(*f).Guess():
			return fmt.Errorf("%s is not a guess waiting in the confirm queue", key)
		case value == "":
			return fmt.Errorf("%s: say what it is instead with --value", key)
		}
		*f = &model.Fact{Value: value, Origin: model.UserStated, Confirmation: model.Rejected, Rejected: (*f).Value}
		return nil
	}, func(r *model.Relation) error {
		switch {
		case r.Counts():
			return fmt.Errorf("%s is part of the actual topology, not a guess waiting in the confirm queue", key)
		case value != "":
			return fmt.Errorf("%s: a relation takes no --value", key)
		}
		r.Confirmation = model.Rejected
		return nil
	})
}

// decide finds the item key names and lets field or relation change a copy of it.
func decide(units []model.Unit, key string, field func(**model.Fact) error, relation func(*model.Relation) error) (model.Unit, error) {
	if id, name, ok := strings.Cut(key, "."); ok && !strings.Contains(name, ".") {
		for _, u := range units {
			if string(u.ID) != id || u.Absent {
				continue
			}
			f := map[string]**model.Fact{"purpose": &u.Purpose, "responsibility": &u.Responsibility}[name]
			if f == nil {
				return u, fmt.Errorf("%s: only purpose and responsibility can be confirmed", key)
			}
			return u, field(f)
		}
		return model.Unit{}, fmt.Errorf("no unit %s", id)
	}
	for _, u := range units {
		for i, r := range u.Relations {
			if relationKey(u.ID, r) != key || u.Absent {
				continue
			}
			rels := append([]model.Relation(nil), u.Relations...)
			if err := relation(&rels[i]); err != nil {
				return u, err
			}
			u.Relations = rels
			return u, nil
		}
	}
	return model.Unit{}, fmt.Errorf("no confirmable item %q", key)
}
