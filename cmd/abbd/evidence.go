package main

import (
	"fmt"
	"slices"
	"strings"

	"github.com/whyjp/abbd/internal/evidence"
	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/screen"
)

// checkEvidence prints, for every claim that shows and cites evidence, whether each
// quote is in the working tree now and where: live, drifted or dead. It reads
// and never writes, so the same tree prints the same lines, and it never
// moves a drifted item to dead; only a sync at a later revision does.
func (e env) checkEvidence() error {
	stored, err := e.st.LoadUnits()
	if err != nil {
		return err
	}
	c := evidence.New(e.root)
	n := map[model.Status]int{}
	line := func(subject string, w model.Writer, via model.Via, ev model.Evidence) {
		now, r := c.Status(ev, "", w != model.Script)
		n[now.Status]++
		if via != "" {
			subject += " via " + string(via)
		}
		switch now.Status {
		case model.Live:
			fmt.Fprintf(e.stdout, "live %s: %s found at %s\n", subject, ev, r.At)
		case model.Drifted:
			since := ""
			if now.Since != "" {
				since = " since " + now.Since[:min(12, len(now.Since))]
			}
			fmt.Fprintf(e.stdout, "drifted%s %s: %s is no longer in the file; refresh or retract it\n", since, subject, ev)
		default:
			fmt.Fprintf(e.stdout, "dead %s: %s (%s)\n", subject, ev, now.Reason)
		}
	}
	units := slices.Clone(stored)
	slices.SortFunc(units, func(a, b model.Unit) int { return strings.Compare(string(a.ID), string(b.ID)) })
	for _, u := range units {
		if u.Absent && u.AbsentReason != model.EvidenceDead {
			continue
		}
		for _, ev := range u.Evidence {
			line(string(u.ID)+" "+u.Name.Value, u.Name.Writer, "", ev)
		}
		if p := u.ShownParent(); p != nil {
			for _, ev := range p.Evidence {
				line(string(u.ID)+".parent "+string(p.To), p.Writer, "", ev)
			}
		}
		for _, r := range u.Relations {
			for _, ev := range r.Evidence {
				line(model.Edge{From: u.ID, To: r.To, Kind: r.Kind}.String(), r.Writer, r.Via, ev)
			}
		}
	}
	fmt.Fprintf(e.stdout, "%d live, %d drifted, %d dead\n", n[model.Live], n[model.Drifted], n[model.Dead])
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
	ev := evidenceCounts(in.Units)
	fmt.Fprintf(e.stdout, "evidence: %d drifted, %d dead\n", ev[model.Drifted], ev[model.Dead])
	for _, u := range in.Units {
		for _, m := range u.Masks {
			fmt.Fprintf(e.stdout, "retracted: %s %s by a %s: %s\n", u.ID, m.Target, m.Writer, m.Reason)
		}
	}
	for _, c := range model.ParentConflicts(in.Units) {
		fmt.Fprintln(e.stdout, "conflict:", c)
	}
	return nil
}
