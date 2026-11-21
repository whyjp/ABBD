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
