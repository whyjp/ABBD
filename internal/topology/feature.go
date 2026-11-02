package topology

import (
	"cmp"
	"maps"
	"slices"

	"github.com/whyjp/abbd/internal/model"
)

// Feature is one feature a change reaches: the top-level FEAT that is, or
// holds, an impacted unit. Items are its impacted units, itself first when
// it is impacted, each with its tier. Uses are the impacted contracts it or
// a unit inside it consumes, each with the entries it uses.
type Feature struct {
	ID    model.ID
	Items []Tiered
	Uses  []ContractUse
}

// Tiered is an impacted unit with the tier impact put it in.
type Tiered struct {
	Item
	Tier string
}

// ContractUse is one impacted contract a feature uses, and the keys of the
// entries it names; none when it uses the contract as a whole.
type ContractUse struct {
	Contract model.ID
	Entries  []string
}

// ByFeature groups an impact by the features it reaches, in ID order, and
// returns the impacted units no feature holds.
func ByFeature(units []model.Unit, im Impact) ([]Feature, []Tiered) {
	parent := map[model.ID]model.ID{}
	kind := map[model.ID]model.Kind{}
	for _, u := range model.Present(units) {
		parent[u.ID], kind[u.ID] = u.Parent, u.Kind
	}
	// home is the outermost feature that holds id, or "".
	home := func(id model.ID) model.ID {
		var out model.ID
		seen := map[model.ID]bool{}
		for k := id; k != "" && !seen[k]; k = parent[k] {
			seen[k] = true
			if kind[k] == model.Feature {
				out = k
			}
		}
		return out
	}
	impacted := map[model.ID]bool{}
	groups := map[model.ID]*Feature{}
	var rest []Tiered
	for _, t := range []struct {
		name  string
		items []Item
	}{{"direct", im.Direct}, {"far", im.Far}} {
		for _, it := range t.items {
			impacted[it.Unit] = true
			f := home(it.Unit)
			if f == "" {
				rest = append(rest, Tiered{it, t.name})
				continue
			}
			if groups[f] == nil {
				groups[f] = &Feature{ID: f}
			}
			groups[f].Items = append(groups[f].Items, Tiered{it, t.name})
		}
	}
	// whole marks a contract a feature, or a unit inside it, uses whole, so
	// no other unit's narrowed use hides it.
	whole := map[[2]model.ID]bool{}
	for _, l := range Links(units) {
		f := home(l.From)
		if g := groups[f]; g != nil && l.Kind == model.Consumes && impacted[l.To] {
			i := slices.IndexFunc(g.Uses, func(c ContractUse) bool { return c.Contract == l.To })
			if i < 0 {
				g.Uses = append(g.Uses, ContractUse{Contract: l.To})
				i = len(g.Uses) - 1
			}
			k := [2]model.ID{f, l.To}
			whole[k] = whole[k] || len(l.Entries) == 0
			if whole[k] {
				g.Uses[i].Entries = nil
			} else {
				g.Uses[i].Entries = slices.Compact(slices.Sorted(slices.Values(append(g.Uses[i].Entries, l.Entries...))))
			}
		}
	}
	var out []Feature
	for _, id := range slices.Sorted(maps.Keys(groups)) {
		g := groups[id]
		slices.SortStableFunc(g.Items, func(a, b Tiered) int {
			switch {
			case a.Unit == id:
				return -1
			case b.Unit == id:
				return 1
			}
			return 0
		})
		slices.SortFunc(g.Uses, func(a, b ContractUse) int { return cmp.Compare(a.Contract, b.Contract) })
		out = append(out, *g)
	}
	return out, rest
}
