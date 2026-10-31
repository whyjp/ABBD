package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/whyjp/abbd/internal/extract"
	"github.com/whyjp/abbd/internal/gitx"
	"github.com/whyjp/abbd/internal/identity"
	"github.com/whyjp/abbd/internal/store"
)

const usage = `abbd ingest [--root DIR] FILE|-
  Reconcile a pstack extraction result into .abbd, keeping IDs stable.`

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "ingest" {
		fmt.Fprintln(stderr, usage)
		return 64
	}
	fs := flag.NewFlagSet("ingest", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", ".", "project root")
	if err := fs.Parse(args[1:]); err != nil || fs.NArg() != 1 {
		fmt.Fprintln(stderr, usage)
		return 64
	}
	if err := ingest(*root, fs.Arg(0), stdin, stdout, stderr); err != nil {
		fmt.Fprintln(stderr, "abbd:", err)
		return 1
	}
	return 0
}

func ingest(root, src string, stdin io.Reader, stdout, stderr io.Writer) error {
	in := stdin
	if src != "-" {
		f, err := os.Open(src)
		if err != nil {
			return err
		}
		defer f.Close()
		in = f
	}
	units, err := extract.Parse(in)
	if err != nil {
		return err
	}
	st := store.Open(root)
	stored, err := st.LoadUnits()
	if err != nil {
		return err
	}
	prior, err := st.LoadCandidates()
	if err != nil {
		return err
	}
	state, err := st.LoadState()
	if err != nil {
		return err
	}
	tree, err := gitx.Snapshot(root, store.Dir)
	if err != nil {
		return fmt.Errorf("snapshotting the working tree: %w", err)
	}
	renames, ok, err := gitx.Renames(root, state.Fingerprint, tree)
	if err != nil {
		return fmt.Errorf("reading git renames since %s: %w", state.Fingerprint, err)
	}
	if !ok {
		fmt.Fprintf(stderr, "abbd: warning: last sync snapshot %s is gone; file moves since then are not detected\n", state.Fingerprint)
	}

	res := identity.Reconcile(stored, renames, units, prior, state.HighWater)
	if err := st.SaveUnits(res.Units); err != nil {
		return err
	}
	if err := st.SaveCandidates(res.Candidates); err != nil {
		return err
	}
	hash, err := st.MDHash()
	if err != nil {
		return err
	}
	if err := st.SaveState(store.State{Revision: gitx.Head(root), Fingerprint: tree, MDHash: hash, HighWater: res.HighWater}); err != nil {
		return err
	}

	for _, e := range res.Events {
		fmt.Fprintf(stdout, "%s %s\n", e.ID, e.Outcome)
	}
	for _, c := range res.Candidates {
		fmt.Fprintf(stdout, "%s candidate-of %s via %s\n", c.New, c.Existing, c.Shared.Key())
	}
	for _, id := range res.Unmatched {
		fmt.Fprintf(stdout, "%s unmatched\n", id)
	}
	return nil
}
