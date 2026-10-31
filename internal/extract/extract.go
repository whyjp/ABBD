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
	Key            string         `json:"key,omitempty"`
	Signature      string         `json:"signature,omitempty"`
	Kind           model.Kind     `json:"kind"`
	Name           string         `json:"name"`
	Anchors        []model.Anchor `json:"anchors"`
	Purpose        string         `json:"purpose,omitempty"`
	Responsibility string         `json:"responsibility,omitempty"`
}

type rawRelation struct {
	From string             `json:"from"`
	To   string             `json:"to"`
	Kind model.RelationKind `json:"kind"`
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
}

func Parse(r io.Reader) (Result, error) {
	units, raw, err := parseUnits(r)
	if err != nil {
		return Result{}, err
	}
	keys := map[string]bool{}
	for _, u := range units {
		if u.Key != "" {
			if keys[u.Key] {
				return Result{}, fmt.Errorf("duplicate unit key %q", u.Key)
			}
			keys[u.Key] = true
		}
	}
	res := Result{Units: units}
	for i, rr := range raw.Relations {
		if err := rr.Kind.Validate(); err != nil {
			return Result{}, fmt.Errorf("relation %d: %w", i, err)
		}
		if !keys[rr.From] || !keys[rr.To] {
			return Result{}, fmt.Errorf("relation %d: %q -> %q names a unit key that does not exist", i, rr.From, rr.To)
		}
		if rr.From == rr.To {
			return Result{}, fmt.Errorf("relation %d: %q relates to itself", i, rr.From)
		}
		res.Relations = append(res.Relations, Relation{rr.From, rr.To, rr.Kind})
	}
	return res, nil
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
	return &model.Fact{Value: v, Origin: model.Inferred, Confirmation: model.Unconfirmed}
}
