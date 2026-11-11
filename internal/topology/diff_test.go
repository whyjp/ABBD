package topology

import (
	"slices"
	"testing"

	"github.com/whyjp/abbd/internal/model"
)

// A unit that keeps its ID and changes its name, parent or purpose, a
// unit retracted, and a reader's coupling whose evidence died each read as
// one line, and a state with none of them reads as before.
func TestCompareNamesWhatChangedBesideTheTopology(t *testing.T) {
	coupled := rel("MOD-0003", model.DependsOn, model.Inferred)
	base := []model.Unit{
		unit("MOD-0001", ""), unit("MOD-0002", ""), unit("MOD-0003", ""),
		unit("MOD-0004", "MOD-0001", coupled), unit("MOD-0005", ""),
	}
	now := slices.Clone(base)
	now[1].Name.Value = "store"
	now[1].Purpose = &model.Fact{Value: "keeps the state", Origin: model.UserStated}
	now[3] = unit("MOD-0004", "MOD-0002", coupled)
	now[3].Relations[0].Dead = true
	now[4].Absent, now[4].AbsentReason = true, model.Retracted
	want := []string{
		"- unit MOD-0005 MOD-0005 (retracted)",
		`~ renamed MOD-0002 "MOD-0002" -> "store", EXTRACTED`,
		`~ purpose MOD-0002 store: "" -> "keeps the state", USER_STATED`,
		"~ moved MOD-0004 MOD-0004: MOD-0001 -> MOD-0002",
		"- inferred dead MOD-0004 depends_on MOD-0003 (MOD-0004 depends_on MOD-0003)",
	}
	if got := Compare(base, now).Lines(); !slices.Equal(got, want) {
		t.Errorf("lines:\n got %q\nwant %q", got, want)
	}
	if got := Compare(base, base).String(); got != "no topology change\n" {
		t.Errorf("an unchanged state reads %q", got)
	}
}

// A reader's coupling whose evidence died is no dead coupling while another
// reader's on the same edge is still live.
func TestADeadCouplingBesideALiveOneIsNoChange(t *testing.T) {
	live := rel("MOD-0002", model.DependsOn, model.Inferred)
	dead := rel("MOD-0002", model.DependsOn, model.Inferred)
	dead.Dead = true
	base := []model.Unit{unit("MOD-0001", "", live), unit("MOD-0002", "")}
	now := []model.Unit{unit("MOD-0001", "", dead, live), unit("MOD-0002", "")}
	if got := Compare(base, now).String(); got != "no topology change\n" {
		t.Errorf("a coupling still live on its edge reads %q", got)
	}
}

// RelationEnds turns a relation lifted to its top-level ends back into the
// units that really join it, and leaves a dead or reader's relation out.
func TestRelationEndsFindsTheUnitsARelationJoins(t *testing.T) {
	units := []model.Unit{
		unit("MOD-0001", ""), unit("MOD-0002", "MOD-0001", rel("MOD-0004", model.DependsOn, model.Extracted), rel("MOD-0005", model.DependsOn, model.Inferred)),
		unit("MOD-0003", ""), unit("MOD-0004", "MOD-0003"), unit("MOD-0005", "MOD-0003"),
	}
	got := RelationEnds(units, []Relation{{From: "MOD-0001", Kind: model.DependsOn, To: "MOD-0003"}})
	if want := []model.ID{"MOD-0002", "MOD-0004"}; !slices.Equal(got, want) {
		t.Errorf("ends: got %v, want %v", got, want)
	}
}
