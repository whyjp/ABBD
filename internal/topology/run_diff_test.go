package topology

import (
	"slices"
	"strings"
	"testing"

	"github.com/whyjp/abbd/internal/model"
)

// A thread that came or went, one that changed how it runs, and code that
// came to run on a thread it owns or to hand work to another, are each a
// line of their own, though the module and its thread are one top-level box.
func TestDiffNamesExecutionContextsAndWhatRunsOnThem(t *testing.T) {
	quotes := []model.Evidence{{Path: "a.c", Lines: "1", Quote: "push"}, {Path: "b.c", Lines: "2", Quote: "pop"}}
	unit := func(id, name, parent, run string, rs ...model.Relation) model.Unit {
		u := model.Unit{ID: model.ID(id), Kind: model.Module, Name: model.Said(model.Script, name), Anchors: []model.Anchor{{Path: name}}, Parent: model.ID(parent), Relations: rs}
		if k, _, _ := model.ParseID(id); k == model.Exec {
			u.Kind = model.Exec
			f := model.Said(model.Supplement, run)
			f.Origin = model.Inferred
			u.Run = &f
		}
		return u
	}
	runsOn := model.Relation{To: "EXEC-0001", Kind: model.RunsOn, Writer: model.Supplement, Via: "call", Evidence: quotes[:1]}
	postsTo := model.Relation{To: "EXEC-0002", Kind: model.PostsTo, Writer: model.Supplement, Via: "queue", Evidence: quotes}
	base := []model.Unit{
		unit("MOD-0001", "libobs", "", "", postsTo),
		unit("EXEC-0001", "video thread", "MOD-0001", "tick 1"),
		unit("EXEC-0002", "audio thread", "MOD-0001", "tick 1"),
		unit("EXEC-0003", "old pool", "MOD-0001", "wait pool"),
	}
	now := []model.Unit{
		unit("MOD-0001", "libobs", "", "", runsOn),
		unit("EXEC-0001", "video thread", "MOD-0001", "tick 1"),
		unit("EXEC-0002", "audio thread", "MOD-0001", "wait 1"),
		unit("EXEC-0004", "graphics thread", "MOD-0001", "tick 1"),
	}
	lines := Compare(base, now).Lines()
	for _, want := range []string{
		"+ exec EXEC-0004 graphics thread: tick 1",
		"- exec EXEC-0003 old pool: wait pool",
		`~ run EXEC-0002 audio thread: "tick 1" -> "wait 1", INFERRED`,
		"+ run MOD-0001 runs_on EXEC-0001 (libobs runs_on video thread) via call: a.c:1 \"push\"",
		"- run MOD-0001 posts_to EXEC-0002 (libobs posts_to audio thread) via queue",
	} {
		if !slices.Contains(lines, want) {
			t.Errorf("diff lacks %q:\n%s", want, strings.Join(lines, "\n"))
		}
	}
	if slices.ContainsFunc(lines, func(l string) bool { return strings.Contains(l, "relation MOD-0001") || strings.Contains(l, "inferred MOD-0001") }) {
		t.Errorf("a run-time relation reads as a relation between top-level units or as a reader's coupling:\n%s", strings.Join(lines, "\n"))
	}
}
