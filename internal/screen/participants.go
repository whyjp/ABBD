package screen

import (
	"fmt"
	"slices"
	"strings"

	"github.com/whyjp/abbd/internal/model"
)

// tiedParticipants says of each participant mark whether a unit that a stage
// of the primary flow holds, or a unit inside it, reaches the external unit
// by a live relation. The mark stands either way, since the reader judges
// whether the flow needs the unit; an untied one is only reported.
func tiedParticipants(ps []ParticipantView, p *PrimaryFlow, units []model.Unit) []ParticipantView {
	if len(ps) == 0 {
		return nil
	}
	byID := map[model.ID]model.Unit{}
	for _, u := range model.Present(units) {
		byID[u.ID] = u
	}
	inside := func(id, top model.ID) bool {
		for seen := map[model.ID]bool{}; id != "" && !seen[id]; id = byID[id].Parent {
			if id == top {
				return true
			}
			seen[id] = true
		}
		return false
	}
	var stages []model.ID
	if p != nil {
		for _, s := range p.Stages {
			for _, u := range s.Units {
				stages = append(stages, model.ID(u))
			}
		}
	}
	out := slices.Clone(ps)
	for i, v := range out {
		for _, u := range byID {
			if !slices.ContainsFunc(stages, func(top model.ID) bool { return inside(u.ID, top) }) {
				continue
			}
			if slices.ContainsFunc(u.Relations, func(r model.Relation) bool { return !r.Dead && string(r.To) == v.Unit }) {
				out[i].Tied = true
				break
			}
		}
	}
	return out
}

// participantAdvice names the participants no stage of the primary flow
// reaches. It never fails lint: the reader may know a tie the stored
// relations lack.
func participantAdvice(ps []ParticipantView, names map[string]string) []string {
	var loose []string
	for _, p := range ps {
		if !p.Tied {
			n := names[p.Unit]
			if n == "" {
				n = p.Unit
			}
			loose = append(loose, n)
		}
	}
	if len(loose) == 0 {
		return nil
	}
	return []string{fmt.Sprintf("no unit of a stage of the primary flow (or no primary flow at all) has a relation to the participant %s. Record the relation the flow crosses it by, or drop the mark if the flow does not need it", strings.Join(loose, ", "))}
}
