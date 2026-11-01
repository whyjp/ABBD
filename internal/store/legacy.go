package store

import (
	"gopkg.in/yaml.v3"

	"github.com/whyjp/abbd/internal/model"
)

// The old format stored an origin instead of a writer, one parent slot, and
// a dead flag on relations. It stays readable for good, because abbd diff
// reads the state committed at any older revision.

type legacyFact struct {
	Value  string       `yaml:"value"`
	Origin model.Origin `yaml:"origin"`
	Source string       `yaml:"source,omitempty"`
}

type legacyRelation struct {
	To       model.ID           `yaml:"to"`
	Kind     model.RelationKind `yaml:"kind"`
	Origin   model.Origin       `yaml:"origin"`
	Source   string             `yaml:"source,omitempty"`
	Via      model.Via          `yaml:"via,omitempty"`
	Evidence []model.Evidence   `yaml:"evidence,omitempty"`
	Dead     bool               `yaml:"dead,omitempty"`
	Digest   string             `yaml:"digest,omitempty"`
}

type legacyUnit struct {
	ID             model.ID         `yaml:"id"`
	Kind           model.Kind       `yaml:"kind"`
	Name           legacyFact       `yaml:"name"`
	Anchors        []model.Anchor   `yaml:"anchors"`
	Parent         model.ID         `yaml:"parent,omitempty"`
	ParentOrigin   model.Origin     `yaml:"parent_origin,omitempty"`
	ParentSource   string           `yaml:"parent_source,omitempty"`
	ParentEvidence []model.Evidence `yaml:"parent_evidence,omitempty"`
	Evidence       []model.Evidence `yaml:"evidence,omitempty"`
	Purpose        *legacyFact      `yaml:"purpose,omitempty"`
	Responsibility *legacyFact      `yaml:"responsibility,omitempty"`
	Signature      *legacyFact      `yaml:"signature,omitempty"`
	Digest         string           `yaml:"digest,omitempty"`
	Relations      []legacyRelation `yaml:"relations,omitempty"`
	Absent         bool             `yaml:"absent,omitempty"`
}

// legacy reports whether frontmatter is in the old format: its name has an
// origin and no writer.
func legacy(front []byte) bool {
	var probe struct {
		Name struct {
			Writer string `yaml:"writer"`
			Origin string `yaml:"origin"`
		} `yaml:"name"`
	}
	return yaml.Unmarshal(front, &probe) == nil && probe.Name.Writer == "" && probe.Name.Origin != ""
}

// DeadBeforeMigration is why an item the old format held dead stays dead.
const DeadBeforeMigration = "dead before migration"

// writerOf maps an old origin to a writer. An INFERRED value may be the
// supplement's or a reader's; the old format cannot tell, so it is the
// supplement's until abbd migrate classifies it.
func writerOf(o model.Origin) model.Writer {
	switch o {
	case model.UserStated:
		return model.Human
	case model.Inferred:
		return model.Supplement
	}
	return model.Script
}

func (f *legacyFact) convert() *model.Fact {
	if f == nil {
		return nil
	}
	return &model.Fact{Value: f.Value, Writer: writerOf(f.Origin), Source: f.Source}
}

// groupMarker is the anchor symbol of a group group.Fallback made.
const groupMarker = "(group)"

func (l legacyUnit) convert() model.Unit {
	u := model.Unit{
		ID: l.ID, Kind: l.Kind, Name: *l.Name.convert(), Anchors: l.Anchors,
		Evidence: l.Evidence, Purpose: l.Purpose.convert(), Responsibility: l.Responsibility.convert(),
		Signature: l.Signature.convert(), Digest: l.Digest,
	}
	made := l.Kind == model.Module && len(l.Anchors) == 1 && l.Anchors[0].Symbol == groupMarker
	if made {
		u.Name.Writer = model.Group
	}
	if l.Parent != "" {
		u.Parents = []model.ParentClaim{{To: l.Parent, Writer: writerOf(l.ParentOrigin), Source: l.ParentSource, Evidence: l.ParentEvidence}}
	}
	for _, r := range l.Relations {
		es := append([]model.Evidence(nil), r.Evidence...)
		if r.Dead {
			for i := range es {
				es[i].Status, es[i].Reason = model.Dead, DeadBeforeMigration
			}
		}
		u.Relations = append(u.Relations, model.Relation{To: r.To, Kind: r.Kind, Writer: writerOf(r.Origin), Source: r.Source, Via: r.Via, Evidence: es, Digest: r.Digest})
	}
	if l.Absent {
		switch {
		case made:
			u.SetAbsent(model.Unneeded)
		case u.Semantic():
			u.SetAbsent(model.EvidenceDead)
		default:
			u.SetAbsent(model.NotReported)
		}
	}
	return u
}

// regroup gives the group writer the script parents that point at a group:
// the old format stored them as the script's.
func regroup(units []model.Unit, old map[model.ID]bool) {
	made := map[model.ID]bool{}
	for _, u := range units {
		if u.Name.Writer == model.Group {
			made[u.ID] = true
		}
	}
	for i := range units {
		if !old[units[i].ID] {
			continue
		}
		for j, c := range units[i].Parents {
			if c.Writer == model.Script && (made[c.To] || made[units[i].ID]) {
				units[i].Parents[j].Writer = model.Group
			}
		}
	}
}
