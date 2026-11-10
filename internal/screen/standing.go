package screen

import (
	"github.com/whyjp/abbd/internal/model"
)

// Standing is what a module's page says of the module as it is now: the
// contracts it provides and uses, the units outside it it depends on with
// no contract between, whether its text is STALE, how many of its notes
// are STALE, and how many evidence items inside it are dead, counted as
// abbd status counts them.
type Standing struct {
	Provides, Uses, Direct int
	TextStale              bool `json:",omitempty"`
	StaleNotes, Dead       int
}

// standing counts a view's standing over its whole subtree.
func (o *overview) standing(id model.ID, in Input) *Standing {
	t := o.tree
	inside := map[model.ID]bool{}
	for _, u := range t.subtree(id) {
		inside[u] = true
	}
	provides, uses, direct := map[model.ID]bool{}, map[model.ID]bool{}, map[model.ID]bool{}
	s := &Standing{}
	for u := range inside {
		shown := t.units[u]
		shown.EachShownEvidence(func(_ model.Writer, ev *model.Evidence) {
			if ev.Status == model.Dead {
				s.Dead++
			}
		})
		for _, r := range t.units[u].Relations {
			if r.Dead {
				continue
			}
			if inside[r.To] {
				continue
			}
			switch {
			case r.Kind == model.Provides:
				provides[r.To] = true
			case t.units[r.To].Kind.IsContract():
				uses[r.To] = true
			case r.Kind == model.DependsOn:
				direct[t.outside(r.To, id)] = true
			}
		}
		for _, n := range in.Notes[u] {
			// As status does, a note no slot holds now is set aside.
			if n.Misfiled || !n.Known || !t.units[u].Kind.HoldsNotes() || n.Ref.Unheld(t.units[u]) {
				continue
			}
			if n.Stale() {
				s.StaleNotes++
			}
		}
	}
	s.Provides, s.Uses, s.Direct = len(provides), len(uses), len(direct)
	if u, ok := in.Understandings[id]; ok {
		s.TextStale = u.Digest != in.Digests[id]
	}
	return s
}
