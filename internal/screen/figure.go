package screen

import (
	"fmt"
	"slices"

	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/store"
)

// A figure answers one question in the drawing its kind gives it, such as a
// flow's numbered steps. Every kind names stored units, rests on quoted code
// and stores no unit and no relation, so what the figures share is drawn
// here: their anchors, the marks of a human's word, and the list of figures
// each module's inside shows.

func figureID[C model.Claim[C]](k model.FigureKey) string {
	return model.KindOf[C]().Noun + "-" + string(k)
}

// partID is a part's anchor. A key holds no dot, so figure a's part 1 and
// figure a-1 never share one.
func partID[C model.Claim[C]](k model.FigureKey, n int) string {
	return fmt.Sprintf("%s.%d", figureID[C](k), n)
}

// sharedSource is the source every part of f was stated in, or "".
func sharedSource[C model.Claim[C]](f store.Figure[C]) string {
	for _, p := range f.Parts {
		if p.Source == "" || p.Source != f.Parts[0].Source {
			return ""
		}
	}
	if len(f.Parts) == 0 {
		return ""
	}
	return f.Parts[0].Source
}

// partMarks are a part's badge and source where they differ from its figure's,
// so a mixed figure marks the parts a human stated.
func partMarks(st store.Stamp, badge, source string) (string, string) {
	b := Badge(st.Writer.Origin())
	if b == badge {
		b = ""
	}
	if st.Source == source {
		return b, ""
	}
	return b, st.Source
}

// linkFigures lists each figure on every inside whose subtree holds a unit
// one of its parts is on or holds, with those parts' numbers.
func (o *overview) linkFigures(m *Model) {
	for i := range m.Insides {
		ins := &m.Insides[i]
		held := map[string]bool{}
		for _, id := range o.tree.subtree(model.ID(ins.ID)) {
			held[string(id)] = true
		}
		for _, f := range m.Flows {
			ref := FlowRef{ID: f.ID, Question: f.Question}
			for _, l := range f.Lanes {
				for _, s := range l.Steps {
					if held[l.Unit] {
						ref.Steps = append(ref.Steps, s.N)
					}
				}
			}
			if len(ref.Steps) > 0 {
				ins.Flows = append(ins.Flows, ref)
			}
		}
		for _, s := range m.Stacks {
			ref := StackRef{ID: s.ID, Question: s.Question}
			for id, n := range s.held {
				if held[string(id)] && !slices.Contains(ref.Layers, n) {
					ref.Layers = append(ref.Layers, n)
				}
			}
			slices.Sort(ref.Layers)
			if len(ref.Layers) > 0 {
				ins.Stacks = append(ins.Stacks, ref)
			}
		}
	}
}
