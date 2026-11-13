package topology

import (
	"slices"
	"testing"

	"github.com/whyjp/abbd/internal/model"
)

func contract(id, name string, shapes ...string) model.Unit {
	u := model.Unit{ID: model.ID(id), Kind: model.Interface, Name: model.Said(model.Script, name), Anchors: []model.Anchor{{Path: name}}}
	for _, s := range shapes {
		u.Entries = append(u.Entries, model.Entry{Key: s, Name: s, Shape: s})
	}
	return u
}

func user(id string, rs ...model.Relation) model.Unit {
	return model.Unit{ID: model.ID(id), Kind: model.Module, Name: model.Said(model.Script, id), Anchors: []model.Anchor{{Path: id}}, Relations: rs}
}

func to(id string, kind model.RelationKind, via model.Via) model.Relation {
	return model.Relation{To: model.ID(id), Kind: kind, Via: via, Origin: model.Extracted}
}

// obs records its plugins as calling the callback table libobs provides,
// where the plugins implement it and libobs calls through it: one provider
// and many callers of a table of slots. vector's sink types have many
// providers and are named, not called, so they stand. A Rust free function
// takes no self and is a plain call, and a host that calls through its own
// table is not one of its callers.
func TestLintNamesAPlugPointRecordedTheWrongWayRound(t *testing.T) {
	units := []model.Unit{
		contract("IF-0001", "source plugin API", "void (*)(void *data)", "void *(*)(obs_data_t *settings, obs_source_t *source)", "const char *id"),
		contract("IF-0002", "Runtime", "fn run(self)", "pub async fn spawn(&self)"),
		contract("IF-0003", "sink types", "sink config", "sink config"),
		contract("IF-0004", "two hosts", "void (*)(void)"),
		contract("IF-0005", "plain calls", "int obs_startup(void)", "void obs_shutdown(void)"),
		user("MOD-0001", to("IF-0001", model.Provides, ""), to("IF-0002", model.Provides, "trait"), to("IF-0004", model.Provides, ""), to("IF-0005", model.Provides, "")),
		user("MOD-0002", to("IF-0001", model.Consumes, "call"), to("IF-0002", model.Consumes, "call"), to("IF-0003", model.Provides, ""), to("IF-0004", model.Provides, ""), to("IF-0005", model.Consumes, "call")),
		user("MOD-0003", to("IF-0001", model.Consumes, "call"), to("IF-0002", model.Consumes, "call"), to("IF-0003", model.Provides, ""), to("IF-0004", model.Consumes, "call"), to("IF-0005", model.Consumes, "call")),
		user("MOD-0004", to("IF-0001", model.Consumes, "import"), to("IF-0003", model.Consumes, "call"), to("IF-0004", model.Consumes, "call")),
	}
	units = append(units,
		contract("IF-0006", "free functions", "pub fn compress(src: &[u8]) -> Vec<u8>", "fn decompress(src: &[u8]) -> Vec<u8>"),
		contract("IF-0007", "fn pointers", "fn(&mut Ctx)", "Option<fn(i32) -> i32>"),
		contract("IF-0008", "host calls itself", "void (*)(void)"),
	)
	units[5].Relations = append(units[5].Relations, to("IF-0006", model.Provides, ""), to("IF-0007", model.Provides, ""), to("IF-0008", model.Provides, ""), to("IF-0008", model.Consumes, "call"))
	for _, i := range []int{6, 7} {
		units[i].Relations = append(units[i].Relations, to("IF-0006", model.Consumes, "call"), to("IF-0007", model.Consumes, "call"))
	}
	units[6].Relations = append(units[6].Relations, to("IF-0008", model.Consumes, "call"))
	got := ReversedPlugPoints(units)
	var ids []model.ID
	for _, r := range got {
		ids = append(ids, r.Contract)
	}
	if !slices.Equal(ids, []model.ID{"IF-0001", "IF-0002", "IF-0007"}) {
		t.Fatalf("flagged %v, want the callback table, the trait and the fn pointers only", got)
	}
	if r := got[0]; r.Provider != "MOD-0001" || !slices.Equal(r.Callers, []model.ID{"MOD-0002", "MOD-0003"}) {
		t.Errorf("IF-0001 names provider %s and callers %v", r.Provider, r.Callers)
	}
	units[6].Relations[0].Dead = true
	if got := ReversedPlugPoints(units); len(got) != 2 || got[0].Contract != "IF-0002" {
		t.Errorf("a dead call still counts: %v", got)
	}
}
