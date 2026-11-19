package screen

import (
	"slices"
	"testing"

	"github.com/whyjp/abbd/internal/model"
)

// pluginHost is a host whose dispatcher calls a handler trait three
// plug-ins implement; the dispatcher names none of them. declared gives the
// trait the host as its parent, the module that declares it.
func pluginHost(declared bool) []model.Unit {
	impl := model.Relation{To: "IF-0001", Kind: model.Provides, Origin: model.Extracted, Via: "trait"}
	trait := called("IF-0001", "trait Handler")
	if declared {
		trait.Parent = "MOD-0001"
	}
	return []model.Unit{
		called("MOD-0001", "host"),
		called("MOD-0002", "dispatcher", model.Relation{To: "IF-0001", Kind: model.Consumes, Origin: model.Extracted, Via: "trait"}),
		called("MOD-0003", "plugin-a", impl),
		called("MOD-0004", "plugin-b", impl),
		called("MOD-0005", "plugin-c", impl),
		trait,
	}
}

func fanoutOf(t *testing.T, units []model.Unit) int {
	t.Helper()
	views, err := Lint(build(units), "overview")
	if err != nil {
		t.Fatal(err)
	}
	return views[0].Totals.Fanout
}

// An extension point its declaring module holds is no top-level contract,
// so the pairing never meets the dispatcher with a plug-in it does not
// name; the dispatcher's and each plug-in's relations lift to that module.
func TestAnExtensionPointItsModuleHoldsPairsNoUserWithAnImplementer(t *testing.T) {
	t.Parallel()
	units := pluginHost(true)
	o := planOverview(units, "", nil, tactic{}, nil, nil, nil)
	if got := pairsOf(o); len(got) != 0 {
		t.Errorf("the dispatcher is paired with the plug-ins: %v", got)
	}
	var edges []string
	for _, e := range o.relations {
		edges = append(edges, e.String())
	}
	for _, want := range []string{"MOD-0002.consumes.MOD-0001", "MOD-0003.provides.MOD-0001", "MOD-0004.provides.MOD-0001", "MOD-0005.provides.MOD-0001"} {
		if !slices.Contains(edges, want) {
			t.Errorf("no edge %s to the declaring module; edges %v", want, edges)
		}
	}
	if n := fanoutOf(t, units); n != 0 {
		t.Errorf("fanout = %d for a contract its declaring module holds, want 0", n)
	}
}

// With no declaring module stored, the trait stands on the overview and
// the pairing meets the dispatcher with each plug-in, as before; lint
// counts those three lines as fanout.
func TestAnExtensionPointWithNoParentKeepsItsPairsAndCountsAsFanout(t *testing.T) {
	t.Parallel()
	units := pluginHost(false)
	got := pairsOf(planOverview(units, "", nil, tactic{}, nil, nil, nil))
	want := []string{"MOD-0002>MOD-0003", "MOD-0002>MOD-0004", "MOD-0002>MOD-0005"}
	if !equalStrings(got, want) {
		t.Errorf("pairs = %v, want %v", got, want)
	}
	if n := fanoutOf(t, units); n != 3 {
		t.Errorf("fanout = %d, want 3", n)
	}
}

// A data contract two modules write keeps one line from its reader to
// each writer, and two providers are under the fanout count.
func TestAnOrdinaryContractKeepsItsPairsAndNoFanout(t *testing.T) {
	t.Parallel()
	data := func(kind model.RelationKind) model.Relation {
		return model.Relation{To: "DATA-0001", Kind: kind, Origin: model.Extracted, Via: "data"}
	}
	units := []model.Unit{
		called("MOD-0001", "writer-a", data(model.Provides)),
		called("MOD-0002", "writer-b", data(model.Provides)),
		called("MOD-0003", "reader", data(model.Consumes)),
		called("DATA-0001", "state file"),
	}
	got := pairsOf(planOverview(units, "", nil, tactic{}, nil, nil, nil))
	if want := []string{"MOD-0003>MOD-0001", "MOD-0003>MOD-0002"}; !equalStrings(got, want) {
		t.Errorf("pairs = %v, want %v", got, want)
	}
	if n := fanoutOf(t, units); n != 0 {
		t.Errorf("fanout = %d, want 0", n)
	}
}
