package screen

import (
	"strings"
	"testing"
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
	if len(got) != 1 || got[0].Got != 2 || got[0].Allowed != 0 || !strings.Contains(got[0].Why, "no obstacle") {
		t.Fatalf("%+v; want one breach of 2 turns, allowed 0, no obstacle", got)
	}
}

// A box that blocks the column between two boxes allows a line four more
// turns, and the breach names it when the line turns more still.
func TestAnObstacleIsNamed(t *testing.T) {
	boxes := []string{"A 40 0 60 20", "C 20 70 100 30", "B 40 160 60 20"}
	around := `<path class="w contract" d="M60 20 V50 H140 V130 H60 V160"/>`
	if got := lookAt(t, boxes, around, [][2]string{{"A", "B"}}).bendsAllowed(); len(got) != 0 {
		t.Fatalf("%+v; want the 4 turns around C allowed", got)
	}
	weave := `<path class="w contract" d="M60 20 V50 H140 V110 H150 V130 H60 V160"/>`
	got := lookAt(t, boxes, weave, [][2]string{{"A", "B"}}).bendsAllowed()
	if len(got) != 1 || got[0].Allowed != 4 || !strings.Contains(got[0].Why, "(C)") {
		t.Fatalf("%+v; want one breach, allowed 4, around C", got)
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
