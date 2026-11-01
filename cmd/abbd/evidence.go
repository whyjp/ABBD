package main

import (
	"fmt"
	"slices"
	"strings"

	"github.com/whyjp/abbd/internal/evidence"
	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/screen"
)

// checkEvidence prints, for every semantic unit and relation, whether each
// quote is in the working tree now and where. It reads and never writes, so
// the same tree prints the same lines.
func (e env) checkEvidence() error {
	stored, err := e.st.LoadUnits()
	if err != nil {
		return err
	}
	c := evidence.New(e.root)
	live, dead := 0, 0
	line := func(subject string, via model.Via, ev model.Evidence, within []model.Anchor) {
		r := c.Check(ev, within)
		state := "dead"
		if r.Live {
			state, live = "live", live+1
		} else {
			dead++
		}
		if via != "" {
			subject += " via " + string(via)
		}
		if r.Live {
			fmt.Fprintf(e.stdout, "%s %s: %s found at %s\n", state, subject, ev, r.At)
		} else {
			fmt.Fprintf(e.stdout, "%s %s: %s\n", state, subject, ev)
		}
	}
	units := slices.Clone(stored)
	slices.SortFunc(units, func(a, b model.Unit) int { return strings.Compare(string(a.ID), string(b.ID)) })
	for _, u := range units {
		if u.Absent && !u.Semantic() {
			continue
		}
		for _, ev := range u.Evidence {
			line(string(u.ID)+" "+u.Name.Value, "", ev, u.Anchors)
		}
		for _, r := range u.Relations {
			for _, ev := range r.Evidence {
				line(model.Edge{From: u.ID, To: r.To, Kind: r.Kind}.String(), r.Via, ev, u.Anchors)
			}
		}
	}
	fmt.Fprintf(e.stdout, "%d live, %d dead\n", live, dead)
	return nil
}

// status prints whether the state matches the working tree, whether the
// blueprint matches the state, and what the state holds.
func (e env) status() error {
	in, err := e.screenInput()
	if err != nil {
		return err
	}
	if len(in.Units) == 0 {
		fmt.Fprintln(e.stdout, "state: none; run abbd setup -- EXTRACT-ARGV")
		return nil
	}
	switch in.ActualFreshness() {
	case screen.Stale:
		fmt.Fprintf(e.stdout, "state: STALE, synced at %s; the code changed since, abbd sync re-extracts\n", in.Synced.Revision)
	default:
		fmt.Fprintf(e.stdout, "state: %s, synced at %s\n", in.ActualFreshness(), in.Synced.Revision)
	}
	fmt.Fprintln(e.stdout, "blueprint:", in.BlueprintFreshness())
	kinds := map[model.Kind]int{}
	origins := map[model.Origin]int{}
	dead := 0
	for _, u := range model.Present(in.Units) {
		kinds[u.Kind]++
		for _, r := range u.Relations {
			if r.Dead {
				dead++
			} else {
				origins[r.Origin]++
			}
		}
	}
	var ks []string
	for _, k := range []model.Kind{model.Module, model.Interface, model.Event, model.Data, model.Config, model.Feature} {
		if kinds[k] > 0 {
			ks = append(ks, fmt.Sprintf("%d %s", kinds[k], k))
		}
	}
	fmt.Fprintln(e.stdout, "units:", strings.Join(ks, ", "))
	fmt.Fprintf(e.stdout, "relations: %d EXTRACTED, %d USER_STATED, %d INFERRED, %d dead\n",
		origins[model.Extracted], origins[model.UserStated], origins[model.Inferred], dead)
	stale := 0
	for id, u := range in.Understandings {
		if u.Digest != in.Digests[id] {
			stale++
		}
	}
	fmt.Fprintf(e.stdout, "understanding: %d module text(s), %d STALE\n", len(in.Understandings), stale)
	return nil
}
