package main

import (
	"fmt"
	"strings"
	"testing"
)

// A reader groups modules the script reports flat. The grouping is the
// reader's structure, so syncs that rerun the script keep it, and only the
// script reporting a parent of its own replaces it.
func TestAReaderGroupingSurvivesSyncs(t *testing.T) {
	p := semanticProject(t)
	p.code("money/README.md", "# money\n\nbilling and ledger move money.\n")
	p.want(0, "recorded MOD-0003", "record", "unit", "MOD", "money", "--evidence", "money/README.md:3:billing and ledger move money.")
	for _, m := range []string{"MOD-0001", "MOD-0002"} {
		p.want(0, "INFERRED", "record", "parent", m, "MOD-0003", "--evidence", "money/README.md:3:billing and ledger move money.")
	}
	for i := 0; i < 2; i++ {
		p.resync()
		for _, m := range []string{"MOD-0001", "MOD-0002"} {
			md := p.read("architecture/" + m + ".md")
			if !strings.Contains(md, "parent: MOD-0003") || !strings.Contains(md, "parent_origin: INFERRED") || !strings.Contains(md, "quote: billing and ledger move money.") {
				t.Fatalf("sync %d dropped the reader's grouping of %s:\n%s", i+1, m, md)
			}
		}
	}

	p.code("money/README.md", "# money\n")
	p.resync()
	if md := p.read("architecture/MOD-0001.md"); strings.Contains(md, "parent: MOD-0003") {
		t.Fatalf("a reader's parent whose evidence is gone must go with it:\n%s", md)
	}
}

func TestAParentThatClosesACycleIsRefused(t *testing.T) {
	p := semanticProject(t)
	p.code("money/README.md", "# money\n\nbilling and ledger move money.\n")
	p.want(0, "recorded MOD-0003", "record", "unit", "MOD", "money", "--evidence", "money/README.md:3:billing and ledger move money.")
	p.want(0, "USER_STATED", "record", "parent", "MOD-0001", "MOD-0003", "--source", "conversation 2026-11-01")
	p.want(1, "its own ancestor", "record", "parent", "MOD-0003", "MOD-0001", "--source", "conversation 2026-11-01")
	p.want(1, "--evidence", "record", "parent", "MOD-0002", "MOD-0003")
}

// The script's word on its own units wins: when it reports a parent, that
// parent replaces what a reader gave.
func TestTheScriptsParentReplacesAReadersParent(t *testing.T) {
	p := semanticProject(t)
	p.code("money/README.md", "# money\n\nbilling and ledger move money.\n")
	p.want(0, "recorded MOD-0003", "record", "unit", "MOD", "money", "--evidence", "money/README.md:3:billing and ledger move money.")
	p.want(0, "INFERRED", "record", "parent", "MOD-0002", "MOD-0003", "--evidence", "money/README.md:3:billing and ledger move money.")
	p.extract(`{"units":[
 {"key":"billing","kind":"MOD","name":"billing","anchors":[{"path":"billing/charge.go","symbol":"Charge"}]},
 {"key":"ledger","kind":"MOD","name":"ledger","parent":"billing","anchors":[{"path":"ledger/book.go","symbol":"Post"}]}
]}`)
	p.resync()
	md := p.read("architecture/MOD-0002.md")
	if !strings.Contains(md, "parent: MOD-0001") || strings.Contains(md, "parent_origin") {
		t.Fatalf("the script's parent replaces the reader's and carries no reader origin:\n%s", md)
	}
}

// A directory group exists only while the top level is too big, so a parent
// on it would leave its unit nowhere once the group goes.
func TestAParentOnADirectoryGroupIsRefused(t *testing.T) {
	p, exe := bare(t)
	var us []string
	for i := 1; i <= 20; i++ {
		dir := "apps"
		if i > 10 {
			dir = "libs"
		}
		f := fmt.Sprintf("%s/m%02d/m.go", dir, i)
		p.code(f, "package m\n")
		us = append(us, fmt.Sprintf(`{"key":"m%02d","kind":"MOD","name":"m%02d","anchors":[{"path":%q}]}`, i, i, f))
	}
	p.extract(`{"units":[` + strings.Join(us, ",") + `]}`)
	p.setup(0, "synced", "--", exe)
	if md := p.read("architecture/MOD-0021.md"); !strings.Contains(md, "apps/") {
		t.Fatalf("MOD-0021 should be the apps/ group:\n%s", md)
	}
	p.want(1, "directory group", "record", "parent", "MOD-0011", "MOD-0021", "--source", "conversation 2026-11-01")
}
