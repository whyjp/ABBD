package screen

import (
	"strings"
	"testing"
)

// A line says, under the pointer, the conditions its relations hold under,
// each with how many of them it gates, and graph.json keeps each one.
func TestALineSaysTheConditionsItsRelationsHoldUnder(t *testing.T) {
	units := ripgrep()
	units[0].Relations[0].When = `feature = "pcre2"`
	m := Build(Input{Units: units})
	found := false
	for _, w := range m.Overview.Wires {
		if strings.Contains(w.Title, "조건: feature = \"pcre2\" 1/") {
			found = true
		}
		if strings.Contains(w.Title, "조건:") && !strings.Contains(w.Title, "pcre2") {
			t.Errorf("a line with no conditional relation says one: %q", w.Title)
		}
	}
	if !found {
		t.Error("no overview line names the condition its relation holds under")
	}
	for _, l := range lines(units) {
		if l.From == string(units[0].ID) && l.To == string(units[0].Relations[0].To) && l.When != `feature = "pcre2"` {
			t.Errorf("graph.json line %+v lost its condition", l)
		}
	}
}

// A link scope says who sees the link, not when it holds, so the tooltip
// names it as the line's 범위 and never as a 조건.
func TestALineSaysALinkScopeAsItsScope(t *testing.T) {
	units := ripgrep()
	units[0].Relations[0].When = "PRIVATE"
	m := Build(Input{Units: units})
	found := false
	for _, w := range m.Overview.Wires {
		if strings.Contains(w.Title, "범위: PRIVATE 1/") {
			found = true
		}
		if strings.Contains(w.Title, "조건:") {
			t.Errorf("a link scope reads as a condition: %q", w.Title)
		}
	}
	if !found {
		t.Error("no overview line names its relation's link scope")
	}
}

// A bus that gathers several units' relations says how many of them a
// condition gates, so one conditional relation among three reads as 1/3,
// not as a condition of the whole bus.
func TestABusSaysHowManyOfItsRelationsAConditionGates(t *testing.T) {
	units := ripgrep()
	units[0].Relations[1].When = "cfg(unix)"
	m := Build(Input{Units: units})
	found := false
	for _, w := range m.Overview.Wires {
		if strings.Contains(w.Title, "조건: cfg(unix) 1/3") {
			found = true
		}
		if strings.Contains(w.Title, "조건: cfg(unix) 3/3") {
			t.Errorf("a bus says its one conditional relation gates all three: %q", w.Title)
		}
	}
	if !found {
		t.Error("no bus into grep-matcher says the condition gates 1 of its 3 relations")
	}
}
