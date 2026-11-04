package screen

import (
	"fmt"
	"strings"
	"testing"

	"github.com/whyjp/abbd/internal/store"
)

func lookAt(t *testing.T, boxes []string, body string, rels [][2]string) *look {
	t.Helper()
	return newLook(view{name: "sketch", d: Drawing{Relations: rels}}, sketch(t, boxes, body))
}

// A line that steps aside within one column breaks B1 with no obstacle to
// excuse it.
func TestAStepWithinAColumnHasNoExcuse(t *testing.T) {
	l := lookAt(t, []string{"A 40 0 60 20", "B 40 160 60 20"},
		`<path class="w contract" d="M60 20 V80 H80 V160"/>`, [][2]string{{"A", "B"}})
	got := l.bendsAllowed()
	if len(got) != 1 || got[0].Got != 2 || got[0].Allowed != 0 || !strings.Contains(got[0].Why, "drop is clear") {
		t.Fatalf("%+v; want one breach of 2 turns, allowed 0, the drop clear", got)
	}
}

// A box that blocks the straight drop between two boxes of one column
// allows a line 2 turns, as rule 4's exit bundle runs, and the breach names
// it when the line turns more.
func TestAnObstacleIsNamed(t *testing.T) {
	boxes := []string{"A 40 0 60 20", "C 70 70 40 30", "B 80 160 60 20"}
	aside := `<path class="w contract" d="M50 20 V140 H120 V160"/>`
	if got := lookAt(t, boxes, aside, [][2]string{{"A", "B"}}).bendsAllowed(); len(got) != 0 {
		t.Fatalf("%+v; want the 2 turns past C allowed", got)
	}
	around := `<path class="w contract" d="M90 20 V50 H150 V130 H100 V160"/>`
	got := lookAt(t, boxes, around, [][2]string{{"A", "B"}}).bendsAllowed()
	if len(got) != 1 || got[0].Allowed != 2 || !strings.Contains(got[0].Why, "(C)") {
		t.Fatalf("%+v; want one breach, allowed 2, around C", got)
	}
}

// The allowance reads where the two ends stand, not the route, so a line
// that leaves and reaches on the sides of two boxes in one clear column
// buys no turns by it.
func TestTheRouteBuysNoAllowance(t *testing.T) {
	l := lookAt(t, []string{"A 40 0 60 20", "B 40 160 60 20"},
		`<path class="w contract" d="M100 10 H120 V170 H100"/>`, [][2]string{{"A", "B"}})
	got := l.bendsAllowed()
	if len(got) != 1 || got[0].Got != 2 || got[0].Allowed != 0 {
		t.Fatalf("%+v; want one breach of 2 turns, allowed 0", got)
	}
}

// A dot where one strand from A splits toward B and D holds J1 only in
// the gap next to their row, not next to A's.
func TestASplitStandsNextToItsDestinations(t *testing.T) {
	boxes := []string{"A 80 0 40 20", "C 0 60 40 20", "B 40 140 40 20", "D 120 140 40 20"}
	rels := [][2]string{{"A", "B"}, {"A", "D"}}
	split := func(y int) []Breach {
		body := fmt.Sprintf(`<path class="w contract" d="M100 20 V%[1]d H60 V140"/><path class="w contract" d="M100 %[1]d H140 V140"/><circle class="junction contract" cx="100" cy="%[1]d" r="3"/>`, y)
		return lookAt(t, boxes, body, rels).splitsAtEnds()
	}
	if got := split(40); len(got) != 1 || got[0].Got != 1 || len(got[0].Items) != 2 || got[0].Items[0].ID != "B" {
		t.Fatalf("%+v; want the split under A reported 1 row from B and D", got)
	}
	if got := split(110); len(got) != 0 {
		t.Fatalf("%+v; want the split above B and D allowed", got)
	}
}

// A hub's dashed lines that climb to it are rule 13's, so H2 counts only
// the bundles of its forward lines.
func TestClimbingLinesAreNotAHubsTrunk(t *testing.T) {
	boxes := []string{"U 0 0 40 20", "V 60 0 40 20", "W 120 0 40 20", "H 60 100 40 20"}
	rels := [][2]string{{"U", "H"}, {"V", "H"}, {"W", "H"}}
	body := func(third string) string {
		return `<path class="w contract" data-net="1" d="M20 20 V60 H70 V100"/><path class="w contract" data-net="1" d="M80 20 V100"/>` +
			`<path class="w contract` + third + `" data-net="2" d="M140 20 V140 H90 V120"/>`
	}
	if got := lookAt(t, boxes, body(" back"), rels).hubsOneTrunk(); len(got) != 0 {
		t.Fatalf("%+v; want the climbing line left out", got)
	}
	if got := lookAt(t, boxes, body(""), rels).hubsOneTrunk(); len(got) != 1 || got[0].Got != 2 {
		t.Fatalf("%+v; want two bundles of forward lines reported", got)
	}
}

// The placement debt may pass its ceiling by 2%, the weighed sum not at
// all, and a hard breach only within its view's debt.
func TestTheGate(t *testing.T) {
	views := []ViewLint{{View: "v", Breaches: []Breach{{Criterion: "R", Hard: true}}, Totals: Totals{Criteria: map[string]int{"R": 1}, Placement: 102, Weighed: 300}}}
	debt := store.LintDebt{Ceilings: map[string]int{"placement": 100, "weighed": 300}, Views: map[string]map[string]int{"v": {"R": 1}}}
	if got := Gate(views, debt, true); len(got) != 0 || !views[0].Breaches[0].Debt {
		t.Fatalf("%v, debt %v; want the gate kept and the breach marked debt", got, views[0].Breaches[0].Debt)
	}
	views[0].Totals.Placement, views[0].Totals.Weighed = 103, 301
	if got := Gate(views, debt, true); len(got) != 2 {
		t.Fatalf("%v; want the placement debt and the weighed sum past their ceilings", got)
	}
	debt.Views = nil
	if got := Gate(views, debt, false); len(got) != 1 || views[0].Breaches[0].Debt {
		t.Fatalf("%v; want the unrecorded hard breach alone", got)
	}
}

// A chip that stands past the box its line reaches breaks F1, by how far.
func TestAChipPastItsBoxIsReported(t *testing.T) {
	s := sketch(t, []string{"B 40 100 60 20"},
		`<rect class="frame" x="0" y="0" width="200" height="200"/>`+
			`<g class="port provided" data-id="port|x"><rect x="130" y="0" width="40" height="17"/><text class="chip">x</text></g>`+
			`<path class="w contract" d="M150 17 V60 H70 V100"/>`)
	l := newLook(view{name: "sketch", d: Drawing{Relations: [][2]string{{"port|x", "B"}}}}, s)
	got := l.chipsOverTheirBoxes()
	if len(got) != 1 || got[0].Got != 30 {
		t.Fatalf("%+v; want the chip 30px right of its box", got)
	}
}

// A line that climbs to a box that never reaches it back breaks O3.
func TestAClimbOutsideACycleIsHard(t *testing.T) {
	boxes := []string{"A 40 0 60 20", "B 40 160 60 20"}
	if got := lookAt(t, boxes, ``, [][2]string{{"B", "A"}}).climbsCloseCycles(); len(got) != 1 {
		t.Fatalf("%+v; want the climb from B to A reported", got)
	}
	if got := lookAt(t, boxes, ``, [][2]string{{"B", "A"}, {"A", "B"}}).climbsCloseCycles(); len(got) != 0 {
		t.Fatalf("%+v; want the climb that closes a cycle allowed", got)
	}
}
