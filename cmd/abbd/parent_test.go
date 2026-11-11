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
	t.Parallel()
	p := semanticProject(t)
	p.code("money/README.md", "# money\n\nbilling and ledger move money.\n")
	p.want(0, "recorded MOD-0003", "record", "unit", "MOD", "money", "--evidence", "money/README.md:3:billing and ledger move money.")
	for _, m := range []string{"MOD-0001", "MOD-0002"} {
		p.want(0, "INFERRED", "record", "parent", m, "MOD-0003", "--evidence", "money/README.md:3:billing and ledger move money.")
	}
	for i := 0; i < 2; i++ {
		p.resync()
		for _, m := range []string{"MOD-0001", "MOD-0002"} {
			if out := p.want(0, "", "explain", m); !strings.Contains(out, "inside MOD-0003 money\n") {
				t.Fatalf("sync %d dropped the reader's grouping of %s:\n%s", i+1, m, out)
			}
		}
	}

	p.code("money/README.md", "# money\n")
	p.resync()
	contains(t, p.want(0, "", "explain", "MOD-0001"), "inside MOD-0003 money\n")
	p.laterRevision()
	p.resync()
	if out := p.want(0, "", "explain", "MOD-0001"); strings.Contains(out, "inside MOD-0003") {
		t.Fatalf("a reader's parent whose evidence died must go with it:\n%s", out)
	}
	if md := p.read("architecture/MOD-0001.md"); !strings.Contains(md, "to: MOD-0003") {
		t.Fatalf("the dead claim stays stored with its reason:\n%s", md)
	}
}

// A human's placement outranks the script's. When the script's own parent
// later changes, the human's may be stale, so sync and status say so.
func TestAHumanParentOutranksTheScriptAndFlagsAScriptChange(t *testing.T) {
	t.Parallel()
	p := semanticProject(t)
	p.code("money/README.md", "# money\n\nbilling and ledger move money.\n")
	p.want(0, "recorded MOD-0003", "record", "unit", "MOD", "money", "--evidence", "money/README.md:3:billing and ledger move money.")
	p.want(0, "USER_STATED", "record", "parent", "MOD-0002", "MOD-0003", "--source", "conversation 2026-11-02")
	p.extract(`{"units":[
 {"key":"billing","kind":"MOD","name":"billing","anchors":[{"path":"billing/charge.go","symbol":"Charge"}]},
 {"key":"ledger","kind":"MOD","name":"ledger","parent":"billing","anchors":[{"path":"ledger/book.go","symbol":"Post"}]}
]}`)
	contains(t, p.resync(), "conflict: MOD-0002: a human placed it in MOD-0003")
	contains(t, p.want(0, "", "explain", "MOD-0002"), "inside MOD-0003 money\n", "parent claims: MOD-0003 by the human, MOD-0001 by the script")
	contains(t, p.want(0, "", "status"), "conflict: MOD-0002")
	p.want(0, "USER_STATED", "record", "parent", "MOD-0002", "MOD-0003", "--source", "conversation 2026-11-03")
	if out := p.resync(); strings.Contains(out, "conflict") {
		t.Fatalf("a human who restates the placement over the new script parent settles it:\n%s", out)
	}
}

func TestAParentThatClosesACycleIsRefused(t *testing.T) {
	t.Parallel()
	p := semanticProject(t)
	p.code("money/README.md", "# money\n\nbilling and ledger move money.\n")
	p.want(0, "recorded MOD-0003", "record", "unit", "MOD", "money", "--evidence", "money/README.md:3:billing and ledger move money.")
	p.want(0, "USER_STATED", "record", "parent", "MOD-0001", "MOD-0003", "--source", "conversation 2026-11-01")
	p.want(1, "its own ancestor", "record", "parent", "MOD-0003", "MOD-0001", "--source", "conversation 2026-11-01")
	p.want(1, "--evidence", "record", "parent", "MOD-0002", "MOD-0003")
}

// The script's word on its own units outranks a reader's: when it reports a
// parent, that parent shows, and the reader's claim stays stored beneath it.
func TestTheScriptsParentReplacesAReadersParent(t *testing.T) {
	t.Parallel()
	p := semanticProject(t)
	p.code("money/README.md", "# money\n\nbilling and ledger move money.\n")
	p.want(0, "recorded MOD-0003", "record", "unit", "MOD", "money", "--evidence", "money/README.md:3:billing and ledger move money.")
	p.want(0, "INFERRED", "record", "parent", "MOD-0002", "MOD-0003", "--evidence", "money/README.md:3:billing and ledger move money.")
	p.extract(`{"units":[
 {"key":"billing","kind":"MOD","name":"billing","anchors":[{"path":"billing/charge.go","symbol":"Charge"}]},
 {"key":"ledger","kind":"MOD","name":"ledger","parent":"billing","anchors":[{"path":"ledger/book.go","symbol":"Post"}]}
]}`)
	p.resync()
	contains(t, p.want(0, "", "explain", "MOD-0002"), "inside MOD-0001 billing\n")
	if md := p.read("architecture/MOD-0002.md"); !strings.Contains(md, "- to: MOD-0001\n      writer: script\n") || !strings.Contains(md, "- to: MOD-0003\n      writer: reader\n") {
		t.Fatalf("both claims stay stored, each with its writer:\n%s", md)
	}
}

// A directory group exists only while the top level is too big, so a parent
// on it would leave its unit nowhere once the group goes.
func TestAParentOnADirectoryGroupIsRefused(t *testing.T) {
	t.Parallel()
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

// A feature holds its parts the way a module holds its units: a skill's
// steps sit inside the skill, and a step's command is reached through it.
// A contract still holds nothing.
func TestAFeatureHoldsItsParts(t *testing.T) {
	t.Parallel()
	p, exe := bare(t)
	p.code("cli/run.go", "package cli\n\nfunc Run() {}\n")
	p.code("skills/go/SKILL.md", "---\nname: go\n---\n\n## Steps\n\n1. Run `cli run`.\n")
	parts := `{"units":[
 {"key":"cli","kind":"MOD","name":"cli","anchors":[{"path":"cli/run.go"}]},
 {"key":"run","kind":"IF","name":"cli run","signature":"run","anchors":[{"path":"cli/run.go","symbol":"Run"}]},
 {"key":"skill","kind":"FEAT","name":"/go","anchors":[{"path":"skills/go/SKILL.md","symbol":"go"}]},
 {"key":"step","kind":"FEAT","name":"1. Run cli run","parent":"skill","anchors":[{"path":"skills/go/SKILL.md","symbol":"Steps 1"}],
  "evidence":[{"path":"skills/go/SKILL.md","lines":"7","quote":"1. Run"}]}
],"relations":[
 {"from":"cli","to":"run","kind":"provides"},
 {"from":"step","to":"run","kind":"consumes","evidence":[{"path":"skills/go/SKILL.md","lines":"7","quote":"cli run"}]}
]}`
	p.extract(parts)
	p.setup(0, "synced", "--", exe)
	contains(t, p.want(0, "", "explain", "1. Run cli run"), "inside FEAT-0002 /go\n", "evidence: skills/go/SKILL.md:7 \"1. Run\"")
	contains(t, p.want(0, "", "path", "/go", "cli run"), "contains", "2 hop(s)")
	if html := p.blueprint(); !strings.Contains(html, "이 기능의 설명은 아직 없습니다") {
		t.Fatalf("a feature's page should name the missing text a feature's")
	}
	text := p.file(t, "go.md", "### Overview\n/go runs the cli.\n")
	p.want(0, "understood FEAT-0002 /go", "understand", "FEAT-0002", text)
	contains(t, p.blueprint(), "/go runs the cli.")

	p.extract(strings.Replace(parts, `"parent":"skill"`, `"parent":"run"`, 1))
	p.want(1, "is not the key of another module or feature", "sync")
}

// A script that comes to agree with a human's parent is no conflict.
func TestNoConflictWhenTheScriptComesToAgreeWithTheHuman(t *testing.T) {
	t.Parallel()
	p := semanticProject(t)
	p.want(0, "", "record", "parent", "MOD-0002", "MOD-0001", "--source", "pr 1")
	p.extract(strings.Replace(ledgerTopo, `"name":"ledger",`, `"name":"ledger","parent":"billing",`, 1))
	if out := p.resync() + p.want(0, "", "status"); strings.Contains(out, "conflict") {
		t.Fatalf("a script that agrees with the human is reported as a conflict:\n%s", out)
	}
}
