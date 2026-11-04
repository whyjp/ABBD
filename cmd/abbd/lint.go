package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/whyjp/abbd/internal/screen"
)

// lintReport is what abbd lint --json prints: every drawing it checked,
// with its breaches and totals, and the sums over them.
type lintReport struct {
	Criteria []screen.Criterion `json:"criteria"`
	Views    []screen.ViewLint  `json:"views"`
	Totals   screen.Totals      `json:"totals"`
}

// lint holds every drawing of the blueprint, or those of one view, to the
// drawing criteria of docs/blueprint-svg.md. It fails only on a hard breach.
func (e env) lint(pos []string) error {
	in, err := e.screenInput()
	if err != nil {
		return err
	}
	if len(in.Units) == 0 {
		return fmt.Errorf("the state holds no units; run abbd sync first")
	}
	only := ""
	if len(pos) > 0 {
		only = pos[0]
	}
	views, err := screen.Lint(screen.Build(in), only)
	if err != nil {
		return err
	}
	if len(views) == 0 {
		return fmt.Errorf("no view is named %q", only)
	}
	r := lintReport{Criteria: screen.Criteria(), Views: views, Totals: screen.Sum(views)}
	if e.asJSON {
		enc := json.NewEncoder(e.stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(r); err != nil {
			return err
		}
	} else {
		printLint(e, r)
	}
	if r.Totals.Hard > 0 {
		return fmt.Errorf("%d hard breaches", r.Totals.Hard)
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
			if b.Hard {
				kind = "hard"
			}
			fmt.Fprintf(e.stdout, "  %s %s  %s\n", b.Criterion, kind, b.Text)
		}
		fmt.Fprintf(e.stdout, "  totals: %s\n", totalsLine(v.Totals))
	}
	fmt.Fprintf(e.stdout, "lint: %d views, %s\n", len(r.Views), totalsLine(r.Totals))
}

func totalsLine(t screen.Totals) string {
	var by []string
	for _, c := range screen.Criteria() {
		if n := t.Criteria[c.ID]; n > 0 {
			by = append(by, fmt.Sprintf("%s %d", c.ID, n))
		}
	}
	return fmt.Sprintf("hard %d, target %d (%s); steps %d, jogs %d, crossings %d, phantoms %d, paths over 2 bends %d, traced bends %d, back %d",
		t.Hard, t.Target, strings.Join(by, ", "), t.Steps, t.Jogs, t.Crossings, t.Phantoms, t.PathsOver2, t.TracedBends, t.Back)
}
