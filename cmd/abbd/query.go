package main

import (
	"fmt"
	"strings"

	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/topology"
)

// resolve finds a present unit by its ID or, failing that, by its name.
func resolve(units []model.Unit, s string) (model.Unit, error) {
	present := model.Present(units)
	if u, ok := presentUnit(present, model.ID(s)); ok {
		return u, nil
	}
	var named, near []string
	var hit model.Unit
	for _, u := range present {
		switch name := strings.ToLower(u.Name.Value); {
		case name == strings.ToLower(s):
			named, hit = append(named, string(u.ID)), u
		case strings.Contains(name, strings.ToLower(s)):
			near = append(near, fmt.Sprintf("%s %s", u.ID, u.Name.Value))
		}
	}
	switch {
	case len(named) == 1:
		return hit, nil
	case len(named) > 1:
		return model.Unit{}, fmt.Errorf("%q names %s; pass the ID", s, strings.Join(named, ", "))
	case len(near) > 0:
		return model.Unit{}, fmt.Errorf("no unit is named %q; close: %s", s, strings.Join(near[:min(len(near), 5)], "; "))
	}
	return model.Unit{}, fmt.Errorf("%q is not a unit of the state", s)
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
	u, err := resolve(units, s)
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
	for _, ev := range u.Evidence {
		fmt.Fprintf(w, "evidence: %s\n", ev)
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
		for _, ev := range l.Evidence {
			line += "\n    " + ev.String()
		}
		guesses = append(guesses, line)
	}
	section("inferred couplings", guesses)
	return e.explainHow(units, u)
}

// explainHow prints pstack how's text for a top-level module, marked STALE
// when the module's code moved since it was written.
func (e env) explainHow(units []model.Unit, u model.Unit) error {
	if u.Kind != model.Module || u.Parent != "" {
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
	from, err := resolve(units, a)
	if err != nil {
		return err
	}
	to, err := resolve(units, b)
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
	at := from.ID
	fmt.Fprintf(e.stdout, "%s %s\n", at, name[at])
	for _, h := range hops {
		next, arrow := h.To, fmt.Sprintf("-%s->", h.Kind)
		if !h.Forward {
			next, arrow = h.From, fmt.Sprintf("<-%s-", h.Kind)
		}
		fmt.Fprintf(e.stdout, "  %s %s %s%s\n", arrow, next, name[next], h.Label())
		at = next
	}
	fmt.Fprintf(e.stdout, "%d hop(s), weakest origin %s\n", len(hops), topology.Weakest(hops))
	return nil
}
