package model

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

// cli is a three-layer program: commands on top, the store in the middle,
// the vocabulary at the bottom. A contract and a submodule sit inside the
// modules that hold them.
func cli() []Unit {
	return []Unit{
		{ID: "MOD-0001", Kind: Module, Anchors: []Anchor{{Path: "cmd"}}, Relations: []Relation{
			{To: "MOD-0002", Kind: DependsOn}, {To: "MOD-0003", Kind: DependsOn}}},
		{ID: "DATA-0001", Kind: Data, Parent: "MOD-0001"},
		{ID: "MOD-0002", Kind: Module, Anchors: []Anchor{{Path: "store"}}, Relations: []Relation{
			{To: "DATA-0001", Kind: Consumes, Origin: Inferred}, {To: "MOD-0003", Kind: DependsOn}, {To: "MOD-0005", Kind: DependsOn}, {To: "IF-0001", Kind: Provides}}},
		{ID: "IF-0001", Kind: Interface, Parent: "MOD-0002", Anchors: []Anchor{{Path: "store/api.go", Symbol: "API"}}},
		{ID: "MOD-0005", Kind: Module, Anchors: []Anchor{{Path: "cache"}}, Relations: []Relation{{To: "MOD-0002", Kind: DependsOn}}},
		{ID: "MOD-0003", Kind: Module, Anchors: []Anchor{{Path: "model"}}, Relations: []Relation{
			{To: "IF-0001", Kind: Provides, Origin: Extracted}, {To: "MOD-0001", Kind: DependsOn, Dead: true}, {To: "MOD-0009", Kind: DependsOn}}},
		{ID: "MOD-0004", Kind: Module, Parent: "MOD-0003", Anchors: []Anchor{{Path: "model/sub"}}, Relations: []Relation{{To: "MOD-0002", Kind: DependsOn}}},
		{ID: "MOD-0009", Kind: Module, Anchors: []Anchor{{Path: "tools"}}},
	}
}

func layer(name string, ids ...ID) Layer {
	return Layer{Name: name, Role: "does " + name, Units: ids, Evidence: []Evidence{{Path: "x.go", Quote: name}}}
}

func TestALayerIsNamedOnOneLineOnLiveEvidence(t *testing.T) {
	ok := layer("store", "MOD-0002")
	if err := ok.Validate(); err != nil {
		t.Fatal(err)
	}
	for want, l := range map[string]Layer{
		"has a name":           {Role: "r", Units: ok.Units, Evidence: ok.Evidence},
		"says its role":        {Name: "store", Role: "a\nb", Units: ok.Units, Evidence: ok.Evidence},
		"names no unit":        {Name: "store", Role: "r", Evidence: ok.Evidence},
		"names MOD-0002 twice": {Name: "store", Role: "r", Units: []ID{"MOD-0002", "MOD-0002"}, Evidence: ok.Evidence},
		"cites no code":        {Name: "store", Role: "r", Units: ok.Units},
	} {
		if err := l.Validate(); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("want an error saying %q, got %v", want, err)
		}
	}
	if _, err := ParseFigureKey[Layer]("CLI Layers"); err == nil || !strings.Contains(err.Error(), "a stack's key") {
		t.Errorf("a stack key is lower-case letters, digits and hyphens: %v", err)
	}
}

// A layer holds each unit it names and every unit inside it, so a unit sits
// in one layer only, counting what the units around it hold.
func TestAStackHoldsEachUnitOnce(t *testing.T) {
	whole := KindOf[Layer]().Whole
	for _, c := range []struct {
		layers []Layer
		want   string
	}{
		{[]Layer{layer("commands", "MOD-0001"), layer("store", "MOD-0002"), layer("model", "MOD-0003")}, ""},
		{[]Layer{layer("commands", "MOD-0001"), layer("store", "MOD-0002", "MOD-0001")}, "MOD-0001 is named by layer 1 and by layer 2"},
		{[]Layer{layer("model", "MOD-0003"), layer("sub", "MOD-0004")}, "MOD-0004 is held by layer 2 and, inside MOD-0003, by layer 1"},
		{[]Layer{layer("sub", "MOD-0004"), layer("model", "MOD-0003")}, "MOD-0004 is held by layer 1 and, inside MOD-0003, by layer 2"},
		{[]Layer{layer("model", "MOD-0003", "MOD-0004"), layer("store", "MOD-0002")}, "layer 1 names MOD-0004 and MOD-0003, which holds it"},
	} {
		got := strings.Join(whole(c.layers, cli()), "; ")
		if c.want == "" && got != "" || !strings.Contains(got, c.want) {
			t.Errorf("want %q, got %q", c.want, got)
		}
	}
	var many []Layer
	for i := range StackUnits + 1 {
		many = append(many, layer(fmt.Sprint(i), ID(fmt.Sprintf("MOD-%04d", 100+i))))
	}
	if got := strings.Join(whole(many, cli()), "; "); !strings.Contains(got, "at most 16 units, and these name 17") {
		t.Errorf("a stack names at most %d units: %q", StackUnits, got)
	}
}

// A breach is a live relation, in the direction it is stored, from a unit a
// lower layer holds to one a higher layer holds. A unit inside a named unit
// sits in that unit's layer. Relations within a layer, ones that skip down,
// dead ones and ones with an end no layer holds are no breach.
func TestABreachRunsUpTheStackInTheStoredDirection(t *testing.T) {
	layers := []Layer{layer("commands", "MOD-0001"), layer("store", "MOD-0002", "MOD-0005"), layer("model", "MOD-0003")}
	held := Holders(layers, cli())
	if held["DATA-0001"] != 1 || held["MOD-0004"] != 3 || held["MOD-0005"] != 2 {
		t.Errorf("a unit inside a named unit sits in its layer: %v", held)
	}
	if _, ok := held["MOD-0009"]; ok {
		t.Errorf("a unit no layer names, nor any unit around it, sits in no layer: %v", held)
	}
	var got []string
	for _, b := range Breaches(layers, cli()) {
		got = append(got, fmt.Sprintf("%s %s %s %d>%d %s", b.From, b.Kind, b.To, b.Lower, b.Upper, b.Origin))
	}
	want := []string{
		"MOD-0002 consumes DATA-0001 2>1 INFERRED",
		"MOD-0003 provides IF-0001 3>2 EXTRACTED",
		"MOD-0004 depends_on MOD-0002 3>2 ",
	}
	if !slices.Equal(got, want) {
		t.Errorf("breaches:\nwant %q\ngot  %q", want, got)
	}
}

// A layer rests on its own units' code: one quote at least lies in code one
// of its units, a unit inside one, or a contract one provides holds most
// closely. A unit it names that left the state is a gap.
func TestALayerCitesTheCodeOfItsUnits(t *testing.T) {
	at := func(l Layer, paths ...string) Layer {
		l.Evidence = nil
		for _, p := range paths {
			path, symbol, _ := strings.Cut(p, "#")
			l.Evidence = append(l.Evidence, Evidence{Path: path, Symbol: symbol, Quote: "x"})
		}
		return l
	}
	gap := "none of the layer's quotes is in the code of its units"
	for _, c := range []struct {
		l    Layer
		want string
	}{
		{at(layer("store", "MOD-0002", "MOD-0005"), "cache/c.go"), ""},
		{at(layer("model", "MOD-0003"), "model/sub/s.go"), ""},
		{at(layer("store", "MOD-0002"), "store/api.go#API"), ""},
		{at(layer("model", "MOD-0003"), "store/api.go#API"), ""},
		{at(layer("commands", "MOD-0001"), "store/s.go"), gap},
		{at(layer("model", "MOD-0004"), "model/m.go"), gap},
		{at(layer("gone", "MOD-0003", "MOD-0042"), "model/m.go"), "MOD-0042 is no longer in the state"},
	} {
		got := strings.Join(c.l.Gaps(cli()), "; ")
		if c.want == "" && got != "" || !strings.Contains(got, c.want) {
			t.Errorf("%v citing %v: want %q, got %q", c.l.Units, c.l.Evidence, c.want, got)
		}
	}
}
