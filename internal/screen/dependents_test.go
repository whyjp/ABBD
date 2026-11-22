package screen

import (
	"slices"
	"strings"
	"testing"

	"github.com/whyjp/abbd/internal/model"
)

// A unit's direct dependents are the other units that depend on or consume
// it: a unit inside it, one that only provides a contract, a dead relation
// and a missing unit add none.
func TestDirectDependentsAreTheOtherUnitsThatDependOrConsume(t *testing.T) {
	t.Parallel()
	dead := rel("MOD-0001", model.DependsOn)
	dead.Dead = true
	child := called("MOD-0009", "inner", rel("MOD-0001", model.DependsOn))
	child.Parent = "MOD-0001"
	units := []model.Unit{
		called("MOD-0001", "base"),
		called("MOD-0003", "second", rel("MOD-0001", model.DependsOn)),
		called("MOD-0002", "first", rel("MOD-0001", model.DependsOn), rel("MOD-0001", model.Consumes)),
		called("MOD-0004", "provider", rel("IF-0001", model.Provides), dead),
		called("MOD-0005", "gone", rel("MOD-0777", model.DependsOn)),
		child,
		called("IF-0001", "api"),
	}
	got := directDependents(units)
	if want := []model.ID{"MOD-0002", "MOD-0003"}; !slices.Equal(got["MOD-0001"], want) {
		t.Errorf("dependents of base = %v, want %v", got["MOD-0001"], want)
	}
	if len(got["IF-0001"]) != 0 || len(got["MOD-0777"]) != 0 {
		t.Errorf("a provider or a missing unit gives a dependent: %v", got)
	}
}

// The units list and a module page's list of inside units say who depends on
// a unit, and a foundation band's hover says which inside unit the count of
// boxes comes from, saying that the count is of boxes.
func TestTheDependentsAreWrittenOnTheUnitsAndTheBand(t *testing.T) {
	t.Parallel()
	m := Build(Input{Units: bandUnits()})
	var log UnitRow
	for _, r := range m.Units {
		if r.ID == "MOD-0005" {
			log = r
		}
	}
	if len(log.UsedBy) != 4 {
		t.Errorf("log's row names %v, want the four units that depend on it", log.UsedBy)
	}
	var band *Box
	for i, b := range m.Overview.Boxes {
		if b.Ambient {
			band = &m.Overview.Boxes[i]
		}
	}
	if band == nil || band.Sub == nil || !strings.HasSuffix(band.Sub.S, "(상자 단위)") {
		t.Fatalf("the band's last line does not say its count is of boxes: %+v", band)
	}
	if !strings.Contains(band.Title, "상자 단위로 센 값") {
		t.Errorf("the band's hover does not say the count is of boxes: %q", band.Title)
	}
}
