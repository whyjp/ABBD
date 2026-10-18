package tests

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/whyjp/abbd/internal/mcp"
	"github.com/whyjp/abbd/internal/workflow"
)

// Defects a skill author found by running the CLI against the PRD.

func TestRepeatedTierCQuestionStaysAStop(t *testing.T) {
	p := newProject(t, "code-only")
	p.ok("init")
	p.ok("run", "배포 준비")
	for i := 0; i < 2; i++ {
		if v, code, _ := p.run("decide", "--tier", "C", "--question", "운영에 배포할까?"); code != 2 || get(v, "blocked") != true {
			t.Fatalf("attempt %d: an unapproved tier C question must stay a stop: exit %d %v", i+1, code, v)
		}
	}
	d := p.ok("decisions", "운영에 배포")
	adr := str(list(d, "list")[0], "id")
	if v := p.ok("decide", "--adr", adr, "--answer", "reject", "--reason", "아직 아님"); str(v, "decision", "chosen") != "reject" {
		t.Fatalf("a tier C stop can be rejected through its ADR: %v", v)
	}
}

func TestAnsweringACardADRDecidesTheCard(t *testing.T) {
	p := newProject(t, "code-only")
	p.ok("init")
	post := p.findContract("POST /messages")
	p.ok("run", "POST /messages 호환성 변경")
	p.ok("intent", "contract", "--id", post, "--compatibility", "v2만 지원")
	card := deltaOf(p.ok("sync"), "contract_compat_changed")
	p.ok("decide", "--adr", str(card, "adr"), "--answer", "reject", "--reason", "v1 클라이언트가 남아 있음")
	for _, d := range list(p.ok("review"), "deltas") {
		if str(d, "id") == str(card, "id") && str(d, "status") != "rejected" {
			t.Fatalf("the card must follow its ADR: %v", d)
		}
	}
	next := p.ok("run", "후속")
	if !strings.Contains(strings.Join(strs(next, "rework"), " "), "v1 클라이언트가 남아 있음") {
		t.Fatalf("a rejection through the ADR still reaches the next turn: %v", get(next, "rework"))
	}
}

func TestNewFeatureFirstOwnerIsNotAnOwnershipMove(t *testing.T) {
	p := newProject(t, "code-only")
	p.ok("init")
	p.ok("run", "GET /messages/count API 추가")
	p.replace("internal/api/api.go", `	s.mux.HandleFunc("GET /health", s.handleHealth)`, countRoute)
	p.replace("internal/api/api.go", "	w.WriteHeader(http.StatusAccepted)\n}\n", "	w.WriteHeader(http.StatusAccepted)\n}\n"+strings.Replace(countHandler, "s.count", "0", 1))
	p.ok("verify")
	s := p.ok("sync")
	for _, k := range deltaKinds(s) {
		if k == "ownership_moved" {
			t.Fatalf("a new feature getting its owner is not an ownership move: %v", deltaKinds(s))
		}
	}
}

func TestTierBQuestionTakesTwoOrThreeOptions(t *testing.T) {
	p := newProject(t, "code-only")
	p.ok("init")
	p.ok("run", "선택")
	if _, code, _ := p.run("decide", "--question", "q", "--option", "a", "--option", "b", "--option", "c", "--option", "d"); code == 0 {
		t.Fatal("four options must be refused")
	}
	v := p.ok("decide", "--question", "q2", "--option", "a=유지=아무것도 바뀌지 않음", "--option", "b=변경=API가 바뀜")
	if str(list(v, "decision", "options")[1], "effect") != "API가 바뀜" {
		t.Fatalf("each option carries what it changes: %v", get(v, "decision", "options"))
	}
}

func TestMCPResourcesUseTheAbbdScheme(t *testing.T) {
	p := newProject(t, "code-only")
	p.ok("init")
	e, _ := workflow.New(p.dir)
	var out bytes.Buffer
	mcp.Serve(e, strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"resources/read","params":{"uri":"abbd://status"}}`+"\n"), &out)
	if !strings.Contains(out.String(), `"contents"`) {
		t.Fatalf("abbd://status must be readable: %s", out.String())
	}
}

func TestHandEditedIDMustMatchItsFile(t *testing.T) {
	p := newProject(t, "code-only")
	p.ok("init")
	f := p.featureFor("POST /messages")
	path := filepath.Join(p.dir, ".abbd", "features", f+".md")
	b, _ := os.ReadFile(path)
	os.WriteFile(path, bytes.Replace(b, []byte("id: "+f), []byte("id: "+f+"X"), 1), 0o644)
	if v, code, _ := p.run("check"); code != 2 || !strings.Contains(strings.Join(strs(v, "errors"), " "), "must match the file name") {
		t.Fatalf("a renamed id must fail check: exit %d %v", code, get(v, "errors"))
	}
}
