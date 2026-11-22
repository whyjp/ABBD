package screen

import (
	"slices"
	"testing"

	"github.com/whyjp/abbd/internal/model"
)

func shapeOfInside(t *testing.T, units []model.Unit) *Shape {
	t.Helper()
	for _, l := range Levels(build(named(units))) {
		if l.View == "MOD-0002 grep" {
			if l.Shape == nil {
				t.Fatal("the inside has no shape")
			}
			return l.Shape
		}
	}
	t.Fatal("no level for MOD-0002")
	return nil
}

// A chain of uses reads as layers, parts that use each other round a cycle
// count as one part of a layer and stand on a cycle.
func TestShapeReadsTheChainAndTheCycle(t *testing.T) {
	units := append(ripgrep(),
		inside("MOD-0010", "MOD-0002", rel("MOD-0011", model.DependsOn)),
		inside("MOD-0011", "MOD-0002", rel("MOD-0012", model.DependsOn)),
		inside("MOD-0012", "MOD-0002", rel("MOD-0013", model.DependsOn)),
		inside("MOD-0013", "MOD-0002"),
		inside("MOD-0014", "MOD-0002", rel("MOD-0015", model.DependsOn)),
		inside("MOD-0015", "MOD-0002", rel("MOD-0014", model.DependsOn)),
	)
	s := shapeOfInside(t, units)
	if s.Boxes != 6 || s.Joined != 6 || s.Pairs != 4 {
		t.Errorf("boxes %d, joined %d, pairs %d, want 6, 6, 4", s.Boxes, s.Joined, s.Pairs)
	}
	if s.Chain != 3 || !slices.Equal(s.Layers, []int{3, 1, 1, 1}) {
		t.Errorf("chain %d, layers %v, want 3 and [3 1 1 1]", s.Chain, s.Layers)
	}
	if s.Cyclic != 2 || s.Same != 2 || s.Down != 3 {
		t.Errorf("cyclic %d, same %d, down %d, want 2, 2, 3", s.Cyclic, s.Same, s.Down)
	}
}

// Four parts no line joins that use the same contract are a family
// candidate, named by the contract.
func TestShapeFindsAFamilyOfPartsThatShareAContract(t *testing.T) {
	units := append(ripgrep(),
		inside("IF-0010", "MOD-0002"),
		inside("MOD-0010", "MOD-0002", rel("IF-0010", model.Consumes)),
		inside("MOD-0011", "MOD-0002", rel("IF-0010", model.Consumes)),
		inside("MOD-0012", "MOD-0002", rel("IF-0010", model.Consumes)),
		inside("MOD-0013", "MOD-0002", rel("IF-0010", model.Consumes)),
	)
	s := shapeOfInside(t, units)
	if len(s.Families) != 1 || s.Families[0].Parts != 4 || s.Families[0].Contract != "IF-0010" {
		t.Errorf("families %+v, want one of 4 parts on IF-0010", s.Families)
	}
}

// A contract drawn as a chip on the frame counts as one the parts share.
func TestShapeFindsAFamilyThroughAFrameChip(t *testing.T) {
	units := ripgrep()
	for i := 10; i < 14; i++ {
		units = append(units, inside(model.ID("MOD-00"+string(rune('0'+i/10))+string(rune('0'+i%10))), "MOD-0002", rel("IF-0001", model.Consumes)))
	}
	s := shapeOfInside(t, units)
	if len(s.Families) != 1 || s.Families[0].Parts != 4 || s.Families[0].Contract != "trait Matcher (grep-matcher)" {
		t.Errorf("families %+v, want one of 4 parts on the chip's contract", s.Families)
	}
	if s.Contract < 1 {
		t.Errorf("%d lines through a contract cross the frame, want 1 or more", s.Contract)
	}
}

// A frame chip for another module met without a contract is no contract:
// parts that all use that module are no family, and their lines count as
// direct.
func TestShapeTakesAModuleChipForNoContract(t *testing.T) {
	units := ripgrep()
	for i := 10; i < 14; i++ {
		units = append(units, inside(model.ID("MOD-00"+string(rune('0'+i/10))+string(rune('0'+i%10))), "MOD-0002", rel("MOD-0004", model.DependsOn)))
	}
	s := shapeOfInside(t, units)
	if len(s.Families) != 0 {
		t.Errorf("families %+v, want none for a module chip", s.Families)
	}
	if s.Direct < 1 || s.Contract != 0 {
		t.Errorf("direct %d, contract %d, want direct lines and no contract lines", s.Direct, s.Contract)
	}
}
