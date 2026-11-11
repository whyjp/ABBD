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
