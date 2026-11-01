package main

import (
	"encoding/json"
	"fmt"

	"github.com/whyjp/abbd/internal/gitx"
	"github.com/whyjp/abbd/internal/topology"
)

// diff prints how the stored topology differs from the one committed at the
// base revision, for the reply and the PR description. It reads only. With
// --check it exits 1 when the change reaches the boundary between modules,
// so a CI job can label the PR; ABBD itself decides nothing with it.
func (e env) diff() error {
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
	now, err := e.st.LoadUnits()
	if err != nil {
		return err
	}
	d := topology.Compare(was, now)
	if e.asJSON {
		b, err := json.MarshalIndent(struct {
			Base string `json:"base"`
			topology.Diff
		}{base, d}, "", "  ")
		if err != nil {
			return err
		}
		fmt.Fprintln(e.stdout, string(b))
	} else {
		fmt.Fprintf(e.stdout, "topology since %s:\n%s", short(base), d)
	}
	if e.check && d.Boundary() {
		return fmt.Errorf("the change reaches the boundary between modules: a contract's signature changed, a contract was added or removed, or a relation between top-level modules appeared or disappeared")
	}
	return nil
}

func short(rev string) string { return rev[:min(len(rev), 12)] }
