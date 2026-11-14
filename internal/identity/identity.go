// Package identity keeps design-unit IDs stable across re-extraction.
//
// A supplement unit takes over the stored unit with the same supplement key.
// Any other extracted unit takes over a stored ID when they share two or
// more anchors, or when their anchor sets are identical. Pairs are assigned
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
	// Stated are the supplement units a setup no longer reports that stay
	// present because a human stated something about them.
	Stated []model.ID
	// Keys maps each extracted unit's key to the ID it took.
	Keys      map[string]model.ID
	HighWater map[model.Kind]int
}

// Writers names the writers an extraction speaks for: their claims on the
// units it reports are replaced by its own, and no one else's are.
type Writers []model.Writer

var (
	// Sync is a re-extraction by the script.
	Sync = Writers{model.Script}
	// Setup is the script with the supplement folded in.
	Setup = Writers{model.Script, model.Supplement}
)

func (ws Writers) own(w model.Writer) bool { return slices.Contains(ws, w) }

type pair struct {
	in, st    int
	identical bool
	overlap   int
	symdiff   int
	// foreign marks a script unit paired with a stored unit only the
	// supplement reports, as a grouping that quotes its member's file.
	foreign bool
}

// Reconcile is pure. renames maps old paths to new paths since the last sync.
// highWater holds the largest number ever minted per kind, so deleted IDs are never reused.
func Reconcile(stored []model.Unit, renames map[string]string, extraction extract.Result, prior []Candidate, highWater map[model.Kind]int, ws Writers) Result {
	hw := map[model.Kind]int{}
	for k, n := range highWater {
		hw[k] = n
	}
	units := make([]model.Unit, len(stored))
	for i, u := range stored {
		u.Anchors = applyRenames(u.Anchors, renames)
		u.Parents = clonedParents(u.Parents)
		u.Evidence = slices.Clone(u.Evidence)
		u.Relations, u.Hidden = clonedRelations(u.Relations), clonedRelations(u.Hidden)
		u.Phrases = slices.Clone(u.Phrases)
		for j := range u.Phrases {
			u.Phrases[j].Evidence = slices.Clone(u.Phrases[j].Evidence)
		}
		u.EachEvidence(func(_ model.Writer, e *model.Evidence) {
			if to, ok := renames[e.Path]; ok {
				e.Path = to
			}
		})
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

	inTo := map[int]int{}
	stTaken := map[int]bool{}
	for i, in := range input {
		if in.SupplementRef == "" {
			continue
		}
		for j, u := range units {
			if !stTaken[j] && u.Kind == in.Kind && u.SupplementRef == in.SupplementRef {
				inTo[i], stTaken[j] = j, true
				break
			}
		}
	}
	var pairs []pair
	for i, in := range input {
		if _, done := inTo[i]; done {
			continue
		}
		for j, u := range units {
			if u.Kind != in.Kind || stTaken[j] {
				continue
			}
			ov := overlap(u.Anchors, in.Anchors)
			same := ov > 0 && identical(u.Anchors, in.Anchors)
			if ov >= 2 || same {
				pairs = append(pairs, pair{i, j, same, ov, len(u.Anchors) + len(in.Anchors) - 2*ov, u.SupplementRef != ""})
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
		case p.foreign != q.foreign:
			return !p.foreign
		case p.st != q.st:
			return p.st < q.st
		default:
			return p.in < q.in
		}
	})
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
			merge(&units[j], in, ws)
			ids[i] = units[j].ID
			res.Events = append(res.Events, Event{ids[i], Matched})
			continue
		}
		hw[in.Kind]++
		nu := model.Unit{
			ID: model.FormatID(in.Kind, hw[in.Kind]), Kind: in.Kind, Name: in.Name,
			SupplementRef: in.SupplementRef, Anchors: in.Anchors, Purpose: in.Purpose,
			Responsibility: in.Responsibility, Signature: in.Signature, Digest: in.Digest, Artifact: in.Artifact, Run: in.Run, Entries: in.Entries,
			Evidence: in.Evidence,
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
	held := heldByHuman(units)
	for j, u := range units {
		switch {
		case stTaken[j]:
			units[j].SetAbsent("")
		case !ws.own(u.Name.Writer):
		case u.Absent:
			res.Unmatched = append(res.Unmatched, u.ID)
		case u.Name.Writer == model.Supplement && held[u.ID]:
			res.Stated = append(res.Stated, u.ID)
		case u.Name.Writer == model.Supplement:
			units[j].SetAbsent(model.Withdrawn)
			res.Unmatched = append(res.Unmatched, u.ID)
		default:
			units[j].SetAbsent(model.NotReported)
			res.Unmatched = append(res.Unmatched, u.ID)
		}
	}
	res.Units = append(units, created...)
	sort.Slice(res.Units, func(i, j int) bool { return res.Units[i].ID < res.Units[j].ID })
	res.Keys = keyed(input, ids)
	setParents(res.Units, input, ids, ws)
	setRelations(res.Units, input, ids, extraction.Relations, ws)
	setPhrases(res.Units, input, ids, extraction.Phrases, ws)
	model.Resolve(res.Units)
	res.Candidates = keepLive(append(prior, fresh...), res.Units)
	res.HighWater = hw
	return res
}

// heldByHuman lists the units a human stated something about, or that a
// human's relation points at. Withdrawing such a supplement unit would leave
// the human's words where no command reaches them.
func heldByHuman(units []model.Unit) map[model.ID]bool {
	held := map[model.ID]bool{}
	for _, u := range units {
		for _, f := range []*model.Fact{&u.Name, u.Purpose, u.Responsibility, u.Signature} {
			if f != nil && f.Writer == model.Human {
				held[u.ID] = true
			}
		}
		if _, ok := u.Claim(model.Human); ok {
			held[u.ID] = true
		}
		for _, r := range u.Claims() {
			if r.Writer == model.Human {
				held[u.ID], held[r.To] = true, true
			}
		}
	}
	return held
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

// setParents replaces the parent claims the extraction's writers hold on
// each unit it reports with the claims it makes: the script's parent and
// the supplement's placements. Every other claim stays stored, and Resolve
// decides which one shows.
func setParents(units []model.Unit, input []extract.Unit, ids []model.ID, ws Writers) {
	byKey := keyed(input, ids)
	at := map[model.ID]int{}
	for i, u := range units {
		at[u.ID] = i
	}
	for i, in := range input {
		u := &units[at[ids[i]]]
		old := *u
		u.DropParents(ws...)
		if p := byKey[in.Parent]; in.Parent != "" && p != "" {
			u.PutParent(model.ParentClaim{To: p, Writer: model.Script})
		}
		for _, pl := range in.Placed {
			if p := byKey[pl.Key]; p != "" && ws.own(pl.Writer) {
				was, _ := old.Claim(pl.Writer)
				u.PutParent(model.ParentClaim{To: p, Writer: pl.Writer, Evidence: carry(pl.Evidence, was.Evidence, items(old))})
			}
		}
	}
}

// setRelations replaces the claims the extraction's writers hold on the
// relations of each unit it reports with the claims it makes. Units it did
// not report keep theirs until a human resolves them, and every other
// writer's claim stays.
func setRelations(units []model.Unit, input []extract.Unit, ids []model.ID, rels []extract.Relation, ws Writers) {
	byKey := keyed(input, ids)
	reported := map[model.ID]bool{}
	for _, id := range ids {
		reported[id] = true
	}
	made := map[model.ID][]model.Relation{}
	for _, r := range rels {
		w := r.Writer
		if w == "" {
			w = model.Script
		}
		f := byKey[r.From]
		made[f] = append(made[f], model.Relation{To: byKey[r.To], Kind: r.Kind, Writer: w, Via: r.Via, Evidence: r.Evidence, Entries: r.Entries, When: r.When})
	}
	for i := range units {
		u := &units[i]
		if !reported[u.ID] {
			continue
		}
		old, all := u.Claims(), items(*u)
		u.DropRelations(func(r model.Relation) bool { return ws.own(r.Writer) })
		for _, r := range made[u.ID] {
			if slices.ContainsFunc(u.Claims(), func(o model.Relation) bool { return o.To == r.To && o.Kind == r.Kind && o.Writer == r.Writer }) {
				continue
			}
			if r.Writer != model.Script {
				var was []model.Evidence
				for _, o := range old {
					if o.To == r.To && o.Kind == r.Kind && o.Writer == r.Writer {
						was = o.Evidence
					}
				}
				r.Evidence = carry(r.Evidence, was, all)
			}
			u.PutRelation(r)
		}
	}
}

// setPhrases replaces the phrase notes the extraction's writers hold on the
// units it reports with the ones it gives, as setRelations does for claims.
func setPhrases(units []model.Unit, input []extract.Unit, ids []model.ID, phrases []extract.Phrase, ws Writers) {
	byKey := keyed(input, ids)
	reported := map[model.ID]bool{}
	for _, id := range ids {
		reported[id] = true
	}
	made := map[model.ID][]model.PhraseNote{}
	for _, p := range phrases {
		f := byKey[p.From]
		made[f] = append(made[f], model.PhraseNote{To: byKey[p.To], Kind: p.Kind, Writer: p.Writer, Text: p.Text, Evidence: p.Evidence})
	}
	for i := range units {
		u := &units[i]
		if !reported[u.ID] {
			continue
		}
		old, all := slices.Clone(u.Phrases), items(*u)
		u.DropNotes(func(n model.PhraseNote) bool { return ws.own(n.Writer) })
		for _, n := range made[u.ID] {
			var was []model.Evidence
			for _, o := range old {
				if o.To == n.To && o.Kind == n.Kind && o.Writer == n.Writer {
					was = o.Evidence
				}
			}
			n.Evidence = carry(n.Evidence, was, all)
			u.PutNote(n)
		}
	}
}

// carry gives each new item the status a stored item citing the same code
// had, so restating a claim never revives its dead items. It looks in each
// list in turn: the claim's own items first, then the rest of the unit's,
// since a status belongs to the code an item cites, not to one claim.
func carry(next []model.Evidence, old ...[]model.Evidence) []model.Evidence {
	out := slices.Clone(next)
	for i := range out {
		for _, es := range old {
			if j := slices.IndexFunc(es, out[i].Same); j >= 0 {
				out[i].Lines, out[i].Status, out[i].Since, out[i].Reason = es[j].Lines, es[j].Status, es[j].Since, es[j].Reason
				break
			}
		}
	}
	return out
}

// items lists every evidence item the unit stores.
func items(u model.Unit) []model.Evidence {
	var out []model.Evidence
	u.EachEvidence(func(_ model.Writer, e *model.Evidence) { out = append(out, *e) })
	return out
}

// merge folds an extracted unit into the stored unit it took over. A
// one-value field is replaced when a writer the extraction speaks for held
// it, or when the extraction's writer outranks the one that did. The unit's
// evidence goes with its name.
func merge(u *model.Unit, in extract.Unit, ws Writers) {
	u.Anchors = in.Anchors
	u.Digest = in.Digest
	u.Entries = in.Entries
	if in.SupplementRef != "" {
		u.SupplementRef = in.SupplementRef
	}
	if ws.own(u.Name.Writer) || model.Outranks(model.NameField, in.Name.Writer, u.Name.Writer) {
		u.Name = in.Name
		u.Evidence = carry(in.Evidence, u.Evidence, items(*u))
		if u.Name.Writer == model.Script {
			u.Evidence = in.Evidence
		}
	}
	for _, f := range []struct {
		field  model.Field
		at     **model.Fact
		offers *model.Fact
	}{
		{model.PurposeField, &u.Purpose, in.Purpose},
		{model.ResponsibilityField, &u.Responsibility, in.Responsibility},
		{model.SignatureField, &u.Signature, in.Signature},
		{model.ArtifactField, &u.Artifact, in.Artifact},
		{model.RunField, &u.Run, in.Run},
	} {
		held := *f.at
		if held == nil || ws.own(held.Writer) || f.offers != nil && model.Outranks(f.field, f.offers.Writer, held.Writer) {
			*f.at = f.offers
		}
	}
}

func clonedParents(cs []model.ParentClaim) []model.ParentClaim {
	out := slices.Clone(cs)
	for i := range out {
		out[i].Evidence = slices.Clone(out[i].Evidence)
	}
	return out
}

func clonedRelations(rs []model.Relation) []model.Relation {
	out := slices.Clone(rs)
	for i := range out {
		out[i].Evidence = slices.Clone(out[i].Evidence)
	}
	return out
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
