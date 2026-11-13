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
