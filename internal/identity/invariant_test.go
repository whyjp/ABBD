package identity_test

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/whyjp/abbd/internal/evidence"
	"github.com/whyjp/abbd/internal/extract"
	"github.com/whyjp/abbd/internal/group"
	"github.com/whyjp/abbd/internal/identity"
	"github.com/whyjp/abbd/internal/model"
)

// world is a small repository whose files hold numbered quotes, a pool of
// script units and of supplement units, and the stored state.
type world struct {
	t     *testing.T
	root  string
	rng   *rand.Rand
	units []model.Unit
	hw    map[model.Kind]int
	rev   int
	quote map[string]bool // which quotes the tree holds now
}

const files, quotes = 4, 6

func (w *world) path(f int) string { return fmt.Sprintf("src/f%d.go", f) }
func (w *world) q(f, n int) string { return fmt.Sprintf("quote_%d_%d", f, n) }

func (w *world) write() {
	for f := 0; f < files; f++ {
		var b strings.Builder
		for n := 0; n < quotes; n++ {
			if w.quote[w.q(f, n)] {
				fmt.Fprintf(&b, "%s()\n", w.q(f, n))
			}
		}
		p := filepath.Join(w.root, filepath.FromSlash(w.path(f)))
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(b.String()), 0o644); err != nil {
			w.t.Fatal(err)
		}
	}
}

func (w *world) cite() []model.Evidence {
	f, n := w.rng.Intn(files), w.rng.Intn(quotes)
	return []model.Evidence{{Path: w.path(f), Quote: w.q(f, n) + "()"}}
}

// liveCite is a citation the tree holds now, as abbd record requires.
func (w *world) liveCite() []model.Evidence {
	for range 200 {
		es := w.cite()
		if evidence.New(w.root).Check(es[0]).Live {
			return es
		}
	}
	return nil
}

func (w *world) extraction(withSupplement bool) extract.Result {
	var ex extract.Result
	for i := 0; i < 6; i++ {
		if w.rng.Intn(5) == 0 {
			continue
		}
		key := fmt.Sprintf("m%d", i)
		u := extract.Unit{Key: key, Kind: model.Module, Name: model.Said(model.Script, key), Anchors: []model.Anchor{{Path: key + "/a.go"}, {Path: key + "/b.go"}}}
		if i > 0 && w.rng.Intn(3) == 0 {
			u.Parent = "m0"
		}
		ex.Units = append(ex.Units, u)
	}
	if len(ex.Units) == 0 || ex.Units[0].Key != "m0" {
		for i := range ex.Units {
			ex.Units[i].Parent = ""
		}
	}
	if !withSupplement {
		return ex
	}
	script := len(ex.Units)
	for k := 0; k < 4; k++ {
		if w.rng.Intn(4) == 0 || script == 0 {
			continue
		}
		es := []model.Evidence{{Path: w.path(k % files), Quote: w.q(k%files, k) + "()"}}
		key := fmt.Sprintf("s%d", k)
		host := ex.Units[w.rng.Intn(script)].Key
		ex.Units = append(ex.Units, extract.Unit{
			Key: "supplement:" + key, SupplementRef: key, Kind: model.Data, Name: model.Said(model.Supplement, key),
			Anchors: []model.Anchor{es[0].Anchor()}, Evidence: es,
			Placed: []extract.Placement{{Key: host, Writer: model.Supplement, Evidence: es}},
		})
		ex.Relations = append(ex.Relations, extract.Relation{From: host, To: "supplement:" + key, Kind: model.DependsOn, Writer: model.Supplement, Via: "data", Evidence: es})
	}
	return ex
}

// held lists the reader's and the human's claims the state stores.
func held(units []model.Unit) []string {
	var out []string
	for _, u := range units {
		if u.Name.Writer == model.Reader {
			out = append(out, "unit "+string(u.ID))
		}
		for _, c := range u.Parents {
			if c.Writer == model.Reader || c.Writer == model.Human {
				// A record upserts the writer's own claim, so the claim is the
				// unit's and the writer's, whatever it names.
				out = append(out, fmt.Sprintf("parent %s %s", u.ID, c.Writer))
			}
		}
		for _, r := range u.Claims() {
			if r.Writer == model.Reader || r.Writer == model.Human {
				out = append(out, fmt.Sprintf("relation %s %s %s %s", u.ID, r.Kind, r.To, r.Writer))
			}
		}
	}
	slices.Sort(out)
	return out
}

func (w *world) sync(ws identity.Writers) {
	ex := w.extraction(len(ws) > 1)
	if len(ex.Units) == 0 {
		return
	}
	rev := fmt.Sprintf("r%d", w.rev)
	res := identity.Reconcile(w.units, nil, ex, nil, w.hw, ws)
	units := evidence.New(w.root).Apply(res.Units, rev)
	w.units, w.hw = group.Fallback(units, res.HighWater)
}

func (w *world) record() {
	present := model.Present(w.units)
	if len(present) < 2 {
		return
	}
	a, b := present[w.rng.Intn(len(present))], present[w.rng.Intn(len(present))]
	i := slices.IndexFunc(w.units, func(u model.Unit) bool { return u.ID == a.ID })
	es := w.liveCite()
	if es == nil {
		return
	}
	switch w.rng.Intn(3) {
	case 0:
		if a.ID != b.ID && !slices.ContainsFunc(a.Claims(), func(r model.Relation) bool {
			return r.To == b.ID && r.Kind == model.DependsOn && r.Writer == model.Script
		}) {
			w.units[i].PutRelation(model.Relation{To: b.ID, Kind: model.DependsOn, Writer: model.Reader, Via: "call", Evidence: es})
		}
	case 1:
		if b.Kind.Holds() && a.ID != b.ID {
			w.units[i].PutParent(model.ParentClaim{To: b.ID, Writer: model.Human, Source: "conversation"})
		}
	default:
		w.hw[model.Data]++
		w.units = append(w.units, model.Unit{ID: model.FormatID(model.Data, w.hw[model.Data]), Kind: model.Data, Name: model.Said(model.Reader, "found"), Anchors: []model.Anchor{es[0].Anchor()}, Evidence: es})
	}
	model.Resolve(w.units)
}

func (w *world) edit() {
	for range 3 {
		q := w.q(w.rng.Intn(files), w.rng.Intn(quotes))
		w.quote[q] = !w.quote[q]
	}
	w.write()
	if w.rng.Intn(2) == 0 {
		w.rev++
	}
}

// Over random sequences of sync, setup, record and code edits, every unit
// that was present is still present or says why it is not, and every claim a
// reader or a human made is still stored. Resolved parents stay a tree.
func TestNothingSilentlyDisappears(t *testing.T) {
	for seed := int64(1); seed <= 12; seed++ {
		w := &world{t: t, root: t.TempDir(), rng: rand.New(rand.NewSource(seed)), hw: map[model.Kind]int{}, quote: map[string]bool{}}
		for f := 0; f < files; f++ {
			for n := 0; n < quotes; n++ {
				w.quote[w.q(f, n)] = true
			}
		}
		w.write()
		w.sync(identity.Setup)
		for step := 0; step < 60; step++ {
			before := slices.Clone(w.units)
			claims := held(w.units)
			var op string
			switch w.rng.Intn(4) {
			case 0:
				op = "sync"
				w.sync(identity.Sync)
			case 1:
				op = "setup"
				w.sync(identity.Setup)
			case 2:
				op = "record"
				w.record()
			default:
				op = "edit"
				w.edit()
			}
			byID := map[model.ID]model.Unit{}
			for _, u := range w.units {
				byID[u.ID] = u
				if err := u.Validate(); err != nil {
					t.Fatalf("seed %d step %d %s: %v", seed, step, op, err)
				}
			}
			for _, u := range model.Present(before) {
				if now, ok := byID[u.ID]; !ok || now.Absent && now.AbsentReason == "" {
					t.Fatalf("seed %d step %d %s: %s disappeared without a reason", seed, step, op, u.ID)
				}
			}
			now := held(w.units)
			for _, c := range claims {
				if !slices.Contains(now, c) {
					t.Fatalf("seed %d step %d %s: lost %s", seed, step, op, c)
				}
			}
			if c := model.ParentCycle(w.units); c != "" {
				t.Fatalf("seed %d step %d %s: parent cycle at %s", seed, step, op, c)
			}
		}
	}
}

// Sync speaks for the script only: run on a state, it changes no claim of
// the supplement, a reader or a human, and two runs on the same tree agree.
func TestSyncChangesOnlyTheScriptsClaimsAndIsIdempotent(t *testing.T) {
	w := &world{t: t, root: t.TempDir(), rng: rand.New(rand.NewSource(3)), hw: map[model.Kind]int{}, quote: map[string]bool{}}
	for f := 0; f < files; f++ {
		for n := 0; n < quotes; n++ {
			w.quote[w.q(f, n)] = true
		}
	}
	w.write()
	w.sync(identity.Setup)
	for range 6 {
		w.record()
	}
	others := func(units []model.Unit) []string {
		var out []string
		for _, u := range units {
			for _, r := range u.Claims() {
				if r.Writer != model.Script {
					out = append(out, fmt.Sprintf("%s %s %s %s %s", u.ID, r.Kind, r.To, r.Writer, r.Via))
				}
			}
			for _, f := range []*model.Fact{u.Purpose, u.Responsibility} {
				if f != nil {
					out = append(out, string(u.ID)+" "+f.Value)
				}
			}
		}
		slices.Sort(out)
		return out
	}
	before := others(w.units)
	ex := w.extraction(false)
	run := func(units []model.Unit) []model.Unit {
		res := identity.Reconcile(units, nil, ex, nil, w.hw, identity.Sync)
		out, _ := group.Fallback(evidence.New(w.root).Apply(res.Units, "r0"), res.HighWater)
		return out
	}
	once := run(w.units)
	if !reflect.DeepEqual(others(once), before) {
		t.Fatalf("sync changed another writer's claims:\n%v\n%v", before, others(once))
	}
	if twice := run(once); !reflect.DeepEqual(once, twice) {
		t.Fatalf("a second sync on the same tree changed the state")
	}
}
