package main

import (
	"fmt"

	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/store"
)

// stack stores a stack from a draft, or prints the stored one, as figure
// does for every kind, then says how many stored relations run up it: on
// stdout after a store, on stderr after a print.
func (e env) stack(key string, args []string) error {
	if err := figure[model.Layer](e, key, args); err != nil {
		return err
	}
	s, _, err := store.LoadFigure[model.Layer](e.st, model.FigureKey(key))
	if err != nil {
		return err
	}
	units, err := e.st.LoadUnits()
	if err != nil {
		return err
	}
	out := e.stdout
	if len(args) == 0 {
		out = e.stderr
	}
	fmt.Fprintf(out, "breaches: %d relation(s) run up the stack, a lower layer using a higher one; abbd status lists them\n", len(model.Breaches(s.Claims(), units)))
	return nil
}

// statusBreaches reports each stored relation by which a lower layer of a
// stack uses a higher one. A breach is debt in the code or a layer drawn
// wrong, and only a design call clears it, so it never fails the check.
func (e env) statusBreaches(units []model.Unit, stacks []store.Stack) {
	name := map[model.ID]string{}
	for _, u := range units {
		name[u.ID] = u.Name.Value
	}
	origins := map[model.Origin]int{}
	var lines []string
	known := 0
	for _, s := range stacks {
		if !s.Known {
			continue
		}
		known++
		layers := s.Claims()
		for _, b := range model.Breaches(layers, units) {
			origins[b.Origin]++
			lines = append(lines, fmt.Sprintf("breach: stack %s: %s %s (layer %d %s) %s %s %s (layer %d %s), %s", s.Key, b.From, name[b.From], b.Lower, layers[b.Lower-1].Name, b.Kind, b.To, name[b.To], b.Upper, layers[b.Upper-1].Name, b.Origin))
		}
	}
	if known == 0 {
		return
	}
	fmt.Fprintf(e.stdout, "breaches: %d relation(s) run up a stack (%d EXTRACTED, %d USER_STATED, %d INFERRED); reported, not checked\n", len(lines), origins[model.Extracted], origins[model.UserStated], origins[model.Inferred])
	for _, l := range lines {
		fmt.Fprintln(e.stdout, l)
	}
}
