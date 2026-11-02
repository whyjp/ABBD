package topology

import (
	"reflect"
	"slices"
	"testing"

	"github.com/whyjp/abbd/internal/model"
)

func unit(id, parent model.ID, rels ...model.Relation) model.Unit {
	k, _, _ := model.ParseID(string(id))
	return model.Unit{ID: id, Kind: k, Parent: parent, Name: model.Fact{Value: string(id), Origin: model.Extracted}, Anchors: []model.Anchor{{Path: "x"}}, Relations: rels}
}

func rel(to model.ID, kind model.RelationKind, origin model.Origin) model.Relation {
	r := model.Relation{To: to, Kind: kind, Origin: origin}
	if origin == model.Inferred {
		r.Via, r.Evidence = "data", []model.Evidence{{Path: "a.go", Quote: "q"}}
	}
	return r
}

func itemIDs(items []Item) []model.ID {
	var out []model.ID
	for _, it := range items {
		out = append(out, it.Unit)
	}
	slices.Sort(out)
	return out
}

// The seed provides a contract. Its consumer is direct and what depends on
// the consumer is far. A live guess is followed and a dead one is not, an
// inside unit speaks for its module, and what the seed itself uses is direct
// but not followed.
func TestImpactTiers(t *testing.T) {
	dead := rel("MOD-0001", model.DependsOn, model.Inferred)
	dead.Dead = true
	units := []model.Unit{
		unit("MOD-0001", "", rel("IF-0001", model.Provides, model.Extracted), rel("DATA-0001", model.Provides, model.Inferred), rel("MOD-0008", model.DependsOn, model.Extracted)),
		unit("IF-0001", ""),
		unit("DATA-0001", ""),
		unit("MOD-0003", "", rel("IF-0001", model.Consumes, model.Extracted)),
		unit("MOD-0002", "", rel("MOD-0003", model.DependsOn, model.UserStated)),
		unit("MOD-0004", "", rel("DATA-0001", model.Consumes, model.Inferred)),
		unit("MOD-0005", "", dead),
		unit("MOD-0006", "MOD-0001", rel("MOD-0007", model.DependsOn, model.Extracted)),
		unit("MOD-0007", ""),
		unit("MOD-0008", "", rel("MOD-0009", model.DependsOn, model.Extracted)),
		unit("MOD-0009", ""),
	}
	units[4].Relations[0].Source = "conversation"
	im := ImpactOf(units, []model.ID{"MOD-0001"})
	wantDirect := []model.ID{"DATA-0001", "IF-0001", "MOD-0001", "MOD-0003", "MOD-0004", "MOD-0006", "MOD-0007", "MOD-0008"}
	wantFar := []model.ID{"MOD-0002"}
	if !reflect.DeepEqual(itemIDs(im.Direct), wantDirect) || !reflect.DeepEqual(itemIDs(im.Far), wantFar) {
		t.Fatalf("direct %v far %v, want direct %v far %v", itemIDs(im.Direct), itemIDs(im.Far), wantDirect, wantFar)
	}
	for _, it := range im.Direct {
		if it.Unit == "MOD-0004" && it.Why != "consumes DATA-0001 (inferred, via data)" {
			t.Errorf("a guess the impact follows says so: %q", it.Why)
		}
	}
}

// A change to a contract reaches its consumers and what depends on them,
// but not the other contracts a consumer provides: the change does not
// travel through those.
func TestImpactStopsAtAConsumersOtherContracts(t *testing.T) {
	units := []model.Unit{
		unit("DATA-0001", ""),
		unit("MOD-0001", "", rel("DATA-0001", model.Consumes, model.Extracted), rel("IF-0001", model.Provides, model.Extracted)),
		unit("IF-0001", ""),
		unit("MOD-0002", "", rel("IF-0001", model.Consumes, model.Extracted)),
		unit("MOD-0003", "", rel("MOD-0001", model.DependsOn, model.Extracted)),
	}
	im := ImpactOf(units, []model.ID{"DATA-0001"})
	wantDirect := []model.ID{"DATA-0001", "MOD-0001"}
	wantFar := []model.ID{"MOD-0003"}
	if !reflect.DeepEqual(itemIDs(im.Direct), wantDirect) || !reflect.DeepEqual(itemIDs(im.Far), wantFar) {
		t.Fatalf("direct %v far %v, want direct %v far %v", itemIDs(im.Direct), itemIDs(im.Far), wantDirect, wantFar)
	}
}

// A unit nested at any depth inside a seed module is a seed too: the
// contract it provides and that contract's consumer are impacted.
func TestImpactReachesUnitsDeepInsideASeed(t *testing.T) {
	units := []model.Unit{
		unit("MOD-0001", ""),
		unit("MOD-0002", "MOD-0001"),
		unit("MOD-0003", "MOD-0002", rel("IF-0001", model.Provides, model.Extracted)),
		unit("IF-0001", ""),
		unit("MOD-0009", "", rel("IF-0001", model.Consumes, model.Extracted)),
	}
	im := ImpactOf(units, []model.ID{"MOD-0001"})
	wantDirect := []model.ID{"IF-0001", "MOD-0001", "MOD-0002", "MOD-0003", "MOD-0009"}
	if !reflect.DeepEqual(itemIDs(im.Direct), wantDirect) || len(im.Far) != 0 {
		t.Fatalf("direct %v far %v, want direct %v and no far", itemIDs(im.Direct), itemIDs(im.Far), wantDirect)
	}
}
