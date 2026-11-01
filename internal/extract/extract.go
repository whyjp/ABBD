// Package extract parses the topology that pstack extracts from code.
// It is the boundary where untrusted model output becomes typed units.
package extract

import (
	"encoding/json"
	"fmt"
	"io"
	"path"
	"path/filepath"
	"strings"

	"github.com/whyjp/abbd/internal/model"
)

type rawUnit struct {
	Key            string           `json:"key,omitempty"`
	Signature      string           `json:"signature,omitempty"`
	Kind           model.Kind       `json:"kind"`
	Name           string           `json:"name"`
	Anchors        []model.Anchor   `json:"anchors"`
	Purpose        string           `json:"purpose,omitempty"`
	Responsibility string           `json:"responsibility,omitempty"`
	Digest         string           `json:"digest,omitempty"`
	Parent         string           `json:"parent,omitempty"`
	ParentEvidence []model.Evidence `json:"parent_evidence,omitempty"`
	Evidence       []model.Evidence `json:"evidence,omitempty"`
}

type rawRelation struct {
	From     string             `json:"from"`
	To       string             `json:"to"`
	Kind     model.RelationKind `json:"kind"`
	Via      model.Via          `json:"via,omitempty"`
	Evidence []model.Evidence   `json:"evidence,omitempty"`
}

type rawResult struct {
	Units     []rawUnit     `json:"units"`
	Relations []rawRelation `json:"relations,omitempty"`
}

// Result is one extraction run. Relations name units by their extractor-local key.
type Result struct {
	Units     []Unit
	Relations []Relation
}

type Relation struct {
	From, To string
	Kind     model.RelationKind
	Origin   model.Origin
	Via      model.Via
	Evidence []model.Evidence
}

// Unit is an extracted design unit before it has an ID.
type Unit struct {
	Key            string
	Signature      *model.Fact
	Kind           model.Kind
	Name           model.Fact
	Anchors        []model.Anchor
	Purpose        *model.Fact
	Responsibility *model.Fact
	Digest         string
	// Parent is the key of the module that contains the unit. A parent the
	// script reports has no ParentOrigin; one a supplement gives is INFERRED
	// and rests on ParentEvidence.
	Parent         string
	ParentOrigin   model.Origin
	ParentEvidence []model.Evidence
	Evidence       []model.Evidence
}

func Parse(r io.Reader) (Result, error) {
	units, raw, err := parseUnits(r)
	if err != nil {
		return Result{}, err
	}
	kinds := map[string]model.Kind{}
	for _, u := range units {
		if u.Key != "" {
			if _, dup := kinds[u.Key]; dup {
				return Result{}, fmt.Errorf("duplicate unit key %q", u.Key)
			}
			kinds[u.Key] = u.Kind
		}
	}
	for _, u := range units {
		if k, ok := kinds[u.Parent]; u.Parent != "" && (!ok || k != model.Module || u.Parent == u.Key) {
			return Result{}, fmt.Errorf("unit %q: parent %q is not the key of another module", u.Name.Value, u.Parent)
		}
		if parentCycle(units, u.Key) {
			return Result{}, fmt.Errorf("unit %q: its parents form a cycle", u.Name.Value)
		}
	}
	res := Result{Units: units}
	for i, rr := range raw.Relations {
		from, okf := kinds[rr.From]
		to, okt := kinds[rr.To]
		switch {
		case !okf || !okt:
			return Result{}, fmt.Errorf("relation %d: %q -> %q names a unit key that does not exist", i, rr.From, rr.To)
		case rr.From == rr.To:
			return Result{}, fmt.Errorf("relation %d: %q relates to itself", i, rr.From)
		}
		if err := rr.Kind.Joins(from, to); err != nil {
			return Result{}, fmt.Errorf("relation %d (%s -> %s): %w", i, rr.From, rr.To, err)
		}
		if err := rr.Via.Validate(); err != nil {
			return Result{}, fmt.Errorf("relation %d (%s -> %s): %w", i, rr.From, rr.To, err)
		}
		evidence, err := NormalizeEvidence(rr.Evidence)
		if err != nil {
			return Result{}, fmt.Errorf("relation %d (%s -> %s): %w", i, rr.From, rr.To, err)
		}
		res.Relations = append(res.Relations, Relation{From: rr.From, To: rr.To, Kind: rr.Kind, Origin: model.Extracted, Via: rr.Via, Evidence: evidence})
	}
	return res, nil
}

func NormalizeEvidence(es []model.Evidence) ([]model.Evidence, error) {
	var out []model.Evidence
	for _, e := range es {
		p, err := normalizePath(e.Path)
		if err != nil {
			return nil, fmt.Errorf("evidence: %w", err)
		}
		e.Path, e.Symbol = p, strings.TrimSpace(e.Symbol)
		if err := e.Validate(); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}

func parseUnits(r io.Reader) ([]Unit, rawResult, error) {
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	var raw rawResult
	if err := dec.Decode(&raw); err != nil {
		return nil, raw, fmt.Errorf("extraction result: %w", err)
	}
	out := make([]Unit, 0, len(raw.Units))
	for i, ru := range raw.Units {
		if err := ru.Kind.Validate(); err != nil {
			return nil, raw, fmt.Errorf("unit %d: %w", i, err)
		}
		if ru.Name == "" {
			return nil, raw, fmt.Errorf("unit %d: name is required", i)
		}
		for j, a := range ru.Anchors {
			p, err := normalizePath(a.Path)
			if err != nil {
				return nil, raw, fmt.Errorf("unit %d (%s): %w", i, ru.Name, err)
			}
			ru.Anchors[j] = model.Anchor{Path: p, Symbol: strings.TrimSpace(a.Symbol)}
		}
		evidence, err := NormalizeEvidence(ru.Evidence)
		if err != nil {
			return nil, raw, fmt.Errorf("unit %d (%s): %w", i, ru.Name, err)
		}
		parentEvidence, err := NormalizeEvidence(ru.ParentEvidence)
		if err != nil {
			return nil, raw, fmt.Errorf("unit %d (%s) parent: %w", i, ru.Name, err)
		}
		// Evidence locates a unit the script cannot see, so it anchors the unit's ID.
		for _, e := range evidence {
			ru.Anchors = append(ru.Anchors, e.Anchor())
		}
		anchors := model.SortAnchors(ru.Anchors)
		if len(anchors) == 0 {
			return nil, raw, fmt.Errorf("unit %d (%s): at least one anchor is required", i, ru.Name)
		}
		var sig *model.Fact
		if ru.Signature != "" {
			if !ru.Kind.IsContract() {
				return nil, raw, fmt.Errorf("unit %d (%s): only contracts carry a signature", i, ru.Name)
			}
			sig = &model.Fact{Value: ru.Signature, Origin: model.Extracted}
		}
		out = append(out, Unit{
			Key:            ru.Key,
			Signature:      sig,
			Kind:           ru.Kind,
			Name:           model.Fact{Value: ru.Name, Origin: model.Extracted},
			Anchors:        anchors,
			Purpose:        inferred(ru.Purpose),
			Responsibility: inferred(ru.Responsibility),
			Digest:         ru.Digest,
			Parent:         ru.Parent,
			ParentEvidence: parentEvidence,
			Evidence:       evidence,
		})
	}
	return out, raw, nil
}

// Anchors must compare equal to the forward-slash, root-relative paths git reports.
func normalizePath(p string) (string, error) {
	p = strings.TrimSpace(strings.ReplaceAll(p, `\`, "/"))
	if p == "" {
		return "", fmt.Errorf("anchor path is required")
	}
	if path.IsAbs(p) || filepath.IsAbs(p) {
		return "", fmt.Errorf("anchor path %q must be relative to the project root", p)
	}
	p = path.Clean(p)
	if p == ".." || strings.HasPrefix(p, "../") {
		return "", fmt.Errorf("anchor path %q leaves the project root", p)
	}
	return p, nil
}

// Extraction reads code, so any statement of intent it makes is a guess.
func inferred(v string) *model.Fact {
	if v == "" {
		return nil
	}
	return &model.Fact{Value: v, Origin: model.Inferred}
}

// parentCycle reports whether following parents from key ever comes back.
func parentCycle(units []Unit, key string) bool {
	parent := map[string]string{}
	for _, u := range units {
		parent[u.Key] = u.Parent
	}
	seen := map[string]bool{}
	for k := key; k != ""; k = parent[k] {
		if seen[k] {
			return true
		}
		seen[k] = true
	}
	return false
}
