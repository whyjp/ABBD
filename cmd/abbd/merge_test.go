package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// mergeBranches has two branches off one synced commit each change the code
// of a different module and sync, then merges the second into the first. It
// returns the files git left conflicted.
func mergeBranches(t *testing.T, attributes string) *project {
	t.Helper()
	p := semanticProject(t)
	if attributes != "" {
		p.code(".gitattributes", attributes)
	}
	p.resync()
	git(t, p.root, "add", "-A")
	git(t, p.root, "commit", "-qm", "synced")
	git(t, p.root, "branch", "base")
	for _, side := range []struct{ branch, file string }{{"a", "billing/charge.go"}, {"b", "ledger/book.go"}} {
		git(t, p.root, "switch", "-q", "-c", side.branch, "base")
		b, _ := os.ReadFile(filepath.Join(p.root, side.file))
		p.code(side.file, string(b)+"\n// changed on "+side.branch+"\n")
		p.resync()
		git(t, p.root, "add", "-A")
		git(t, p.root, "commit", "-qm", "change "+side.file)
	}
	git(t, p.root, "switch", "-q", "a")
	exec.Command("git", "-C", p.root, "-c", "user.name=t", "-c", "user.email=t@t", "-c", "core.autocrlf=false", "merge", "-q", "--no-edit", "b").Run()
	return p
}

func conflicted(t *testing.T, root string) []string {
	t.Helper()
	out, err := exec.Command("git", "-C", root, "diff", "--name-only", "--diff-filter=U").Output()
	if err != nil {
		t.Fatal(err)
	}
	return strings.Fields(string(out))
}

// Two syncs of unrelated changes differ only in the one state file, and abbd
// sync on the merged tree resolves it, conflict markers and all.
func TestTwoSyncedBranchesConflictOnlyInTheState(t *testing.T) {
	p := mergeBranches(t, "")
	if got := conflicted(t, p.root); len(got) != 1 || got[0] != ".abbd/state.yaml" {
		t.Fatalf("the merge should conflict in .abbd/state.yaml alone, got %v", got)
	}
	p.want(1, "the state is STALE: a merge kept two syncs, run abbd sync on the merged tree", "status", "--check")
	p.resync()
	if strings.Contains(p.read("state.yaml"), "<<<<<<<") {
		t.Fatalf("abbd sync left the conflict in the state:\n%s", p.read("state.yaml"))
	}
	p.want(0, "blueprint: CURRENT", "status", "--check")
}

// With the state merged by union, git merges cleanly and the state reads
// STALE until a sync of the merged tree.
func TestAUnionMergedStateReadsStale(t *testing.T) {
	p := mergeBranches(t, ".abbd/state.yaml merge=union\n")
	if got := conflicted(t, p.root); len(got) != 0 {
		t.Fatalf("a union merge of the state should leave no conflict, got %v", got)
	}
	out := p.want(1, "a merge kept two syncs", "status", "--check")
	if !strings.Contains(out, "the blueprint is STALE") {
		t.Fatalf("the blueprint rendered on one branch is not the merged tree's:\n%s", out)
	}
	p.resync()
	p.want(0, "state: CURRENT", "status", "--check")
}

// A sync whose units did not change rewrites no rendered file, so the
// blueprint of two such syncs never conflicts.
func TestTheBlueprintCarriesNoPerSyncValue(t *testing.T) {
	p := semanticProject(t)
	graph, page := p.read("blueprint/graph.json"), p.blueprint()
	p.code("billing/charge.go", p.read("../billing/charge.go")+"\n// unrelated\n")
	git(t, p.root, "add", "-A")
	git(t, p.root, "commit", "-qm", "next")
	p.resync()
	if p.read("blueprint/graph.json") != graph || p.blueprint() != page {
		t.Fatalf("a sync that changed no unit rewrote the blueprint")
	}
	p.want(0, "blueprint: CURRENT", "status", "--check")
}

// A checkout without the pages passes the check while the state is
// current; the pages are drawn by the next sync.
func TestABlueprintNotDrawnHereStillPassesTheCheck(t *testing.T) {
	p := semanticProject(t)
	os.RemoveAll(filepath.Join(p.root, ".abbd", "blueprint"))
	p.want(0, "blueprint: NOT DRAWN here", "status", "--check")
	p.resync()
	p.want(0, "blueprint: CURRENT", "status", "--check")
}
