package topology

import (
	"reflect"
	"testing"

	"github.com/whyjp/abbd/internal/model"
)

func pathUnits() []model.Unit {
	dead := rel("MOD-0001", model.DependsOn, model.Inferred)
	dead.Dead = true
	stated := rel("MOD-0002", model.DependsOn, model.UserStated)
	stated.Source = "conversation 2026-11-01"
	return []model.Unit{
		unit("MOD-0001", "", rel("DATA-0001", model.Provides, model.Inferred), rel("IF-0001", model.Provides, model.Extracted), rel("MOD-0006", model.DependsOn, model.Extracted)),
		unit("IF-0001", ""),
		unit("DATA-0001", ""),
		unit("MOD-0002", "", rel("DATA-0001", model.Consumes, model.Inferred), rel("IF-0001", model.Consumes, model.Extracted)),
		unit("MOD-0003", "", stated),
		unit("MOD-0004", "", dead),
		unit("MOD-0006", "", rel("MOD-0007", model.DependsOn, model.Extracted)),
		unit("MOD-0007", "", rel("MOD-0008", model.DependsOn, model.Extracted)),
		unit("MOD-0008", "", rel("MOD-0003", model.DependsOn, model.Extracted)),
	}
}

type step struct {
	From, To model.ID
	Forward  bool
	Origin   model.Origin
}

func steps(hops []Hop) []step {
	var out []step
	for _, h := range hops {
		out = append(out, step{h.From, h.To, h.Forward, h.Origin})
	}
	return out
}

// The path is the fewest hops, walked against a link's direction where it
// must. Of two equally short paths it takes the one the code vouches for,
// so a guess shows only where nothing firmer connects the units.
func TestPathIsShortestThenFirmest(t *testing.T) {
	units := pathUnits()
	hops, ok := Path(units, "MOD-0001", "MOD-0003")
	want := []step{
		{"MOD-0001", "IF-0001", true, model.Extracted},
		{"MOD-0002", "IF-0001", false, model.Extracted},
		{"MOD-0003", "MOD-0002", false, model.UserStated},
	}
	if !ok || !reflect.DeepEqual(steps(hops), want) {
		t.Fatalf("got %+v, want the three-hop path through the contract, not the four extracted hops or the guessed table: %+v", steps(hops), want)
	}
	if w := Weakest(hops); w != model.UserStated {
		t.Fatalf("weakest origin %s, want USER_STATED", w)
	}

	hops, _ = Path(units, "DATA-0001", "IF-0001")
	if w := Weakest(hops); len(hops) != 2 || w != model.Inferred {
		t.Fatalf("the table reaches the contract only through a guess: %+v weakest %s", steps(hops), w)
	}
}

func TestPathFollowsNoDeadLink(t *testing.T) {
	if hops, ok := Path(pathUnits(), "MOD-0001", "MOD-0004"); ok {
		t.Fatalf("MOD-0004's only link is dead, so no path reaches it: %+v", steps(hops))
	}
}

// A module's explanation lists its drawn links by direction and the guesses
// that touch it or a unit inside it.
func TestExplainSplitsDrawnLinksFromGuesses(t *testing.T) {
	units := append(pathUnits(), unit("DATA-0002", "MOD-0002", rel("MOD-0007", model.DependsOn, model.Inferred)))
	ex := Explain(units, "MOD-0002")
	if ex.Unit.ID != "MOD-0002" || len(ex.Inside) != 1 || ex.Inside[0].ID != "DATA-0002" {
		t.Fatalf("explanation of %s with inside %+v", ex.Unit.ID, ex.Inside)
	}
	var out, in, guessed []model.Edge
	for _, l := range ex.Out {
		out = append(out, l.Edge)
	}
	for _, l := range ex.In {
		in = append(in, l.Edge)
	}
	for _, l := range ex.Inferred {
		guessed = append(guessed, l.Edge)
	}
	if want := []model.Edge{{From: "MOD-0002", To: "IF-0001", Kind: model.Consumes}}; !reflect.DeepEqual(out, want) {
		t.Fatalf("out %v, want %v", out, want)
	}
	if want := []model.Edge{{From: "MOD-0003", To: "MOD-0002", Kind: model.DependsOn}}; !reflect.DeepEqual(in, want) {
		t.Fatalf("in %v, want %v", in, want)
	}
	if want := []model.Edge{{From: "DATA-0002", To: "MOD-0007", Kind: model.DependsOn}, {From: "MOD-0002", To: "DATA-0001", Kind: model.Consumes}}; !reflect.DeepEqual(guessed, want) {
		t.Fatalf("guesses %v, want %v", guessed, want)
	}
}
