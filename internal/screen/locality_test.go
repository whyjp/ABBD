package screen

import (
	"fmt"
	"maps"
	"slices"
	"testing"

	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/store"
)

// rewritten names the files of the blueprint that differ between two states.
func rewritten(t *testing.T, before, after []model.Unit) []string {
	t.Helper()
	a, err := Files(Build(Input{Units: before}))
	if err != nil {
		t.Fatal(err)
	}
	b, err := Files(Build(Input{Units: after}))
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for f := range maps.Keys(a) {
		if string(a[f]) != string(b[f]) {
			out = append(out, f)
		}
	}
	for f := range maps.Keys(b) {
		if _, ok := a[f]; !ok {
			out = append(out, f)
		}
	}
	slices.Sort(out)
	return out
}

func clone(us []model.Unit) []model.Unit {
	out := slices.Clone(us)
	for i := range out {
		out[i].Relations = slices.Clone(out[i].Relations)
		for j := range out[i].Relations {
			out[i].Relations[j].Evidence = slices.Clone(out[i].Relations[j].Evidence)
		}
		out[i].Evidence = slices.Clone(out[i].Evidence)
	}
	return out
}

// sampleModules are a few modules of a state that hold two units or more
// with no relation between the first two, the first, middle and last by ID.
func sampleModules(us []model.Unit) [][3]model.ID {
	kids := map[model.ID][]model.ID{}
	for _, u := range model.Present(us) {
		if u.Parent != "" && !u.Kind.IsContract() {
			kids[u.Parent] = append(kids[u.Parent], u.ID)
		}
	}
	joined := map[[2]model.ID]bool{}
	for _, u := range us {
		for _, r := range u.Relations {
			joined[[2]model.ID{u.ID, r.To}], joined[[2]model.ID{r.To, u.ID}] = true, true
		}
	}
	var all [][3]model.ID
	for _, p := range slices.Sorted(maps.Keys(kids)) {
		k := kids[p]
		if len(k) >= 2 && !joined[[2]model.ID{k[0], k[1]}] {
			all = append(all, [3]model.ID{p, k[0], k[1]})
		}
	}
	if len(all) <= 3 {
		return all
	}
	return [][3]model.ID{all[0], all[len(all)/2], all[len(all)-1]}
}

// A change confined to one module rewrites that module's page and the data
// file, and no other page, and a unit added there the unit index too. An edit that only moves the lines a quote sits on
// rewrites nothing. So a pull request's blueprint diff stays where its code
// diff is.
func TestAChangeInOneModuleRewritesOnlyItsPage(t *testing.T) {
	for _, name := range fixtures {
		base, err := store.Open("testdata/" + name).LoadUnits()
		if err != nil {
			t.Fatal(err)
		}
		t.Run(name+" lines", func(t *testing.T) {
			moved := clone(base)
			n := 0
			for i := range moved {
				for j := range moved[i].Relations {
					for k := range moved[i].Relations[j].Evidence {
						moved[i].Relations[j].Evidence[k].Lines = fmt.Sprint(900 + n)
						n++
					}
				}
			}
			if got := rewritten(t, base, moved); len(got) > 0 {
				t.Errorf("moving every quote's lines rewrote %v", got)
			}
		})
		for _, s := range sampleModules(base) {
			mod, a, b := s[0], s[1], s[2]
			own := []string{GraphFile, string(mod) + ".html"}
			t.Run(fmt.Sprintf("%s relation in %s", name, mod), func(t *testing.T) {
				joined := clone(base)
				i := slices.IndexFunc(joined, func(u model.Unit) bool { return u.ID == a })
				joined[i].Relations = append(joined[i].Relations, model.Relation{To: b, Kind: model.DependsOn, Origin: model.Extracted})
				got := rewritten(t, base, joined)
				for _, f := range got {
					if !slices.Contains(own, f) {
						t.Errorf("a relation between two units of %s rewrote %s", mod, f)
					}
				}
				for _, f := range own {
					if !slices.Contains(got, f) {
						t.Errorf("a relation between two units of %s left %s as it was", mod, f)
					}
				}
			})
			t.Run(fmt.Sprintf("%s unit in %s", name, mod), func(t *testing.T) {
				grown := clone(base)
				u := inside("MOD-9999", mod, model.Relation{To: a, Kind: model.DependsOn, Origin: model.Extracted})
				grown = append(grown, u)
				got := rewritten(t, base, grown)
				want := append(slices.Clone(own), UnitsFile)
				for _, f := range got {
					if !slices.Contains(want, f) {
						t.Errorf("a unit added inside %s rewrote %s", mod, f)
					}
				}
				for _, f := range want {
					if !slices.Contains(got, f) {
						t.Errorf("a unit added inside %s left %s as it was", mod, f)
					}
				}
			})
		}
	}
}
