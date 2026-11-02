package main

import (
	"fmt"
	"strings"

	"github.com/whyjp/abbd/internal/gitx"
	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/topology"
)

// shown is how many matches a query prints before it counts the rest.
const shown = 5

// resolve finds the unit a query names: its ID or name, a file, directory or
// symbol its code holds, or a phrase. When the query is not the unit's own ID
// or name, it prints what the query matched, best first, so the reader sees
// why this unit and which others came close.
func (e env) resolve(units []model.Unit, s string) (model.Unit, error) {
	ms, err := e.find(units, s)
	if err != nil {
		return model.Unit{}, err
	}
	return ms[0].Unit, nil
}

// find lists the units s names, best first, printing the list when s is not
// a unit's own ID or name. It errs when s names nothing or several units by
// one name.
func (e env) find(units []model.Unit, s string) ([]topology.Match, error) {
	if _, _, err := model.ParseID(s); err == nil {
		if u, ok := presentUnit(units, model.ID(s)); ok {
			return []topology.Match{{Unit: u, Tier: topology.ByID}}, nil
		}
		return nil, fmt.Errorf("%q is not a unit of the state", s)
	}
	ms := topology.Find(units, func() []string { return gitx.WorkingFiles(e.root) }, s)
	switch {
	case len(ms) == 0:
		var near []string
		for _, u := range topology.Nearest(units, s, shown) {
			near = append(near, fmt.Sprintf("%s %s", u.ID, u.Name.Value))
		}
		return nil, fmt.Errorf("%q names no unit, file, symbol or phrase of the state; nearest: %s", s, strings.Join(near, "; "))
	case ms[0].Tier == topology.ByName && len(ms) > 1:
		var ids []string
		for _, m := range ms {
			ids = append(ids, string(m.Unit.ID))
		}
		return nil, fmt.Errorf("%q names %s; pass the ID", s, strings.Join(ids, ", "))
	case !ms[0].Exact():
		fmt.Fprintf(e.stdout, "%q is not a unit; it matches, best first:\n", s)
		for _, m := range ms[:min(len(ms), shown)] {
			fmt.Fprintf(e.stdout, "  %s %s: %s\n", m.Unit.ID, m.Unit.Name.Value, m.Why)
		}
		if len(ms) > shown {
			fmt.Fprintf(e.stdout, "  and %d more\n", len(ms)-shown)
		}
		fmt.Fprintln(e.stdout)
	}
	return ms, nil
}

func names(units []model.Unit) map[model.ID]string {
	out := map[model.ID]string{}
	for _, u := range model.Present(units) {
		out[u.ID] = u.Name.Value
	}
	return out
}

func origin(f *model.Fact) string {
	s := string(f.Origin)
	if f.Source != "" {
		s += ", " + f.Source
	}
	return s
}

// explain prints one unit at a glance. It reads only.
func (e env) explain(s string) error {
	units, err := e.st.LoadUnits()
	if err != nil {
		return err
	}
	u, err := e.resolve(units, s)
	if err != nil {
		return err
	}
	name := names(units)
	ex := topology.Explain(units, u.ID)
	w := e.stdout
	fmt.Fprintf(w, "%s %s (%s, %s)\n", u.ID, u.Name.Value, u.Kind, u.Name.Origin)
	if u.Parent != "" {
		fmt.Fprintf(w, "inside %s %s\n", u.Parent, name[u.Parent])
	}
	for _, f := range []struct {
		label string
		fact  *model.Fact
	}{{"responsibility", u.Responsibility}, {"purpose", u.Purpose}, {"signature", u.Signature}} {
		if f.fact != nil {
			fmt.Fprintf(w, "%s: %s (%s)\n", f.label, f.fact.Value, origin(f.fact))
		}
	}
	if len(u.Entries) > 0 {
		fmt.Fprintln(w, "entries:")
		uses := topology.EntryUses(units, u.ID)
		for _, en := range u.Entries {
			l := "  " + en.Key + ": " + en.Name
			if look := en.Look(); look != "" {
				l += " " + look
			}
			for _, us := range uses[en.Key] {
				l += fmt.Sprintf("; %s by %s %s", us.Kind, us.From, name[us.From])
			}
			fmt.Fprintln(w, l)
		}
	}
	for _, ev := range u.Evidence {
		fmt.Fprintf(w, "evidence: %s%s\n", ev, mark(ev))
	}
	if len(u.Parents) > 1 {
		var cs []string
		for _, c := range u.Parents {
			cs = append(cs, fmt.Sprintf("%s by the %s", place(c.To), c.Writer))
		}
		fmt.Fprintf(w, "parent claims: %s\n", strings.Join(cs, ", "))
	}
	section := func(title string, lines []string) {
		if len(lines) == 0 {
			return
		}
		fmt.Fprintln(w, title+":")
		for _, l := range lines {
			fmt.Fprintln(w, "  "+l)
		}
	}
	var inside []string
	for _, r := range ex.Inside {
		inside = append(inside, fmt.Sprintf("%s %s (%s)", r.ID, r.Name, r.Kind))
	}
	section("inside", inside)
	out := map[model.RelationKind][]string{}
	through := func(inside model.ID) string {
		if inside == u.ID {
			return ""
		}
		return fmt.Sprintf(" via inside %s %s", inside, name[inside])
	}
	for _, l := range ex.Out {
		out[l.Kind] = append(out[l.Kind], fmt.Sprintf("%s %s%s%s", l.To, name[l.To], l.Label(), through(l.From)))
	}
	section("provides", out[model.Provides])
	section("consumes", out[model.Consumes])
	section("depends on", out[model.DependsOn])
	var in []string
	for _, l := range ex.In {
		in = append(in, fmt.Sprintf("%s %s %s%s%s", l.From, name[l.From], l.Kind, l.Label(), through(l.To)))
	}
	section("used by", in)
	var guesses []string
	for _, l := range ex.Inferred {
		line := fmt.Sprintf("%s %s %s %s %s", l.From, name[l.From], l.Kind, l.To, name[l.To])
		if l.Via != "" {
			line += " via " + string(l.Via)
		}
		if l.Drifted {
			line += " (drifted: refresh or retract it)"
		}
		for _, ev := range l.Evidence {
			line += "\n    " + ev.String() + mark(ev)
		}
		guesses = append(guesses, line)
	}
	section("inferred couplings", guesses)
	if err := e.explainHow(units, u); err != nil {
		return err
	}
	return e.explainNotes(u)
}

// explainNotes prints what readers wrote into the unit's slots, each marked
// STALE with the places that moved when the code it cites changed since.
func (e env) explainNotes(u model.Unit) error {
	if !u.Kind.HoldsNotes() {
		return nil
	}
	notes, _, err := e.st.LoadNotes()
	if err != nil {
		return err
	}
	for _, n := range notes[u.ID] {
		if !n.Known {
			continue
		}
		state := "CURRENT"
		if n.Stale() {
			state = "STALE, the code it cites changed: " + strings.Join(n.Changed, ", ")
		}
		fmt.Fprintf(e.stdout, "%s (%s, %s):\n", n.Ref.Name(), n.Origin, state)
		for _, l := range strings.Split(n.Text, "\n") {
			fmt.Fprintln(e.stdout, strings.TrimRight("  "+l, " "))
		}
		if n.Why != nil {
			fmt.Fprintf(e.stdout, "  why (USER_STATED, %s): %s\n", n.Why.Source, n.Why.Value)
		}
		for _, ev := range n.Evidence {
			fmt.Fprintf(e.stdout, "  cites %s\n", ev)
		}
	}
	return nil
}

// mark notes an item that is not live.
func mark(ev model.Evidence) string {
	switch ev.Status {
	case model.Drifted:
		return " (drifted)"
	case model.Dead:
		return " (dead: " + ev.Reason + ")"
	}
	return ""
}

// explainHow prints pstack how's text for a module, marked STALE
// when the module's code moved since it was written.
func (e env) explainHow(units []model.Unit, u model.Unit) error {
	if _, ok := textModule(units, u.ID); !ok {
		return nil
	}
	texts, err := e.st.LoadUnderstandings()
	if err != nil {
		return err
	}
	t, ok := texts[u.ID]
	if !ok {
		fmt.Fprintf(e.stdout, "how: none; abbd understand %s FILE stores pstack how's text\n", u.ID)
		return nil
	}
	state := "CURRENT"
	if t.Digest != e.st.ModuleDigest(units, u.ID) {
		state = "STALE, the code changed since it was written"
	}
	fmt.Fprintf(e.stdout, "how (%s, %s):\n", t.Origin, state)
	for _, l := range strings.Split(t.Text, "\n") {
		fmt.Fprintln(e.stdout, strings.TrimRight("  "+l, " "))
	}
	return nil
}

// path prints the shortest path between two units, hop by hop, and the
// origin of its least certain hop. It reads only.
func (e env) path(a, b string) error {
	units, err := e.st.LoadUnits()
	if err != nil {
		return err
	}
	from, err := e.resolve(units, a)
	if err != nil {
		return err
	}
	to, err := e.resolve(units, b)
	if err != nil {
		return err
	}
	if from.ID == to.ID {
		return fmt.Errorf("%s and %s are the same unit", a, b)
	}
	hops, ok := topology.Path(units, from.ID, to.ID)
	if !ok {
		fmt.Fprintf(e.stdout, "no path from %s %s to %s %s\n", from.ID, from.Name.Value, to.ID, to.Name.Value)
		return nil
	}
	name := names(units)
	before, at := from.ID, from.ID
	fmt.Fprintf(e.stdout, "%s %s\n", at, name[at])
	for _, h := range hops {
		next, arrow := h.To, fmt.Sprintf("-%s->", h.Kind)
		if !h.Forward {
			next, arrow = h.From, fmt.Sprintf("<-%s-", h.Kind)
		}
		fmt.Fprintf(e.stdout, "  %s %s %s%s\n", arrow, next, name[next], h.Label())
		if h.Shares != "" {
			verb := map[model.RelationKind]string{model.Consumes: "consume", model.DependsOn: "depend on"}[h.Kind]
			provider := ""
			if h.Provider != "" {
				provider = fmt.Sprintf(", which %s %s provides", h.Provider, name[h.Provider])
			}
			fmt.Fprintf(e.stdout, "      both %s %s %s%s; no relation joins %s and %s\n", verb, h.Shares, name[h.Shares], provider, before, next)
		}
		before, at = at, next
	}
	fmt.Fprintf(e.stdout, "%d hop(s), weakest origin %s\n", len(hops), topology.Weakest(hops))
	return nil
}
