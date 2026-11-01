package group

import (
	"testing"

	"github.com/whyjp/abbd/internal/model"
)

// A state may hold a reader's parent on a directory group from before
// `abbd record parent` refused one. When the top level shrinks under the
// budget the group goes, and the reader's unit goes to the top level.
func TestAParentOnAGroupNoLongerNeededGoes(t *testing.T) {
	us, hw := Fallback(flat(), map[model.Kind]int{model.Module: 20})
	var apps model.ID
	for _, u := range us {
		if Made(u) && u.Anchors[0].Path == "apps" {
			apps = u.ID
		}
	}
	if apps == "" {
		t.Fatal("no apps group")
	}
	for i := range us {
		if us[i].ID == "MOD-0001" {
			us[i].PutParent(model.ParentClaim{To: apps, Writer: model.Reader, Evidence: []model.Evidence{{Path: "README.md", Lines: "1", Quote: "x"}}})
		}
	}
	// Drop five native modules so the top level fits the budget.
	var smaller []model.Unit
	for _, u := range us {
		if u.ID >= "MOD-0016" && u.ID <= "MOD-0020" {
			continue
		}
		smaller = append(smaller, u)
	}
	got, _ := Fallback(smaller, hw)
	byID := map[model.ID]model.Unit{}
	for _, u := range got {
		byID[u.ID] = u
	}
	m := byID["MOD-0001"]
	if p, ok := byID[m.Parent]; m.Parent != "" && (!ok || p.Absent) {
		t.Errorf("MOD-0001 (present) is inside %s, which is absent", m.Parent)
	}
}
