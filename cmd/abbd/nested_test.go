package main

import (
	"os"
	"path/filepath"
	"testing"
)

// A repository inside the project with no commit yet is an untracked
// directory: sync runs, and its files count toward the state's fingerprint.
func TestANestedRepositoryWithNoCommitIsAnUntrackedDirectory(t *testing.T) {
	p, exe := bare(t)
	p.code("app/main.go", "package app\n")
	p.code("nested/a.go", "package nested\n")
	git(t, filepath.Join(p.root, "nested"), "init", "-q")
	p.extract(`{"units":[{"key":"n","kind":"MOD","name":"nested","anchors":[{"path":"nested"}]}]}`)
	p.setup(0, "synced", "--", exe)
	p.want(0, "state: CURRENT", "status")
	p.code("nested/a.go", "package nested\n\nfunc A() {}\n")
	p.want(0, "state: STALE", "status")
	p.resync()
	p.want(0, "state: CURRENT", "status")
}

// A file staged in such a repository and then deleted is not on disk, so
// it is not part of the tree, and sync still runs.
func TestANestedRepositoryWithADeletedStagedFileSyncs(t *testing.T) {
	p, exe := bare(t)
	p.code("app/main.go", "package app\n")
	p.code("nested/a.go", "package nested\n")
	p.code("nested/gone.go", "package nested\n")
	nested := filepath.Join(p.root, "nested")
	git(t, nested, "init", "-q")
	git(t, nested, "add", "gone.go")
	if err := os.Remove(filepath.Join(nested, "gone.go")); err != nil {
		t.Fatal(err)
	}
	p.extract(`{"units":[{"key":"n","kind":"MOD","name":"nested","anchors":[{"path":"nested"}]}]}`)
	p.setup(0, "synced", "--", exe)
	p.want(0, "state: CURRENT", "status")
}

// The project's .gitignore holds inside such a repository, as it does for
// any untracked directory, so an ignored file there never stales the state.
func TestTheProjectsIgnoresHoldInsideANestedRepository(t *testing.T) {
	p, exe := bare(t)
	p.code(".gitignore", "nested/build.log\n")
	p.code("app/main.go", "package app\n")
	p.code("nested/a.go", "package nested\n")
	p.code("nested/build.log", "one\n")
	git(t, filepath.Join(p.root, "nested"), "init", "-q")
	p.extract(`{"units":[{"key":"n","kind":"MOD","name":"nested","anchors":[{"path":"nested"}]}]}`)
	p.setup(0, "synced", "--", exe)
	p.code("nested/build.log", "two\n")
	p.want(0, "state: CURRENT", "status")
}
