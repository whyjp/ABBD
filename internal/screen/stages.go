package screen

import "github.com/whyjp/abbd/internal/model"

// PrimaryFlow is the repository's primary flow as graph.json carries it:
// the stack marked primary, its layers the stages in order. A unit whose
// box stands on the flow ranks by its stage (Rank).
type PrimaryFlow struct {
	Key, Question string
	Stages        []Stage
}

// Stage is one stage of the primary flow: its place from 1, its name and
// the units it holds.
type Stage struct {
	N     int
	Name  string
	Units []string
}

// primaryOf is the layering marked the primary flow among stacks, nil for
// none.
func primaryOf(stacks []layering) *layering {
	for i := range stacks {
		if stacks[i].primary {
			return &stacks[i]
		}
	}
	return nil
}

// stagesOn is the stage, from 1, of each box a view draws of the primary
// flow p: the first stage that names the box's unit or a unit inside it. It
// is nil for a nil p, or when the boxes show fewer than two stages, so a
// view with one stage or none lays out as if no flow were stored.
func (p *layering) stagesOn(box func(model.ID) string) map[string]int {
	if p == nil {
		return nil
	}
	out := map[string]int{}
	shown := map[int]bool{}
	for i, us := range p.units {
		for _, u := range us {
			if b := box(u); b != "" && out[b] == 0 {
				out[b], shown[i+1] = i+1, true
			}
		}
	}
	if len(shown) < 2 {
		return nil
	}
	return out
}

// record is the flow as graph.json carries it, nil for a nil p.
func (p *layering) record() *PrimaryFlow {
	if p == nil {
		return nil
	}
	f := &PrimaryFlow{Key: p.key, Question: p.question}
	for i, name := range p.names {
		st := Stage{N: i + 1, Name: name}
		for _, u := range p.units[i] {
			st.Units = append(st.Units, string(u))
		}
		f.Stages = append(f.Stages, st)
	}
	return f
}
