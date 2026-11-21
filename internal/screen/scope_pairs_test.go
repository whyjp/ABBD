package screen

import (
	"slices"
	"testing"

	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/store"
)

// libraryWithSamples is a library that consumes a handler contract and a
// notification contract; an extension of its own provides the second, and so
// do a sample and a benchmark that only implement it. Another sample uses
// what the others provide.
func libraryWithSamples() []model.Unit {
	return []model.Unit{
		called("MOD-0001", "MediatR", rel("IF-0001", model.Provides), rel("IF-0002", model.Consumes)),
		called("MOD-0002", "Extensions", rel("IF-0001", model.Consumes), rel("IF-0002", model.Provides)),
		called("MOD-0003", "Samples", rel("IF-0001", model.Consumes), rel("IF-0002", model.Provides)),
		called("MOD-0004", "Benchmarks", rel("IF-0002", model.Provides)),
		called("MOD-0005", "Examples", rel("IF-0002", model.Consumes)),
		called("IF-0001", "IRequestHandler"),
		called("IF-0002", "INotificationHandler"),
	}
}

func pairsOf(o *overview) []string {
	var out []string
	for _, k := range o.pairs {
		out = append(out, string(k[0])+">"+string(k[1]))
	}
	slices.Sort(out)
	return out
}

func equalStrings(a, b []string) bool { return slices.Equal(a, b) }

func TestAProviderMarkedOutOfScopeIsPairedWithItsOwnScopeOnly(t *testing.T) {
	t.Parallel()
	units := libraryWithSamples()
	plain := pairsOf(planOverview(units, "", nil, tactic{}, nil, nil, nil))
	wantPlain := []string{
		"MOD-0001>MOD-0002", "MOD-0001>MOD-0003", "MOD-0001>MOD-0004",
		"MOD-0002>MOD-0001", "MOD-0003>MOD-0001",
		"MOD-0005>MOD-0002", "MOD-0005>MOD-0003", "MOD-0005>MOD-0004",
	}
	if !equalStrings(plain, wantPlain) {
		t.Fatalf("without marks the pairs are %v, want %v", plain, wantPlain)
	}
	scope := map[model.ID]string{"MOD-0003": model.ScopeSample, "MOD-0005": model.ScopeSample, "MOD-0004": model.ScopeTool}
	o := planScoped(units, "", nil, tactic{}, nil, nil, nil, scope)
	got := pairsOf(o)
	want := []string{
		"MOD-0001>MOD-0002",
		"MOD-0002>MOD-0001", "MOD-0003>MOD-0001",
		"MOD-0005>MOD-0002", "MOD-0005>MOD-0003",
	}
	if !equalStrings(got, want) {
		t.Errorf("with marks the pairs are %v, want %v", got, want)
	}
	if !slices.Contains(o.unreached["MOD-0004"], "IF-0002") {
		t.Errorf("the benchmark's contract no line reaches is not counted on its box: %v", o.unreached)
	}
}

func TestAMarkNoReaderCanHoldCountsAsNoMark(t *testing.T) {
	t.Parallel()
	units := libraryWithSamples()
	units[2].Anchors = []model.Anchor{{Path: "samples"}}
	quote := []model.Evidence{{Path: "samples/a.cs", Lines: "1", Quote: "class A"}}
	mark := func(unit model.ID, scope string, ev []model.Evidence) store.Part[model.ScopeMark] {
		return store.Part[model.ScopeMark]{Claim: model.ScopeMark{Unit: unit, Scope: scope, Evidence: ev}}
	}
	changed := mark("MOD-0003", model.ScopeSample, quote)
	changed.Changed = []string{"samples/a.cs"}
	cases := map[string]struct {
		part store.Part[model.ScopeMark]
		live bool
	}{
		"live":            {mark("MOD-0003", model.ScopeSample, quote), true},
		"unknown scope":   {mark("MOD-0003", "plugin", quote), false},
		"unit is gone":    {mark("MOD-0099", model.ScopeSample, quote), false},
		"quote not held":  {mark("MOD-0003", model.ScopeSample, []model.Evidence{{Path: "elsewhere/b.cs", Lines: "1", Quote: "class B"}}), false},
		"code has moved":  {changed, false},
		"no overview yet": {mark("MOD-0003", model.ScopeSample, quote), false},
	}
	for name, c := range cases {
		in := Input{Units: units, Overview: store.Overview{Known: name != "no overview yet", Scopes: []store.Part[model.ScopeMark]{c.part}}}
		scope := scopeOf(in)
		if live := scope["MOD-0003"] == model.ScopeSample; live != c.live {
			t.Errorf("%s: live = %v, want %v", name, live, c.live)
		}
		plain := pairsOf(planOverview(units, "", nil, tactic{}, nil, nil, nil))
		if got := pairsOf(planScoped(units, "", nil, tactic{}, nil, nil, nil, scope)); !c.live && !equalStrings(got, plain) {
			t.Errorf("%s: pairs %v differ from the unmarked %v", name, got, plain)
		}
	}
}

// A plug-in registers into a host: the host consumes the contract and the
// plug-in provides it. A plug-in is the product's own and takes no mark, so
// its line from the host stays even when a sample beside them is marked.
func TestAPluginTheHostLoadsKeepsItsLine(t *testing.T) {
	t.Parallel()
	units := []model.Unit{
		called("MOD-0001", "host", rel("IF-0001", model.Consumes)),
		called("MOD-0002", "plugin", rel("IF-0001", model.Provides)),
		called("MOD-0003", "examples", rel("IF-0001", model.Provides)),
		called("IF-0001", "Plugin"),
	}
	scope := map[model.ID]string{"MOD-0003": model.ScopeSample}
	got := pairsOf(planScoped(units, "", nil, tactic{}, nil, nil, nil, scope))
	want := []string{"MOD-0001>MOD-0002"}
	if !equalStrings(got, want) {
		t.Errorf("pairs = %v, want %v", got, want)
	}
	if plain := pairsOf(planOverview(units, "", nil, tactic{}, nil, nil, nil)); !slices.Contains(plain, "MOD-0001>MOD-0002") || !slices.Contains(plain, "MOD-0001>MOD-0003") {
		t.Errorf("without marks the pairs are %v", plain)
	}
}

func TestABoxMarkedOutOfScopeLeavesThePictureAndIsNamedUnderIt(t *testing.T) {
	t.Parallel()
	units := libraryWithSamples()
	scope := map[model.ID]string{"MOD-0003": model.ScopeSample, "MOD-0005": model.ScopeSample, "MOD-0004": model.ScopeTool}
	o := planScoped(units, "", nil, tactic{}, nil, nil, nil, scope)
	c := o.asideBoxes(nil, scope)
	if c == nil || !c.asides() || len(c.aside) != 3 {
		t.Fatalf("the three marked boxes are set aside and their lines left out: %+v", c)
	}
	p := planScoped(units, "", nil, tactic{}, nil, nil, c, scope)
	d := p.draw(func(model.ID) string { return "" })
	var names []string
	for _, b := range d.Boxes {
		names = append(names, b.Name)
	}
	slices.Sort(names)
	if !slices.Equal(names, []string{"Extensions", "MediatR"}) {
		t.Errorf("the picture draws %v, want the product's two boxes", names)
	}
	var aside []string
	for _, a := range d.Aside {
		aside = append(aside, a.Name+":"+a.Scope)
	}
	if want := []string{"Samples:sample", "Benchmarks:tool", "Examples:sample"}; !slices.Equal(aside, want) {
		t.Errorf("the line under the picture names %v, want %v", aside, want)
	}
	if o2 := o.asideBoxes(nil, nil); o2 != nil {
		t.Errorf("with no mark the cut stays as it was: %+v", o2)
	}
}
