package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/screen"
	"github.com/whyjp/abbd/internal/store"
)

// lintSchema is the version of the JSON abbd lint prints. It rises when a
// field changes meaning or goes away.
const lintSchema = 1

// lintReport is what abbd lint --json prints: every drawing it checked,
// with its breaches and totals, the sums over them, the ceilings the debt
// file sets, and every way the drawings break the gate.
type lintReport struct {
	Schema   int                `json:"schema"`
	Criteria []screen.Criterion `json:"criteria"`
	Views    []screen.ViewLint  `json:"views"`
	Totals   screen.Totals      `json:"totals"`
	Ceilings map[string]int     `json:"ceilings,omitempty"`
	Failures []string           `json:"failures"`
	// Levels says how each full view stands as a level of abstraction,
	// and what to change in the units when it is off.
	Levels []screen.Level `json:"levels"`
	// Lowerable is what the debt file records above what lint measures.
	Lowerable []string `json:"lowerable,omitempty"`
	// Unphrased names the pairs of overview boxes whose lines carry no
	// phrase, which say only a via word at rest.
	Unphrased []string `json:"unphrased,omitempty"`
	// Undescribed names the drawn boxes that say only their kind, since
	// their units record neither a purpose nor a responsibility.
	Undescribed []model.ID `json:"undescribed,omitempty"`
	// Prose names each stored text that does not read in the language
	// project.yaml names, as unit field, note or figure.
	Prose    []string       `json:"prose,omitempty"`
	Language model.Language `json:"language,omitempty"`
}

// lint holds every drawing of the blueprint, or those of one view, to the
// drawing criteria of docs/blueprint-svg.md and to the debt the repository
// records in .abbd/lint-debt.yaml. It fails on a hard breach a view does not
// owe, and, over every view, on a total past its ceiling.
func (e env) lint(pos []string) error {
	in, err := e.screenInput()
	if err != nil {
		return err
	}
	if len(in.Units) == 0 {
		return fmt.Errorf("the state holds no units; run abbd sync first")
	}
	m := screen.Build(in)
	if e.list {
		return e.listViews(m)
	}
	only := ""
	if len(pos) > 0 {
		only = pos[0]
	}
	if e.record && only != "" {
		return fmt.Errorf("--record records every view; name no VIEW")
	}
	if !e.record && len(e.keepLevel) > 0 {
		return fmt.Errorf("--keep-level is recorded with --record")
	}
	views, err := screen.Lint(m, only)
	if err != nil {
		return err
	}
	if len(views) == 0 {
		return fmt.Errorf("no view is named %q; abbd lint --list names them", only)
	}
	debt, err := e.st.LoadLintDebt()
	if err != nil {
		return err
	}
	levels := levelsOf(m, views)
	if e.record {
		kept, err := keptLevels(levels, debt.Levels, e.keepLevel)
		if err != nil {
			return err
		}
		if off := offLevel(views, levels, kept); len(debt.Views) == 0 && len(debt.Ceilings) == 0 && len(off) > 0 {
			return fmt.Errorf("will not take the first debt while a view with hard breaches stands at the wrong level, since its breaches come from its units: %s. Change the units as each level line of abbd lint says (abbd-setup step 1), rerun setup, and record only the routing debt that stays. A level you can justify stays with --keep-level VIEW=REASON", strings.Join(off, "; "))
		}
		d := screen.Record(views, debt)
		d.Levels = kept
		var unowed []string
		if t := in.Project.Language; t != "" {
			found := foreignProse(in, t)
			d.Prose = proseDebt(found, t, debt.Prose)
			unowed, _ = proseGate(found, t, d.Prose)
		}
		if err := e.st.SaveLintDebt(d); err != nil {
			return err
		}
		if n := len(unowed); n > 0 {
			t := in.Project.Language
			fmt.Fprintf(e.stdout, "lint: prose: %s not in %s left out of the debt, which only shrinks; lint fails until each is written in %s\n", plural(n, "text"), t, t)
		}
		for _, l := range levels {
			if r, ok := kept[l.View]; ok {
				fmt.Fprintf(e.stdout, "lint: kept the level of %s: %s\n", l.View, r)
			}
		}
		fmt.Fprintf(e.stdout, "lint: recorded the debt of %s and the ceilings weighed %d, tees %d, excess turns %d, placement %d, overflow %d, detours %d in %s/lint-debt.yaml\n", plural(len(d.Views), "view"), d.Ceilings["weighed"], d.Ceilings["tees"], d.Ceilings["excess_turns"], d.Ceilings["placement"], d.Ceilings["overflow"], d.Ceilings["detours"], store.Dir)
		return nil
	}
	every := only == ""
	r := lintReport{Schema: lintSchema, Criteria: screen.Criteria(), Views: views, Failures: screen.Gate(views, debt, every), Lowerable: screen.Lowerable(views, debt, every)}
	r.Totals = screen.Sum(views)
	r.Levels = levels
	for i, l := range r.Levels {
		if len(l.Advice) > 0 {
			r.Levels[i].Kept = debt.Levels[l.View]
		}
	}
	if every {
		r.Ceilings = debt.Ceilings
		r.Unphrased = unphrased(in.Units, m.Overview)
		r.Undescribed = undescribed(in.Units)
		// Only a language project.yaml names gates the prose, so the
		// gate does not move with the machine lint runs on.
		if t := in.Project.Language; t != "" {
			r.Language, r.Prose = t, foreignProse(in, t)
			fails, lower := proseGate(r.Prose, t, debt.Prose)
			r.Failures = append(r.Failures, fails...)
			r.Lowerable = append(r.Lowerable, lower...)
		}
	}
	if r.Failures == nil {
		r.Failures = []string{}
	}
	if e.asJSON {
		enc := json.NewEncoder(e.stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(r); err != nil {
			return err
		}
	} else {
		printLint(e, r)
	}
	if n := len(r.Failures); n > 0 {
		return fmt.Errorf("the drawings break the gate in %s", plural(n, "way"))
	}
	return nil
}

func (e env) listViews(m screen.Model) error {
	names := screen.ViewNames(m)
	if e.asJSON {
		return json.NewEncoder(e.stdout).Encode(struct {
			Schema int      `json:"schema"`
			Views  []string `json:"views"`
		}{lintSchema, names})
	}
	for _, n := range names {
		fmt.Fprintln(e.stdout, n)
	}
	return nil
}

func printLint(e env, r lintReport) {
	for _, v := range r.Views {
		if len(v.Breaches) == 0 {
			continue
		}
		fmt.Fprintln(e.stdout, v.View)
		for _, b := range v.Breaches {
			kind := "target"
			switch {
			case b.Debt:
				kind = "hard debt"
			case b.Hard:
				kind = "hard"
			}
			fmt.Fprintf(e.stdout, "  %s %s  %s\n", b.Criterion, kind, b.Text)
		}
		fmt.Fprintf(e.stdout, "  totals: %s\n", totalsLine(v.Totals))
	}
	fmt.Fprintf(e.stdout, "lint: %s, %s\n", plural(len(r.Views), "view"), totalsLine(r.Totals))
	if len(r.Ceilings) > 0 {
		fmt.Fprintf(e.stdout, "lint: ceilings weighed %d, tees %d, excess turns %d, placement %d, overflow %d, detours %d\n", r.Ceilings["weighed"], r.Ceilings["tees"], r.Ceilings["excess_turns"], r.Ceilings["placement"], r.Ceilings["overflow"], r.Ceilings["detours"])
	}
	if n := len(r.Unphrased); n > 0 {
		fmt.Fprintf(e.stdout, "lint: phrases: %s between overview boxes with no live phrase on any relation, so each says only how it arises; give one relation of each a phrase, or more until half carry one:\n", plural(n, "line"))
		for _, p := range r.Unphrased[:min(n, 10)] {
			fmt.Fprintf(e.stdout, "  %s\n", p)
		}
		if n > 10 {
			fmt.Fprintf(e.stdout, "  and %d more; abbd lint --json lists them\n", n-10)
		}
	}
	printProse(e.stdout, r.Prose, r.Language)
	if len(r.Undescribed) > 0 {
		fmt.Fprintf(e.stdout, "lint: %s; abbd brief MOD-ID drafts one for the module, its inside units and the contracts it provides\n", describedLine(r.Undescribed))
	}
	// The levels come last, where a reader of the report's tail sees them.
	for _, l := range r.Levels {
		for _, a := range l.Advice {
			fmt.Fprintf(e.stdout, "level %s: %s\n", l.View, a)
		}
		if l.Kept != "" {
			fmt.Fprintf(e.stdout, "level %s: kept, %s\n", l.View, l.Kept)
		}
	}
	for _, f := range r.Failures {
		fmt.Fprintf(e.stdout, "lint: breaks the gate: %s\n", f)
	}
	for _, f := range r.Lowerable {
		fmt.Fprintf(e.stdout, "lint: may be lowered: %s\n", f)
	}
}

// keptLevels is the reason, by view, that each view stays at a level lint
// would change: the reasons the debt file gave for a view that still has a
// level line, and those --keep-level gives now. A --keep-level must name a
// view with a level line and give a reason.
func keptLevels(levels []screen.Level, prior map[string]string, flags []string) (map[string]string, error) {
	off := map[string]bool{}
	for _, l := range levels {
		off[l.View] = len(l.Advice) > 0
	}
	kept := map[string]string{}
	for v, r := range prior {
		if off[v] {
			kept[v] = r
		}
	}
	for _, f := range flags {
		name, reason, _ := strings.Cut(f, "=")
		name, reason = strings.TrimSpace(name), strings.TrimSpace(reason)
		if name == "" || reason == "" {
			return nil, fmt.Errorf("--keep-level %q: give VIEW=REASON, the view as abbd lint --list names it or its unit ID, and why it stays at its level", f)
		}
		view := ""
		for _, l := range levels {
			if l.View == name || strings.HasPrefix(l.View, name+" ") {
				view = l.View
			}
		}
		if !off[view] {
			return nil, fmt.Errorf("--keep-level %q: no view of that name has a level line to keep", f)
		}
		kept[view] = reason
	}
	if len(kept) == 0 {
		return nil, nil
	}
	return kept, nil
}

// offLevel names each view with a hard breach that a level line says is
// drawn at the wrong level, unless the repository keeps it there. Its
// breaches are the cost of the units, not routing debt.
func offLevel(views []screen.ViewLint, levels []screen.Level, kept map[string]string) []string {
	hard := map[string]bool{}
	for _, v := range views {
		for _, b := range v.Breaches {
			hard[v.View] = hard[v.View] || b.Hard
		}
	}
	var out []string
	for _, l := range levels {
		if _, ok := kept[l.View]; hard[l.View] && len(l.Advice) > 0 && !ok {
			out = append(out, l.View+" ("+l.Advice[0]+")")
		}
	}
	return out
}

// levelsOf is the levels of the full views lint checked.
func levelsOf(m screen.Model, views []screen.ViewLint) []screen.Level {
	checked := map[string]bool{}
	for _, v := range views {
		checked[v.View] = true
	}
	out := []screen.Level{}
	for _, l := range screen.Levels(m) {
		if checked[l.View] {
			out = append(out, l)
		}
	}
	return out
}

func totalsLine(t screen.Totals) string {
	var by []string
	for _, c := range screen.Criteria() {
		if n := t.Criteria[c.ID]; n > 0 {
			by = append(by, fmt.Sprintf("%s %d", c.ID, n))
		}
	}
	return fmt.Sprintf("hard %d, target %d (%s); excess turns %d, placement %d, crossings %d, tees %d, weighed %d; steps %d, jogs %d, phantoms %d, paths over 2 bends %d, traced bends %d, back %d, detours %d",
		t.Hard, t.Target, strings.Join(by, ", "), t.ExcessTurns, t.Placement, t.Crossings, t.Tees, t.Weighed, t.Steps, t.Jogs, t.Phantoms, t.PathsOver2, t.TracedBends, t.Back, t.Detours)
}

// plural is n with the noun, in the plural unless n is 1.
func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}
