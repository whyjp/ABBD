// Package bootstrap folds documents that are not code onto the result of the
// project extraction script, and renders what setup found.
package bootstrap

import (
	"fmt"
	"sort"
	"strings"

	"github.com/whyjp/abbd/internal/extract"
	"github.com/whyjp/abbd/internal/model"
)

// Source says which document an overlay comes from. It fixes the origin of
// every fact the document adds and whether a mismatch is fatal.
type Source struct {
	Name   string
	Origin model.Origin
	// Strict sources come from the same agent that wrote the script, so a
	// unit the script did not report means the script is incomplete.
	Strict bool
	// Names says whether the document may rename units.
	Names bool
}

var (
	Supplement = Source{Name: "supplement", Origin: model.Inferred, Strict: true}
	Adopt      = Source{Name: "adopt", Origin: model.Adopted, Names: true}
)

// Findings lists what a human should look at. Conflicts are claims the code
// contradicts; gaps are things one side has and the other lacks.
type Findings struct {
	Matched   int
	Conflicts []string
	Gaps      []string
}

// Overlay matches each document unit to the script unit of the same kind that
// shares the most anchors, then adds the document's why-fields and the
// couplings the script did not extract. Code facts (anchors, signatures)
// always come from the script. A guessing source may also add a unit the
// script cannot see, when the unit carries evidence ABBD can check.
func Overlay(base, doc extract.Result, src Source) (extract.Result, Findings, error) {
	out := extract.Result{Units: append([]extract.Unit(nil), base.Units...), Relations: append([]extract.Relation(nil), base.Relations...)}
	for i := range out.Units {
		if out.Units[i].Key == "" {
			out.Units[i].Key = fmt.Sprintf("#%d", i)
		}
	}
	var f Findings
	docTo := map[string]int{}
	covered := map[int]bool{}
	scriptUnits := len(out.Units)
	for n, d := range doc.Units {
		label := fmt.Sprintf("%s %q", d.Kind, d.Name.Value)
		i, err := match(out.Units[:scriptUnits], d)
		if err != nil && src.Origin == model.Inferred && len(d.Evidence) > 0 {
			out.Units = append(out.Units, semantic(d, fmt.Sprintf("%s#%d", src.Name, n)))
			i, err = len(out.Units)-1, nil
		}
		if err != nil {
			f.Gaps = append(f.Gaps, fmt.Sprintf("%s %s: %v", src.Name, label, err))
			continue
		}
		if covered[i] {
			f.Conflicts = append(f.Conflicts, fmt.Sprintf("%s %s describes the same code as another %s unit", src.Name, label, src.Name))
			continue
		}
		covered[i] = true
		if d.Key != "" {
			docTo[d.Key] = i
		}
		f.Matched++
		u := &out.Units[i]
		if src.Names && d.Name.Value != u.Name.Value {
			u.Name = model.Fact{Value: d.Name.Value, Origin: src.Origin}
		}
		if d.Signature != nil && (u.Signature == nil || u.Signature.Value != d.Signature.Value) {
			f.Conflicts = append(f.Conflicts, fmt.Sprintf("%s %s documents signature %q, code has %q", src.Name, label, d.Signature.Value, sig(u.Signature)))
		}
		if d.Purpose != nil {
			u.Purpose = &model.Fact{Value: d.Purpose.Value, Origin: src.Origin, Confirmation: unconfirmed(src.Origin)}
		}
		if d.Responsibility != nil {
			u.Responsibility = &model.Fact{Value: d.Responsibility.Value, Origin: src.Origin, Confirmation: unconfirmed(src.Origin)}
		}
	}
	if src.Strict && len(f.Gaps) > 0 {
		return base, f, fmt.Errorf("the %s names units the script did not report and gives them no evidence; fix the script, or cite the code each one rests on:\n  %s", src.Name, strings.Join(f.Gaps, "\n  "))
	}
	for _, d := range doc.Units {
		if i, ok := docTo[d.Key]; ok && d.Parent != "" {
			if p, ok := docTo[d.Parent]; ok {
				out.Units[i].Parent = out.Units[p].Key
			}
		}
	}
	if !src.Strict {
		for i, u := range out.Units {
			if !covered[i] {
				f.Gaps = append(f.Gaps, fmt.Sprintf("code %s %q is not in the %s document", u.Kind, u.Name.Value, src.Name))
			}
		}
	}

	type edge struct {
		from, to string
		kind     model.RelationKind
	}
	extracted := map[edge]bool{}
	earlier := map[edge]int{}
	for i, r := range out.Relations {
		e := edge{r.From, r.To, r.Kind}
		if r.Origin == model.Extracted {
			extracted[e] = true
		} else {
			earlier[e] = i
		}
	}
	baseKey := func(i int) string { return out.Units[i].Key }
	for _, r := range doc.Relations {
		fi, okf := docTo[r.From]
		ti, okt := docTo[r.To]
		if !okf || !okt {
			f.Gaps = append(f.Gaps, fmt.Sprintf("%s relation %s -%s-> %s names a unit that matched no code", src.Name, r.From, r.Kind, r.To))
			continue
		}
		e := edge{baseKey(fi), baseKey(ti), r.Kind}
		if extracted[e] {
			continue
		}
		if !src.Strict {
			f.Conflicts = append(f.Conflicts, fmt.Sprintf("%s relation %s -%s-> %s is not in the code; queued for confirmation", src.Name, r.From, r.Kind, r.To))
		}
		// An earlier overlay guessed the same edge; this document's origin replaces the guess.
		if i, ok := earlier[e]; ok {
			out.Relations[i].Origin = src.Origin
			continue
		}
		out.Relations = append(out.Relations, extract.Relation{From: e.from, To: e.to, Kind: r.Kind, Origin: src.Origin, Via: r.Via, Evidence: r.Evidence})
	}
	sort.Strings(f.Conflicts)
	sort.Strings(f.Gaps)
	return out, f, nil
}

// semantic is a unit only a reader vouches for. Its name is a guess too, and
// its evidence anchors its ID.
func semantic(d extract.Unit, key string) extract.Unit {
	d.Key = key
	d.Name = model.Fact{Value: d.Name.Value, Origin: model.Inferred, Confirmation: model.Unconfirmed}
	d.Signature, d.Digest, d.Parent = nil, "", ""
	return d
}

// The three-axis vocabulary keeps a guess in the confirm queue; a reconstructed
// document is already a human's intent.
func unconfirmed(o model.Origin) model.Confirmation {
	if o == model.Inferred {
		return model.Unconfirmed
	}
	return ""
}

func match(units []extract.Unit, d extract.Unit) (int, error) {
	best, bestN, tie := -1, 0, false
	for i, u := range units {
		if u.Kind != d.Kind {
			continue
		}
		n := overlap(u.Anchors, d.Anchors)
		switch {
		case n > bestN:
			best, bestN, tie = i, n, false
		case n == bestN && n > 0:
			tie = true
		}
	}
	switch {
	case best < 0:
		return 0, fmt.Errorf("no %s in the code shares an anchor with it", d.Kind)
	case tie:
		return 0, fmt.Errorf("its anchors match more than one %s equally", d.Kind)
	}
	return best, nil
}

func overlap(a, b []model.Anchor) int {
	keys := map[string]bool{}
	for _, x := range a {
		keys[x.Key()] = true
	}
	n := 0
	for _, x := range b {
		if keys[x.Key()] {
			n++
		}
	}
	return n
}

func sig(f *model.Fact) string {
	if f == nil {
		return ""
	}
	return f.Value
}
