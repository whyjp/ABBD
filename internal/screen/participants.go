package screen

import (
	"fmt"
	"maps"
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

// Outside is one external unit the overview record says takes part in the
// primary flow, drawn in a dotted box on the edge of the picture where the
// flow starts, outside the system's boxes: a platform the flow hands work to
// or the library that implements a stage. It is no box of the overview, so
// the layout, the lines and the count of boxes leave it out. It draws no
// line; Reached says which stages reach it.
type Outside struct {
	Name    string
	Kind    string
	Title   string
	Href    string
	Reached []string `json:",omitempty"`
}

// outsideOf lists the participant marks that stand and name a unit the state
// still holds, each with the stages of the primary flow whose units, or
// units inside them, reach it by a live relation.
func outsideOf(ps []ParticipantView, p *PrimaryFlow, units []model.Unit) []Outside {
	var out []Outside
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
	for _, v := range ps {
		x, ok := byID[model.ID(v.Unit)]
		if !ok || len(v.Reasons) > 0 {
			continue
		}
		o := Outside{Name: x.Name.Value, Kind: string(x.Kind), Href: "#u-" + v.Unit}
		if o.Name == "" {
			o.Name = v.Unit
		}
		if p != nil {
			for _, s := range p.Stages {
				for _, id := range slices.Sorted(maps.Keys(byID)) {
					if !slices.ContainsFunc(s.Units, func(top string) bool { return inside(id, model.ID(top)) }) {
						continue
					}
					if slices.ContainsFunc(byID[id].Relations, func(r model.Relation) bool { return !r.Dead && string(r.To) == v.Unit }) {
						o.Reached = append(o.Reached, fmt.Sprintf("%d단계 %s", s.N, s.Name))
						break
					}
				}
			}
		}
		o.Title = o.Name + " (저장소 밖의 단위)"
		if len(o.Reached) > 0 {
			o.Title += "\n주 흐름이 닿는 곳: " + strings.Join(o.Reached, ", ") + ". 선은 그리지 않고, 그 단계 상자의 초점 뷰가 모두 그립니다"
		} else {
			o.Title += "\n주 흐름의 어느 단계도 저장된 관계로 닿지 않습니다"
		}
		out = append(out, o)
	}
	return out
}

// outsideLeftOut lists under the picture, as lines it leaves out, the stages
// each outside unit is reached from.
func outsideLeftOut(os []Outside) []LeftOut {
	var out []LeftOut
	for _, o := range os {
		if len(o.Reached) > 0 {
			out = append(out, LeftOut{Name: o.Name, Href: o.Href, With: o.Reached})
		}
	}
	return out
}
