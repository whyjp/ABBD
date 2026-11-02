package main

import (
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
