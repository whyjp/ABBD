package ssot

import (
	"fmt"
	"strconv"

	"github.com/whyjp/abbd/internal/model"
)

// Mark is one meaning field as the engine last wrote it: its value and the provenance only the
// user's word raises (origin USER_STATED, confirmation CONFIRMED, an answered decision, an
// approved tier C card).
type Mark struct {
	Value        string             `yaml:"value"`
	Origin       model.Origin       `yaml:"origin,omitempty"`
	Confirmation model.Confirmation `yaml:"confirmation,omitempty"`
}

// Raised: the mark says the user stated or confirmed it.
func (m Mark) Raised() bool { return m.Origin == model.UserStated || m.Confirmation == model.Confirmed }

func (m Mark) lowered() Mark {
	if m.Origin == model.UserStated {
		m.Origin = model.Inferred
	}
	if m.Confirmation == model.Confirmed {
		m.Confirmation = model.Unconfirmed
	}
	return m
}

type markRef struct {
	get func() Mark
	set func(Mark)
}

func semanticRef(sem *model.Semantic) markRef {
	return markRef{
		get: func() Mark { return Mark{sem.Value, sem.Origin, sem.Confirmation} },
		set: func(m Mark) { sem.Value, sem.Origin, sem.Confirmation = m.Value, m.Origin, m.Confirmation },
	}
}

// marks lists every meaning field by a stable key.
func (s *Snapshot) marks() map[string]markRef {
	out := map[string]markRef{}
	for id, f := range s.Features {
		out[id+".purpose"] = semanticRef(&f.Intent.Purpose)
		for i := range f.Intent.Invariants {
			inv := &f.Intent.Invariants[i]
			out[fmt.Sprintf("%s.invariants[%d]", id, i)] = markRef{
				get: func() Mark { return Mark{inv.Claim, inv.Origin, inv.Confirmation} },
				set: func(m Mark) { inv.Claim, inv.Origin, inv.Confirmation = m.Value, m.Origin, m.Confirmation },
			}
		}
	}
	for id, m := range s.Modules {
		out[id+".responsibility"] = semanticRef(&m.Responsibility)
		for i := range m.ExternalContracts {
			c := &m.ExternalContracts[i]
			out[c.ID+".compatibility"] = semanticRef(&c.Compatibility)
			out[c.ID+".external_consumers"] = markRef{
				get: func() Mark {
					v := ""
					if c.ExternalConsumers != nil {
						v = strconv.FormatBool(*c.ExternalConsumers)
					}
					return Mark{Value: v, Origin: c.ConsumersOrigin}
				},
				set: func(mk Mark) {
					c.ExternalConsumers, c.ConsumersOrigin = nil, mk.Origin
					if b, err := strconv.ParseBool(mk.Value); err == nil {
						c.ExternalConsumers = &b
					}
				},
			}
		}
	}
	for id, d := range s.Decisions {
		out[id+".rationale"] = semanticRef(&d.Rationale)
		out[id+".answer"] = markRef{
			get: func() Mark { return Mark{Value: d.Chosen, Confirmation: d.Confirmation} },
			set: func(m Mark) {
				d.Chosen, d.Confirmation, d.Answered = m.Value, m.Confirmation, m.Confirmation == model.Confirmed
			},
		}
	}
	for id, r := range s.Requirements {
		out[id+".statement"] = semanticRef(&r.Statement)
	}
	for _, c := range s.Changes {
		for i := range c.StructuralDeltas {
			d := &c.StructuralDeltas[i]
			if d.Tier != model.TierC {
				continue
			}
			out[d.ID+".status"] = markRef{
				get: func() Mark {
					m := Mark{Value: string(d.Status)}
					if d.Status == model.DeltaApproved {
						m.Confirmation = model.Confirmed
					}
					return m
				},
				set: func(m Mark) {
					if m.Value == string(model.DeltaApproved) && m.Confirmation != model.Confirmed {
						m.Value = string(model.DeltaPending)
					}
					d.Status = model.DeltaStatus(m.Value)
					if d.Status == model.DeltaPending {
						d.Decision = nil
					}
				},
			}
		}
	}
	return out
}

func (s *Snapshot) recordMarks() {
	s.State.Marks = map[string]Mark{}
	for k, r := range s.marks() {
		s.State.Marks[k] = r.get()
	}
}

// holdHandRaises runs on load when the md originals were edited by hand. A meaning field whose
// user-only provenance the edit raised, or whose user-stated value it changed, is not trusted:
// the engine's view keeps what it last wrote (a field it never wrote is lowered to INFERRED /
// UNCONFIRMED), and the typed value waits in State.HeldEdits for `abbd check --accept-edits`.
func (s *Snapshot) holdHandRaises() {
	if s.State.Marks == nil {
		return // written before the ledger existed: nothing to compare with
	}
	for k, r := range s.marks() {
		cur := r.get()
		rec, known := s.State.Marks[k]
		if cur == rec || !cur.Raised() && !(known && rec.Raised()) {
			continue
		}
		if s.State.HeldEdits == nil {
			s.State.HeldEdits = map[string]Mark{}
		}
		s.State.HeldEdits[k] = cur
		if known {
			r.set(rec)
		} else {
			r.set(cur.lowered())
		}
	}
}

// AcceptHeldEdits applies the held hand edits. The user's acceptance keeps them as typed; any
// other acceptance never raises provenance: a user-stated field keeps its recorded value and a
// raised one is lowered.
func (s *Snapshot) AcceptHeldEdits(byUser bool) {
	refs := s.marks()
	for k, held := range s.State.HeldEdits {
		r, ok := refs[k]
		if !ok {
			continue
		}
		switch rec, known := s.State.Marks[k]; {
		case byUser:
			r.set(held)
		case known && rec.Raised():
			r.set(rec)
		default:
			r.set(held.lowered())
		}
	}
	s.State.HeldEdits = nil
}
