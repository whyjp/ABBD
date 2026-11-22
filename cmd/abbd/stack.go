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
	if s.Primary {
		fmt.Fprintln(out, "breaches: none counted on the primary flow, whose later stages use earlier ones as a flow runs")
		return nil
	}
	fmt.Fprintf(out, "breaches: %d relation(s) run up the stack, a lower layer using a higher one; abbd status lists them\n", len(store.Breaches(s, units)))
	return nil
}

// statusCompositions reports the stacks that compose a view: how many of the
// view's boxes the groups hold, which the rest stand as lines, a record that
// is no composition, and two records of one view. A claim the relations do
// not bear out reads STALE with its stack; this is a report and fails nothing
// itself.
func (e env) statusCompositions(units []model.Unit, stacks []store.Stack) {
	views := map[string]model.FigureKey{}
	for _, s := range stacks {
		if !s.Known || !s.Composition().Said() {
			continue
		}
		c := s.Composition()
		if err := c.Validate(s.Claims()); err != nil {
			fmt.Fprintf(e.stdout, "warning: stack %s is no composition record, so abbd reads it as a plain stack: %v\n", s.Key, err)
			continue
		}
		if c.View == "" {
			continue
		}
		if other, dup := views[c.View]; dup {
			fmt.Fprintf(e.stdout, "conflict: stacks %s and %s both compose view %s, so abbd reads past the second; a view has one record\n", other, s.Key, c.View)
			continue
		}
		views[c.View] = s.Key
		held := model.Holders(s.Claims(), units)
		boxes := model.ViewBoxes(units, c.View)
		in := 0
		for _, b := range boxes {
			if held[b] > 0 {
				in++
			}
		}
		kind := c.Kind
		if kind == "" {
			kind = model.KindLayers
		}
		fmt.Fprintf(e.stdout, "composition: stack %s, kind %s, composes %s: %d of %d box(es) held by a group, the rest stand as lines; a claim its relations stop bearing out shows as STALE under stacks\n", s.Key, kind, c.View, in, len(boxes))
	}
}

// statusBreaches reports each stored relation by which a lower layer of a
// stack uses a higher one. A breach is debt in the code or a layer drawn
// wrong, and only a design call clears it, so it never fails the check. The
// stack marked the primary flow has none (store.Breaches).
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
		for _, b := range store.Breaches(s, units) {
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
