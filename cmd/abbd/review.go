package main

import (
	"bytes"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/whyjp/abbd/internal/gitx"
	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/store"
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
	// The sections go to a buffer so a one-line headline can lead them.
	var body bytes.Buffer
	w := &body
	fmt.Fprintln(w, "\nchanges:")
	for _, l := range d.Lines() {
		fmt.Fprintln(w, "  "+l)
	}
	if len(d.Lines()) == 0 {
		fmt.Fprintln(w, "  none in the topology")
	}
	codeChanged := e.anchoredAt(now, base)
	if len(codeChanged) > 0 {
		var ids []string
		for _, id := range codeChanged {
			ids = append(ids, string(id))
		}
		fmt.Fprintln(w, "  code changed under "+strings.Join(ids, ", "))
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
	touchedLines := 0
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
				// A removed entry is no longer listed, so its key is
				// the one its name would take.
				key := model.EntryKey(entry)
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
				for _, l := range links {
					if l.To == id && l.Kind != model.Provides && len(l.Entries) == 0 {
						users = append(users, fmt.Sprintf("%s %s %s the whole contract (%s%s)", l.From, name[l.From], l.Kind, l.Origin, feature(l.From)))
					}
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
			touchedLines++
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
	// A relation counts at its top-level ends in the diff; the units it
	// really joins seed the impact, so a module's whole inside does not.
	seeds = append(seeds, topology.RelationEnds(now, d.RelationsAdded)...)
	// A removed relation's end may be gone too; the nearest unit above
	// it that is still there stands for it.
	wasByID := map[model.ID]model.Unit{}
	for _, u := range was {
		wasByID[u.ID] = u
	}
	for _, id := range topology.RelationEnds(was, d.RelationsRemoved) {
		seen := map[model.ID]bool{}
		for p := id; p != "" && !seen[p]; p = wasByID[p].Parent {
			seen[p] = true
			if _, ok := byID[p]; ok {
				seeds = append(seeds, p)
				break
			}
		}
	}
	// Most changes edit code and leave the topology as it was, so the
	// units anchored at a file changed since the base seed it too.
	seeds = append(seeds, codeChanged...)
	seeds = slices.DeleteFunc(seeds, func(id model.ID) bool { _, ok := byID[id]; return !ok })
	slices.Sort(seeds)
	seeds = slices.Compact(seeds)
	fmt.Fprintln(w, "\nwhat the change reaches directly:")
	reached := 0
	if len(seeds) == 0 {
		fmt.Fprintln(w, "  nothing the topology records")
	} else if e.features {
		im := topology.ImpactOf(now, seeds)
		for _, it := range im.Direct {
			if it.Why != "seed" {
				reached++
			}
		}
		(env{opts: e.opts, stdout: w, st: e.st}).byFeature(now, im)
	} else {
		// The seeds are the change itself, listed above; what they reach
		// is the news, and only its first lines are worth reading here.
		for _, it := range topology.ImpactOf(now, seeds).Direct {
			if it.Why == "seed" {
				continue
			}
			if reached++; reached <= reachShown {
				fmt.Fprintf(w, "  %s %s: %s\n", it.Unit, name[it.Unit], it.Why)
			}
		}
		if reached > reachShown {
			fmt.Fprintf(w, "  and %d more; abbd impact lists them all\n", reached-reachShown)
		}
	}

	// What is worth a look is what sits in, above or under what the change
	// touched, so a STALE text elsewhere in the repository stays out.
	seeded, above := map[model.ID]bool{}, map[model.ID]bool{}
	for _, id := range seeds {
		seeded[id] = true
		seen := map[model.ID]bool{id: true}
		for p := byID[id].Parent; p != "" && !seen[p]; p = byID[p].Parent {
			seen[p] = true
			above[p] = true
		}
	}
	related := func(id model.ID) bool {
		if above[id] {
			return true
		}
		seen := map[model.ID]bool{}
		for p := id; p != "" && !seen[p]; p = byID[p].Parent {
			seen[p] = true
			if seeded[p] {
				return true
			}
		}
		return false
	}
	fmt.Fprintln(w, "\nworth a look in what the change touched:")
	var look []string
	for _, c := range d.InferredAdded {
		look = append(look, fmt.Sprintf("a reader's coupling no human stated: %s %s %s (%s %s %s)", c.From, c.Kind, c.To, c.FromName, c.Kind, c.ToName))
	}
	for _, id := range slices.Sorted(maps.Keys(in.Understandings)) {
		if _, ok := textModule(now, id); ok && related(id) && in.Understandings[id].Digest != in.Digests[id] {
			look = append(look, fmt.Sprintf("the text of %s %s is STALE: its code changed since it was read", id, name[id]))
		}
	}
	for _, id := range slices.Sorted(maps.Keys(in.Notes)) {
		u, present := presentUnit(now, id)
		for _, n := range in.Notes[id] {
			if n.Misfiled || !present || !u.Kind.HoldsNotes() || !n.Known || n.Ref.Unheld(u) || !related(id) {
				continue
			}
			ref := string(id) + "." + n.Slot
			if n.Stale() {
				look = append(look, fmt.Sprintf("the note %s is STALE: the code it cites changed", ref))
			}
			if n.Ref.Slot.Name == "decisions" && (n.Why == nil || n.Why.Value == "") {
				look = append(look, fmt.Sprintf("the decision %s records no why from a human", ref))
			}
		}
	}
	var ev bytes.Buffer
	if err := (env{opts: e.opts, stdout: &ev, st: e.st}).checkEvidence(); err != nil {
		look = append(look, "the evidence could not be checked: "+err.Error())
	}
	for _, l := range strings.Split(ev.String(), "\n") {
		f := strings.Fields(l)
		if len(f) < 2 || f[0] != "drifted" {
			continue
		}
		subject := f[1]
		if subject == "since" && len(f) > 3 {
			subject = f[3]
		}
		// A relation's subject is FROM.KIND.TO; either end counts.
		ends := strings.Split(strings.TrimSuffix(subject, ":"), ".")
		if related(model.ID(ends[0])) || len(ends) > 2 && related(model.ID(ends[2])) {
			look = append(look, "evidence "+l)
		}
	}
	for _, l := range look {
		fmt.Fprintln(w, "  "+l)
	}
	if len(look) == 0 {
		fmt.Fprintln(w, "  nothing")
	}
	fmt.Fprintf(e.stdout, "review since %s: %d topology change(s), code changed under %d unit(s), %d touched contract(s) or entries with their users, %d unit(s) reached directly, %d thing(s) worth a look\n",
		short(base), len(d.Lines()), len(codeChanged), touchedLines, reached, len(look))
	_, err = e.stdout.Write(body.Bytes())
	return err
}

// reachShown is how many units review lists of what a change reaches.
const reachShown = 15

// anchoredAt lists the present units anchored at a file that changed
// since base, the working tree included, in ID order.
func (e env) anchoredAt(units []model.Unit, base string) []model.ID {
	files, err := gitx.ChangedSince(e.root, base)
	if err != nil {
		fmt.Fprintln(e.stderr, "abbd: warning: could not list the files changed since the base:", err)
		return nil
	}
	var out []model.ID
	for _, u := range model.Present(units) {
		if slices.ContainsFunc(u.Anchors, func(a model.Anchor) bool {
			return slices.ContainsFunc(files, func(f string) bool {
				return !strings.HasPrefix(f, store.Dir+"/") && (f == a.Path || strings.HasPrefix(f, strings.TrimSuffix(a.Path, "/")+"/"))
			})
		}) {
			out = append(out, u.ID)
		}
	}
	slices.Sort(out)
	return out
}
