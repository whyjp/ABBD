package identity

import (
	"testing"

	"github.com/whyjp/abbd/internal/extract"
	"github.com/whyjp/abbd/internal/model"
)

// A build artifact keeps whoever said it: one the supplement gave at setup
// stays through a sync of the script alone, and one the script read goes
// when the script stops reading it.
func TestAnArtifactKeepsItsWriterThroughSync(t *testing.T) {
	anchors := []model.Anchor{{Path: "lib"}, {Path: "lib/zstd.c"}}
	unit := func(a *model.Fact) extract.Result {
		return extract.Result{Units: []extract.Unit{{Key: "lib", Kind: model.Module, Name: model.Said(model.Script, "lib"), Anchors: anchors, Artifact: a}}}
	}
	fact := func(w model.Writer, v string) *model.Fact { f := model.Said(w, v); return &f }
	hw := map[model.Kind]int{}

	set := Reconcile(nil, nil, unit(fact(model.Supplement, "shared")), nil, hw, Setup)
	synced := Reconcile(set.Units, nil, unit(nil), nil, set.HighWater, Sync)
	if got := model.ArtifactSet(synced.Units[0].Built()); got != "shared" {
		t.Errorf("a sync of the script alone dropped the supplement's artifact: %q", got)
	}

	set = Reconcile(nil, nil, unit(fact(model.Script, "static")), nil, hw, Setup)
	synced = Reconcile(set.Units, nil, unit(nil), nil, set.HighWater, Sync)
	if got := model.ArtifactSet(synced.Units[0].Built()); got != "" {
		t.Errorf("an artifact the script no longer reads stayed: %q", got)
	}
}
