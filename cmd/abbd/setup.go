package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/whyjp/abbd/internal/bootstrap"
	"github.com/whyjp/abbd/internal/extract"
	"github.com/whyjp/abbd/internal/identity"
	"github.com/whyjp/abbd/internal/model"
)

// setup registers the extraction script, proves it deterministic, folds in
// the semantic supplement and syncs. It never asks.
func (e env) setup() error {
	p, err := e.st.LoadProject()
	if err != nil {
		return err
	}
	argv := e.argv
	if len(argv) == 0 {
		argv = p.Extract
	}
	if len(argv) == 0 {
		return fmt.Errorf("name the extraction script after --, for example: abbd setup -- python .abbd/extractor/extract.py")
	}
	out, err := e.deterministic(argv)
	if err != nil {
		return err
	}
	ex, err := extract.Parse(bytes.NewReader(out))
	if err != nil {
		return fmt.Errorf("extraction script output: %w", err)
	}
	if len(ex.Units) == 0 {
		return fmt.Errorf("the extraction script reported no units")
	}
	var f bootstrap.Findings
	if e.supplement != "" {
		doc, err := e.readSupplement(e.supplement)
		if err != nil {
			return fmt.Errorf("supplement: %w", err)
		}
		if ex, f, err = bootstrap.Overlay(ex, doc); err != nil {
			return err
		}
	}
	p.Extract = portable(e.root, argv)
	if err := e.st.SaveProject(p); err != nil {
		return err
	}
	res, err := e.syncActual(ex, identity.Setup)
	if err != nil {
		return err
	}
	units := res.Units
	for _, id := range res.Stated {
		fmt.Fprintf(e.stdout, "%s kept: the supplement no longer reports it, but a human stated something about it\n", id)
	}
	for _, c := range f.Conflicts {
		fmt.Fprintln(e.stdout, "conflict:", c)
	}
	fmt.Fprintf(e.stdout, "synced: %d unit(s), %d matched by the supplement\n", len(model.Present(units)), f.Matched)
	e.attention(units)
	return nil
}

// portable spells each command that runs this abbd binary, by whatever
// path setup was given, as the bare abbd that runScript resolves to the
// running binary, so project.yaml and the page footer hold no local path.
// A command heads the argv or follows a "--", and a relative path is
// relative to the root, where the script runs.
func portable(root string, argv []string) []string {
	out := slices.Clone(argv)
	self, err := os.Executable()
	for i, a := range out {
		if i > 0 && out[i-1] != "--" || a == "abbd" {
			continue
		}
		path := a
		if !filepath.IsAbs(path) {
			path = filepath.Join(root, path)
		}
		stem := strings.TrimSuffix(strings.ToLower(filepath.Base(a)), ".exe")
		if stem == "abbd" || err == nil && sameFile(path, self) {
			out[i] = "abbd"
		}
	}
	return out
}

func sameFile(a, b string) bool {
	fa, err := os.Stat(a)
	if err != nil {
		return false
	}
	fb, err := os.Stat(b)
	return err == nil && os.SameFile(fa, fb)
}

// deterministic runs the script three times. A script whose output changes
// while the code does not would make every diff noise. The third run changes
// the hash seeds that interpreters use to randomize iteration order per
// process, which two runs miss when the environment pins the seed.
func (e env) deterministic(argv []string) ([]byte, error) {
	first, err := e.runScript(argv)
	if err != nil {
		return nil, err
	}
	reseed := reseeded()
	runs := []struct {
		env  []string
		what string
	}{
		{nil, "two runs on the same code printed different output"},
		{reseed, fmt.Sprintf("a run with %s printed different output", strings.Join(reseed, " "))},
	}
	for _, r := range runs {
		out, err := e.runScript(argv, r.env...)
		if err != nil {
			return nil, err
		}
		if !bytes.Equal(first, out) {
			return nil, fmt.Errorf("the extraction script is not deterministic: %s (first difference at line %d); sort its output and drop timestamps", r.what, firstDiffLine(first, out))
		}
	}
	return first, nil
}

func reseeded() []string {
	var env []string
	for _, name := range []string{"PYTHONHASHSEED", "PERL_HASH_SEED"} {
		seed := "1"
		if os.Getenv(name) == seed {
			seed = "2"
		}
		env = append(env, name+"="+seed)
	}
	return env
}

func firstDiffLine(a, b []byte) int {
	la, lb := strings.Split(string(a), "\n"), strings.Split(string(b), "\n")
	for i := range min(len(la), len(lb)) {
		if la[i] != lb[i] {
			return i + 1
		}
	}
	return min(len(la), len(lb)) + 1
}

// readSupplement resolves a relative path against the root, where the
// extraction script runs, not against the cwd of abbd.
func (e env) readSupplement(path string) (extract.Result, error) {
	if !filepath.IsAbs(path) {
		path = filepath.Join(e.root, path)
	}
	f, err := os.Open(path)
	if err != nil {
		return extract.Result{}, err
	}
	defer f.Close()
	return extract.ParseSupplement(f)
}
