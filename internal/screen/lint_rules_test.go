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

// A line that rides right past the x of its drop, steps down a lane, and
// comes back left turns back (B3) once. Its two lanes run against each
// other, so they are that turn-back and not a doubled track (B4). The same
// line stepping down above its drop does neither.
func TestALineThatRidesPastItsTurnTurnsBack(t *testing.T) {
	boxes := []string{"A 40 0 60 20", "B 80 160 60 20"}
	rels := [][2]string{{"A", "B"}}
	past := lookAt(t, boxes, `<path class="w contract" d="M60 20 V80 H200 V88 H110 V160"/>`, rels)
	if got := past.noTurnBacks(); len(got) != 1 || got[0].Got != 1 || !strings.Contains(got[0].Text, "riding 90px") {
		t.Fatalf("%+v; want one turn back, riding 90px past", got)
	}
	if got := past.noDoubles(); len(got) != 0 {
		t.Fatalf("%+v; want the turn-back's two lanes left to B3", got)
	}
	above := lookAt(t, boxes, `<path class="w contract" d="M60 20 V80 H110 V160"/>`, rels)
	if got := append(above.noTurnBacks(), above.noDoubles()...); len(got) != 0 {
		t.Fatalf("%+v; want no detour", got)
	}
}

// Two subpaths of one drawn bundle that run the same way 8px apart for
// 40px read as one line drawn twice (B4). Run against each other they do
// not: that is a turn-back's shape, which B3 counts on a line.
func TestABundleBesideItselfIsDoubled(t *testing.T) {
	boxes := []string{"A 40 0 60 20", "B 40 160 60 20"}
	same := lookAt(t, boxes, `<path class="w contract" d="M150 40 H190 M150 48 H190"/>`, nil)
	if got := same.noDoubles(); len(got) != 1 || got[0].Got != 1 || !strings.Contains(got[0].Text, "bundle") {
		t.Fatalf("%+v; want the bundle beside itself once", got)
	}
	against := lookAt(t, boxes, `<path class="w contract" d="M150 40 H190 M190 48 H150"/>`, nil)
	if got := against.noDoubles(); len(got) != 0 {
		t.Fatalf("%+v; want subpaths that run against each other left alone", got)
	}
}

// A line that climbs from a box to one above it leaves downward, runs
// round through the gutter and comes down into its end, so it turns back
// by its nature. Rule 13 counts it as a back line, and B3 leaves it out.
func TestAClimbIsNotATurnBack(t *testing.T) {
	boxes := []string{"A 40 0 60 20", "B 40 160 60 20"}
	l := lookAt(t, boxes, `<path class="w contract" d="M70 180 V200 H10 V-20 H70 V0"/>`, [][2]string{{"B", "A"}})
	if len(l.trace.Ways) == 0 || !backWay(l.trace.Ways[0].pts) {
		t.Fatalf("%+v; want the climb traced as a back line", l.trace.Ways)
	}
	if got := l.noTurnBacks(); len(got) != 0 {
		t.Fatalf("%+v; want no turn-back on a climb", got)
	}
}

// Two lines that share one turn-back, as the lines of one trunk do, count
// it once.
func TestASharedTurnBackCountsOnce(t *testing.T) {
	boxes := []string{"A 40 0 60 20", "B 80 160 40 20", "C 160 160 40 20"}
	l := lookAt(t, boxes, `<path class="w contract" data-net="1" d="M60 20 V80 H240 V88 H100 V160"/><path class="w contract" data-net="1" d="M180 88 V160"/>`, [][2]string{{"A", "B"}, {"A", "C"}})
	if len(l.trace.Ways) != 2 {
		t.Fatalf("%+v; want both lines traced", l.trace.Ways)
	}
	n := 0
	for _, b := range l.noTurnBacks() {
		n += b.Got
	}
	if n != 1 {
		t.Fatalf("%d turn-backs; want the shared one counted once", n)
	}
}

// Lines from one box that leave by one stem and fork off one flat track
// into the row below read as one line splitting, so a branch into a box in
// the box's own column may turn twice. The same bent line alone, on no
// fork, still breaks B1.
func TestAForkIntoTheRowBelowIsNoBend(t *testing.T) {
	boxes := []string{"A 100 0 160 20", "B 60 80 80 20", "C 200 80 80 20", "D 320 80 80 20"}
	toB := `<path class="w contract" d="M230 20 V50 H100 V80"/>`
	fork := toB + `<path class="w contract" d="M230 20 V80"/><path class="w contract" d="M230 20 V50 H360 V80"/>`
	rels := [][2]string{{"A", "B"}, {"A", "C"}, {"A", "D"}}
	if got := lookAt(t, boxes, fork, rels).bendsAllowed(); len(got) != 0 {
		t.Fatalf("%+v; want the fork's branch into B allowed its 2 turns", got)
	}
	got := lookAt(t, boxes, toB, [][2]string{{"A", "B"}}).bendsAllowed()
	if len(got) != 1 || got[0].Got != 2 || got[0].Allowed != 0 {
		t.Fatalf("%+v; want the lone bent line to break B1, 2 turns allowed 0", got)
	}
}

// A stem that stands over no box of its own, whose two lines both bend onto
// a track another box's line arrives along, is a knot, not a fork: both
// bent lines still break B1. So does a fork whose track another place's
// line runs along.
func TestAKnotOnAnotherBoxsTrackStillBends(t *testing.T) {
	boxes := []string{"A 100 0 160 20", "E 300 0 120 20", "B 60 80 80 20", "C 200 80 80 20"}
	knot := `<path class="w contract" d="M340 20 V50 H110 V80"/>` +
		`<path class="w contract" d="M180 20 V50 H110 V80"/><path class="w contract" d="M180 20 V50 H240 V80"/>`
	got := lookAt(t, boxes, knot, [][2]string{{"E", "B"}, {"A", "B"}, {"A", "C"}}).bendsAllowed()
	if len(got) != 2 {
		t.Fatalf("%+v; want both of A's bent lines to break B1", got)
	}

	boxes = []string{"A 100 0 160 20", "E 420 0 60 20", "B 60 80 80 20", "C 200 80 80 20", "D 320 80 80 20"}
	fork := `<path class="w contract" d="M230 20 V50 H100 V80"/><path class="w contract" d="M230 20 V80"/>` +
		`<path class="w contract" d="M230 20 V50 H360 V80"/><path class="w depends_on" d="M450 20 V50 H90 V80"/>`
	got = lookAt(t, boxes, fork, [][2]string{{"A", "B"}, {"A", "C"}, {"A", "D"}, {"E", "B"}}).bendsAllowed()
	if len(got) != 1 || got[0].Items[1].ID != "B" || got[0].Allowed != 0 {
		t.Fatalf("%+v; want the branch into B, on E's track, to break B1", got)
	}
}
