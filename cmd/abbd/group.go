package main

import (
	"fmt"

	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/screen"
)

// explainGroup answers for a composition group's address, stack-KEY.N: its
// name and role, what kind of record holds it and what it is there, the view it
// stands in, what its boxes say of its inside, and the boxes it holds.
func (e env) explainGroup(units []model.Unit, address string) error {
	st, n, err := e.st.Group(address)
	if err != nil {
		return err
	}
	l := st.Claims()[n-1]
	c := st.Composition()
	name := names(units)
	kind := c.Kind
	if kind == "" {
		kind = model.KindLayers
	}
	fmt.Fprintf(e.stdout, "%s %s (group %d of stack %s, kind %s, as %s)\n", address, l.Name, n, st.Key, kind, c.AsOf(l))
	fmt.Fprintf(e.stdout, "role: %s\n", l.Role)
	module := model.ID(c.View)
	if c.View == model.OverviewView {
		module = ""
	}
	if c.View != "" {
		if c.View == model.OverviewView {
			fmt.Fprintln(e.stdout, "inside the overview")
		} else {
			fmt.Fprintf(e.stdout, "inside %s %s\n", c.View, name[model.ID(c.View)])
		}
		cycle, boundary := model.GroupFacts(units, l.Units, module)
		fmt.Fprintf(e.stdout, "inside it: %d box(es), %d in the largest cycle, %d reach outside the module\n", len(l.Units), cycle, boundary)
	}
	if len(l.Promises) > 0 {
		fmt.Fprintf(e.stdout, "promises: %v\n", l.Promises)
	}
	fmt.Fprintln(e.stdout, "holds:")
	for _, id := range l.Units {
		fmt.Fprintf(e.stdout, "  %s %s\n", id, name[id])
	}
	if stale := st.Parts[n-1].Why(units); len(stale) > 0 {
		fmt.Fprintf(e.stdout, "STALE: %v\n", stale)
	}
	figs, err := e.st.LoadFigures()
	if err != nil {
		return err
	}
	if drawn, why := screen.GroupDrawn(units, figs.Stacks, address); drawn {
		fmt.Fprintf(e.stdout, "drawn: a folded box in %s, its page %s.html\n", c.View, address)
	} else if why != "" {
		fmt.Fprintf(e.stdout, "not drawn as a box: %s\n", why)
	}
	return nil
}

// groupSeeds are the boxes of the group an address names, which an impact
// reads as the seeds.
func (e env) groupSeeds(address string) ([]model.ID, error) {
	st, n, err := e.st.Group(address)
	if err != nil {
		return nil, err
	}
	return st.Claims()[n-1].Units, nil
}

// explainGroups says, for a unit, which group of which composition record
// holds it, as explain says which stage or zone does.
func (e env) explainGroups(units []model.Unit, id model.ID) error {
	figs, err := e.st.LoadFigures()
	if err != nil {
		return err
	}
	for _, s := range figs.Stacks {
		if !s.Known || s.View == "" || s.Primary {
			continue
		}
		layers := s.Claims()
		if n, ok := model.Holders(layers, units)[id]; ok {
			address := model.GroupAddress(s.Key, n)
			line := fmt.Sprintf("group %s %s (%s)", address, layers[n-1].Name, s.Composition().AsOf(layers[n-1]))
			if drawn, _ := screen.GroupDrawn(units, figs.Stacks, address); !drawn {
				line += ", not drawn as a box"
			}
			fmt.Fprintln(e.stdout, line)
		}
	}
	return nil
}
