package main

import (
	"encoding/json"
	"fmt"
	"strings"

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
	// Lowerable is what the debt file records above what lint measures.
	Lowerable []string `json:"lowerable,omitempty"`
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
	if e.record {
		d := screen.Record(views, debt)
		if err := e.st.SaveLintDebt(d); err != nil {
			return err
		}
		fmt.Fprintf(e.stdout, "lint: recorded the debt of %s and the ceilings weighed %d, placement %d in %s/lint-debt.yaml\n", plural(len(d.Views), "view"), d.Ceilings["weighed"], d.Ceilings["placement"], store.Dir)
		return nil
	}
	every := only == ""
	r := lintReport{Schema: lintSchema, Criteria: screen.Criteria(), Views: views, Failures: screen.Gate(views, debt, every), Lowerable: screen.Lowerable(views, debt, every)}
	r.Totals = screen.Sum(views)
	if every {
		r.Ceilings = debt.Ceilings
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
		fmt.Fprintf(e.stdout, "lint: ceilings weighed %d, placement %d\n", r.Ceilings["weighed"], r.Ceilings["placement"])
	}
	for _, f := range r.Failures {
		fmt.Fprintf(e.stdout, "lint: breaks the gate: %s\n", f)
	}
	for _, f := range r.Lowerable {
		fmt.Fprintf(e.stdout, "lint: may be lowered: %s\n", f)
	}
}

func totalsLine(t screen.Totals) string {
	var by []string
	for _, c := range screen.Criteria() {
		if n := t.Criteria[c.ID]; n > 0 {
			by = append(by, fmt.Sprintf("%s %d", c.ID, n))
		}
	}
	return fmt.Sprintf("hard %d, target %d (%s); excess turns %d, placement %d, crossings %d, weighed %d; steps %d, jogs %d, phantoms %d, paths over 2 bends %d, traced bends %d, back %d",
		t.Hard, t.Target, strings.Join(by, ", "), t.ExcessTurns, t.Placement, t.Crossings, t.Weighed, t.Steps, t.Jogs, t.Phantoms, t.PathsOver2, t.TracedBends, t.Back)
}

// plural is n with the noun, in the plural unless n is 1.
func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}
