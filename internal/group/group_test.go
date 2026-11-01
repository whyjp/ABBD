package group

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/whyjp/abbd/internal/model"
)

func module(n int, file string) model.Unit {
	return model.Unit{ID: model.FormatID(model.Module, n), Kind: model.Module, Name: model.Said(model.Script, file), Anchors: []model.Anchor{{Path: file}}}
}

// Twenty top-level modules, ten under apps/ and ten under native/.
func flat() []model.Unit {
	var us []model.Unit
	for i := 1; i <= 20; i++ {
		dir := "apps"
		if i > 10 {
			dir = "native"
		}
		us = append(us, module(i, fmt.Sprintf("%s/m%d/main.go", dir, i)))
	}
	return us
}

func parents(us []model.Unit) map[model.ID]model.ID {
	out := map[model.ID]model.ID{}
	for _, u := range model.Present(us) {
		out[u.ID] = u.Parent
	}
	return out
}

func TestTooManyModulesAreGroupedByDirectory(t *testing.T) {
	us, hw := Fallback(flat(), map[model.Kind]int{model.Module: 20})
	p := parents(us)
	if p["MOD-0001"] != "MOD-0021" || p["MOD-0011"] != "MOD-0022" || p["MOD-0021"] != "" || p["MOD-0022"] != "" {
		t.Fatalf("want apps/ and native/ groups at the top: %v", p)
	}
	if hw[model.Module] != 22 {
		t.Fatalf("the groups take the next IDs: %v", hw)
	}
	for _, u := range us {
		if err := u.Validate(); err != nil {
			t.Fatal(err)
		}
	}
	again, _ := Fallback(us, hw)
	if !reflect.DeepEqual(parents(again), p) {
		t.Fatalf("a second sync must give the same groups with the same IDs:\n%v\n%v", parents(again), p)
	}
}

func TestAReadersGroupingIsNeverMoved(t *testing.T) {
	us := flat()
	for i := range us[:19] {
		us[i].PutParent(model.ParentClaim{To: "MOD-0020", Writer: model.Reader, Evidence: []model.Evidence{{Path: "README.md", Lines: "1", Quote: "x"}}})
	}
	got, _ := Fallback(us, map[model.Kind]int{model.Module: 20})
	for _, u := range got[:19] {
		if u.Parent != "MOD-0020" {
			t.Fatalf("the fallback moved %s out of the reader's group: %v", u.ID, parents(got))
		}
	}
	if len(got) != 20 {
		t.Fatalf("no group is needed beside a reader's grouping, got %d units", len(got))
	}
}

func TestASmallTopLevelStaysFlat(t *testing.T) {
	us := flat()[:Budget]
	got, _ := Fallback(us, nil)
	if !reflect.DeepEqual(got, us) {
		t.Fatalf("a top level within the budget is left alone")
	}
}

// A group whose units sit both in its own directory and below it splits
// only the ones below, and never contains itself.
func TestAGroupNeverHoldsItself(t *testing.T) {
	var us []model.Unit
	for i := 1; i <= 40; i++ {
		file := fmt.Sprintf("src/m%d.go", i)
		if i > 20 {
			file = fmt.Sprintf("src/sub%d/m.go", i%3)
		}
		us = append(us, module(i, file))
	}
	us = append(us, module(41, "web/a.go"), module(42, "web/b.go"))
	got, _ := Fallback(us, map[model.Kind]int{model.Module: 42})
	if model.ParentCycle(got) != "" {
		t.Fatalf("the groups form a cycle: %v", parents(got))
	}
}

// A module inside a group may need a group of the same directory as the
// top level's; each stays in its own container.
func TestSameDirectoryGroupsInTwoContainersStayApart(t *testing.T) {
	var us []model.Unit
	for i := 1; i <= 20; i++ {
		dir := "src"
		if i > 10 {
			dir = "web"
		}
		us = append(us, module(i, fmt.Sprintf("%s/p%d/m.go", dir, i)))
	}
	for i := 21; i <= 38; i++ {
		dir := "src"
		if i > 29 {
			dir = "docs"
		}
		u := module(i, fmt.Sprintf("%s/k%d/c.go", dir, i))
		u.PutParent(model.ParentClaim{To: "MOD-0001", Writer: model.Script})
		us = append(us, u)
	}
	got, _ := Fallback(us, map[model.Kind]int{model.Module: 38})
	if model.ParentCycle(got) != "" {
		t.Fatalf("the groups form a cycle: %v", parents(got))
	}
	for _, u := range model.Present(got) {
		if Made(u) && u.Name.Value == "src/" && u.Parent != "" && u.Parent != "MOD-0001" {
			t.Fatalf("a src/ group moved into another container: %s in %s", u.ID, u.Parent)
		}
	}
	tops := 0
	for _, u := range model.Present(got) {
		if Made(u) && u.Parent == "" {
			tops++
		}
	}
	if tops != 2 {
		t.Fatalf("want the src/ and web/ groups at the top, got %d: %v", tops, parents(got))
	}
}

// The overview draws a top-level contract as a chip, so contracts do not
// count against the budget there.
func TestTopLevelContractsDoNotCrowdTheOverview(t *testing.T) {
	us := flat()[:Budget]
	for i := 1; i <= 5; i++ {
		us = append(us, model.Unit{ID: model.FormatID(model.Interface, i), Kind: model.Interface, Name: model.Said(model.Script, "c"), Anchors: []model.Anchor{{Path: fmt.Sprintf("api/c%d.go", i)}}})
	}
	got, _ := Fallback(us, nil)
	if len(got) != len(us) || !reflect.DeepEqual(parents(got), parents(us)) {
		t.Fatalf("twelve modules and five contracts fit one overview")
	}
}
