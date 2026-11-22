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
	if !strings.Contains(band.Title, "상자 수로 센 값") {
		t.Errorf("the band's hover does not say the count is of boxes: %q", band.Title)
	}
}

// A unit that consumes a contract is a dependent of the contract and not of
// the unit that provides it, as `abbd impact` reads the consuming step; the
// provider of a contract is not listed on the contract's row.
func TestAConsumerIsADependentOfTheContractAndNotOfItsProvider(t *testing.T) {
	t.Parallel()
	units := []model.Unit{
		called("MOD-0001", "provider", rel("IF-0001", model.Provides)),
		called("MOD-0002", "consumer", rel("IF-0001", model.Consumes)),
		called("IF-0001", "api"),
	}
	got := directDependents(units)
	if want := []model.ID{"MOD-0002"}; !slices.Equal(got["IF-0001"], want) {
		t.Errorf("dependents of the contract = %v, want %v", got["IF-0001"], want)
	}
	if len(got["MOD-0001"]) != 0 || len(got["MOD-0002"]) != 0 {
		t.Errorf("a provider or a consumer gets a dependent: %v", got)
	}
}

// insideUsed is one box, MOD-0001, with four units inside it that other boxes
// use, the first by four boxes, the second by three, the third and fourth by
// one.
func insideUsed() []model.Unit {
	in := func(id model.ID, name string) model.Unit {
		u := called(id, name)
		u.Parent = "MOD-0001"
		return u
	}
	return []model.Unit{
		called("MOD-0001", "core"),
		in("MOD-0011", "alpha"), in("MOD-0012", "beta"), in("MOD-0013", "gamma"), in("MOD-0014", "delta"),
		called("MOD-0002", "a", rel("MOD-0011", model.DependsOn), rel("MOD-0012", model.DependsOn), rel("MOD-0013", model.DependsOn)),
		called("MOD-0003", "b", rel("MOD-0011", model.DependsOn), rel("MOD-0012", model.DependsOn), rel("MOD-0014", model.DependsOn)),
		called("MOD-0004", "c", rel("MOD-0011", model.DependsOn), rel("MOD-0012", model.DependsOn)),
		called("MOD-0005", "d", rel("MOD-0011", model.DependsOn)),
	}
}

// The hover of a foundation band names the three inside units the most other
// boxes use, the most used first and ties by name, each with how many boxes.
func TestTopUsedNamesTheMostUsedInsideUnitsFirstAndAtMostThree(t *testing.T) {
	t.Parallel()
	units := insideUsed()
	o := planOverview(units, "", nil, tactic{}, nil, nil, nil)
	o.dependents = directDependents(units)
	want := []string{"alpha 4개 상자가 사용", "beta 3개 상자가 사용", "delta 1개 상자가 사용"}
	if got := o.topUsed("MOD-0001", 3); !slices.Equal(got, want) {
		t.Errorf("topUsed = %v, want %v", got, want)
	}
	if got := o.topUsed("MOD-0002", 3); len(got) != 0 {
		t.Errorf("a box nothing inside it is used by other boxes names %v", got)
	}
}

// A module page's list of inside units names each unit's dependents, a
// module's with the link to its page and any other unit's with the link to its
// row in the list of units.
func TestAModulePageListsEachInsideUnitsDependentsWithTheirLinks(t *testing.T) {
	t.Parallel()
	units := insideUsed()
	// A unit with units inside it has a page of its own.
	units = append(units, called("MOD-0006", "e", rel("MOD-0013", model.DependsOn)))
	inner := called("MOD-0021", "x", rel("MOD-0013", model.DependsOn))
	inner.Parent = "MOD-0002"
	units = append(units, inner)
	m := Build(Input{Units: units})
	var core *Inside
	for i := range m.Insides {
		if m.Insides[i].ID == "MOD-0001" {
			core = &m.Insides[i]
		}
	}
	if core == nil {
		t.Fatal("no page for core")
	}
	for _, iu := range core.Units {
		if iu.ID != "MOD-0013" {
			continue
		}
		var got []string
		for _, d := range iu.Dependents {
			got = append(got, d.Name+" "+d.Href)
		}
		want := []string{"a #m-MOD-0002", "e #m-MOD-0006", "x #u-MOD-0021"}
		if !slices.Equal(got, want) {
			t.Errorf("gamma's dependents = %v, want %v", got, want)
		}
		return
	}
	t.Error("gamma is not listed on core's page")
}

// A unit's row in the list of all units names the dependents of other
// modules only, so a relation inside one module leaves the list as it was; a
// library's row names every unit that depends on it.
func TestARowNamesOtherModulesDependentsAndALibraryAllOfThem(t *testing.T) {
	t.Parallel()
	sib := func(id model.ID, name string, rels ...model.Relation) model.Unit {
		u := called(id, name, rels...)
		u.Parent = "MOD-0001"
		return u
	}
	units := []model.Unit{
		called("MOD-0001", "core"),
		sib("MOD-0011", "near", rel("MOD-0012", model.DependsOn), rel("EXT-0001", model.DependsOn)),
		sib("MOD-0012", "target"),
		called("MOD-0002", "far", rel("MOD-0012", model.DependsOn), rel("EXT-0001", model.DependsOn)),
		called("EXT-0001", "lib"),
	}
	rows := map[string][]string{}
	for _, r := range Build(Input{Units: units}).Units {
		rows[r.ID] = r.UsedBy
	}
	if want := []string{"far"}; !slices.Equal(rows["MOD-0012"], want) {
		t.Errorf("target's row names %v, want %v", rows["MOD-0012"], want)
	}
	if want := []string{"far", "near"}; !slices.Equal(rows["EXT-0001"], want) {
		t.Errorf("the library's row names %v, want %v", rows["EXT-0001"], want)
	}
}
