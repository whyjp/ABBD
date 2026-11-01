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
	names := map[model.ID]string{}
	for _, u := range model.Present(units) {
		names[u.ID] = u.Name.Value
	}
	var seeds []model.ID
	for _, s := range strings.Split(list, ",") {
		id := model.ID(strings.TrimSpace(s))
		if _, ok := names[id]; !ok {
			return fmt.Errorf("%q is not a unit of the state", id)
		}
		seeds = append(seeds, id)
	}
	im := topology.ImpactOf(units, seeds)
	for _, t := range []struct {
		name  string
		items []topology.Item
	}{{"direct", im.Direct}, {"far", im.Far}} {
		for _, it := range t.items {
			fmt.Fprintf(e.stdout, "%s %s %s: %s\n", t.name, it.Unit, names[it.Unit], it.Why)
		}
	}
	return nil
}
