package model

import (
	"slices"
	"testing"
)

// The draft offers one line per pair of boxes a live relation joins, a use
// of a contract drawn as a chip landing on its provider's box, ranked by
// what the relations carry: runs before wires before imports, then by how
// many relations of that rank, then by how many in all.
func TestLineCandidatesRankWhatTheRelationsCarry(t *testing.T) {
	t.Parallel()
	ev := []Evidence{{Path: "a.go", Lines: "1", Quote: "b.Do()"}}
	rel := func(to ID, k RelationKind, via Via) Relation {
		return Relation{To: to, Kind: k, Via: via, Evidence: ev}
	}
	units := []Unit{
		{ID: "MOD-0001", Kind: Module, Relations: []Relation{rel("MOD-0002", DependsOn, "call"), rel("MOD-0003", DependsOn, "import"), rel("MOD-0003", DependsOn, "config"), rel("IF-0001", Consumes, "call")}},
		{ID: "MOD-0002", Kind: Module, Relations: []Relation{rel("IF-0001", Provides, "")}},
		{ID: "MOD-0003", Kind: Module},
		{ID: "MOD-0004", Kind: Module, Relations: []Relation{rel("IF-0001", Consumes, "data"), rel("IF-0002", Consumes, "import")}},
		{ID: "MOD-0005", Kind: Module, Parent: "MOD-0003", Relations: []Relation{rel("MOD-0001", DependsOn, "import"), {To: "MOD-0002", Kind: DependsOn, Via: "call", Dead: true}}},
		{ID: "IF-0001", Kind: Interface},
		{ID: "IF-0002", Kind: Interface},
	}
	if got, want := Boxes(units), []ID{"IF-0002", "MOD-0001", "MOD-0002", "MOD-0003", "MOD-0004"}; !slices.Equal(got, want) {
		t.Errorf("the boxes are the top-level units but a contract a unit provides, a chip: got %v, want %v", got, want)
	}
	var got [][3]string
	for _, c := range LineCandidates(units) {
		got = append(got, [3]string{string(c.From), string(c.To), c.Carry().String()})
	}
	want := [][3]string{
		{"MOD-0001", "MOD-0002", "runs"},
		{"MOD-0004", "MOD-0002", "runs"},
		{"MOD-0001", "MOD-0003", "wires"},
		{"MOD-0003", "MOD-0001", "imports"},
		{"MOD-0004", "IF-0002", "imports"},
	}
	if !slices.Equal(got, want) {
		t.Errorf("candidates:\n got %v\nwant %v", got, want)
	}
	top := LineCandidates(units)[0]
	if top.Relations() != 2 || top.Relations(Runs) != 2 || top.Quote.Quote != "b.Do()" {
		t.Errorf("the first candidate counts the call and the use of the contract its end provides, and quotes one: %+v", top)
	}
	if gaps := (Line{From: "MOD-0004", To: "MOD-0002", Word: "x", Evidence: ev}).Gaps(units); slices.ContainsFunc(gaps, func(g string) bool { return len(g) > 10 && g[:10] == "no live re" }) {
		t.Errorf("a line from a contract's user to its provider draws the use: %v", gaps)
	}
}

// The overview takes a contract's providers out of its users, and counts
// providers and users in module and feature boxes only, so the draft offers
// no line the overview would not draw.
func TestLineCandidatesMatchWhatTheOverviewDraws(t *testing.T) {
	t.Parallel()
	ev := []Evidence{{Path: "a.go", Lines: "1", Quote: "b.Do()"}}
	rel := func(to ID, k RelationKind, via Via) Relation {
		return Relation{To: to, Kind: k, Via: via, Evidence: ev}
	}
	units := []Unit{
		{ID: "MOD-0001", Kind: Module, Relations: []Relation{rel("DATA-0001", Provides, ""), rel("DATA-0001", Consumes, "data")}},
		{ID: "MOD-0002", Kind: Module, Relations: []Relation{rel("DATA-0001", Provides, "")}},
		{ID: "EXEC-0001", Kind: Exec, Relations: []Relation{rel("DATA-0001", Consumes, "data"), rel("DATA-0002", Provides, "")}},
		{ID: "MOD-0003", Kind: Module, Relations: []Relation{rel("DATA-0002", Consumes, "data")}},
		{ID: "DATA-0001", Kind: Data},
		{ID: "DATA-0002", Kind: Data},
	}
	if got, want := Boxes(units), []ID{"DATA-0002", "MOD-0001", "MOD-0002", "MOD-0003"}; !slices.Equal(got, want) {
		t.Errorf("the boxes are the modules, features and contracts no module provides, not an execution context: got %v, want %v", got, want)
	}
	got := LineCandidates(units)
	if len(got) != 1 || got[0].From != "MOD-0003" || got[0].To != "DATA-0002" {
		t.Errorf("only the use of the contract no module provides draws a line, not a user that provides the contract nor an execution context: %+v", got)
	}
	for _, l := range []Line{{From: "MOD-0001", To: "MOD-0002"}, {From: "EXEC-0001", To: "MOD-0001"}} {
		l.Word, l.Evidence = "x", ev
		if gaps := l.Gaps(units); !slices.ContainsFunc(gaps, func(g string) bool { return len(g) > 10 && g[:10] == "no live re" }) {
			t.Errorf("line %s to %s carries nothing the overview draws: %v", l.From, l.To, gaps)
		}
	}
}

// A unit two levels inside a module is drawn in the outermost box, so the
// draft offers its relation as a line from that box, and names the unit it
// runs from among the candidate's ends.
func TestADeepUnitIsDrawnInItsOutermostBox(t *testing.T) {
	t.Parallel()
	units := []Unit{
		{ID: "MOD-0001", Kind: Module},
		{ID: "MOD-0002", Kind: Module, Parent: "MOD-0001"},
		{ID: "MOD-0003", Kind: Module, Parent: "MOD-0002", Relations: []Relation{{To: "MOD-0004", Kind: DependsOn, Via: "call"}}},
		{ID: "MOD-0004", Kind: Module},
	}
	if got, want := Boxes(units), []ID{"MOD-0001", "MOD-0004"}; !slices.Equal(got, want) {
		t.Errorf("boxes: got %v, want %v", got, want)
	}
	got := LineCandidates(units)
	if len(got) != 1 || got[0].From != "MOD-0001" || got[0].To != "MOD-0004" || !slices.Equal(got[0].Ends, [][2]ID{{"MOD-0003", "MOD-0004"}}) {
		t.Errorf("the line runs from the outermost box, its end the deep unit: %+v", got)
	}
}
