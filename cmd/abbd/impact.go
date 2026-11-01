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
