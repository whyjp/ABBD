package main

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

// runtimeProject is the nested project with a Go worker in app/sub that
// starts a goroutine and hands it work on a channel, set up with sup.
func runtimeProject(t *testing.T, sup string) (*project, string) {
	t.Helper()
	p, exe := bare(t)
	p.code("app/main.py", "def serve():\n    return 'GET /x'\n")
	p.code("app/sub/x.py", "INSERT INTO charges\n")
	p.code("app/sub/w.go", "package sub\n\nfunc Start(q chan int) {\n\tgo work(q)\n\tq <- 1\n}\n\nfunc work(q chan int) { <-q }\n")
	p.extract(nested)
	var o, e bytes.Buffer
	code := run([]string{"setup", "--root", p.root, "--supplement", p.file(t, "sup.json", sup), "--", exe}, neverRead{t}, &o, &e)
	return p, fmt.Sprintf("exit %d\n%s%s", code, o.String(), e.String())
}

const workerExec = `{"units":[
 {"key":"worker","kind":"EXEC","name":"work goroutine","parent":"app/sub","run":"wait 1",
  "evidence":[{"path":"app/sub/w.go","lines":"4","quote":"go work(q)"}]}]}`

func TestTheRunTimeWorkListNamesEachSiteTheStateDoesNotHold(t *testing.T) {
	t.Parallel()
	p, out := runtimeProject(t, `{}`)
	if p.setupCode(out) != 0 {
		t.Fatalf("setup:\n%s", out)
	}
	contains(t, out, "runtime: 2 of 2 candidate(s) not stored: 1 spawn site(s) with no EXEC, 1 channel pair(s) with no posts_to; abbd runtime lists them")
	contains(t, p.want(0, "", "status"), "runtime: 2 of 2 candidate(s) not stored")
	list := p.want(0, "", "runtime")
	contains(t, list, `open EXEC MOD-0002 go runs work: app/sub/w.go:4 "go work(q)"`)
	contains(t, list, `open posts_to MOD-0002 -> MOD-0002 <- on q: app/sub/w.go:5 "q <- 1" -> app/sub/w.go:8 "func work(q chan int) { <-q }"`)
	contains(t, list, "2 candidate(s), 2 not stored")
}

func TestAStoredExecLeavesTheWorkList(t *testing.T) {
	t.Parallel()
	p, out := runtimeProject(t, workerExec)
	if p.setupCode(out) != 0 {
		t.Fatalf("setup:\n%s", out)
	}
	contains(t, p.want(0, "", "status"), "runtime: 1 of 2 candidate(s) not stored: 0 spawn site(s) with no EXEC, 1 channel pair(s) with no posts_to")
	if list := p.want(0, "", "runtime"); strings.Contains(list, "EXEC") {
		t.Errorf("a stored EXEC is still on the work list:\n%s", list)
	}
	contains(t, p.want(0, "", "runtime", "--verbose"), `stored EXEC MOD-0002 go runs work: app/sub/w.go:4`)
}

func TestRuntimeRecordsAreTheSharedJSONLines(t *testing.T) {
	t.Parallel()
	p, out := runtimeProject(t, `{}`)
	if p.setupCode(out) != 0 {
		t.Fatalf("setup:\n%s", out)
	}
	got := p.want(0, "", "runtime", "--records")
	want := `{"kind":"spawn","api":"go","module":"MOD-0002","from":"","to":{"name":"work","def":"app/sub/w.go:8"},"channel":"","evidence":{"path":"app/sub/w.go","line":4,"quote":"go work(q)"},"configs":[],"confidence":"syntactic","via":"catalog"}
{"kind":"send","api":"<-","module":"MOD-0002","from":"","to":{},"channel":"q","evidence":{"path":"app/sub/w.go","line":5,"quote":"q <- 1"},"configs":[],"confidence":"syntactic","via":"catalog"}
{"kind":"recv","api":"<-","module":"MOD-0002","from":"","to":{},"channel":"q","evidence":{"path":"app/sub/w.go","line":8,"quote":"func work(q chan int) { <-q }"},"configs":[],"confidence":"syntactic","via":"catalog"}
`
	if got != want {
		t.Errorf("records:\n%s\nwant:\n%s", got, want)
	}
}

func TestARepositoryThatStartsNothingHasNoRuntimeLine(t *testing.T) {
	t.Parallel()
	p, out := nestedProject(t, `{}`)
	if p.setupCode(out) != 0 {
		t.Fatalf("setup:\n%s", out)
	}
	if strings.Contains(out+p.want(0, "", "status"), "runtime:") {
		t.Errorf("a repository with no spawn or channel site reports a runtime line")
	}
}
