package main

import (
	"strings"
	"testing"
)

// execSup is a reader's supplement that records a thread inside app/sub,
// the app code that runs on it, and the queue app hands its work through.
func execSup(run, postsEvidence string) string {
	return `{"units":[
 {"key":"worker","kind":"EXEC","name":"charge worker","parent":"app/sub","run":"` + run + `",
  "evidence":[{"path":"app/sub/x.py","lines":"1","quote":"INSERT INTO charges"}]}],
 "relations":[
 {"from":"app","to":"worker","kind":"runs_on","via":"call",
  "evidence":[{"path":"app/main.py","lines":"1","quote":"def serve():"}]},
 {"from":"app","to":"worker","kind":"posts_to","via":"queue","phrase":"charge",
  "evidence":[` + postsEvidence + `]}]}`
}

const producer = `{"path":"app/main.py","lines":"2","quote":"return 'GET /x'"}`
const consumer = `{"path":"app/sub/x.py","lines":"1","quote":"INSERT INTO charges"}`

func TestAnExecutionContextKeepsItsRunAndItsRunTimeRelationsThroughSetup(t *testing.T) {
	t.Parallel()
	p, out := nestedProject(t, execSup(" wait  pool", producer+","+consumer))
	if p.setupCode(out) != 0 {
		t.Fatalf("setup should take an EXEC with a run:\n%s", out)
	}
	stored := p.read("architecture/EXEC-0001.md")
	for _, want := range []string{"kind: EXEC", "run:\n    value: wait pool\n    writer: supplement", "kind: runs_on", "kind: posts_to", "via: queue"} {
		if !strings.Contains(stored+p.read("architecture/MOD-0001.md"), want) {
			t.Errorf("the store lacks %q:\n%s\n%s", want, stored, p.read("architecture/MOD-0001.md"))
		}
	}
	p.want(0, "", "sync")
	if again := p.read("architecture/EXEC-0001.md"); again != stored {
		t.Errorf("a sync changed the EXEC:\n%s\n->\n%s", stored, again)
	}
}

func TestSetupRefusesARunWithAWordABBDDoesNotKnow(t *testing.T) {
	t.Parallel()
	p, out := nestedProject(t, execSup("spin 1", producer+","+consumer))
	if p.setupCode(out) == 0 || !strings.Contains(out, "use tick, wait, ready, complete, steal or inline") {
		t.Fatalf("setup should refuse run spin 1 and list the shapes:\n%s", out)
	}
}

func TestSetupRefusesAPostsToThatQuotesOnlyOneEnd(t *testing.T) {
	t.Parallel()
	p, out := nestedProject(t, execSup("wait 1", producer))
	if p.setupCode(out) == 0 || !strings.Contains(out, "needs 2 or more evidence items, got 1") {
		t.Fatalf("setup should refuse a posts_to with one quote:\n%s", out)
	}
}

func TestAHumanStatesARunOnlyOnAnExecutionContext(t *testing.T) {
	t.Parallel()
	p, out := nestedProject(t, execSup("wait 1", producer+","+consumer))
	if p.setupCode(out) != 0 {
		t.Fatalf("setup:\n%s", out)
	}
	p.want(0, "recorded EXEC-0001.run", "record", "fact", "EXEC-0001.run", "--value", "tick N", "--source", "conversation")
	contains(t, p.read("architecture/EXEC-0001.md"), "value: tick N")
	p.want(1, "unknown count", "record", "fact", "EXEC-0001.run", "--value", "tick 3", "--source", "conversation")
	p.want(1, "only an execution context", "record", "fact", "MOD-0001.run", "--value", "tick 1", "--source", "conversation")
}

// A reviewer reads a changed run and a run-time relation that went in the
// diff against the base the change started from.
func TestDiffNamesAChangedRunAndARunTimeRelationThatWent(t *testing.T) {
	t.Parallel()
	p, out := nestedProject(t, execSup("wait 1", producer+","+consumer))
	if p.setupCode(out) != 0 {
		t.Fatalf("setup:\n%s", out)
	}
	git(t, p.root, "add", "-A")
	git(t, p.root, "commit", "-qm", "base")
	sup := strings.Replace(execSup("tick N", producer+","+consumer), `"phrase":"charge",`, "", 1)
	sup = sup[:strings.Index(sup, `,
 {"from":"app","to":"worker","kind":"posts_to"`)] + "]}"
	_, exe := bare(t)
	p.setup(0, "", "--supplement", p.file(t, "sup2.json", sup), "--", exe)
	got := p.want(0, "", "diff", "--base", "HEAD")
	for _, want := range []string{`~ run EXEC-0001 charge worker: "wait 1" -> "tick N", INFERRED`, "- run MOD-0001 posts_to EXEC-0001 (app posts_to charge worker) via queue"} {
		contains(t, got, want)
	}
}

// Lint names a plug-point contract whose one provider looks like the host
// that calls through it, and still passes.
func TestLintAdvisesOnAPlugPointRecordedTheWrongWayRound(t *testing.T) {
	t.Parallel()
	p, exe := bare(t)
	p.code("host/api.h", "struct source_info { void (*render)(void *data); void (*tick)(void *data, float s); };\n")
	p.code("a/a.c", "x\n")
	p.code("b/b.c", "x\n")
	p.extract(`{"units":[
 {"key":"host","kind":"MOD","name":"host","anchors":[{"path":"host"}]},
 {"key":"a","kind":"MOD","name":"plugin a","anchors":[{"path":"a"}]},
 {"key":"b","kind":"MOD","name":"plugin b","anchors":[{"path":"b"}]},
 {"key":"api","kind":"IF","name":"source table","anchors":[{"path":"host/api.h"}],"signature":"struct source_info",
  "entries":[{"name":"render","shape":"void (*)(void *data)"},{"name":"tick","shape":"void (*)(void *data, float s)"}]}],
 "relations":[{"from":"host","to":"api","kind":"provides"},
 {"from":"a","to":"api","kind":"consumes","via":"call"},{"from":"b","to":"api","kind":"consumes","via":"call"}]}`)
	p.setup(0, "synced", "--", exe)
	code, out := lintAt(t, p.root)
	contains(t, out, "lint: direction: IF-0001 source table looks reversed: its entries are callback slots, MOD-0003 provides it and 2 units consume it via call")
	if code != exitOK {
		t.Errorf("the direction advice failed lint, exit %d:\n%s", code, out)
	}
}
