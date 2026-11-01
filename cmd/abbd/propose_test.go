package main

import (
	"strings"
	"testing"
)

// An agent that finds a coupling mid-turn records it; only a human makes it count.
func TestAnAgentProposesACouplingInsideATurnAndCannotConfirmIt(t *testing.T) {
	p := activeTurnProject(t, topoV1)
	p.start("MOD-0002")
	if o, _ := p.fire("PreToolUse", map[string]any{"session_id": "s1", "tool_name": "Bash", "tool_input": map[string]any{"command": "abbd propose relation MOD-0002 depends_on MOD-0001 --via call --evidence billing/charge.go:3:func Charge"}}); denied(o) != "" {
		t.Fatalf("propose is the agent's command inside a turn: %s", denied(o))
	}
	p.want(1, "the code does not hold billing/charge.go:3", "propose", "relation", "MOD-0002", "depends_on", "MOD-0001", "--via", "call", "--evidence", "billing/charge.go:3:func Charges()")
	p.want(1, "say how the coupling arises", "propose", "relation", "MOD-0002", "depends_on", "MOD-0001", "--evidence", "billing/charge.go:3:func Charge")
	p.want(1, "MOD provides MOD is not a relation", "propose", "relation", "MOD-0002", "provides", "MOD-0001", "--via", "call", "--evidence", "billing/charge.go:3:func Charge")
	before := p.intended()
	p.want(0, "proposed MOD-0002.depends_on.MOD-0001 via call, INFERRED and UNCONFIRMED", "propose", "relation", "MOD-0002", "depends_on", "MOD-0001", "--via", "call", "--evidence", "billing/charge.go:3:func Charge() {}")

	p.want(0, `MOD-0002.depends_on.MOD-0001 "INFERRED via call: billing/charge.go:3 \"func Charge() {}\"" (design unit)`, "confirm", "--list")
	if md := p.read("architecture/MOD-0002.md"); !strings.Contains(md, "confirmation: UNCONFIRMED") || !strings.Contains(md, "via: call") {
		t.Fatalf("a proposal is stored as an unconfirmed guess with its via:\n%s", md)
	}
	p.want(0, "", "turn", "diff", "--session", "s1")
	if p.intended() != before {
		t.Fatalf("a proposal must not reach the intended topology")
	}
	p.want(0, "impact MOD-0001: depended on by MOD-0002 (semantic, via call) (computed, direct)", "turn", "impact", "--session", "s1")
	if o, _ := p.fire("PreToolUse", map[string]any{"session_id": "s1", "tool_name": "Bash", "tool_input": map[string]any{"command": "abbd confirm MOD-0002.depends_on.MOD-0001"}}); denied(o) == "" {
		t.Fatalf("confirming stays a human's command")
	}
}

func TestAnAgentProposesAUnitTheScriptCannotSee(t *testing.T) {
	p := activeTurnProject(t, topoV1)
	p.want(1, "--parent IF-0001 is not a module", "propose", "unit", "DATA", "refunds", "--parent", "IF-0001", "--evidence", "billing/charge.go:5:func Refund() {}")
	p.want(0, "proposed DATA-0001 refunds, INFERRED", "propose", "unit", "DATA", "refunds", "--parent", "MOD-0001", "--evidence", "billing/charge.go:5:func Refund() {}")
	p.want(1, "DATA-0001 refunds already rests on the same code", "propose", "unit", "DATA", "refunds again", "--evidence", "billing/charge.go:5:func Refund() {}")
	p.want(0, "proposed MOD-0001.provides.DATA-0001", "propose", "relation", "MOD-0001", "provides", "DATA-0001", "--via", "data", "--evidence", "billing/charge.go::func Refund")
	p.want(0, "active", "mode", "on", "--session", "s1")
	md := p.read("architecture/DATA-0001.md")
	if !strings.Contains(md, "parent: MOD-0001") || strings.Contains(md, "absent") {
		t.Fatalf("a proposed unit stays through re-extraction while its evidence is live:\n%s", md)
	}
	p.want(0, "live MOD-0001.provides.DATA-0001 via data: billing/charge.go \"func Refund\" found at 5", "evidence")
}

// A prompt can be skipped, so the CLI notes every direct design unit no one
// checked for semantic couplings. It proceeds; a human sees the card.
func TestAnUnscannedDirectUnitLeavesAGradeBCard(t *testing.T) {
	p := activeTurnProject(t, topoV1)
	p.start("MOD-0001")
	p.want(1, "not in the impact", "turn", "scan", "--session", "s1", "--unit", "MOD-0002", "--by", "how")
	p.want(1, "--by names the pstack skill", "turn", "scan", "--session", "s1", "--unit", "MOD-0001", "--by", "grep")
	p.want(0, "CHG-0001: MOD-0001 scanned for semantic couplings by how", "turn", "scan", "--session", "s1", "--unit", "MOD-0001", "--by", "how")
	p.editCode("billing/charge.go")
	p.verify("pass", 0, "PASSED")
	out := p.want(0, "CHG-0001 SYNCED", "turn", "end", "--session", "s1")
	if !strings.Contains(out, "B PROCEED semantic-unscanned:IF-0001") || strings.Contains(out, "semantic-unscanned:MOD-0001") {
		t.Fatalf("only the direct design unit no one scanned leaves a card:\n%s", out)
	}
	if adr := p.read("decisions/ADR-0001.md"); !strings.Contains(adr, "semantic-unscanned:IF-0001") || !strings.Contains(adr, "confirmation: UNCONFIRMED") {
		t.Fatalf("the card is an unconfirmed ADR for a human:\n%s", adr)
	}
}

// A confirmed coupling gates like any other, so an edit to the lines it rests
// on is a change of meaning the gate cannot read.
func TestEditingTheEvidenceOfAConfirmedCouplingLeavesAGradeBCard(t *testing.T) {
	p := activeTurnProject(t, topoV1)
	p.want(0, "proposed", "propose", "relation", "MOD-0002", "depends_on", "MOD-0001", "--via", "call", "--evidence", "billing/charge.go:3:func Charge()")
	p.want(0, "approved new-relation:MOD-0002.depends_on.MOD-0001", "confirm", "MOD-0002.depends_on.MOD-0001")
	p.want(0, "active", "mode", "on", "--session", "s1")
	p.start("MOD-0002")
	for _, id := range []string{"MOD-0001", "MOD-0002"} {
		p.want(0, "scanned", "turn", "scan", "--session", "s1", "--unit", id, "--by", "blast-radius")
	}
	p.code("billing/charge.go", "package billing\n\nfunc Charge() { audit() }\n\nfunc Refund() {}\n")
	p.verify("pass", 0, "PASSED")
	out := p.want(0, "CHG-0001 SYNCED", "turn", "end", "--session", "s1")
	if !strings.Contains(out, "B PROCEED evidence-touched:MOD-0002.depends_on.MOD-0001") || strings.Contains(out, "semantic-unscanned") {
		t.Fatalf("an edit to a confirmed coupling's quoted lines should leave one card:\n%s", out)
	}
}
