package screen

import (
	"slices"
	"strings"
	"testing"
)

// In a fanning view, folded lines of one kind and dotting name as their hub
// the end they share with another such line; a folded line that shares no
// end, and any line that does not fan, names none.
func TestFoldHubsNameTheSharedEnd(t *testing.T) {
	req := func(kind string, fan bool) wireReq { return wireReq{kind: kind, both: fan, fan: fan} }
	reqs := []wireReq{req("depends_on", true), req("depends_on", true), req("depends_on", true), req("depends_on", false), req("consumes", true)}
	from := []string{"h", "a", "x", "h", "h"}
	to := []string{"a2", "h", "y", "b", "c"}
	got := foldHubs(reqs, from, to)
	want := []string{"h", "h", "", "", ""}
	if !slices.Equal(got, want) {
		t.Errorf("hubs %q, want %q", got, want)
	}
}

// Two folded lines of one hub's fan may share their end runs at the hub,
// and no other end run.
func TestFanAtSharesOnlyTheHub(t *testing.T) {
	v := roadView{both: []bool{true, true, false}, hub: []string{"h", "h", ""}, from: []string{"h", "h", "h"}, to: []string{"a", "b", "c"}}
	ways := [][]pt{{{0, 0}, {0, 10}, {10, 10}}, {{0, 0}, {0, 10}, {20, 10}}, {{0, 0}, {0, 10}, {30, 10}}}
	if !v.fanAt(ways, [2]int{0, 0}, [2]int{1, 0}) {
		t.Error("two lines of one fan may not share their runs at the hub")
	}
	if v.fanAt(ways, [2]int{0, 1}, [2]int{1, 1}) {
		t.Error("two lines of one fan share the runs at their far ends")
	}
	if v.fanAt(ways, [2]int{0, 0}, [2]int{2, 0}) {
		t.Error("a folded line shares the hub's run with a line that is not folded")
	}
	// mix keeps two lines of a fan apart at the hub where their kinds or
	// dottings differ, and lets them meet there where they agree.
	box := func(id string) end { return end{node: id, key: id} }
	v.kinds, v.plan = []string{"depends_on", "depends_on", "depends_on"}, []bool{false, false, false}
	v.ends = [][2]end{{box("h"), box("a")}, {box("h"), box("b")}, {box("h"), box("c")}}
	if v.mix(ways, [2]int{0, 0}, [2]int{1, 0}) {
		t.Error("two lines of one fan may not meet at the hub")
	}
	v.kinds[1] = "consumes"
	if !v.mix(ways, [2]int{0, 0}, [2]int{1, 0}) {
		t.Error("two folded lines of two kinds meet at the hub")
	}
	v.kinds[1] = "depends_on"
	if !v.mix(ways, [2]int{0, 0}, [2]int{2, 0}) {
		t.Error("a folded line meets a line that reads one way at the hub")
	}
}

// A near tactic seats the boxes on both sides of the focus in the ring row
// next to it, below or above the focus, and keeps brick's rows where no box
// is mutual or the mutual ones alone pass the room.
func TestNearFirstSeatsMutualBoxesNextToTheFocus(t *testing.T) {
	byID := map[string]lnode{}
	ids := []string{"a", "b", "c", "d", "e", "f"}
	for _, id := range ids {
		byID[id] = lnode{id: id, w: 100}
	}
	room := rowWidth(ids[:3], byID)
	rs := brick(ids, byID, room)
	mutual := func(id string) bool { return id == "b" || id == "e" }
	below := nearFirst(ids, rs, byID, room, false, mutual)
	if !slices.Equal(below[0], []string{"b", "e"}) {
		t.Errorf("below the focus the first row is %v, want b and e", below[0])
	}
	above := nearFirst(ids, rs, byID, room, true, mutual)
	if !slices.Equal(above[len(above)-1], []string{"b", "e"}) {
		t.Errorf("above the focus the last row is %v, want b and e", above[len(above)-1])
	}
	if got := nearFirst(ids, rs, byID, room, false, func(string) bool { return false }); !slices.EqualFunc(got, rs, slices.Equal) {
		t.Errorf("with no mutual box the rows are %v, want brick's %v", got, rs)
	}
	if got := nearFirst(ids, rs, byID, rowWidth(ids[:1], byID), false, mutual); !slices.EqualFunc(got, rs, slices.Equal) {
		t.Errorf("mutual boxes too wide for one row moved: %v", got)
	}
}

// Where orca's inside focus views fan folded lines into one hub, only
// folded pairs ride the shared ink: every relation of a joined folded wire
// has its reverse there too.
func TestAFanCarriesOnlyFoldedPairs(t *testing.T) {
	m := fixture(t, "orca")
	fans := 0
	for _, ins := range m.Insides {
		for _, f := range ins.Focus {
			// Every path that carries one relation of a folded pair along
			// with other lines, a fan's stem or a branch, carries whole
			// folded pairs only: no line that reads one way joins a fan.
			for _, w := range f.Drawing.Wires {
				paired := slices.ContainsFunc(w.Rels, func(r string) bool {
					p := strings.Split(r, ".")
					return len(p) == 3 && slices.Contains(w.Rels, p[2]+"."+p[1]+"."+p[0])
				})
				if !paired || len(w.Rels) < 3 {
					continue
				}
				fans++
				for _, r := range w.Rels {
					p := strings.Split(r, ".")
					if len(p) != 3 || !slices.Contains(w.Rels, p[2]+"."+p[1]+"."+p[0]) {
						t.Errorf("%s: %q rides a fan without its reverse", f.ID, r)
					}
				}
			}
		}
	}
	if fans == 0 {
		t.Fatal("no inside focus view of orca draws a fan")
	}
}
