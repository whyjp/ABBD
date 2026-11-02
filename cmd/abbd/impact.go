package main

import (
	"fmt"
	"strings"

	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/topology"
)

// impact prints the neighbourhood of the seeds for an agent to read before
// it works: the direct tier closely, the far tier briefly. It reads only.
func (e env) impact(list string) error {
	units, err := e.st.LoadUnits()
	if err != nil {
		return err
	}
	name := names(units)
	var seeds []model.ID
	for _, s := range strings.Split(list, ",") {
		ms, err := e.find(units, strings.TrimSpace(s))
		if err != nil {
			return err
		}
		seeds = append(seeds, ms[0].Unit.ID)
		// A change to a path changes every unit anchored there, not only
		// the innermost one explain answers for.
		for _, m := range ms[1:] {
			if m.Anchored {
				seeds = append(seeds, m.Unit.ID)
			}
		}
	}
	im := topology.ImpactOf(units, seeds)
	if e.features {
		e.byFeature(units, im)
		return nil
	}
	for _, t := range []struct {
		name  string
		items []topology.Item
	}{{"direct", im.Direct}, {"far", im.Far}} {
		for _, it := range t.items {
			fmt.Fprintf(e.stdout, "%s %s %s: %s\n", t.name, it.Unit, name[it.Unit], it.Why)
		}
	}
	return nil
}

// byFeature prints the impact one feature at a time: the feature, what it
// holds that the change reaches, and the impacted contracts it uses with the
// entries it names, then what no feature holds.
func (e env) byFeature(units []model.Unit, im topology.Impact) {
	name := names(units)
	keyed := map[model.ID]model.Unit{}
	for _, u := range units {
		keyed[u.ID] = u
	}
	fs, rest := topology.ByFeature(units, im)
	for _, f := range fs {
		fmt.Fprintf(e.stdout, "feature %s %s\n", f.ID, name[f.ID])
		for _, it := range f.Items {
			fmt.Fprintf(e.stdout, "  %s %s %s: %s\n", it.Tier, it.Unit, name[it.Unit], it.Why)
		}
		for _, c := range f.Uses {
			l := fmt.Sprintf("  uses %s %s", c.Contract, name[c.Contract])
			if len(c.Entries) > 0 {
				var es []string
				for _, k := range c.Entries {
					if en, ok := keyed[c.Contract].EntryNamed(k); ok {
						k = en.Name
					}
					es = append(es, k)
				}
				l += ": " + strings.Join(es, ", ")
			} else {
				l += ", no entry named"
			}
			fmt.Fprintln(e.stdout, l)
		}
	}
	if len(rest) > 0 {
		fmt.Fprintln(e.stdout, "no feature")
		for _, it := range rest {
			fmt.Fprintf(e.stdout, "  %s %s %s: %s\n", it.Tier, it.Unit, name[it.Unit], it.Why)
		}
	}
}
