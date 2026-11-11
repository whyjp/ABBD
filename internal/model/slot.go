package model

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// Slot is one part of a unit's account that a reader writes on its own and
// that goes STALE on its own: what the unit means, how it fails, its states,
// a decision it embodies, a constraint or invariant its code keeps, a sample
// of what crosses it. Title is the heading
// the blueprint gives it. A keyed slot holds one note per key, so each
// decision carries its own evidence and its own why.
type Slot struct {
	Name, Title string
	Keyed       bool
}

// Slots is every slot, in the order the blueprint shows them.
var Slots = []Slot{
	{Name: "semantics", Title: "의미"},
	{Name: "failure", Title: "실패와 재시도"},
	{Name: "states", Title: "상태"},
	{Name: "decisions", Title: "결정", Keyed: true},
	{Name: "constraints", Title: "제약과 불변식", Keyed: true},
	{Name: "wire_sample", Title: "전송 예시"},
	{Name: "entries", Title: "항목", Keyed: true},
}

// EntrySlot holds one note per entry of a contract, keyed by the entry's key.
const EntrySlot = "entries"

// Unheld reports whether the note names an entry the unit does not list, so
// nothing shows it and the check skips it.
func (r NoteRef) Unheld(u Unit) bool {
	_, ok := u.EntryNamed(r.Key)
	return r.Slot.Name == EntrySlot && !ok
}

// SlotNamed finds a slot by name.
func SlotNamed(name string) (Slot, bool) {
	i := slices.IndexFunc(Slots, func(s Slot) bool { return s.Name == name })
	if i < 0 {
		return Slot{}, false
	}
	return Slots[i], true
}

// HoldsNotes reports whether a unit of kind k takes slots: a module or a
// contract.
func (k Kind) HoldsNotes() bool { return k == Module || k.IsContract() }

// NoteRef names one note: a unit, a slot, and for a keyed slot the key.
type NoteRef struct {
	Unit ID
	Slot Slot
	Key  string
}

var noteKey = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// ParseNoteRef reads ID.SLOT or ID.SLOT.KEY. A keyed slot needs a key and
// any other slot takes none.
func ParseNoteRef(s string) (NoteRef, error) {
	parts := strings.Split(s, ".")
	if len(parts) < 2 || len(parts) > 3 {
		return NoteRef{}, fmt.Errorf("%q is not ID.SLOT or ID.SLOT.KEY for a keyed slot such as decisions or constraints", s)
	}
	if _, _, err := ParseID(parts[0]); err != nil {
		return NoteRef{}, err
	}
	slot, ok := SlotNamed(parts[1])
	if !ok {
		return NoteRef{}, fmt.Errorf("%q is no slot; the slots are %s", parts[1], slotNames())
	}
	r := NoteRef{Unit: ID(parts[0]), Slot: slot}
	if len(parts) == 3 {
		r.Key = parts[2]
	}
	switch {
	case slot.Keyed && r.Key == "":
		return NoteRef{}, fmt.Errorf("%s holds one note per key; name it %s.%s.KEY, such as %s.%s.lf-endings", slot.Name, r.Unit, slot.Name, r.Unit, slot.Name)
	case !slot.Keyed && r.Key != "":
		return NoteRef{}, fmt.Errorf("%s holds one note and takes no key; name it %s.%s", slot.Name, r.Unit, slot.Name)
	case r.Key != "" && !noteKey.MatchString(r.Key):
		return NoteRef{}, fmt.Errorf("a key is lower-case letters, digits and hyphens, not %q", r.Key)
	}
	return r, nil
}

// Name is the note's part after the unit ID: SLOT or SLOT.KEY.
func (r NoteRef) Name() string {
	if r.Key == "" {
		return r.Slot.Name
	}
	return r.Slot.Name + "." + r.Key
}

func (r NoteRef) String() string { return string(r.Unit) + "." + r.Name() }

func slotNames() string {
	var names []string
	for _, s := range Slots {
		names = append(names, s.Name)
	}
	return strings.Join(names, ", ")
}
