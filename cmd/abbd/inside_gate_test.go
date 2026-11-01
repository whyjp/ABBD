package main

import (
	"strings"
	"testing"
)

// An inside unit of ledger that consumes billing's intended contract is a
// boundary-crossing coupling; without the parent it is BLOCKED.
func TestAnInsideUnitsCouplingToAnotherModuleGates(t *testing.T) {
	probe := `{"key":"probe","kind":"MOD","name":"probe","parent":"ledger","anchors":[{"path":"ledger/probe.go","symbol":"Probe"}]},`
	rels := `{"from":"billing","to":"api","kind":"provides"},{"from":"probe","to":"api","kind":"consumes"},{"from":"probe","to":"billing","kind":"depends_on"}`
	for _, parented := range []bool{false, true} {
		name := "top-level"
		if parented {
			name = "inside-ledger"
		}
		t.Run(name, func(t *testing.T) {
			p := aligned(t)
			p.editCode("ledger/probe.go")
			u := probe
			if !parented {
				u = strings.Replace(u, `"parent":"ledger",`, "", 1)
			}
			v2 := strings.Replace(topoV1, `"units":[`, `"units":[`+u, 1)
			v2 = strings.Replace(v2, `{"from":"billing","to":"api","kind":"provides"}`, rels, 1)
			p.extract(v2)
			code, out := p.cmd("", "mode", "on", "--session", "s1")
			t.Logf("exit %d:\n%s", code, out)
			if code != 2 || !strings.Contains(out, "BLOCKED new-relation:MOD-0003.consumes.IF-0001") {
				t.Errorf("a new consumer of billing's contract must gate wherever it sits")
			}
			if !strings.Contains(out, "new-relation:MOD-0003.depends_on.MOD-0001") {
				t.Errorf("a dependency from ledger's inside onto billing crosses the boundary and must surface")
			}
		})
	}
}

// Giving the design module ledger a parent (billing) makes its new
// consumption of a contract inside billing vanish from the gate.
func TestAParentOnADesignUnitSilencesNothing(t *testing.T) {
	for _, reparent := range []bool{false, true} {
		name := "ledger-top-level"
		if reparent {
			name = "ledger-reparented-under-billing"
		}
		t.Run(name, func(t *testing.T) {
			p := aligned(t)
			p.editCode("billing/table.go")
			v2 := strings.Replace(topoV1, `"units":[`,
				`"units":[{"key":"table","kind":"DATA","name":"charges table","parent":"billing","anchors":[{"path":"billing/table.go","symbol":"Table"}]},`, 1)
			if reparent {
				v2 = strings.Replace(v2, `{"key":"ledger","kind":"MOD",`, `{"key":"ledger","parent":"billing","kind":"MOD",`, 1)
			}
			v2 = strings.Replace(v2, `{"from":"billing","to":"api","kind":"provides"}`,
				`{"from":"billing","to":"api","kind":"provides"},{"from":"ledger","to":"table","kind":"consumes"}`, 1)
			p.extract(v2)
			code, out := p.cmd("", "mode", "on", "--session", "s1")
			t.Logf("exit %d:\n%s", code, out)
			if code != 2 || !strings.Contains(out, "BLOCKED new-relation:MOD-0002.consumes.DATA-0001") {
				t.Errorf("ledger consuming a contract inside billing crosses the boundary and must gate")
			}
		})
	}
}

// An inside unit whose parent is a brand-new module, or a parent cycle,
// still hides a new consumer of an intended contract.
func TestParentCyclesAreRefused(t *testing.T) {
	p := aligned(t)
	p.editCode("shim/a.go")
	v2 := strings.Replace(topoV1, `"units":[`, `"units":[`+
		`{"key":"a","kind":"MOD","name":"a","parent":"b","anchors":[{"path":"shim/a.go","symbol":"A"}]},`+
		`{"key":"b","kind":"MOD","name":"b","parent":"a","anchors":[{"path":"shim/b.go","symbol":"B"}]},`, 1)
	v2 = strings.Replace(v2, `{"from":"billing","to":"api","kind":"provides"}`,
		`{"from":"billing","to":"api","kind":"provides"},{"from":"a","to":"api","kind":"consumes"}`, 1)
	p.extract(v2)
	code, out := p.cmd("", "mode", "on", "--session", "s1")
	t.Logf("exit %d:\n%s", code, out)
	if code == 0 || !strings.Contains(out, "cycle") {
		t.Errorf("a parent cycle must be refused by name, exit %d:\n%s", code, out)
	}
}

// An agent may store how's text mid-turn, but not restamp a stale text onto
// newer code, and the record says an agent stored it.
func TestStaleUnderstandingCannotBeRestamped(t *testing.T) {
	tp := activeTurnProject(t, topoV1)
	p := tp.project
	p.wantDenied(t, false, p.bash("abbd understand MOD-0001 how.md"))
	text := p.file(t, "how.md", "### Overview\nbilling charges cards.\n")
	p.want(0, "understood MOD-0001", "understand", "MOD-0001", text)
	p.code("billing/charge.go", "package billing\n\nfunc Charge(currency string) {}\n")
	if !strings.Contains(p.screen().view(t), `title="이 설명을 쓴 뒤`) {
		t.Fatalf("precondition: the text should be STALE after the code moved")
	}
	p.want(1, "run pstack how on it again", "understand", "MOD-0001", text)
	tp.start("MOD-0001")
	if !strings.Contains(p.screen().view(t), `title="이 설명을 쓴 뒤`) {
		t.Errorf("re-running understand with the same old text marks it fresh again")
	}
	fresh := p.file(t, "how2.md", "### Overview\nbilling charges cards in a currency.\n")
	p.want(0, "understood MOD-0001", "understand", "MOD-0001", fresh)
	if u := p.read("understanding/MOD-0001.md"); !strings.Contains(u, "by: agent in CHG-") {
		t.Errorf("text stored during a turn must say an agent stored it:\n%s", u)
	}
}
