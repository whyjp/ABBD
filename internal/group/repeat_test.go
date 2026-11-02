package group

import (
	"fmt"
	"testing"

	"github.com/whyjp/abbd/internal/model"
)

// Orca's shared types and schemas is src/shared. Its units live in
// src/shared, one in a subdirectory of it, and one in mobile/. No group
// inside it may repeat src/shared or a directory above it.
func TestNoGroupRepeatsItsContainersDirectory(t *testing.T) {
	shared := model.Unit{ID: "MOD-0001", Kind: model.Module, Name: model.Said(model.Script, "shared types and schemas"),
		Anchors: []model.Anchor{{Path: "src/shared/a.ts"}, {Path: "src/shared/icons"}}}
	us := []model.Unit{shared}
	in := func(u model.Unit) model.Unit {
		u.PutParent(model.ParentClaim{To: "MOD-0001", Writer: model.Script})
		return u
	}
	for i := 2; i <= 19; i++ {
		us = append(us, in(module(i, fmt.Sprintf("src/shared/m%d", i))))
	}
	us = append(us, in(module(20, "src/shared/bundle/rpc.ts")), in(module(21, "mobile/src/relay.ts")))
	got, _ := Fallback(us, map[model.Kind]int{model.Module: 21})
	for _, u := range model.Present(got) {
		if Made(u) {
			t.Errorf("%s %q repeats the directory of the module it sits in: %v", u.ID, u.Name.Value, parents(got))
		}
	}
}

// Inside a container, units in its subdirectories still group by them, and
// a unit outside its directory stays where it is.
func TestAContainersSubdirectoriesStillGroup(t *testing.T) {
	us := []model.Unit{{ID: "MOD-0001", Kind: model.Module, Name: model.Said(model.Script, "core"), Anchors: []model.Anchor{{Path: "src/core.ts"}}}}
	in := func(u model.Unit) model.Unit {
		u.PutParent(model.ParentClaim{To: "MOD-0001", Writer: model.Script})
		return u
	}
	for i := 2; i <= 19; i++ {
		dir := "src/a"
		if i > 10 {
			dir = "src/b"
		}
		us = append(us, in(module(i, fmt.Sprintf("%s/m%d.ts", dir, i))))
	}
	us = append(us, in(module(20, "web/x.ts")))
	got, _ := Fallback(us, map[model.Kind]int{model.Module: 20})
	p := parents(got)
	byID := map[model.ID]model.Unit{}
	for _, u := range got {
		byID[u.ID] = u
	}
	a, b := byID[p["MOD-0002"]], byID[p["MOD-0011"]]
	if !Made(a) || a.Anchors[0].Path != "src/a" || !Made(b) || b.Anchors[0].Path != "src/b" || a.Parent != "MOD-0001" || b.Parent != "MOD-0001" {
		t.Fatalf("want src/a/ and src/b/ groups in core: %v", p)
	}
	if p["MOD-0020"] != "MOD-0001" {
		t.Fatalf("web/x.ts left core: %v", p)
	}
}

// A module anchored at a directory spans that directory, not the one above
// it, so no group inside it repeats the directory, whether or not it also
// anchors a file there.
func TestADirectoryAnchoredModuleIsItsDirectory(t *testing.T) {
	for _, anchors := range [][]model.Anchor{
		{{Path: "src/core"}},
		{{Path: "src/core"}, {Path: "src/core/a.ts"}},
	} {
		us := []model.Unit{{ID: "MOD-0001", Kind: model.Module, Name: model.Said(model.Script, "core"), Anchors: anchors}}
		in := func(u model.Unit) model.Unit {
			u.PutParent(model.ParentClaim{To: "MOD-0001", Writer: model.Script})
			return u
		}
		for i := 2; i <= 19; i++ {
			dir := "src/core"
			if i > 10 {
				dir = "src/core/sub"
			}
			us = append(us, in(module(i, fmt.Sprintf("%s/m%d.ts", dir, i))))
		}
		got, _ := Fallback(us, map[model.Kind]int{model.Module: 19})
		for _, u := range model.Present(got) {
			if Made(u) && !below(u.Anchors[0].Path, "src/core") {
				t.Errorf("anchors %v: %s %q repeats the directory of the module it sits in: %v", anchors, u.ID, u.Name.Value, parents(got))
			}
		}
	}
}
