// Package bootstrap folds the semantic supplement, what pstack's readers
// found with evidence, onto the result of the project extraction script.
package bootstrap

import (
	"fmt"
	"slices"
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
	sc := newScript(slices.Clip(out.Units))
	for n, d := range doc.Units {
		label := fmt.Sprintf("%s %q", d.Kind, d.Name.Value)
		i, err := sc.match(d)
		if len(d.Evidence) > 0 {
			// Evidence says the script cannot see the unit. Folding it into a
			// script unit that shares an anchor would drop it without a word.
			// A unit that names the module it shares with as its parent sits
			// inside that module's anchors, so it stays a unit of its own as
			// long as some anchor of the parent's tells the two apart.
			if err == nil && !(sc.within(i, sc.parentOf(d, doc)) && !sameAnchors(sc.units[i].Anchors, d.Anchors)) {
				return base, f, fmt.Errorf("supplement %s cites evidence, so it is a unit the script cannot see, but it shares an anchor with the script's %s %q and would merge into it; name %q as its parent if it sits inside it, anchor its evidence on code of its own, or drop the evidence if it describes %q", label, out.Units[i].Kind, out.Units[i].Name.Value, out.Units[i].Key, out.Units[i].Name.Value)
			}
			out.Units = append(out.Units, semantic(d, n))
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
		// The code's signature wins. Where the script reads none, the
		// reader's is the only one there is, and it is a guess.
		switch {
		case d.Signature == nil:
		case u.Signature == nil || u.Signature.Writer != model.Script:
			u.Signature = supplemented(d.Signature)
		case u.Signature.Value != d.Signature.Value:
			f.Conflicts = append(f.Conflicts, fmt.Sprintf("supplement %s documents signature %q, code has %q", label, d.Signature.Value, u.Signature.Value))
		}
		if d.Purpose != nil {
			u.Purpose = supplemented(d.Purpose)
		}
		if d.Responsibility != nil {
			u.Responsibility = supplemented(d.Responsibility)
		}
		// A build artifact the script read stays; one only the supplement
		// names, as when no script reads the build files, is added.
		if u.Artifact == nil && d.Artifact != nil {
			u.Artifact = supplemented(d.Artifact)
		}
	}
	if len(gaps) > 0 {
		sort.Strings(gaps)
		return base, f, fmt.Errorf("the supplement names units the script did not report and gives them no evidence; fix the script, or cite the code each one rests on:\n  %s", strings.Join(gaps, "\n  "))
	}
	// A ref names the supplement's unit first, then the script's, so a
	// supplement ref that names another unit hides the script's.
	for ref, i := range docTo {
		if j, ok := sc.key[ref]; ok && j != i {
			f.Conflicts = append(f.Conflicts, fmt.Sprintf("supplement ref %q names %s %q, which shadows the script's ref %q for %s %q", ref, out.Units[i].Kind, out.Units[i].Name.Value, ref, out.Units[j].Kind, out.Units[j].Name.Value))
		}
	}
	resolve := func(ref string) (int, bool) {
		if i, ok := docTo[ref]; ok {
			return i, true
		}
		i, ok := sc.key[ref]
		return i, ok
	}
	// A parent the supplement gives is its own claim, INFERRED: it rests on
	// the parent evidence the reader cites, or on the unit's own evidence.
	// The script's parent, if any, stays beside it.
	for _, d := range doc.Units {
		i, ok := docTo[d.Key]
		if !ok || d.Parent == "" {
			continue
		}
		p, ok := resolve(d.Parent)
		if !ok || !out.Units[p].Kind.Holds() {
			return base, f, fmt.Errorf("supplement %s %q: parent %q is the key of no module or feature in the supplement or the script", d.Kind, d.Name.Value, d.Parent)
		}
		es := d.ParentEvidence
		if len(es) == 0 {
			es = d.Evidence
		}
		if len(es) == 0 {
			return base, f, fmt.Errorf("supplement %s %q puts it in %q without evidence; cite parent_evidence, the code that shows it belongs there", d.Kind, d.Name.Value, d.Parent)
		}
		u := &out.Units[i]
		u.Placed = append(u.Placed, extract.Placement{Key: out.Units[p].Key, Writer: model.Supplement, Evidence: es})
	}

	type edge struct {
		from, to string
		kind     model.RelationKind
	}
	extracted := map[edge]model.Via{}
	for _, r := range out.Relations {
		extracted[edge{r.From, r.To, r.Kind}] = r.Via
	}
	for _, r := range doc.Relations {
		fi, okf := resolve(r.From)
		ti, okt := resolve(r.To)
		if !okf || !okt {
			name := r.To
			if !okf {
				name = r.From
			}
			return base, f, fmt.Errorf("supplement relation %s -%s-> %s names %q, which is neither the supplement's ref nor the script's", r.From, r.Kind, r.To, name)
		}
		if fi == ti {
			return base, f, fmt.Errorf("supplement relation %s -%s-> %s relates %s %q to itself; both refs name the same unit", r.From, r.Kind, r.To, out.Units[fi].Kind, out.Units[fi].Name.Value)
		}
		if err := r.Kind.Joins(out.Units[fi].Kind, out.Units[ti].Kind); err != nil {
			return base, f, fmt.Errorf("supplement relation %s -%s-> %s: %w", r.From, r.Kind, r.To, err)
		}
		e := edge{out.Units[fi].Key, out.Units[ti].Key, r.Kind}
		via, read := extracted[e]
		if !read {
			if len(r.Evidence) == 0 {
				return base, f, fmt.Errorf("supplement relation %s -%s-> %s cites no evidence; a reader's coupling rests on the code it quotes", r.From, r.Kind, r.To)
			}
			out.Relations = append(out.Relations, extract.Relation{From: e.from, To: e.to, Kind: r.Kind, Writer: model.Supplement, Via: r.Via, Evidence: r.Evidence, When: r.When})
		}
		// A phrase only names what crosses the edge, so on an edge the script
		// reads it is all the supplement adds.
		for _, p := range doc.Phrases {
			if p.From == r.From && p.To == r.To && p.Kind == r.Kind {
				if err := model.ValidatePhrase(p.Text, via); err != nil {
					return base, f, fmt.Errorf("supplement relation %s -%s-> %s, on the edge the script reads: %w", r.From, r.Kind, r.To, err)
				}
				out.Phrases = append(out.Phrases, extract.Phrase{From: e.from, To: e.to, Kind: r.Kind, Writer: model.Supplement, Text: p.Text, Evidence: p.Evidence})
			}
		}
	}
	sort.Strings(f.Conflicts)
	return out, f, nil
}

// semantic is a unit only a reader vouches for. Its name is a guess too, and
// its evidence anchors its ID. It keeps the supplement's ref, by which a
// re-run of setup finds it again.
func semantic(d extract.Unit, n int) extract.Unit {
	d.SupplementRef, d.Key = d.Key, fmt.Sprintf("supplement#%d", n)
	if d.SupplementRef != "" {
		d.Key = "supplement:" + d.SupplementRef
	}
	d.Name = model.Said(model.Supplement, d.Name.Value)
	if d.Artifact != nil {
		d.Artifact = supplemented(d.Artifact)
	}
	d.Signature, d.Digest, d.Parent, d.ParentEvidence = nil, "", "", nil
	return d
}

func supplemented(f *model.Fact) *model.Fact {
	s := model.Said(model.Supplement, f.Value)
	return &s
}

// script is the units the extraction script reported, indexed by key.
type script struct {
	units []extract.Unit
	key   map[string]int
}

func newScript(units []extract.Unit) script {
	s := script{units: units, key: map[string]int{}}
	for i, u := range units {
		s.key[u.Key] = i
	}
	return s
}

// within reports whether script unit i is j or one of j's ancestors. A j
// of -1 is no unit.
func (s script) within(i, j int) bool {
	for range s.units {
		if j < 0 {
			return false
		}
		if i == j {
			return true
		}
		p, ok := s.key[s.units[j].Parent]
		if s.units[j].Parent == "" || !ok {
			return false
		}
		j = p
	}
	return false
}

// parentOf is the script unit d names as its parent, by a script key or by
// a supplement unit that restates one, or -1.
func (s script) parentOf(d extract.Unit, doc extract.Result) int {
	if d.Parent == "" {
		return -1
	}
	for _, p := range doc.Units {
		if p.Key == d.Parent {
			if i, err := s.match(p); err == nil && len(p.Evidence) == 0 {
				return i
			}
			return -1
		}
	}
	if i, ok := s.key[d.Parent]; ok {
		return i
	}
	return -1
}

func sameAnchors(a, b []model.Anchor) bool {
	return len(a) == len(b) && overlap(a, b) == len(a)
}

// match finds the script unit of d's kind that shares the most anchors with
// d. A package module anchored on its directory also holds the directory of
// each module inside it, so when the best matches nest, the innermost wins.
func (s script) match(d extract.Unit) (int, error) {
	var best []int
	bestN := 0
	for i, u := range s.units {
		if u.Kind != d.Kind {
			continue
		}
		switch n := overlap(u.Anchors, d.Anchors); {
		case n > bestN:
			best, bestN = []int{i}, n
		case n == bestN && n > 0:
			best = append(best, i)
		}
	}
	if len(best) == 0 {
		return 0, fmt.Errorf("no %s in the code shares an anchor with it", d.Kind)
	}
	for _, c := range best {
		if !slices.ContainsFunc(best, func(o int) bool { return !s.within(o, c) }) {
			return c, nil
		}
	}
	var names []string
	for _, i := range best {
		names = append(names, fmt.Sprintf("%s %q", s.units[i].Kind, s.units[i].Name.Value))
	}
	return 0, fmt.Errorf("its anchors match %s and %s equally; give it an anchor only one of them holds", strings.Join(names[:len(names)-1], ", "), names[len(names)-1])
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
