package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/whyjp/abbd/internal/bootstrap"
	"github.com/whyjp/abbd/internal/confirm"
	"github.com/whyjp/abbd/internal/extract"
	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/topology"
)

// setup never asks. It ends bootstrapped, except greenfield, where the human
// already stated the design units and the project is aligned at once.
func (e env) setup(string, []string) error {
	if e.greenfield != "" {
		if len(e.argv) > 0 || e.supplement != "" || e.adopt != "" {
			return fmt.Errorf("--greenfield takes no extraction script, --supplement or --adopt")
		}
		return e.setupGreenfield()
	}
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
	rep := bootstrap.Report{Mode: "code-only", Extract: argv, Overlays: map[string]bootstrap.Findings{}, Failures: map[string]string{}}
	sum := sha256.Sum256(out)
	rep.Digest = hex.EncodeToString(sum[:8])

	if e.supplement != "" {
		doc, err := e.readExtraction(e.supplement)
		if err != nil {
			return fmt.Errorf("supplement: %w", err)
		}
		next, f, err := bootstrap.Overlay(ex, doc, bootstrap.Supplement)
		if err != nil {
			return err
		}
		ex, rep.Overlays[bootstrap.Supplement.Name] = next, f
	}
	if e.adopt != "" {
		rep.Mode = "adopt"
		if next, f, err := e.adoptDoc(ex); err != nil {
			rep.Failures[bootstrap.Adopt.Name] = err.Error()
			fmt.Fprintln(e.stderr, "abbd: warning: adopt not applied:", err)
		} else {
			ex, rep.Overlays[bootstrap.Adopt.Name] = next, f
		}
	}

	p.Extract = argv
	if err := e.st.SaveProject(p); err != nil {
		return err
	}
	if _, err := e.syncActual(ex); err != nil {
		return err
	}
	v, err := e.load()
	if err != nil {
		return err
	}
	rep.Units = topology.Present(v.units)
	rep.Queue = confirm.Queue(v.units, v.designIDs())
	if v.phase == topology.Bootstrapped {
		rep.NextSteps = []string{"abbd align --all", "abbd align --confirm IDS", "abbd align --exclude IDS"}
	}
	if err := e.st.SaveBootstrapReport(rep.Markdown()); err != nil {
		return err
	}
	fmt.Fprintf(e.stdout, "%s: %d design unit candidate(s), %d item(s) in the confirm queue, report in .abbd/bootstrap-report.md\n",
		v.phase, len(rep.Units), len(rep.Queue))
	return nil
}

// deterministic runs the script twice. A script whose output changes while the
// code does not would make every later gate decision noise.
func (e env) deterministic(argv []string) ([]byte, error) {
	first, err := e.runScript(argv)
	if err != nil {
		return nil, err
	}
	second, err := e.runScript(argv)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(first, second) {
		return nil, fmt.Errorf("the extraction script is not deterministic: two runs on the same code printed different output (first difference at line %d); sort its output and drop timestamps", firstDiffLine(first, second))
	}
	return first, nil
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

func (e env) adoptDoc(ex extract.Result) (extract.Result, bootstrap.Findings, error) {
	doc, err := e.readExtraction(e.adopt)
	if err != nil {
		return ex, bootstrap.Findings{}, err
	}
	return bootstrap.Overlay(ex, doc, bootstrap.Adopt)
}

func (e env) setupGreenfield() error {
	v, err := e.load()
	if err != nil {
		return err
	}
	if v.phase != topology.Uninitialized {
		return fmt.Errorf("greenfield setup needs a project with no design units; this one is %s", v.phase)
	}
	doc, err := e.readExtraction(e.greenfield)
	if err != nil {
		return err
	}
	if len(doc.Units) == 0 {
		return fmt.Errorf("state at least one design unit")
	}
	if _, err := e.syncActual(doc.StatedBy(model.UserStated)); err != nil {
		return err
	}
	v, err = e.load()
	if err != nil {
		return err
	}
	present := topology.Present(v.units)
	all := map[model.ID]bool{}
	for _, u := range present {
		all[u.ID] = true
	}
	if err := e.st.SaveIntended(topology.Select(v.units, all)); err != nil {
		return err
	}
	rep := bootstrap.Report{Mode: "greenfield", Units: present, Queue: confirm.Queue(v.units, all)}
	if err := e.st.SaveBootstrapReport(rep.Markdown()); err != nil {
		return err
	}
	fmt.Fprintf(e.stdout, "aligned: %d design unit(s) stated by a human\n", len(present))
	return nil
}

// readExtraction resolves a relative path against the root, where the
// extraction script runs, not against the cwd of abbd.
func (e env) readExtraction(path string) (extract.Result, error) {
	if !filepath.IsAbs(path) {
		path = filepath.Join(e.root, path)
	}
	f, err := os.Open(path)
	if err != nil {
		return extract.Result{}, err
	}
	defer f.Close()
	return extract.Parse(f)
}
