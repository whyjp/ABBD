package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/whyjp/abbd/internal/bootstrap"
	"github.com/whyjp/abbd/internal/evidence"
	"github.com/whyjp/abbd/internal/extract"
	"github.com/whyjp/abbd/internal/identity"
	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/store"
)

const defaultSupplement = ".abbd/extractor/supplement.json"

// migrate converts an old-format state once. The old format stored INFERRED
// for both the supplement's and a reader's claims, so it asks the supplement
// which claims are its own: a claim is the reader's when the supplement makes
// none on the same target. It keeps today's dead items dead, and it writes
// nothing when the graph would change.
func (e env) migrate() error {
	units, old, err := e.st.Legacy()
	if err != nil {
		return err
	}
	if len(old) == 0 {
		fmt.Fprintln(e.stdout, "the state is already in the current format")
		return nil
	}
	path := cmpOr(e.supplement, defaultSupplement)
	full := path
	if !filepath.IsAbs(full) {
		full = filepath.Join(e.root, path)
	}
	if _, err := os.Stat(full); err != nil {
		return fmt.Errorf("no supplement at %s; abbd migrate needs the supplement setup used, to tell its claims from a reader's; pass --supplement FILE", path)
	}
	p, err := e.st.LoadProject()
	if err != nil {
		return err
	}
	if len(p.Extract) == 0 {
		return fmt.Errorf("project.yaml names no extraction script")
	}
	out, err := e.runScript(p.Extract)
	if err != nil {
		return err
	}
	ex, err := extract.Parse(bytes.NewReader(out))
	if err != nil {
		return fmt.Errorf("extraction script output: %w", err)
	}
	doc, err := e.readSupplement(path)
	if err != nil {
		return fmt.Errorf("supplement: %w", err)
	}
	over, _, err := bootstrap.Overlay(ex, doc)
	if err != nil {
		return err
	}
	state, err := e.st.LoadState()
	if err != nil {
		return err
	}
	c := supplementClaims(units, over, state.HighWater)

	before := slices.Clone(units)
	cloneClaims(before)
	killed := markDead(before, evidence.New(e.root))
	model.Resolve(before)

	migrated := slices.Clone(units)
	cloneClaims(migrated)
	var tally classification
	for i := range migrated {
		if old[migrated[i].ID] {
			c.classify(&migrated[i], &tally)
		}
	}
	markDead(migrated, evidence.New(e.root))
	model.Resolve(migrated)

	if diff := graphDiff(graph(before), graph(migrated)); len(diff) > 0 {
		return fmt.Errorf("the migration would change the graph, so nothing was written:\n  %s", strings.Join(diff, "\n  "))
	}
	changed := graphDiff(graph(units), graph(before))
	if err := e.st.SaveUnits(migrated); err != nil {
		return err
	}
	fmt.Fprintf(e.stdout, "migrated %d unit file(s) with %s\n", len(old), path)
	tally.print(e.stdout)
	if c.unpaired > 0 {
		fmt.Fprintf(e.stdout, "%d supplement unit(s) matched no stored unit, so unclear claims stayed the supplement's\n", c.unpaired)
	}
	fmt.Fprintf(e.stdout, "evidence: %d item(s) no longer in the tree stay dead (%s)\n", killed, store.DeadBeforeMigration)
	for _, d := range changed {
		fmt.Fprintln(e.stdout, "since the last sync:", d)
	}
	return nil
}

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func cloneClaims(units []model.Unit) {
	for i := range units {
		u := &units[i]
		u.Evidence = slices.Clone(u.Evidence)
		u.Parents = slices.Clone(u.Parents)
		for j := range u.Parents {
			u.Parents[j].Evidence = slices.Clone(u.Parents[j].Evidence)
		}
		for _, rs := range []*[]model.Relation{&u.Relations, &u.Hidden} {
			*rs = slices.Clone(*rs)
			for j := range *rs {
				(*rs)[j].Evidence = slices.Clone((*rs)[j].Evidence)
			}
		}
		for _, f := range []**model.Fact{&u.Purpose, &u.Responsibility, &u.Signature} {
			if *f != nil {
				v := **f
				*f = &v
			}
		}
	}
}

// markDead keeps today's dead items dead: every reader or supplement item
// the tree does not hold now is dead, and a present semantic unit with no
// live item goes absent, as the old sync would have done.
func markDead(units []model.Unit, c *evidence.Checker) int {
	n := 0
	for i := range units {
		u := &units[i]
		u.EachEvidence(func(w model.Writer, ev *model.Evidence) {
			if w != model.Script && ev.Status != model.Dead && !c.Check(*ev).Live {
				ev.Status, ev.Since, ev.Reason = model.Dead, "", store.DeadBeforeMigration
				n++
			}
		})
		if !u.Absent && u.Semantic() && model.StatusOf(u.Evidence) == model.Dead {
			u.SetAbsent(model.EvidenceDead)
		}
	}
	return n
}

// claims is what the supplement says, by stored ID.
type claims struct {
	ref map[model.ID]string
	// ids maps the overlay's keys to the stored units they took.
	ids      map[string]model.ID
	input    map[model.ID]extract.Unit
	edges    map[model.Edge]bool
	unpaired int
	reader   map[model.ID]bool
}

func supplementClaims(units []model.Unit, over extract.Result, hw map[model.Kind]int) claims {
	res := identity.Reconcile(units, nil, over, nil, hw, identity.Setup)
	stored := map[model.ID]bool{}
	for _, u := range units {
		stored[u.ID] = true
	}
	c := claims{ref: map[model.ID]string{}, ids: map[string]model.ID{}, input: map[model.ID]extract.Unit{}, edges: map[model.Edge]bool{}, reader: map[model.ID]bool{}}
	for _, in := range over.Units {
		id := res.Keys[in.Key]
		if !stored[id] {
			if in.SupplementRef != "" {
				c.unpaired++
			}
			continue
		}
		c.input[id] = in
		if in.Key != "" {
			c.ids[in.Key] = id
		}
		if in.SupplementRef != "" {
			c.ref[id] = in.SupplementRef
		}
	}
	for _, r := range over.Relations {
		if r.Writer == model.Supplement {
			c.edges[model.Edge{From: res.Keys[r.From], To: res.Keys[r.To], Kind: r.Kind}] = true
		}
	}
	for _, u := range units {
		_, mapped := c.input[u.ID]
		c.reader[u.ID] = u.Semantic() && !mapped && c.unpaired == 0
	}
	return c
}

type classification map[string][2]int

func (t classification) count(what string, reader bool) {
	v := t[what]
	if reader {
		v[1]++
	} else {
		v[0]++
	}
	t[what] = v
}

func (t classification) print(w interface{ Write([]byte) (int, error) }) {
	var ks []string
	for k := range t {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	for _, k := range ks {
		fmt.Fprintf(w, "%s: %d supplement, %d reader\n", k, t[k][0], t[k][1])
	}
}

// classify gives each INFERRED claim of u to the supplement or a reader. A
// claim the supplement makes on the same target is the supplement's; one it
// makes none on is a reader's. A unit the supplement does not report follows
// its own name, and anything unclear stays the supplement's. A human's
// parent takes the script's parent the extraction reports as the one it
// overrode, which the old format did not store.
func (c claims) classify(u *model.Unit, t *classification) {
	if *t == nil {
		*t = classification{}
	}
	in, mapped := c.input[u.ID]
	readerOf := func(supplementSays bool) bool {
		if mapped {
			return !supplementSays
		}
		return c.reader[u.ID]
	}
	if u.Name.Writer == model.Supplement {
		r := c.reader[u.ID]
		if r {
			u.Name.Writer = model.Reader
		}
		u.SupplementRef = c.ref[u.ID]
		t.count("units", r)
	}
	for _, f := range []struct {
		what  string
		at    *model.Fact
		offer *model.Fact
	}{{"purposes", u.Purpose, in.Purpose}, {"responsibilities", u.Responsibility, in.Responsibility}, {"signatures", u.Signature, in.Signature}} {
		if f.at != nil && f.at.Writer == model.Supplement {
			r := readerOf(f.offer != nil && f.offer.Writer == model.Supplement)
			if r {
				f.at.Writer = model.Reader
			}
			t.count(f.what, r)
		}
	}
	for i, p := range u.Parents {
		if p.Writer == model.Human && p.Over == "" && mapped {
			u.Parents[i].Over = c.ids[in.Parent]
		}
		if p.Writer == model.Supplement {
			r := readerOf(slices.ContainsFunc(in.Placed, func(pl extract.Placement) bool {
				return pl.Writer == model.Supplement && c.ids[pl.Key] == p.To
			}))
			if r {
				u.Parents[i].Writer = model.Reader
			}
			t.count("parents", r)
		}
	}
	for _, rs := range []*[]model.Relation{&u.Relations, &u.Hidden} {
		for i, r := range *rs {
			if r.Writer != model.Supplement {
				continue
			}
			_, toMapped := c.input[r.To]
			reader := false
			switch {
			case c.edges[model.Edge{From: u.ID, To: r.To, Kind: r.Kind}]:
			case mapped && toMapped:
				reader = true
			default:
				reader = c.reader[u.ID] || c.reader[r.To]
			}
			if reader {
				(*rs)[i].Writer = model.Reader
			}
			t.count("relations", reader)
		}
	}
}

// graph is what the screen and the queries see: which units are present,
// where each sits, and each shown relation with its origin.
func graph(units []model.Unit) map[string]string {
	g := map[string]string{}
	for _, u := range model.Present(units) {
		g["unit "+string(u.ID)] = "present"
		if u.Parent != "" {
			g["parent of "+string(u.ID)] = string(u.Parent)
		}
		for _, r := range u.Relations {
			v := string(r.Origin)
			if r.Dead {
				v += " dead"
			}
			g[model.Edge{From: u.ID, To: r.To, Kind: r.Kind}.String()] = v
		}
	}
	return g
}

func graphDiff(a, b map[string]string) []string {
	var out []string
	for k, v := range a {
		if b[k] != v {
			out = append(out, fmt.Sprintf("%s: %q became %q", k, v, b[k]))
		}
	}
	for k, v := range b {
		if _, ok := a[k]; !ok {
			out = append(out, fmt.Sprintf("%s: %q appeared", k, v))
		}
	}
	sort.Strings(out)
	return out
}
