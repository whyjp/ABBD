package main

import (
	"bytes"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/whyjp/abbd/internal/gitx"
	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/topology"
)

// review prints, in one place, what a reviewer of a change reads: the
// change since the base, who uses each contract it touched, what else it
// reaches, and the claims and texts worth a look because no human stated
// them or the code under them moved. It reads only and always exits 0;
// ABBD reports and never judges.
func (e env) review() error {
	base := e.base
	if base == "" {
		var err error
		if base, err = gitx.MergeBase(e.root); err != nil {
			return err
		}
	}
	was, err := e.st.UnitsAt(base)
	if err != nil {
		return err
	}
	in, err := e.screenInput()
	if err != nil {
		return err
	}
	now := in.Units
	d := topology.Compare(was, now)
	w := e.stdout
	fmt.Fprintf(w, "review since %s\n\nchanges:\n", short(base))
	for _, l := range d.Lines() {
		fmt.Fprintln(w, "  "+l)
	}
	if len(d.Lines()) == 0 {
		fmt.Fprintln(w, "  none in the topology")
	}

	byID := map[model.ID]model.Unit{}
	for _, u := range model.Present(now) {
		byID[u.ID] = u
	}
	name := names(now)
	feature := func(id model.ID) string {
		seen := map[model.ID]bool{}
		for p := byID[id].Parent; p != "" && !seen[p]; p = byID[p].Parent {
			seen[p] = true
			if byID[p].Kind == model.Feature {
				return fmt.Sprintf(", in %s %s", p, name[p])
			}
		}
		return ""
	}
	touched := map[model.ID][]string{}
	note := func(id model.ID, entry string) {
		if _, ok := byID[id]; ok && !slices.Contains(touched[id], entry) {
			touched[id] = append(touched[id], entry)
		}
	}
	for _, r := range d.UnitsAdded {
		if r.Kind.IsContract() {
			note(r.ID, "")
		}
	}
	for _, s := range d.Signatures {
		note(s.ID, "")
	}
	for _, r := range d.Code {
		note(r.ID, "")
	}
	for _, l := range d.Listed {
		note(l.ID, "")
	}
	for _, en := range d.Entries {
		note(en.ID, en.Entry)
	}
	links := topology.Links(now)
	fmt.Fprintln(w, "\nwho uses what changed:")
	none := true
	for _, id := range slices.Sorted(maps.Keys(touched)) {
		c := byID[id]
		for _, entry := range touched[id] {
			var users []string
			if entry == "" {
				for _, l := range links {
					if l.To == id && l.Kind != model.Provides {
						users = append(users, fmt.Sprintf("%s %s %s (%s%s)", l.From, name[l.From], l.Kind, l.Origin, feature(l.From)))
					}
				}
			} else {
				key := entry
				for _, en := range c.Entries {
					if en.Name == entry {
						key = en.Key
					}
				}
				for _, u := range topology.EntryUses(now, id)[key] {
					if u.Kind == model.Provides {
						continue
					}
					users = append(users, fmt.Sprintf("%s %s %s (%s%s)", u.From, name[u.From], u.Kind, u.Origin, feature(u.From)))
				}
			}
			what := fmt.Sprintf("%s %s", id, c.Name.Value)
			if entry != "" {
				what += " " + entry
			}
			slices.Sort(users)
			users = slices.Compact(users)
			if len(users) == 0 {
				users = []string{"no recorded user"}
			}
			none = false
			fmt.Fprintf(w, "  %s: %s\n", what, strings.Join(users, "; "))
		}
	}
	if none {
		fmt.Fprintln(w, "  no contract changed")
	}

	var seeds []model.ID
	for _, r := range d.UnitsAdded {
		seeds = append(seeds, r.ID)
	}
	for _, c := range d.Changed {
		seeds = append(seeds, c.ID)
	}
	seeds = append(seeds, slices.Collect(maps.Keys(touched))...)
	for _, r := range append(slices.Clone(d.RelationsAdded), d.RelationsRemoved...) {
		seeds = append(seeds, r.From, r.To)
	}
	seeds = slices.DeleteFunc(seeds, func(id model.ID) bool { _, ok := byID[id]; return !ok })
	slices.Sort(seeds)
	seeds = slices.Compact(seeds)
	fmt.Fprintln(w, "\nwhat the change reaches directly:")
	if len(seeds) == 0 {
		fmt.Fprintln(w, "  nothing the topology records")
	} else if e.features {
		e.byFeature(now, topology.ImpactOf(now, seeds))
	} else {
		for _, it := range topology.ImpactOf(now, seeds).Direct {
			fmt.Fprintf(w, "  %s %s: %s\n", it.Unit, name[it.Unit], it.Why)
		}
	}

	fmt.Fprintln(w, "\nworth a look:")
	var look []string
	for _, c := range d.InferredAdded {
		look = append(look, fmt.Sprintf("a reader's coupling no human stated: %s %s %s (%s %s %s)", c.From, c.Kind, c.To, c.FromName, c.Kind, c.ToName))
	}
	for _, id := range slices.Sorted(maps.Keys(in.Understandings)) {
		if _, ok := textModule(now, id); ok && in.Understandings[id].Digest != in.Digests[id] {
			look = append(look, fmt.Sprintf("the text of %s %s is STALE: its code changed since it was read", id, name[id]))
		}
	}
	for _, id := range slices.Sorted(maps.Keys(in.Notes)) {
		for _, n := range in.Notes[id] {
			ref := string(id) + "." + n.Slot
			if n.Stale() {
				look = append(look, fmt.Sprintf("the note %s is STALE: the code it cites changed", ref))
			}
			if strings.HasPrefix(n.Slot, "decisions") && (n.Why == nil || n.Why.Value == "") {
				look = append(look, fmt.Sprintf("the decision %s records no why from a human", ref))
			}
		}
	}
	var ev bytes.Buffer
	if err := (env{opts: e.opts, stdout: &ev, st: e.st}).checkEvidence(); err == nil {
		for _, l := range strings.Split(ev.String(), "\n") {
			if strings.HasPrefix(l, "drifted") {
				look = append(look, "evidence "+l)
			}
		}
	}
	for _, l := range look {
		fmt.Fprintln(w, "  "+l)
	}
	if len(look) == 0 {
		fmt.Fprintln(w, "  nothing")
	}
	return nil
}

func mapsKeys[K comparable, V any](m map[K]V) func(func(K) bool) {
	return func(yield func(K) bool) {
		for k := range m {
			if !yield(k) {
				return
			}
		}
	}
}
