package main

import (
	"strings"
	"testing"
)

// ledger is gone and billing now depends on a new audit module: two REALIGN rows.
var twoRealignRows = withRelations(strings.Replace(withUnit(topoV1, auditUnit),
	` {"key":"ledger","kind":"MOD","name":"ledger","anchors":[{"path":"ledger/book.go","symbol":"Post"}]},
`, "", 1), `{"from":"billing","to":"audit","kind":"depends_on"}`)

func TestShortRealignAcknowledgesOnlyTheChosenRows(t *testing.T) {
	p := aligned(t)
	p.editCode("audit/log.go")
	p.extract(twoRealignRows)
	out := p.want(2, "REALIGN unit-removed:MOD-0002", "mode", "on", "--session", "s1")
	if !strings.Contains(out, "REALIGN new-relation:MOD-0001.depends_on.MOD-0003") {
		t.Fatalf("both rows should need realignment:\n%s", out)
	}

	p.want(1, "not a REALIGN row", "align", "--confirm", "unit-added:MOD-0003")
	p.want(1, "not a REALIGN row", "align", "--confirm", "unit-removed:IF-0001")
	p.want(0, "aligned: 2 design unit(s); 1 REALIGN row(s) left", "align", "--confirm", "unit-removed:MOD-0002")
	in := p.intended()
	if strings.Contains(in, "MOD-0002") || strings.Contains(in, "MOD-0003") {
		t.Fatalf("only the chosen row should enter the intended topology:\n%s", in)
	}
	out = p.want(2, "REALIGN new-relation:MOD-0001.depends_on.MOD-0003", "mode", "on", "--session", "s1")
	if strings.Contains(out, "unit-removed:MOD-0002") {
		t.Fatalf("an acknowledged row is gone:\n%s", out)
	}
	p.want(0, "aligned: 2 design unit(s)", "align", "--confirm", "new-relation:MOD-0001.depends_on.MOD-0003")
	p.want(0, "active", "mode", "on", "--session", "s1")
	p.want(1, "choose the first design units only", "align", "--exclude", "MOD-0001")
}

func TestChosenRealignStillWaitsForBlockedChanges(t *testing.T) {
	p := aligned(t)
	p.editCode("audit/log.go")
	p.extract(withRelations(twoRealignRows, `{"from":"audit","to":"api","kind":"consumes"}`))
	p.want(2, "BLOCKED new-relation:MOD-0003.consumes.IF-0001", "mode", "on", "--session", "s1")
	before := p.intended()
	p.want(2, "pending approval", "align", "--confirm", "unit-removed:MOD-0002")
	if p.intended() != before {
		t.Fatalf("realignment must wait for the blocked change")
	}
}

func TestScreenRealignsTheRowsTheHumanTicks(t *testing.T) {
	p := aligned(t)
	p.editCode("audit/log.go")
	p.extract(twoRealignRows)
	p.want(2, "drifted", "mode", "on", "--session", "s1")
	s := p.screen()
	html := s.view(t)
	if !strings.Contains(html, `name="key" value="unit-removed:MOD-0002" checked`) || !strings.Contains(html, `name="key" value="new-relation:MOD-0001.depends_on.MOD-0003" checked`) {
		t.Fatalf("each REALIGN row should have its own tick:\n%s", html)
	}
	if out := s.decide(t, "do", "realign"); !strings.Contains(out, "고르세요") {
		t.Fatalf("realigning nothing is refused, never read as all:\n%s", out)
	}
	html = s.decide(t, "do", "realign", "key", "unit-removed:MOD-0002")
	if !strings.Contains(html, "1 REALIGN row(s) left") || strings.Contains(p.intended(), "MOD-0002") {
		t.Fatalf("the screen should realign the ticked row only:\n%s", html)
	}
}

func TestScreenHidesRealignWhileAChangeIsBlocked(t *testing.T) {
	p := aligned(t)
	p.editCode("audit/log.go")
	p.extract(withRelations(twoRealignRows, `{"from":"audit","to":"api","kind":"consumes"}`))
	p.want(2, "BLOCKED", "mode", "on", "--session", "s1")
	html := p.screen().view(t)
	if strings.Contains(html, `name="do" value="realign"`) || !strings.Contains(html, "C 변경을 먼저 결정") {
		t.Fatalf("the short realignment waits for the blocked change:\n%s", html)
	}
}

func TestAnEmptyChoiceAcknowledgesNoRealignRow(t *testing.T) {
	p := aligned(t)
	p.editCode("audit/log.go")
	p.extract(twoRealignRows)
	p.want(2, "drifted", "mode", "on", "--session", "s1")
	before := p.intended()
	for _, choice := range []string{",", " , ", ""} {
		p.want(1, "name at least one REALIGN row", "align", "--confirm="+choice)
	}
	s := p.screen()
	if out := s.decide(t, "do", "realign", "key", ""); !strings.Contains(out, "name at least one REALIGN row") {
		t.Fatalf("an empty key from the screen must acknowledge nothing:\n%s", out)
	}
	if p.intended() != before {
		t.Fatalf("an empty choice changed the intended topology:\n%s", p.intended())
	}
	p.want(0, "aligned: 2 design unit(s)", "align")
}
