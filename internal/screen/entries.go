package screen

import (
	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/topology"
)

// Entries is a contract's entries as the page shows them: one table row
// each, with the columns only some contracts fill left out when no row
// fills them.
type Entries struct {
	// ID is the contract's, so a row's id is unique on the page.
	ID string
	// Shape heads the column of what each entry looks like: a signature, or
	// for a contract that holds data its type.
	Shape string
	Rows  []EntryRow
	// Need, Providers and Users say which optional columns any row fills.
	Need, Providers, Users bool
	// Data words the use columns as writers and readers.
	Data bool
}

// EntryRow is one entry: the code's name and shape, the units that provide
// or use it, and a reader's note on what it means.
type EntryRow struct {
	Key, Name, Shape, Need string
	Providers, Users       []EntryUser `json:",omitempty"`
	Note                   *Note       `json:",omitempty"`
}

// EntryUser is a unit that provides or uses one entry, and where that came
// from.
type EntryUser struct {
	ID, Name, Badge string
}

// entries builds a contract's table, or nil when it lists no entries.
func entries(in Input, u model.Unit) *Entries {
	if len(u.Entries) == 0 {
		return nil
	}
	data := u.Kind == model.Data || u.Kind == model.Config
	t := &Entries{ID: string(u.ID), Shape: "시그니처", Data: data}
	if data {
		t.Shape = "타입"
	}
	name := map[model.ID]string{}
	for _, v := range in.Units {
		name[v.ID] = v.Name.Value
	}
	uses := topology.EntryUses(in.Units, u.ID)
	ns := map[string]Note{}
	for _, n := range notes(in, u) {
		if n.slot == model.EntrySlot {
			ns[n.Key] = n
		}
	}
	for _, e := range u.Entries {
		row := EntryRow{Key: e.Key, Name: e.Name, Shape: e.Shape, Need: map[string]string{"required": "필수", "optional": "선택"}[e.Need()]}
		for _, us := range uses[e.Key] {
			c := EntryUser{ID: string(us.From), Name: name[us.From], Badge: Badge(us.Origin)}
			if us.Kind == model.Provides {
				row.Providers = append(row.Providers, c)
			} else {
				row.Users = append(row.Users, c)
			}
		}
		if n, ok := ns[e.Key]; ok {
			row.Note = &n
		}
		t.Need = t.Need || row.Need != ""
		t.Providers = t.Providers || len(row.Providers) > 0
		t.Users = t.Users || len(row.Users) > 0
		t.Rows = append(t.Rows, row)
	}
	return t
}
