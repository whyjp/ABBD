package main

import (
	"strings"
	"testing"
)

var dependsOnLedger = strings.Replace(topoV1, `{"from":"billing","to":"api","kind":"provides"}`,
	`{"from":"billing","to":"api","kind":"provides"},{"from":"billing","to":"ledger","kind":"depends_on"}`, 1)

func TestRejectedChangeStaysBlockedAndTellsTheNextWorkWhy(t *testing.T) {
	p := activeTurnProject(t, topoV1)
	p.start("MOD-0001")
	edge := "new-relation:MOD-0001.depends_on.MOD-0002"
	p.editCode("billing/charge.go")
	p.extract(dependsOnLedger)
	p.verify("pass", 0, "PASSED")
	p.want(2, "CHG-0001 BLOCKED", "turn", "end", "--session", "s1")
	before := p.intended()

	p.want(1, "needs --reason", "reject", edge)
	p.want(0, "rejected "+edge+" for CHG-0001 as ADR-0001; CHG-0001 stays BLOCKED", "reject", edge, "--reason", "billing must not read the ledger")
	if p.intended() != before {
		t.Fatalf("a rejection must leave the intended topology as it was:\n%s", p.intended())
	}
	if adr := p.read("decisions/ADR-0001.md"); !strings.Contains(adr, "decision: rejected") || !strings.Contains(adr, "reason: billing must not read the ledger") || !strings.Contains(adr, "origin: USER_STATED") {
		t.Fatalf("the rejection should be a USER_STATED ADR with its reason:\n%s", adr)
	}
	reason := "REJECTED " + edge + " by ADR-0001: billing must not read the ledger"
	p.want(0, reason, "turn", "show")
	p.want(2, reason, "turn", "diff", "--session", "s1")
	out := p.want(2, "CHG-0001 BLOCKED", "turn", "end", "--session", "s1")
	if !strings.Contains(out, reason) || !strings.Contains(out, "a human rejected it") {
		t.Fatalf("the blocked end should carry the reason to the agent:\n%s", out)
	}
	if !strings.Contains(p.read("changes/CHG-0001.md"), "## Rejected") {
		t.Fatalf("the turn record should list the rejection")
	}
	p.want(1, "neither a question", "reject", "new-relation:MOD-0002.depends_on.MOD-0001", "--reason", "x")

	p.want(0, "approved", "approve", edge)
	if out := p.want(0, "CHG-0001 SYNCED", "turn", "end", "--session", "s1"); strings.Contains(out, "REJECTED") {
		t.Fatalf("an approval after a rejection replaces it:\n%s", out)
	}
}

func TestRejectedQuestionIsNotAskedAgain(t *testing.T) {
	p := activeTurnProject(t, topoV1)
	p.start("MOD-0001")
	edge := "new-relation:MOD-0001.depends_on.MOD-0002"
	p.want(2, "C QUESTION "+edge, "turn", "declare", "--session", "s1", "--changes", edge)
	p.want(0, "stays OPEN", "reject", edge, "--reason", "use the api")
	p.want(2, "C REJECTED "+edge+" by ADR-0001: use the api; undo it or declare without it", "turn", "declare", "--session", "s1", "--changes", edge)
	p.want(0, "declares 0 unit(s) and 0 change(s)", "turn", "declare", "--session", "s1")
}

func TestRejectingOutsideATurnKeepsTheDrift(t *testing.T) {
	p := aligned(t)
	edge := "new-relation:MOD-0001.depends_on.MOD-0002"
	p.editCode("billing/charge.go")
	p.extract(dependsOnLedger)
	p.want(2, "BLOCKED "+edge, "mode", "on", "--session", "s1")
	before := p.intended()
	p.want(0, "rejected "+edge+" as ADR-0001; the intended topology is unchanged", "reject", edge, "--reason", "undo it")
	if p.intended() != before {
		t.Fatalf("a rejection must not change the intended topology")
	}
	p.want(2, "REJECTED "+edge+" by ADR-0001: undo it", "mode", "on", "--session", "s1")
}

func TestRejectingAGuessKeepsTheReplacement(t *testing.T) {
	p, exe := bare(t)
	p.extract(codeOnly)
	p.setup(0, "", "--supplement", p.file(t, "sup.json", supplement), "--", exe)
	p.want(1, "say what it is instead", "confirm", "MOD-0002.purpose", "--reject")
	p.want(0, "rejected MOD-0002.purpose", "confirm", "MOD-0002.purpose", "--reject", "--value", "posts journal entries")
	md := p.read("architecture/MOD-0002.md")
	if !strings.Contains(md, "value: posts journal entries\n    origin: USER_STATED\n    confirmation: REJECTED\n    rejected: keeps the books") {
		t.Fatalf("a rejected guess keeps the replacement and the rejected value:\n%s", md)
	}
	p.want(0, "rejected MOD-0002.consumes.MOD-0001", "confirm", "MOD-0002.consumes.MOD-0001", "--reject")
	p.want(1, "not a guess", "confirm", "MOD-0002.purpose", "--reject", "--value", "again")
	if q := p.want(0, "", "confirm", "--list"); strings.Contains(q, "MOD-0002") {
		t.Fatalf("rejected items leave the queue:\n%s", q)
	}
	p.want(0, "aligned", "align", "--all")
	if strings.Contains(p.read("intended.yaml"), "consumes") {
		t.Fatalf("a rejected coupling must not enter the intended topology")
	}
	p.setup(0, "", "--supplement", p.file(t, "sup.json", supplement))
	if !strings.Contains(p.read("architecture/MOD-0002.md"), "confirmation: REJECTED") {
		t.Fatalf("a new supplement must not overwrite what a human rejected")
	}
}

func TestCoupleRegistersWhatTheScriptCannotSee(t *testing.T) {
	p := aligned(t)
	edge := "MOD-0002.consumes.MOD-0001"
	p.want(1, "not a unit", "couple", "MOD-0002.consumes.MOD-0009")
	p.want(0, "approved new-relation:"+edge, "couple", edge)
	if !strings.Contains(p.read("architecture/MOD-0002.md"), "origin: USER_STATED") || !strings.Contains(p.intended(), "kind: consumes") {
		t.Fatalf("a registered coupling between design units should be stated and intended")
	}
	p.want(0, "active", "mode", "on", "--session", "s1")
	if !strings.Contains(p.read("architecture/MOD-0002.md"), "kind: consumes") {
		t.Fatalf("re-extraction must keep a coupling a human registered")
	}
	p.want(1, "already part of the actual topology", "couple", edge)
}

func TestAHumanCanRejectAfterAnApproval(t *testing.T) {
	p, edge := blockedTurn(t)
	p.want(0, "as ADR-0001", "approve", edge)
	p.want(0, "rejected "+edge+" for CHG-0001 as ADR-0002", "reject", edge, "--reason", "changed my mind")
	out := p.want(2, "CHG-0001 BLOCKED", "turn", "end", "--session", "s1")
	if !strings.Contains(out, "REJECTED "+edge+" by ADR-0002: changed my mind") {
		t.Fatalf("the later rejection should stand over the approval:\n%s", out)
	}
	p.want(0, "as ADR-0003", "approve", edge)
	p.want(0, "CHG-0001 SYNCED", "turn", "end", "--session", "s1")
}

func TestALaterRejectionOverridesAnEarlierAnswer(t *testing.T) {
	p := activeTurnProject(t, topoV1)
	edge := "new-relation:MOD-0001.depends_on.MOD-0002"
	p.start("MOD-0001")
	p.want(2, "QUESTION", "turn", "declare", "--session", "s1", "--changes", edge)
	p.want(0, "as ADR-0001", "approve", edge)
	p.editCode("billing/charge.go")
	p.want(2, "CHG-0001 DRAFT", "turn", "end", "--session", "s1")

	p.want(0, "CHG-0002 OPEN", "turn", "start", "--session", "s1", "--seeds", "MOD-0001", "--request", "again")
	p.want(0, "answered before", "turn", "declare", "--session", "s1", "--changes", edge)
	p.want(0, "as ADR-0002", "reject", edge, "--reason", "not any more")
	p.want(2, "C REJECTED "+edge+" by ADR-0002: not any more", "turn", "declare", "--session", "s1", "--changes", edge)
	p.want(0, "CHG-0002 CLOSED", "turn", "end", "--session", "s1")

	p.want(0, "CHG-0003 OPEN", "turn", "start", "--session", "s1", "--seeds", "MOD-0001", "--request", "third")
	out := p.want(2, "C QUESTION "+edge, "turn", "declare", "--session", "s1", "--changes", edge)
	if !strings.Contains(out, "rejected before by ADR-0002: not any more") || strings.Contains(out, "APPROVED") {
		t.Fatalf("a standing rejection must be asked again with its reason, never approved:\n%s", out)
	}
}
