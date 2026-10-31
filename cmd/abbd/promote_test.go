package main

import (
	"strings"
	"testing"
)

const auditUnit = `{"key":"audit","kind":"MOD","name":"audit","anchors":[{"path":"audit/log.go","symbol":"Log"}]},`

func withUnit(topo, unit string) string {
	return strings.Replace(topo, `"units":[`, `"units":[`+unit, 1)
}

func withRelations(topo, rels string) string {
	return strings.Replace(topo, `{"from":"billing","to":"api","kind":"provides"}`, `{"from":"billing","to":"api","kind":"provides"},`+rels, 1)
}

func TestAdoptingANewModuleMakesItADesignUnit(t *testing.T) {
	p := aligned(t)
	p.editCode("audit/log.go")
	p.extract(withRelations(withUnit(topoV1, auditUnit), `{"from":"billing","to":"audit","kind":"depends_on"}`))
	p.want(2, "REALIGN new-relation:MOD-0001.depends_on.MOD-0003", "mode", "on", "--session", "s1")
	p.want(0, "PROMOTE unit-added:MOD-0003 (relates to MOD-0001)", "status")

	p.want(0, "adopted unit-added:MOD-0003 as ADR-0001; MOD-0003 is a design unit", "approve", "unit-added:MOD-0003")
	in := p.intended()
	if !strings.Contains(in, "id: MOD-0003") || !strings.Contains(in, "from: MOD-0001\n      to: MOD-0003\n      kind: depends_on") {
		t.Fatalf("adoption should take the unit and the relations it has now:\n%s", in)
	}
	if adr := p.read("decisions/ADR-0001.md"); !strings.Contains(adr, "decision: approved") || !strings.Contains(adr, "origin: USER_STATED") {
		t.Fatalf("adoption is a human's decision on record:\n%s", adr)
	}
	if out := p.want(0, "active", "mode", "on", "--session", "s1"); strings.Contains(out, "PROMOTE") {
		t.Fatalf("an adopted unit is no longer a candidate:\n%s", out)
	}
	p.want(1, "already a design unit", "approve", "unit-added:MOD-0003")
}

func TestRejectedCandidateStaysBelowTheLineUntilItsRelationsChange(t *testing.T) {
	p := aligned(t)
	p.editCode("audit/log.go")
	p.extract(withUnit(topoV1, auditUnit))
	p.want(0, "PROMOTE unit-added:MOD-0003", "mode", "on", "--session", "s1")
	before := p.intended()

	p.want(1, "needs --reason", "reject", "unit-added:MOD-0003")
	p.want(0, "rejected unit-added:MOD-0003 as ADR-0001; MOD-0003 stays below the line until its relations change", "reject", "unit-added:MOD-0003", "--reason", "a logging helper")
	if p.intended() != before {
		t.Fatalf("a rejected candidate must not enter the intended topology")
	}
	if out := p.want(0, "active", "mode", "on", "--session", "s1"); strings.Contains(out, "PROMOTE") {
		t.Fatalf("a rejected candidate is not proposed again:\n%s", out)
	}

	p.editCode("audit/log2.go")
	p.extract(withRelations(withUnit(topoV1, auditUnit), `{"from":"billing","to":"audit","kind":"depends_on"}`))
	p.want(2, "drifted", "mode", "on", "--session", "s1")
	p.want(0, "PROMOTE unit-added:MOD-0003 (relates to MOD-0001)", "status")
}

func TestBelowTheLineFeatureIsProposedOnceTwoDesignUnitsShareIt(t *testing.T) {
	p := newProject(t)
	feature := `{"key":"export","kind":"FEAT","name":"export","anchors":[{"path":"export/run.go","symbol":"Run"}]},`
	p.extract(withRelations(withUnit(topoV1, feature), `{"from":"export","to":"billing","kind":"depends_on"}`))
	p.want(0, "new", "ingest", p.extraction)
	p.want(0, "aligned", "align", "--confirm", "MOD-0001,MOD-0002,IF-0001")
	if out := p.want(0, "phase: aligned", "status"); strings.Contains(out, "PROMOTE") {
		t.Fatalf("a feature only one design unit touches is an implementation detail:\n%s", out)
	}

	p.editCode("export/run.go")
	p.extract(withRelations(withUnit(topoV1, feature), `{"from":"export","to":"billing","kind":"depends_on"},{"from":"export","to":"ledger","kind":"depends_on"}`))
	p.want(2, "drifted", "mode", "on", "--session", "s1")
	p.want(0, "PROMOTE unit-added:FEAT-0001 (relates to MOD-0001, MOD-0002)", "status")
}

func TestAdoptionLeavesAWaitingGradeCChangeWaiting(t *testing.T) {
	p := aligned(t)
	p.editCode("audit/log.go")
	p.extract(withRelations(withUnit(topoV1, auditUnit), `{"from":"audit","to":"api","kind":"consumes"}`))
	p.want(2, "BLOCKED new-relation:MOD-0003.consumes.IF-0001", "mode", "on", "--session", "s1")
	p.want(0, "adopted", "approve", "unit-added:MOD-0003")
	if strings.Contains(p.intended(), "kind: consumes") {
		t.Fatalf("adopting a unit must not approve its grade C coupling:\n%s", p.intended())
	}
	p.want(0, "BLOCKED new-relation:MOD-0003.consumes.IF-0001", "status")
}

func TestPromotionWaitsForAnUnfinishedTurn(t *testing.T) {
	p := activeTurnProject(t, topoV1)
	p.start("MOD-0001")
	p.editCode("audit/log.go")
	p.extract(withUnit(topoV1, auditUnit))
	p.want(0, "PROMOTE unit-added:MOD-0003", "turn", "diff", "--session", "s1")
	p.want(1, "once it ends", "approve", "unit-added:MOD-0003")
	p.want(1, "once it ends", "reject", "unit-added:MOD-0003", "--reason", "x")
	p.verify("pass", 0, "PASSED")
	p.want(0, "CHG-0001 SYNCED", "turn", "end", "--session", "s1")
	p.want(0, "adopted", "approve", "unit-added:MOD-0003")
}

func TestScreenAdoptsAndRejectsPromotionCandidates(t *testing.T) {
	p := aligned(t)
	p.editCode("audit/log.go")
	report := `{"key":"report","kind":"MOD","name":"report","anchors":[{"path":"report/pdf.go","symbol":"Render"}]},`
	p.extract(withUnit(withUnit(topoV1, auditUnit), report))
	p.want(0, "active", "mode", "on", "--session", "s1")
	s := p.screen()
	html := s.view(t)
	if !strings.Contains(html, "승격 후보") || !strings.Contains(html, `value="unit-added:MOD-0003"`) || !strings.Contains(html, "채택") {
		t.Fatalf("the screen should offer each candidate with adopt and reject:\n%s", html)
	}
	html = s.decide(t, "do", "approve", "key", "unit-added:MOD-0003")
	if !strings.Contains(html, "adopted unit-added:MOD-0003") || !strings.Contains(p.intended(), "id: MOD-0003") {
		t.Fatalf("adopting on the screen should make the unit a design unit:\n%s", html)
	}
	html = s.decide(t, "do", "reject", "key", "unit-added:MOD-0004", "reason", "a one-off report")
	if !strings.Contains(html, "rejected unit-added:MOD-0004") || strings.Contains(html, `value="unit-added:MOD-0004"`) {
		t.Fatalf("a rejected candidate leaves the screen:\n%s", html)
	}
}
