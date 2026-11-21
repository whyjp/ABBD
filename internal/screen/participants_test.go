package screen

import (
	"slices"
	"strings"
	"testing"

	"github.com/whyjp/abbd/internal/model"
)

// A participant is tied when a unit of a stage of the primary flow, or one
// inside it, has a live relation to it. An untied one is reported, never
// refused.
func TestParticipantsAreTiedByTheFlowsStages(t *testing.T) {
	units := []model.Unit{
		{ID: "MOD-0001", Kind: model.Module},
		{ID: "MOD-0002", Kind: model.Module, Parent: "MOD-0001", Relations: []model.Relation{{To: "EXT-0001", Kind: model.DependsOn}}},
		{ID: "MOD-0003", Kind: model.Module, Relations: []model.Relation{{To: "EXT-0002", Kind: model.DependsOn}}},
		{ID: "EXT-0001", Kind: model.External},
		{ID: "EXT-0002", Kind: model.External},
	}
	p := &PrimaryFlow{Stages: []Stage{{N: 1, Name: "a", Units: []string{"MOD-0001"}}}}
	got := tiedParticipants([]ParticipantView{{Unit: "EXT-0001"}, {Unit: "EXT-0002"}}, p, units)
	if !got[0].Tied || got[1].Tied {
		t.Errorf("tied: %+v", got)
	}
	advice := strings.Join(participantAdvice(got, map[string]string{"EXT-0002": "libx"}), "\n")
	if !strings.Contains(advice, "participant libx") || strings.Contains(advice, "EXT-0001") {
		t.Errorf("advice: %q", advice)
	}
	if tiedParticipants(nil, p, units) != nil || participantAdvice(got[:1], nil) != nil {
		t.Error("no participants, or only tied ones, ask for nothing")
	}
}

// A participant no stage reaches is no advice about the overview's level:
// advice moves the level verdict that lint --record and --keep-level read.
func TestAnUntiedParticipantIsNoLevelAdvice(t *testing.T) {
	m := Model{Participants: []ParticipantView{{Unit: "EXT-0001"}}}
	l, base := Levels(m)[0], Levels(Model{})[0]
	if !slices.Equal(l.Advice, base.Advice) || len(l.Participants) != 1 {
		t.Errorf("advice %q, participants %q; want the advice of a model without participants and one finding", l.Advice, l.Participants)
	}
}

// An outside unit is drawn for each participant mark that stands, with the
// stages that reach it, and a mark that is stale or names a unit the state no
// longer holds draws nothing. No mark gives no outside unit.
func TestAParticipantStandsOutsideTheBoxesWithTheStagesThatReachIt(t *testing.T) {
	units := []model.Unit{
		{ID: "MOD-0001", Kind: model.Module},
		{ID: "MOD-0002", Kind: model.Module, Parent: "MOD-0001", Relations: []model.Relation{{To: "EXT-0001", Kind: model.DependsOn}}},
		{ID: "EXT-0001", Kind: model.External, Name: model.Fact{Value: "libx"}},
		{ID: "EXT-0002", Kind: model.External},
	}
	p := &PrimaryFlow{Stages: []Stage{{N: 1, Name: "a", Units: []string{"MOD-0001"}}}}
	got := outsideOf([]ParticipantView{{Unit: "EXT-0001"}, {Unit: "EXT-0002"}, {Unit: "EXT-0001", Reasons: []string{"stale"}}, {Unit: "EXT-0099"}}, p, units)
	if len(got) != 2 || got[0].Name != "libx" || !slices.Equal(got[0].Reached, []string{"1단계 a"}) || len(got[1].Reached) != 0 {
		t.Fatalf("outside = %+v", got)
	}
	left := outsideLeftOut(got)
	if len(left) != 1 || left[0].Name != "libx" || !slices.Equal(left[0].With, []string{"1단계 a"}) {
		t.Errorf("left out = %+v", left)
	}
	if outsideOf(nil, p, units) != nil || outsideLeftOut(nil) != nil {
		t.Error("no participant mark, nothing outside")
	}
}
