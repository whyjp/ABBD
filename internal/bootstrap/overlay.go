// Package bootstrap folds the semantic supplement, what pstack's readers
// found with evidence, onto the result of the project extraction script.
package bootstrap

import (
	"fmt"
	"sort"
	"strings"

	"github.com/whyjp/abbd/internal/extract"
	"github.com/whyjp/abbd/internal/model"
)

// Findings lists what the supplement said that the code contradicts.
type Findings struct {
	Matched   int
	Conflicts []string
}

// Overlay matches each supplement unit to the script unit of the same kind
// that shares the most anchors, then adds the supplement's why-fields and the
// couplings the script did not extract. Code facts (anchors, signatures)
// always come from the script. A supplement unit with evidence is one the
// script cannot see, and is added as a unit of its own.
func Overlay(base, doc extract.Result) (extract.Result, Findings, error) {
	out := extract.Result{Units: append([]extract.Unit(nil), base.Units...), Relations: append([]extract.Relation(nil), base.Relations...)}
	for i := range out.Units {
		if out.Units[i].Key == "" {
			out.Units[i].Key = fmt.Sprintf("#%d", i)
		}
	}
	var f Findings
	var gaps []string
	docTo := map[string]int{}
	covered := map[int]bool{}
	scriptUnits := len(out.Units)
	for n, d := range doc.Units {
		label := fmt.Sprintf("%s %q", d.Kind, d.Name.Value)
		i, err := match(out.Units[:scriptUnits], d)
		if len(d.Evidence) > 0 {
			// Evidence says the script cannot see the unit. Folding it into a
			// script unit that shares an anchor would drop it without a word.
			if err == nil {
				return base, f, fmt.Errorf("supplement %s cites evidence, so it is a unit the script cannot see, but it shares an anchor with the script's %s %q and would merge into it; anchor its evidence on code of its own, or drop the evidence if it describes %q", label, out.Units[i].Kind, out.Units[i].Name.Value, out.Units[i].Name.Value)
			}
			out.Units = append(out.Units, semantic(d, fmt.Sprintf("supplement#%d", n)))
			i, err = len(out.Units)-1, nil
		}
		if err != nil {
			gaps = append(gaps, fmt.Sprintf("supplement %s: %v", label, err))
			continue
		}
		if covered[i] {
			f.Conflicts = append(f.Conflicts, fmt.Sprintf("supplement %s describes the same code as another supplement unit", label))
			continue
		}
		covered[i] = true
		if d.Key != "" {
			docTo[d.Key] = i
		}
		f.Matched++
		u := &out.Units[i]
		if d.Signature != nil && (u.Signature == nil || u.Signature.Value != d.Signature.Value) {
			f.Conflicts = append(f.Conflicts, fmt.Sprintf("supplement %s documents signature %q, code has %q", label, d.Signature.Value, sig(u.Signature)))
		}
		if d.Purpose != nil {
			u.Purpose = &model.Fact{Value: d.Purpose.Value, Origin: model.Inferred}
		}
		if d.Responsibility != nil {
			u.Responsibility = &model.Fact{Value: d.Responsibility.Value, Origin: model.Inferred}
		}
	}
	if len(gaps) > 0 {
		sort.Strings(gaps)
		return base, f, fmt.Errorf("the supplement names units the script did not report and gives them no evidence; fix the script, or cite the code each one rests on:\n  %s", strings.Join(gaps, "\n  "))
	}
	scriptKey := map[string]int{}
	for i, u := range out.Units[:scriptUnits] {
		scriptKey[u.Key] = i
	}
	for _, d := range doc.Units {
		i, ok := docTo[d.Key]
		if !ok || d.Parent == "" {
			continue
		}
		p, ok := docTo[d.Parent]
		if !ok {
			p, ok = scriptKey[d.Parent]
		}
		if !ok || out.Units[p].Kind != model.Module {
			return base, f, fmt.Errorf("supplement %s %q: parent %q is the key of no module in the supplement or the script", d.Kind, d.Name.Value, d.Parent)
		}
		out.Units[i].Parent = out.Units[p].Key
	}

	type edge struct {
		from, to string
		kind     model.RelationKind
	}
	extracted := map[edge]bool{}
	for _, r := range out.Relations {
		extracted[edge{r.From, r.To, r.Kind}] = true
	}
	for _, r := range doc.Relations {
		fi, okf := docTo[r.From]
		ti, okt := docTo[r.To]
		if !okf || !okt {
			return base, f, fmt.Errorf("supplement relation %s -%s-> %s names a unit that matched no code", r.From, r.Kind, r.To)
		}
		e := edge{out.Units[fi].Key, out.Units[ti].Key, r.Kind}
		if extracted[e] {
			continue
		}
		if len(r.Evidence) == 0 {
			return base, f, fmt.Errorf("supplement relation %s -%s-> %s cites no evidence; a reader's coupling rests on the code it quotes", r.From, r.Kind, r.To)
		}
		out.Relations = append(out.Relations, extract.Relation{From: e.from, To: e.to, Kind: r.Kind, Origin: model.Inferred, Via: r.Via, Evidence: r.Evidence})
	}
	sort.Strings(f.Conflicts)
	return out, f, nil
}

// semantic is a unit only a reader vouches for. Its name is a guess too, and
// its evidence anchors its ID.
func semantic(d extract.Unit, key string) extract.Unit {
	d.Key = key
	d.Name = model.Fact{Value: d.Name.Value, Origin: model.Inferred}
	d.Signature, d.Digest, d.Parent = nil, "", ""
	return d
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
