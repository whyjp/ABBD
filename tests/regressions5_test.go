package tests

import (
	"strings"
	"testing"
)

// Bypasses an adversarial review of fabc15a reproduced.

func TestHandTypedModuleIntentIsNotTheUsers(t *testing.T) {
	forge := func(p *project) {
		chg := str(p.ok("run", "util 패키지 추가"), "change")
		p.write("internal/util/util.go", "package util\n\nfunc Clamp(v int) int { return v }\n")
		md := ".abbd/changes/" + chg + ".md"
		s := p.read(md)
		end := 4 + strings.Index(s[4:], "\n---\n")
		p.write(md, s[:end]+"\nmodule_intents:\n    internal/util:\n        value: FORGED responsibility\n        origin: USER_STATED\n        confirmation: CONFIRMED"+s[end:])
		p.ok("verify")
	}
	responsibility := func(p *project) any {
		return get(p.ok("inspect", p.moduleAt("internal/util")), "module", "responsibility")
	}

	p := newProject(t, "code-only")
	p.ok("init")
	forge(p)
	p.ok("check", "--accept-edits")
	if s := p.ok("sync"); get(s, "completed") != true {
		t.Fatalf("the turn completes: %v", get(s, "reason"))
	}
	if r := responsibility(p); str(r, "origin") == "USER_STATED" || str(r, "confirmation") == "CONFIRMED" {
		t.Fatalf("a hand-typed module meaning accepted without --user is not the user's: %v", r)
	}

	p = newProject(t, "code-only")
	p.ok("init")
	forge(p)
	p.ok("check", "--accept-edits", "--user")
	p.ok("sync")
	if r := responsibility(p); str(r, "origin") != "USER_STATED" || str(r, "confirmation") != "CONFIRMED" {
		t.Fatalf("the user accepting their own edit keeps it: %v", r)
	}
}

const t3Driver = "go test ./internal/api -run TestPostMessagePublishes -v"

func dedupeChange(p *project) {
	p.replace("internal/api/api.go", "	w.WriteHeader(http.StatusAccepted)\n}", "	w.WriteHeader(http.StatusAccepted)\n}\n\nfunc dedupeKey(k string) string { return k }")
}

func TestDriverProvenanceIsNotEditedAway(t *testing.T) {
	p := newProject(t, "code-only")
	p.ok("init")
	feat := p.featureFor("POST /messages")
	p.ok("run", "POST /messages 재시도 시 중복 발행 방지")
	dedupeChange(p)
	p.ok("verify")
	p.ok("driver", "set", feat, "--run", t3Driver, "--expect", "PASS: TestPostMessagePublishes")
	var kept []string
	for _, line := range strings.Split(p.read(".abbd/project.yaml"), "\n") {
		if l := strings.TrimSpace(line); !strings.HasPrefix(l, "set_in:") && !strings.HasPrefix(l, "prompt:") {
			kept = append(kept, line)
		}
	}
	p.write(".abbd/project.yaml", strings.Join(kept, "\n"))
	p.ok("verify", "--drive")
	if _, code, _ := p.run("sync"); code != 2 {
		t.Fatal("a driver set in this turn stays this turn's after its set_in is deleted from project.yaml")
	}
}

func TestHandTypedDriverCommandIsSetNow(t *testing.T) {
	p := newProject(t, "code-only")
	p.ok("init")
	feat := p.featureFor("POST /messages")
	p.ok("run", "POST /messages 사용자 수준 driver 등록")
	p.ok("driver", "set", feat, "--run", "go version", "--expect", "go")
	p.ok("sync")
	p.ok("run", "POST /messages 재시도 시 중복 발행 방지")
	dedupeChange(p)
	p.write(".abbd/project.yaml", strings.Replace(p.read(".abbd/project.yaml"), "run: go version", "run: "+t3Driver, 1))
	p.write(".abbd/project.yaml", strings.Replace(p.read(".abbd/project.yaml"), "expect: go", `expect: "PASS: TestPostMessagePublishes"`, 1))
	p.ok("verify")
	p.ok("verify", "--drive")
	if _, code, _ := p.run("sync"); code != 2 {
		t.Fatal("a driver command typed into project.yaml during the turn does not prove it")
	}
}

func TestProjectSettingsStayHandEditable(t *testing.T) {
	p := newProject(t, "code-only")
	p.ok("init")
	feat := p.featureFor("POST /messages")
	p.ok("run", "POST /messages 사용자 수준 driver 등록")
	p.ok("driver", "set", feat, "--run", t3Driver, "--expect", "PASS: TestPostMessagePublishes")
	p.ok("sync")
	p.write(".abbd/project.yaml", strings.Replace(p.read(".abbd/project.yaml"), "max_questions_per_turn: 3", "max_questions_per_turn: 5", 1))
	if c := p.ok("check"); get(c, "ok") != true {
		t.Fatalf("editing a project setting is not a hand edit of the blueprint: %v", get(c, "errors"))
	}
	p.ok("run", "POST /messages 재시도 시 중복 발행 방지")
	dedupeChange(p)
	p.ok("verify")
	p.ok("verify", "--drive")
	if s := p.ok("sync"); get(s, "completed") != true {
		t.Fatalf("a driver from an earlier turn still proves T3 after a settings edit: %v", get(s, "reason"))
	}
	if !strings.Contains(p.read(".abbd/project.yaml"), "max_questions_per_turn: 5") {
		t.Fatal("the edited setting is kept")
	}
}

func TestDriverDeletedAfterItsRunStillCountsAsThisTurns(t *testing.T) {
	p := newProject(t, "code-only")
	p.ok("init")
	feat := p.featureFor("POST /messages")
	p.ok("run", "POST /messages 재시도 시 중복 발행 방지")
	dedupeChange(p)
	p.ok("verify")
	p.ok("driver", "set", feat, "--run", t3Driver, "--expect", "PASS: TestPostMessagePublishes")
	p.ok("verify", "--drive")
	yaml := p.read(".abbd/project.yaml")
	start := strings.Index(yaml, "drivers:")
	if start < 0 {
		t.Fatal("setup: no drivers block in project.yaml")
	}
	end := start + len("drivers:")
	for _, line := range strings.SplitAfter(yaml[end:], "\n")[1:] {
		if line != "" && line[0] != ' ' {
			break
		}
		end += len(line)
	}
	p.write(".abbd/project.yaml", yaml[:start]+yaml[end+1:])
	if _, code, _ := p.run("sync"); code != 2 {
		t.Fatal("the run recorded who set its driver; deleting the driver afterwards must not turn it into proof")
	}
}

func TestReproDriverLoosenedInTheFixTurnIsNotProof(t *testing.T) {
	p := newProject(t, "code-only")
	p.ok("init")
	// A driver on the contract covers any turn whose feature exposes it.
	feat := p.findContract("GET /health")
	probe := `grep -c "ok!" internal/api/api.go; true`
	p.ok("run", "사용자 수준 점검 명령 등록")
	p.ok("driver", "set", feat, "--run", probe, "--expect", "1")
	p.ok("sync")
	p.ok("run", "health 응답 버그 고쳐줘")
	if r, code, _ := p.run("verify", "--drive"); code != 2 || str(r, "overall") != "FAILED" {
		t.Fatalf("setup: the earlier driver must fail before the fix: exit %d %v", code, r)
	}
	p.ok("driver", "set", feat, "--run", probe, "--expect", "0")
	p.replace("internal/api/api.go", `w.Write([]byte("ok"))`, `w.Write([]byte("ok")) // fixed`)
	p.ok("verify")
	p.ok("verify", "--drive")
	if v, code, _ := p.run("sync"); code != 2 {
		t.Fatalf("a repro and its fix must be the same unchanged driver, not one re-set to expect less: %v", get(v, "reason"))
	}
}
