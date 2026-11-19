package main

import (
	"fmt"
	"slices"
	"strings"

	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/store"
)

// scopeWords are the words that name samples, examples, tests and generated
// code, as a whole word of a unit's name or a whole directory name of its
// anchors. "testing" and "generator" name product code too, so they are not
// among them.
var scopeWords = []string{
	"example", "examples", "sample", "samples", "demo", "demos",
	"test", "tests", "testdata", "bench", "benches", "benchmark", "benchmarks",
	"generated", "mock", "mocks", "e2e", "fixtures",
}

func scopeWordIn(s string) string {
	for _, w := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9')
	}) {
		if slices.Contains(scopeWords, w) {
			return w
		}
	}
	return ""
}

// unscoped names each top-level module the overview draws that is named or
// placed like samples, examples, tests or generated code and carries no
// scope mark in the overview record, nor sits inside a marked unit. The
// overview draws a box for the product's parts, and a reader marks what is
// not the product with its scope so whoever draws it can tell the two apart.
func unscoped(units []model.Unit, o store.Overview) []string {
	present := map[model.ID]model.Unit{}
	for _, u := range model.Present(units) {
		present[u.ID] = u
	}
	marked := map[model.ID]bool{}
	for _, m := range o.Scopes {
		marked[m.Claim.Unit] = true
	}
	covered := func(id model.ID) bool {
		for seen := map[model.ID]bool{}; id != "" && !seen[id]; id = present[id].Parent {
			if marked[id] {
				return true
			}
			seen[id] = true
		}
		return false
	}
	var out []string
	for _, u := range model.Present(units) {
		if u.Kind != model.Module || u.Parent != "" || covered(u.ID) {
			continue
		}
		why := ""
		if w := scopeWordIn(u.Name.Value); w != "" {
			why = fmt.Sprintf("its name says %q", w)
		}
		if why == "" {
			why = anchorsUnderScope(u.Anchors)
		}
		if why != "" {
			out = append(out, fmt.Sprintf("%s %s (%s)", u.ID, u.Name.Value, why))
		}
	}
	slices.Sort(out)
	return out
}

// anchorsUnderScope says why when every anchor of a unit lies under a
// directory named for samples, examples, tests or generated code. One such
// anchor is not enough, since a product module may anchor a test file.
func anchorsUnderScope(anchors []model.Anchor) string {
	if len(anchors) == 0 {
		return ""
	}
	first := ""
	for _, a := range anchors {
		hit := ""
		dirs := strings.Split(strings.ReplaceAll(a.Path, "\\", "/"), "/")
		for _, d := range dirs[:max(len(dirs)-1, 0)] {
			if scopeWordIn(d) != "" {
				hit = d
				break
			}
		}
		if hit == "" {
			return ""
		}
		if first == "" {
			first = fmt.Sprintf("its anchor %s lies under %q", a.Path, hit)
		}
	}
	return first
}
