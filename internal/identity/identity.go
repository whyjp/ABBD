// Package identity keeps design-unit IDs stable across re-extraction.
//
// An extracted unit takes over a stored ID when they share two or more
// anchors, or when their anchor sets are identical. Pairs are assigned
// globally, best match first, so input order never changes the outcome.
// Any leftover overlap between a unit and a stored unit it did not take
// over is recorded as a merge candidate for a human to resolve.
package identity

import (
	"slices"
	"sort"
	"strings"

	"github.com/whyjp/abbd/internal/extract"
	"github.com/whyjp/abbd/internal/model"
)

type Candidate struct {
	New      model.ID     `yaml:"new"`
	Existing model.ID     `yaml:"existing"`
	Shared   model.Anchor `yaml:"shared"`
}

type Outcome string

const (
	Matched Outcome = "matched"
	Created Outcome = "new"
)

type Event struct {
	ID      model.ID
	Outcome Outcome
}

type Result struct {
	Units      []model.Unit
	Candidates []Candidate
	Events     []Event
	Unmatched  []model.ID
	// Stated are the semantic units a setup's supplement no longer reports
	// that stay present because a human stated something about them.
	Stated    []model.ID
	HighWater map[model.Kind]int
}

// Guesses says what happens to the stored INFERRED facts.
type Guesses int

const (
	// KeepGuesses is a re-extraction by the script, which cannot see them.
	KeepGuesses Guesses = iota
	// ReplaceGuesses is a setup, whose supplement owns them: a guess the new
	// supplement no longer makes is withdrawn. What a human stated stays.
	ReplaceGuesses
)

type pair struct {
	in, st    int
	identical bool
	overlap   int
	symdiff   int
}

// Reconcile is pure. renames maps old paths to new paths since the last sync.
// highWater holds the largest number ever minted per kind, so deleted IDs are never reused.
func Reconcile(stored []model.Unit, renames map[string]string, extraction extract.Result, prior []Candidate, highWater map[model.Kind]int, guesses Guesses) Result {
	hw := map[model.Kind]int{}
	for k, n := range highWater {
		hw[k] = n
	}
	units := make([]model.Unit, len(stored))
	for i, u := range stored {
		u.Anchors = applyRenames(u.Anchors, renames)
		units[i] = u
		if _, n, err := model.ParseID(string(u.ID)); err == nil && n > hw[u.Kind] {
			hw[u.Kind] = n
		}
	}
	sort.Slice(units, func(i, j int) bool { return units[i].ID < units[j].ID })
	prior = keepLive(prior, units)

	input := append([]extract.Unit(nil), extraction.Units...)
	sort.SliceStable(input, func(i, j int) bool {
		if input[i].Kind != input[j].Kind {
			return input[i].Kind < input[j].Kind
		}
		return anchorList(input[i].Anchors) < anchorList(input[j].Anchors)
	})

	var pairs []pair
	for i, in := range input {
		for j, u := range units {
			if u.Kind != in.Kind {
				continue
			}
			ov := overlap(u.Anchors, in.Anchors)
			same := ov > 0 && identical(u.Anchors, in.Anchors)
			if ov >= 2 || same {
				pairs = append(pairs, pair{i, j, same, ov, len(u.Anchors) + len(in.Anchors) - 2*ov})
			}
		}
	}
	sort.SliceStable(pairs, func(a, b int) bool {
		p, q := pairs[a], pairs[b]
		switch {
		case p.identical != q.identical:
			return p.identical
		case p.overlap != q.overlap:
			return p.overlap > q.overlap
		case p.symdiff != q.symdiff:
			return p.symdiff < q.symdiff
		case p.st != q.st:
			return p.st < q.st
		default:
			return p.in < q.in
		}
	})
	inTo := map[int]int{}
	stTaken := map[int]bool{}
	for _, p := range pairs {
		if _, done := inTo[p.in]; done || stTaken[p.st] {
			continue
		}
		inTo[p.in] = p.st
		stTaken[p.st] = true
	}

	before := make([][]model.Anchor, len(units))
	for j, u := range units {
		before[j] = u.Anchors
	}

	var res Result
	var created []model.Unit
	ids := make([]model.ID, len(input))
	for i, in := range input {
		if j, ok := inTo[i]; ok {
			merge(&units[j], in, guesses)
			ids[i] = units[j].ID
			res.Events = append(res.Events, Event{ids[i], Matched})
			continue
		}
		hw[in.Kind]++
		nu := model.Unit{
			ID: model.FormatID(in.Kind, hw[in.Kind]), Kind: in.Kind, Name: in.Name,
			Anchors: in.Anchors, Purpose: in.Purpose, Responsibility: in.Responsibility,
			Signature: in.Signature, Digest: in.Digest, Evidence: in.Evidence,
		}
		created = append(created, nu)
		ids[i] = nu.ID
		res.Events = append(res.Events, Event{nu.ID, Created})
	}

	var fresh []Candidate
	for i, in := range input {
		_, matched := inTo[i]
		for j, u := range units {
			if u.Kind != in.Kind || u.ID == ids[i] {
				continue
			}
			// A matched unit only links to stored units left without a match: a merge.
			if matched && stTaken[j] {
				continue
			}
			if overlap(before[j], in.Anchors) > 0 {
				fresh = append(fresh, Candidate{New: ids[i], Existing: u.ID, Shared: shared(before[j], in.Anchors)})
			}
		}
	}
	held := heldByHuman(units, stTaken)
	for j, u := range units {
		switch {
		case stTaken[j] || u.Semantic() && guesses == KeepGuesses:
			units[j].Absent = false
		case u.Semantic() && held[u.ID]:
			units[j].Absent = false
			res.Stated = append(res.Stated, u.ID)
		default:
			units[j].Absent = true
			res.Unmatched = append(res.Unmatched, u.ID)
		}
	}
	res.Units = append(units, created...)
	sort.Slice(res.Units, func(i, j int) bool { return res.Units[i].ID < res.Units[j].ID })
	setParents(res.Units, input, ids)
	setRelations(res.Units, input, ids, extraction.Relations, guesses)
	res.Candidates = keepLive(append(prior, fresh...), res.Units)
	res.HighWater = hw
	return res
}

// heldByHuman lists the stored semantic units a human stated something
// about: a fact, a relation of their own, or a relation from a unit that
// stays. A setup's supplement owns the guesses, but withdrawing such a unit
// would leave the human's words where no command reaches them.
func heldByHuman(units []model.Unit, matched map[int]bool) map[model.ID]bool {
	stays := map[model.ID]bool{}
	for j, u := range units {
		if matched[j] {
			stays[u.ID] = true
		}
	}
	held := map[model.ID]bool{}
	for grew := true; grew; {
		grew = false
		pointed := map[model.ID]bool{}
		for _, u := range units {
			if stays[u.ID] {
				for _, r := range u.Relations {
					if r.Origin == model.UserStated {
						pointed[r.To] = true
					}
				}
			}
		}
		for _, u := range units {
			if !stays[u.ID] && u.Semantic() && (stated(u) || pointed[u.ID]) {
				stays[u.ID], held[u.ID], grew = true, true, true
			}
		}
	}
	return held
}

func stated(u model.Unit) bool {
	for _, f := range []*model.Fact{&u.Name, u.Purpose, u.Responsibility, u.Signature} {
		if f != nil && f.Origin == model.UserStated {
			return true
		}
	}
	return slices.ContainsFunc(u.Relations, func(r model.Relation) bool { return r.Origin == model.UserStated })
}

func keyed(input []extract.Unit, ids []model.ID) map[string]model.ID {
	byKey := map[string]model.ID{}
	for i, in := range input {
		if in.Key != "" {
			byKey[in.Key] = ids[i]
		}
	}
	return byKey
}

// setParents gives each reported unit the module its extraction names, by
// key. A unit the extraction places nowhere keeps a parent a reader or a
// human gave it, so structure a reader found survives a script that cannot
// see it; a parent the script gave earlier goes when the script drops it.
// A kept parent that would close a cycle with the script's parents is
// dropped, since the script's word on code it reads wins.
func setParents(units []model.Unit, input []extract.Unit, ids []model.ID) {
	byKey := keyed(input, ids)
	type placed struct {
		id       model.ID
		origin   model.Origin
		evidence []model.Evidence
	}
	parent := map[model.ID]placed{}
	for i, in := range input {
		parent[ids[i]] = placed{byKey[in.Parent], in.ParentOrigin, in.ParentEvidence}
	}
	var kept []int
	for i := range units {
		u := &units[i]
		p, ok := parent[u.ID]
		switch {
		case !ok:
		case p.id != "":
			u.Parent, u.ParentOrigin, u.ParentEvidence, u.ParentSource = p.id, p.origin, p.evidence, ""
			if p.origin == model.Extracted {
				u.ParentOrigin = ""
			}
		case u.Parent != "" && (u.ParentOrigin == model.Inferred || u.ParentOrigin == model.UserStated):
			kept = append(kept, i)
		default:
			u.Parent, u.ParentOrigin, u.ParentEvidence, u.ParentSource = "", "", nil, ""
		}
	}
	for _, i := range kept {
		if model.ParentCycle(units) == "" {
			break
		}
		u := &units[i]
		u.Parent, u.ParentOrigin, u.ParentEvidence, u.ParentSource = "", "", nil, ""
	}
}

// Extraction owns the EXTRACTED relations of every unit it reports; units it
// did not report keep theirs until a human resolves them. A relation from any
// other origin survives, because the script cannot see it, unless guesses are
// replaced. When the same edge arrives twice, the extracted one wins, then the
// stored one.
func setRelations(units []model.Unit, input []extract.Unit, ids []model.ID, rels []extract.Relation, guesses Guesses) {
	byKey := keyed(input, ids)
	reported := map[model.ID]bool{}
	for _, id := range ids {
		reported[id] = true
	}
	extracted := map[model.ID][]model.Relation{}
	stated := map[model.ID][]model.Relation{}
	for _, r := range rels {
		origin := r.Origin
		if origin == "" {
			origin = model.Extracted
		}
		rel := model.Relation{To: byKey[r.To], Kind: r.Kind, Origin: origin, Via: r.Via, Evidence: r.Evidence}
		f := byKey[r.From]
		if origin == model.Extracted {
			extracted[f] = append(extracted[f], rel)
		} else {
			stated[f] = append(stated[f], rel)
		}
	}
	for i := range units {
		id := units[i].ID
		if !reported[id] {
			continue
		}
		next := extracted[id]
		for _, r := range units[i].Relations {
			if r.Origin != model.Extracted && !(guesses == ReplaceGuesses && r.Guess()) {
				next = append(next, r)
			}
		}
		units[i].Relations = dedupRelations(append(next, stated[id]...))
	}
}

func dedupRelations(rs []model.Relation) []model.Relation {
	type edge struct {
		to   model.ID
		kind model.RelationKind
	}
	seen := map[edge]bool{}
	var out []model.Relation
	for _, r := range rs {
		if e := (edge{r.To, r.Kind}); !seen[e] {
			seen[e] = true
			out = append(out, r)
		}
	}
	return model.SortRelations(out)
}

// Extraction owns anchors, signatures and EXTRACTED names. Human-given values
// and decided guesses survive.
func merge(u *model.Unit, in extract.Unit, guesses Guesses) {
	u.Anchors = in.Anchors
	u.Signature = in.Signature
	u.Digest = in.Digest
	u.Evidence = in.Evidence
	// A unit moves between the script and the reader whole: once the script
	// reports a semantic unit its name is extracted, and the other way round.
	if u.Semantic() || in.Name.Origin == model.Inferred {
		u.Name = in.Name
	}
	if u.Name.Origin == model.Extracted {
		u.Name = in.Name
	}
	replace := func(f *model.Fact) bool { return f == nil || (guesses == ReplaceGuesses && f.Guess()) }
	if replace(u.Purpose) {
		u.Purpose = in.Purpose
	}
	if replace(u.Responsibility) {
		u.Responsibility = in.Responsibility
	}
}

func applyRenames(as []model.Anchor, renames map[string]string) []model.Anchor {
	out := make([]model.Anchor, len(as))
	for i, a := range as {
		if to, ok := renames[a.Path]; ok {
			a.Path = to
		}
		out[i] = a
	}
	return model.SortAnchors(out)
}

func anchorList(as []model.Anchor) string {
	ks := make([]string, len(as))
	for i, a := range as {
		ks[i] = a.Key()
	}
	return strings.Join(ks, "\x00")
}

func keys(as []model.Anchor) map[string]bool {
	m := make(map[string]bool, len(as))
	for _, a := range as {
		m[a.Key()] = true
	}
	return m
}

func overlap(a, b []model.Anchor) int {
	ka, n := keys(a), 0
	for _, x := range b {
		if ka[x.Key()] {
			n++
		}
	}
	return n
}

func identical(a, b []model.Anchor) bool {
	return len(a) == len(b) && overlap(a, b) == len(a)
}

func shared(a, b []model.Anchor) model.Anchor {
	ka := keys(a)
	for _, x := range b {
		if ka[x.Key()] {
			return x
		}
	}
	return model.Anchor{}
}

func keepLive(cs []Candidate, units []model.Unit) []Candidate {
	live := map[model.ID]bool{}
	for _, u := range units {
		live[u.ID] = true
	}
	seen := map[Candidate]bool{}
	var out []Candidate
	for _, c := range cs {
		if live[c.New] && live[c.Existing] && !seen[c] {
			seen[c] = true
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].New != out[j].New {
			return out[i].New < out[j].New
		}
		return out[i].Existing < out[j].Existing
	})
	return out
}
