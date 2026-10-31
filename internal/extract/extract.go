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
	Kind           model.Kind     `json:"kind"`
	Name           string         `json:"name"`
	Anchors        []model.Anchor `json:"anchors"`
	Purpose        string         `json:"purpose,omitempty"`
	Responsibility string         `json:"responsibility,omitempty"`
}

type rawResult struct {
	Units []rawUnit `json:"units"`
}

// Unit is an extracted design unit before it has an ID.
type Unit struct {
	Kind           model.Kind
	Name           model.Fact
	Anchors        []model.Anchor
	Purpose        *model.Fact
	Responsibility *model.Fact
}

func Parse(r io.Reader) ([]Unit, error) {
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	var raw rawResult
	if err := dec.Decode(&raw); err != nil {
		return nil, fmt.Errorf("extraction result: %w", err)
	}
	out := make([]Unit, 0, len(raw.Units))
	for i, ru := range raw.Units {
		if err := ru.Kind.Validate(); err != nil {
			return nil, fmt.Errorf("unit %d: %w", i, err)
		}
		if ru.Name == "" {
			return nil, fmt.Errorf("unit %d: name is required", i)
		}
		for j, a := range ru.Anchors {
			p, err := normalizePath(a.Path)
			if err != nil {
				return nil, fmt.Errorf("unit %d (%s): %w", i, ru.Name, err)
			}
			ru.Anchors[j] = model.Anchor{Path: p, Symbol: strings.TrimSpace(a.Symbol)}
		}
		anchors := model.SortAnchors(ru.Anchors)
		if len(anchors) == 0 {
			return nil, fmt.Errorf("unit %d (%s): at least one anchor is required", i, ru.Name)
		}
		out = append(out, Unit{
			Kind:           ru.Kind,
			Name:           model.Fact{Value: ru.Name, Origin: model.Extracted},
			Anchors:        anchors,
			Purpose:        inferred(ru.Purpose),
			Responsibility: inferred(ru.Responsibility),
		})
	}
	return out, nil
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
