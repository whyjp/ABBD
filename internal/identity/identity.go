// Package identity keeps design-unit IDs stable across re-extraction.
//
// An extracted unit takes over a stored ID when they share two or more
// anchors, or when their anchor sets are identical. Pairs are assigned
// globally, best match first, so input order never changes the outcome.
// Any leftover overlap between a unit and a stored unit it did not take
// over is recorded as a merge candidate for a human to resolve.
package identity

import (
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
	HighWater  map[model.Kind]int
}

type pair struct {
	in, st    int
	identical bool
	overlap   int
	symdiff   int
}

// Reconcile is pure. renames maps old paths to new paths since the last sync.
// highWater holds the largest number ever minted per kind, so deleted IDs are never reused.
func Reconcile(stored []model.Unit, renames map[string]string, extraction extract.Result, prior []Candidate, highWater map[model.Kind]int) Result {
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
			merge(&units[j], in)
			ids[i] = units[j].ID
			res.Events = append(res.Events, Event{ids[i], Matched})
			continue
		}
		hw[in.Kind]++
		nu := model.Unit{
			ID: model.FormatID(in.Kind, hw[in.Kind]), Kind: in.Kind, Name: in.Name,
			Anchors: in.Anchors, Purpose: in.Purpose, Responsibility: in.Responsibility,
			Signature: in.Signature,
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
	for j := range units {
		units[j].Absent = !stTaken[j]
		if !stTaken[j] {
			res.Unmatched = append(res.Unmatched, units[j].ID)
		}
	}
	res.Units = append(units, created...)
	sort.Slice(res.Units, func(i, j int) bool { return res.Units[i].ID < res.Units[j].ID })
	setRelations(res.Units, input, ids, extraction.Relations)
	res.Candidates = keepLive(append(prior, fresh...), res.Units)
	res.HighWater = hw
	return res
}

// Extraction owns the relations of every unit it reports; units it did not
// report keep theirs until a human resolves them.
func setRelations(units []model.Unit, input []extract.Unit, ids []model.ID, rels []extract.Relation) {
	byKey := map[string]model.ID{}
	for i, in := range input {
		if in.Key != "" {
			byKey[in.Key] = ids[i]
		}
	}
	reported := map[model.ID]bool{}
	for _, id := range ids {
		reported[id] = true
	}
	from := map[model.ID][]model.Relation{}
	for _, r := range rels {
		f := byKey[r.From]
		from[f] = append(from[f], model.Relation{To: byKey[r.To], Kind: r.Kind, Origin: model.Extracted})
	}
	for i := range units {
		if reported[units[i].ID] {
			units[i].Relations = dedupRelations(from[units[i].ID])
		}
	}
}

func dedupRelations(rs []model.Relation) []model.Relation {
	rs = model.SortRelations(rs)
	var out []model.Relation
	for i, r := range rs {
		if i == 0 || r != rs[i-1] {
			out = append(out, r)
		}
	}
	return out
}

// Extraction owns anchors, signatures and EXTRACTED names. Human-given values survive.
func merge(u *model.Unit, in extract.Unit) {
	u.Anchors = in.Anchors
	u.Signature = in.Signature
	if u.Name.Origin == model.Extracted && u.Name.Value != in.Name.Value {
		confirmation := u.Name.Confirmation
		u.Name = in.Name
		if confirmation != "" {
			u.Name.Confirmation = model.Unconfirmed
		}
	}
	if u.Purpose == nil {
		u.Purpose = in.Purpose
	}
	if u.Responsibility == nil {
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
