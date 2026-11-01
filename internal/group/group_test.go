package group

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/whyjp/abbd/internal/model"
)

func module(n int, file string) model.Unit {
	return model.Unit{ID: model.FormatID(model.Module, n), Kind: model.Module, Name: model.Fact{Value: file, Origin: model.Extracted}, Anchors: []model.Anchor{{Path: file}}}
}

// Fourteen top-level modules, eight under apps/ and six under native/.
func flat() []model.Unit {
	var us []model.Unit
	for i := 1; i <= 14; i++ {
		dir := "apps"
		if i > 8 {
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
	us, hw := Fallback(flat(), map[model.Kind]int{model.Module: 14})
	p := parents(us)
	if p["MOD-0001"] != "MOD-0015" || p["MOD-0009"] != "MOD-0016" || p["MOD-0015"] != "" || p["MOD-0016"] != "" {
		t.Fatalf("want apps/ and native/ groups at the top: %v", p)
	}
	if hw[model.Module] != 16 {
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
	for i := range us[:13] {
		us[i].Parent, us[i].ParentOrigin = "MOD-0014", model.Inferred
		us[i].ParentEvidence = []model.Evidence{{Path: "README.md", Lines: "1", Quote: "x"}}
	}
	got, _ := Fallback(us, map[model.Kind]int{model.Module: 14})
	for _, u := range got[:13] {
		if u.Parent != "MOD-0014" {
			t.Fatalf("the fallback moved %s out of the reader's group: %v", u.ID, parents(got))
		}
	}
	if len(got) != 14 {
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

// The overview draws a top-level contract as a chip, so contracts do not
// count against the budget there.
func TestTopLevelContractsDoNotCrowdTheOverview(t *testing.T) {
	us := flat()[:Budget]
	for i := 1; i <= 5; i++ {
		us = append(us, model.Unit{ID: model.FormatID(model.Interface, i), Kind: model.Interface, Name: model.Fact{Value: "c", Origin: model.Extracted}, Anchors: []model.Anchor{{Path: fmt.Sprintf("api/c%d.go", i)}}})
	}
	got, _ := Fallback(us, nil)
	if len(got) != len(us) || !reflect.DeepEqual(parents(got), parents(us)) {
		t.Fatalf("twelve modules and five contracts fit one overview")
	}
}
