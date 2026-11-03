package main

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/whyjp/abbd/internal/evidence"
	"github.com/whyjp/abbd/internal/gitx"
	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/screen"
	"github.com/whyjp/abbd/internal/store"
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
// blueprint matches the state, and what the state holds. With --check it
// fails on what the change can still bring current: a STALE state, blueprint
// or module text, and drifted evidence, which dies at the next revision's
// sync. Dead evidence is already out of every view, so it does not fail.
// UNKNOWN fails too: the check cannot vouch for what it could not compare.
// An orphan text does not fail, because no command can restamp it.
func (e env) status() error {
	in, err := e.screenInput()
	if err != nil {
		return err
	}
	var treeErr error
	if in.Tree == "" {
		_, treeErr = gitx.Snapshot(e.root, store.Dir)
	}
	if len(in.Units) == 0 {
		fmt.Fprintln(e.stdout, "state: none; run abbd setup -- EXTRACT-ARGV")
		if e.check {
			return fmt.Errorf("not current: there is no state")
		}
		return nil
	}
	var behind []string
	switch {
	case in.Synced.Merged:
		behind = append(behind, "the state is STALE: a merge kept two syncs, run abbd sync on the merged tree")
	case in.ActualFreshness() == screen.Stale:
		behind = append(behind, "the state is STALE, run abbd sync")
	case treeErr != nil:
		behind = append(behind, fmt.Sprintf("the state is UNKNOWN: git could not fingerprint the working tree: %v", treeErr))
	case in.Tree == "":
		behind = append(behind, fmt.Sprintf("the state is UNKNOWN: %s is not a git work tree, so abbd cannot fingerprint the code; run the check in a git checkout", e.root))
	case in.Synced.Fingerprint == "":
		behind = append(behind, "the state is UNKNOWN: it was synced without git and has no fingerprint, run abbd sync in a git checkout")
	}
	switch in.BlueprintFreshness() {
	case screen.Stale:
		behind = append(behind, "the blueprint is STALE, run abbd sync")
	case screen.Unknown:
		behind = append(behind, "the blueprint is UNKNOWN: blueprint/ holds no files or the state does not say what was rendered, run abbd sync")
	}
	switch {
	case in.Synced.Merged:
		fmt.Fprintf(e.stdout, "state: STALE, a merge kept two syncs, one at %s; abbd sync on the merged tree writes one\n", in.Synced.Revision)
	case in.ActualFreshness() == screen.Stale:
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
	var orphans []model.ID
	texts := map[string]string{}
	for id, u := range in.Understandings {
		if _, ok := textModule(in.Units, id); !ok {
			orphans = append(orphans, id)
			continue
		}
		texts[string(id)] = u.Text
		if u.Digest != in.Digests[id] {
			stale++
		}
	}
	slices.Sort(orphans)
	fmt.Fprintf(e.stdout, "understanding: %d module text(s), %d STALE, %d orphaned\n", len(in.Understandings), stale, len(orphans))
	for _, id := range orphans {
		fmt.Fprintf(e.stdout, "orphaned: %s is not a module now, so no command restamps its text and the check skips it; delete %s/understanding/%s.md to drop it\n", id, store.Dir, id)
	}
	if stale > 0 {
		behind = append(behind, fmt.Sprintf("%d module text(s) STALE, run pstack how and abbd understand", stale))
	}
	if n := e.statusNotes(in.Units, in.Notes, in.Strays, texts); n > 0 {
		behind = append(behind, fmt.Sprintf("%d note(s) STALE, read the code each cites again and store it with abbd understand ID.SLOT FILE --evidence, or with abbd understand ID.SLOT --reread-by WHO when it still holds as written, or drop it with abbd forget ID.SLOT", n))
	}
	if n := e.statusFlows(in.Units, in.Flows, in.FlowStrays); n > 0 {
		behind = append(behind, fmt.Sprintf("%d flow step(s) STALE, read the code each cites again and store the flow with abbd flow KEY FILE, or delete the file status names", n))
	}
	if len(texts) > 0 {
		readTexts(e.stdout, texts)
	}
	ev := evidenceCounts(in.Units)
	fmt.Fprintf(e.stdout, "evidence: %d drifted, %d dead\n", ev[model.Drifted], ev[model.Dead])
	if ev[model.Drifted] > 0 {
		behind = append(behind, fmt.Sprintf("%d evidence item(s) drifted, abbd evidence lists them", ev[model.Drifted]))
	}
	for _, u := range in.Units {
		for _, m := range u.Masks {
			fmt.Fprintf(e.stdout, "retracted: %s %s by a %s: %s\n", u.ID, m.Target, m.Writer, m.Reason)
		}
	}
	for _, c := range model.ParentConflicts(in.Units) {
		fmt.Fprintln(e.stdout, "conflict:", c)
	}
	if e.check && len(behind) > 0 {
		return fmt.Errorf("not current: %s", strings.Join(behind, "; "))
	}
	return nil
}

// dropNote says how to remove an orphaned note: by abbd forget when its file
// has a ref that names it alone, else by deleting the file.
func dropNote(n store.Note, does string) string {
	if ref, ok := n.FileRef(); ok {
		return fmt.Sprintf("abbd forget %s %s", ref, does)
	}
	return fmt.Sprintf("delete %s to drop it", n.File)
}

// statusNotes reports the notes in slots: how many, which are STALE with the
// places that moved, which are orphaned, so the check skips them, and the
// markdown files beside them that are no notes. It
// adds each note the check reads to texts, for the readability report, and
// returns how many are STALE.
func (e env) statusNotes(units []model.Unit, notes map[model.ID][]store.Note, strays []string, texts map[string]string) int {
	var total, stale, orphans int
	var lines []string
	for _, id := range slices.Sorted(maps.Keys(notes)) {
		u, present := presentUnit(units, id)
		for _, n := range notes[id] {
			total++
			ref := string(id) + "." + n.Slot
			switch {
			case n.Misfiled:
				orphans++
				lines = append(lines, fmt.Sprintf("orphaned: %s says it is %s, whose note belongs at %s, so nothing reads it and the check skips it; %s", n.File, ref, store.NoteFile(id, n.Slot), dropNote(n, "drops it")))
				continue
			case !present || !u.Kind.HoldsNotes() || !n.Known:
				orphans++
				lines = append(lines, fmt.Sprintf("orphaned: %s is no slot of a module or a contract now, so the check skips it; %s", ref, dropNote(n, "removes "+n.File)))
				continue
			case n.Ref.Unheld(u):
				orphans++
				lines = append(lines, fmt.Sprintf("orphaned: %s lists no entry %s now, so the check skips its note; %s", id, n.Ref.Key, dropNote(n, "removes "+n.File)))
				continue
			case n.Stale():
				stale++
				lines = append(lines, fmt.Sprintf("stale: %s, the code it cites changed: %s; read that code again and store the note with abbd understand %s FILE --evidence PATH:LINES:QUOTE, or with abbd understand %s --reread-by WHO when it still holds as written, or drop it with abbd forget %s", ref, strings.Join(n.Changed, ", "), ref, ref, ref))
			}
			texts[ref] = n.Text
		}
	}
	fmt.Fprintf(e.stdout, "notes: %d slot note(s), %d STALE, %d orphaned\n", total, stale, orphans)
	for _, f := range strays {
		fmt.Fprintf(e.stdout, "warning: %s has no frontmatter, so it is no note and abbd reads past it; move it out of %s/understanding to silence this\n", f, store.Dir)
	}
	for _, l := range lines {
		fmt.Fprintln(e.stdout, l)
	}
	return stale
}
