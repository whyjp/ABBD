package tests

import (
	"strings"
	"testing"
)

// Rules that used to be prose in the skills and are now held by the engine.

func TestAgentAnswerIsNotTheUsersDecision(t *testing.T) {
	p := newProject(t, "code-only")
	p.ok("init")
	p.ok("run", "선택")
	v := p.ok("decide", "--question", "어느 쪽?", "--option", "a=하나", "--option", "b=둘", "--answer", "b")
	if str(v, "decision", "chosen") != "b" || str(v, "decision", "confirmation") != "UNCONFIRMED" || get(v, "decision", "answered") == true {
		t.Fatalf("an answer without --user is the agent's choice, still open: %v", get(v, "decision"))
	}
	if c, code, _ := p.run("decide", "--tier", "C", "--question", "운영 배포?", "--answer", "approve"); code != 2 || get(c, "blocked") != true {
		t.Fatalf("an agent cannot approve a tier C stop: exit %d", code)
	}
}

func TestBugFixNeedsFailingFirstEvidence(t *testing.T) {
	p := newProject(t, "code-only")
	p.ok("init")
	p.ok("run", "health 응답 버그 고쳐줘")
	p.replace("internal/api/api.go", `w.Write([]byte("ok"))`, `w.Write([]byte("ok"))
	_ = 0`)
	p.ok("verify")
	if v, code, _ := p.run("sync"); code != 2 || !strings.Contains(str(v, "reason"), "재현") {
		t.Fatalf("a bug fix without a failing-first run must not sync: exit %d %v", code, get(v, "reason"))
	}
	p.ok("verify", "--no-repro", "--reason", "보고된 증상이 로컬에서 재현되지 않음")
	if s := p.ok("sync"); get(s, "completed") != true {
		t.Fatalf("a recorded waiver lets it complete: %v", get(s, "reason"))
	}
}

func TestBugFixWithFailingFirstEvidenceSyncs(t *testing.T) {
	p := newProject(t, "code-only")
	p.ok("init")
	p.ok("run", "health 응답 버그 고쳐줘")
	src := p.read("internal/api/api_test.go")
	p.write("internal/api/api_test.go", strings.Replace(src, `rec.Body.String() != "ok"`, `rec.Body.String() != "ok!"`, 1))
	if _, code, _ := p.run("verify"); code != 2 {
		t.Fatal("the reproduction must fail first")
	}
	p.replace("internal/api/api.go", `w.Write([]byte("ok"))`, `w.Write([]byte("ok!"))`)
	p.ok("verify")
	if s, code, _ := p.run("sync"); code != 0 || get(s, "completed") != true {
		t.Fatalf("failing-first then passing completes: exit %d %v", code, get(s, "reason"))
	}
}

func TestT3NeedsAUserLevelRun(t *testing.T) {
	p := newProject(t, "code-only")
	p.ok("init")
	feat := p.featureFor("POST /messages")
	plan := p.ok("run", "POST /messages 재시도 시 중복 발행 방지")
	if str(plan, "verification_tier") != "T3" {
		t.Fatalf("retry/duplicate work is T3: %v", get(plan, "verification_tier"))
	}
	p.replace("internal/api/api.go", "	w.WriteHeader(http.StatusAccepted)\n}", "	w.WriteHeader(http.StatusAccepted)\n}\n\nfunc dedupeKey(k string) string { return k }")
	p.ok("verify")
	if v, code, _ := p.run("sync"); code != 2 || !strings.Contains(str(v, "reason"), "사용자 수준") {
		t.Fatalf("T3 must not sync without user-level evidence: exit %d %v", code, get(v, "reason"))
	}
	p.ok("driver", "set", feat, "--run", "go test ./internal/api -run TestPostMessagePublishes -v", "--expect", "PASS: TestPostMessagePublishes")
	if r := p.ok("verify", "--drive"); str(r, "overall") != "PASSED" || str(list(r, "runs")[0], "level") != "user" {
		t.Fatalf("the driver run is user-level evidence: %v", r)
	}
	if s := p.ok("sync"); get(s, "completed") != true {
		t.Fatalf("T3 with a user-level run completes: %v", get(s, "reason"))
	}
}

func TestDriverFailsWhenTheExpectedOutcomeIsMissing(t *testing.T) {
	p := newProject(t, "code-only")
	p.ok("init")
	feat := p.featureFor("POST /messages")
	p.ok("driver", "set", feat, "--run", "go version", "--expect", "this text never appears")
	p.ok("run", "POST /messages 재시도 시 중복 발행 방지")
	if v, code, _ := p.run("verify", "--drive"); code != 2 || str(v, "overall") != "FAILED" {
		t.Fatalf("exit 0 without the expected output is not a pass: exit %d %v", code, v)
	}
}

func TestLoweringTheTierNeedsAReason(t *testing.T) {
	p := newProject(t, "code-only")
	p.ok("init")
	if _, code, _ := p.run("run", "POST /messages 재시도 시 중복 발행 방지", "--tier", "T1"); code == 0 {
		t.Fatal("T3 lowered to T1 without a reason must be refused")
	}
	v := p.ok("run", "POST /messages 재시도 시 중복 발행 방지", "--tier", "T1", "--reason", "문서 주석만 수정")
	if str(p.ok("inspect", str(v, "change")), "plan", "tier_reason") != "문서 주석만 수정" {
		t.Fatal("the reason stays on the receipt")
	}
}

func TestMeaningOutsideTheTurnIsNotRewritten(t *testing.T) {
	p := newProject(t, "code-only")
	p.ok("init")
	storage := p.moduleAt("internal/storage")
	p.ok("run", "README 문서 보강")
	if _, code, _ := p.run("intent", "module", "--id", storage, "--responsibility", "다른 해석"); code == 0 {
		t.Fatal("an INFERRED meaning outside this turn must not be rewritten by the agent")
	}
	if v := p.ok("intent", "module", "--id", storage, "--responsibility", "사용자가 말한 책임", "--origin", "USER_STATED"); str(v, "responsibility", "origin") != "USER_STATED" {
		t.Fatal("the user's own words may still be recorded")
	}
}

func TestHandEditsOfTheBlueprintAreDetected(t *testing.T) {
	p := newProject(t, "code-only")
	p.ok("init")
	m := p.moduleAt("internal/storage")
	md := ".abbd/architecture/" + m + ".md"
	p.replace(md, "(추정) storage 패키지가 노출하는 기능 묶음", "손으로 바꾼 책임")
	v, code, _ := p.run("check")
	if code != 2 || !strings.Contains(strings.Join(strs(v, "errors"), " "), "edited outside abbd") {
		t.Fatalf("a hand edit of the md originals must fail check: exit %d %v", code, get(v, "errors"))
	}
	p.replace(md, "## Notes", "## Notes\n\n사용자 메모는 자유롭게 쓴다.")
}

func TestLearnFromReceiptsNeedsRepetition(t *testing.T) {
	p := newProject(t, "code-only")
	p.ok("init")
	for i := 0; i < 2; i++ {
		p.ok("run", "health 응답 버그 고쳐줘")
		p.replace("internal/api/api.go", `w.Write([]byte("ok"))`, `w.Write([]byte("nope"))`)
		p.run("verify")
		p.replace("internal/api/api.go", `w.Write([]byte("nope"))`, `w.Write([]byte("ok"))`)
		p.ok("turn", "abort", "--reason", "실패 반복")
	}
	v := p.ok("learn", "--from-receipts")
	found := false
	for _, c := range list(v, "list") {
		if strings.Contains(str(c, "signal"), "TestHealth") && get(c, "count") == float64(2) {
			found = true
		}
	}
	if !found {
		t.Fatalf("a test that failed in two turns is a candidate: %v", v)
	}
}
