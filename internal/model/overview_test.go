package model

import (
	"strings"
	"testing"
)

// boxes is two boxes: MOD-0001 holds a module and its data, MOD-0002 reads
// the data, and IF-0001's parent left the state, so the overview draws it
// as a box of its own.
func boxes() []Unit {
	return []Unit{
		{ID: "MOD-0001", Kind: Module, Anchors: []Anchor{{Path: "a"}}},
		{ID: "MOD-0003", Kind: Module, Parent: "MOD-0001", Anchors: []Anchor{{Path: "a/b"}}, Relations: []Relation{{To: "DATA-0001", Kind: Consumes}}},
		{ID: "DATA-0001", Kind: Data, Parent: "MOD-0001"},
		{ID: "MOD-0002", Kind: Module, Anchors: []Anchor{{Path: "c"}}, Relations: []Relation{{To: "DATA-0001", Kind: Consumes}}},
		{ID: "IF-0001", Kind: Interface, Parent: "MOD-0009", Anchors: []Anchor{{Path: "d"}}},
	}
}

// A zone names the boxes the overview draws, and a line joins two
// of them, so neither runs inside one box.
func TestAnOverviewPartNamesTheBoxesTheOverviewDraws(t *testing.T) {
	for want, units := range map[string][]ID{
		"MOD-0003 sits inside MOD-0001, which the overview draws as one box; name MOD-0001": {"MOD-0003"},
		"": {"MOD-0001", "IF-0001"},
	} {
		z := Zone{Name: "a", Units: units, Evidence: []Evidence{{Path: "a/x.go", Quote: "x"}}}
		got := strings.Join(z.Gaps(boxes()), "; ")
		if want == "" && strings.Contains(got, "sits inside") || want != "" && !strings.Contains(got, want) {
			t.Errorf("%v: want %q, got %q", units, want, got)
		}
	}
	for want, l := range map[string]Line{
		"DATA-0001 sits inside MOD-0001, so the line runs inside one box":                  {From: "MOD-0001", To: "DATA-0001"},
		"MOD-0003 and DATA-0001 both sit inside MOD-0001, so the line runs inside one box": {From: "MOD-0003", To: "DATA-0001"},
		"": {From: "MOD-0002", To: "MOD-0001"},
	} {
		l.Evidence = []Evidence{{Path: "c/x.go", Quote: "x"}}
		got := strings.Join(l.Gaps(boxes()), "; ")
		if want == "" && strings.Contains(got, "inside one box") || want != "" && !strings.Contains(got, want) {
			t.Errorf("%s to %s: want %q, got %q", l.From, l.To, want, got)
		}
	}
}

// Zone names are labels on the drawing: each follows the phrase rule, and
// no two zones share one.
func TestZoneNamesAreShortAndEachItsOwn(t *testing.T) {
	ev := []Evidence{{Path: "a/x.go", Quote: "x"}}
	if err := (Zone{Name: "one two three four five", Units: []ID{"MOD-0001"}, Evidence: ev}).Validate(); err == nil || !strings.Contains(err.Error(), "at most 4 words") {
		t.Errorf("a long zone name is refused: %v", err)
	}
	same := []Zone{{Name: "read", Units: []ID{"MOD-0001"}, Evidence: ev}, {Name: "read", Units: []ID{"MOD-0002"}, Evidence: ev}}
	gaps := KindOf[Zone]().Whole(nil, same, boxes())
	if len(gaps) != 1 || !strings.Contains(gaps[0].Text, "zones 1 and 2 are both named read") {
		t.Errorf("two zones with one name are refused: %v", gaps)
	}
}

// A participant is an external unit the flow reaches: one the state lacks or
// one of the project's own is refused, and its quote lies in its own code or
// in the code of a unit that uses it.
func TestAParticipantIsAnExternalUnitTheFlowReaches(t *testing.T) {
	units := append(boxes(), Unit{ID: "EXT-0001", Kind: External, Anchors: []Anchor{{Path: "go.mod", Symbol: "lib"}}})
	units[3].Relations = append(units[3].Relations, Relation{To: "EXT-0001", Kind: DependsOn})
	for _, c := range []struct {
		name string
		unit ID
		path string
		want string
	}{
		{"gone", "EXT-0009", "c/x.go", "is no longer in the state"},
		{"own", "MOD-0002", "c/x.go", "is a unit of the project"},
		{"user's code", "EXT-0001", "c/x.go", ""},
		{"no user's code", "EXT-0001", "a/x.go", "none of the quotes of participant EXT-0001"},
	} {
		p := Participant{Unit: c.unit, Evidence: []Evidence{{Path: c.path, Quote: "x"}}}
		got := strings.Join(p.Gaps(units), "; ")
		if c.want == "" && got != "" || c.want != "" && !strings.Contains(got, c.want) {
			t.Errorf("%s: want %q, got %q", c.name, c.want, got)
		}
	}
	if err := (Participant{Unit: "EXT-0001"}).Validate(); err == nil || !strings.Contains(err.Error(), "cites no code") {
		t.Errorf("a participant with no quote: %v", err)
	}
}
