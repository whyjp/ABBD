// Package extract parses the topology that pstack extracts from code.
// It is the boundary where untrusted model output becomes typed units.
package extract

import (
	"cmp"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/whyjp/abbd/internal/model"
)

type rawUnit struct {
	// Ref is the extraction's own name for the unit. Key is its older
	// spelling, which gitleaks' generic-api-key rule reads as a secret when
	// the value is a path with enough entropy; both stay accepted.
	Ref            string           `json:"ref,omitempty"`
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
	Entries        []model.Entry    `json:"entries,omitempty"`
}

type rawRelation struct {
	From     string             `json:"from"`
	To       string             `json:"to"`
	Kind     model.RelationKind `json:"kind"`
	Via      model.Via          `json:"via,omitempty"`
	Evidence []model.Evidence   `json:"evidence,omitempty"`
	Entries  []string           `json:"entries,omitempty"`
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
	Writer   model.Writer
	Via      model.Via
	Evidence []model.Evidence
	// Entries are the keys of the entries of To the relation narrows to.
	Entries []string
}

// Placement is one writer's claim on the unit, by key, that contains another.
type Placement struct {
	Key      string
	Writer   model.Writer
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
	Entries        []model.Entry
	// Parent is the key of the module or feature the input says contains the
	// unit, and ParentEvidence what a supplement cites for it. The script's
	// parent is the script's claim; a supplement's becomes a Placement.
	Parent         string
	ParentEvidence []model.Evidence
	// Placed are the parent claims other writers add, such as the supplement's.
	Placed   []Placement
	Evidence []model.Evidence
	// SupplementRef is the supplement's own ref for a unit only it reports.
	SupplementRef string
}

func Parse(r io.Reader) (Result, error) { return parse(r, false) }

// ParseSupplement reads a semantic supplement. Its units may name a module
// only the script reports as their parent, by the script's key, so a parent
// key the supplement does not hold is left for the overlay to resolve.
func ParseSupplement(r io.Reader) (Result, error) { return parse(r, true) }

func parse(r io.Reader, foreignParents bool) (Result, error) {
	units, raw, err := parseUnits(r)
	if err != nil {
		return Result{}, err
	}
	kinds := map[string]model.Kind{}
	entries := map[string][]model.Entry{}
	for _, u := range units {
		if u.Key != "" {
			if _, dup := kinds[u.Key]; dup {
				return Result{}, fmt.Errorf("duplicate unit key %q", u.Key)
			}
			kinds[u.Key] = u.Kind
			entries[u.Key] = u.Entries
		}
		if foreignParents && len(u.Entries) > 0 {
			return Result{}, fmt.Errorf("supplement unit %q lists entries; entries are what the code spells, so the extraction script reports them", u.Name.Value)
		}
	}
	for _, u := range units {
		k, ok := kinds[u.Parent]
		if u.Parent != "" && (ok && !k.Holds() || !ok && !foreignParents || u.Parent == u.Key) {
			return Result{}, fmt.Errorf("unit %q: parent %q is not the key of another module or feature", u.Name.Value, u.Parent)
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
		keys, err := entryKeys(rr.Entries, entries[rr.To])
		if err != nil {
			return Result{}, fmt.Errorf("relation %d (%s -> %s): %w", i, rr.From, rr.To, err)
		}
		// One claim holds an edge, so the entries of two lines of one edge add
		// up, and a line that names none uses the whole contract.
		if j := slices.IndexFunc(res.Relations, func(o Relation) bool { return o.From == rr.From && o.To == rr.To && o.Kind == rr.Kind }); j >= 0 {
			if had := res.Relations[j].Entries; len(had) == 0 || len(keys) == 0 {
				res.Relations[j].Entries = nil
			} else {
				res.Relations[j].Entries = slices.Compact(slices.Sorted(slices.Values(append(had, keys...))))
			}
			continue
		}
		res.Relations = append(res.Relations, Relation{From: rr.From, To: rr.To, Kind: rr.Kind, Writer: model.Script, Via: rr.Via, Evidence: evidence, Entries: keys})
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
		if ru.Ref != "" && ru.Key != "" && ru.Ref != ru.Key {
			return nil, raw, fmt.Errorf("unit %d (%s): \"ref\" %q and \"key\" %q name it twice; key is the older spelling of ref, so keep one", i, ru.Name, ru.Ref, ru.Key)
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
			f := model.Said(model.Script, ru.Signature)
			sig = &f
		}
		es, err := normalizeEntries(ru.Kind, ru.Entries)
		if err != nil {
			return nil, raw, fmt.Errorf("unit %d (%s): %w", i, ru.Name, err)
		}
		out = append(out, Unit{
			Key:            cmp.Or(ru.Ref, ru.Key),
			Entries:        es,
			Signature:      sig,
			Kind:           ru.Kind,
			Name:           model.Said(model.Script, ru.Name),
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

// normalizeEntries gives each entry its key, from its name unless the
// extraction names one, and keeps the code's order.
func normalizeEntries(k model.Kind, es []model.Entry) ([]model.Entry, error) {
	if len(es) == 0 {
		return nil, nil
	}
	if !k.IsContract() {
		return nil, fmt.Errorf("only contracts carry entries")
	}
	out := make([]model.Entry, len(es))
	for i, e := range es {
		e.Name, e.Shape, e.Key = strings.TrimSpace(e.Name), strings.TrimSpace(e.Shape), strings.TrimSpace(e.Key)
		if e.Key == "" {
			e.Key = model.EntryKey(e.Name)
		}
		out[i] = e
	}
	return out, model.ValidateEntries(out)
}

// entryKeys reads the entries a relation narrows to, each by its name or its
// key, as the keys of the target's entries.
func entryKeys(names []string, of []model.Entry) ([]string, error) {
	var out []string
	for _, n := range names {
		i := slices.IndexFunc(of, func(e model.Entry) bool { return e.Name == n })
		if i < 0 {
			i = slices.IndexFunc(of, func(e model.Entry) bool { return e.Key == n })
		}
		if i < 0 {
			return nil, fmt.Errorf("its target has no entry %q", n)
		}
		out = append(out, of[i].Key)
	}
	slices.Sort(out)
	return slices.Compact(out), nil
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

// Extraction reads code, so any statement of intent it makes is a guess, and
// it ranks with the supplement's.
func inferred(v string) *model.Fact {
	if v == "" {
		return nil
	}
	f := model.Said(model.Supplement, v)
	return &f
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
