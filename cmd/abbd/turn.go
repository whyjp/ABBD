package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/whyjp/abbd/internal/gitx"
	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/store"
	"github.com/whyjp/abbd/internal/topology"
	"github.com/whyjp/abbd/internal/turn"
)

func splitList(s string) []string {
	var out []string
	for _, f := range strings.Split(s, ",") {
		if f = strings.TrimSpace(f); f != "" {
			out = append(out, f)
		}
	}
	return out
}

// extractorHash fingerprints what the gate trusts to read the code: the
// registered argv, each project file it names, and all of .abbd/extractor/.
func (e env) extractorHash() (string, error) {
	p, err := e.st.LoadProject()
	if err != nil || len(p.Extract) == 0 {
		return "", err
	}
	files := scriptFiles(e.root, p.Extract)
	err = filepath.WalkDir(filepath.Join(e.root, store.Dir, "extractor"), func(path string, d fs.DirEntry, err error) error {
		if err == nil && d.Type().IsRegular() {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	slices.Sort(files)
	h := sha256.New()
	fmt.Fprintf(h, "%q\n", p.Extract)
	for _, f := range slices.Compact(files) {
		b, err := os.ReadFile(f)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(h, "%s %d\n", relTo(e.root, f), len(b))
		h.Write(b)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func (e env) tree() (string, error) {
	tree, err := gitx.Snapshot(e.root, store.Dir)
	if err != nil {
		return "", fmt.Errorf("snapshotting the working tree: %w", err)
	}
	if tree == "" {
		return "", fmt.Errorf("a turn needs a git repository to fingerprint the working tree; run git init")
	}
	return tree, nil
}

// current returns the latest turn when ok accepts its state.
func (e env) current(ok func(turn.State) bool, what string) (*turn.Turn, error) {
	t, err := e.st.LatestTurn()
	if err != nil {
		return nil, err
	}
	if t == nil || !ok(t.State) {
		return nil, fmt.Errorf("no turn to %s; start one with abbd turn start", what)
	}
	if t.Session != e.session {
		return nil, fmt.Errorf("%s belongs to session %s, not %s", t.ID, t.Session, e.session)
	}
	return t, nil
}

// knownUnits checks that every id names a unit of either topology.
func knownUnits(v view, ids []string) ([]model.ID, error) {
	known := v.designIDs()
	for _, u := range topology.Present(v.units) {
		known[u.ID] = true
	}
	var out []model.ID
	for _, s := range ids {
		if !known[model.ID(s)] {
			return nil, fmt.Errorf("%q is not a unit of this project", s)
		}
		out = append(out, model.ID(s))
	}
	return out, nil
}

// impact loads the view and restores any computed impact item the turn lost.
func (e env) impact(t *turn.Turn) (view, error) {
	v, err := e.load()
	if err != nil {
		return v, err
	}
	if v.intended == nil {
		return v, fmt.Errorf("nothing is aligned yet")
	}
	for _, id := range t.EnsureImpact(turn.ComputeImpact(*v.intended, t.Seeds)) {
		fmt.Fprintf(e.stderr, "abbd: %s is computed impact and cannot be removed; restored\n", id)
	}
	return v, nil
}

func (e env) turnStart() error {
	if !e.st.ModeOn(e.session) {
		return fmt.Errorf("session %s is not active; run abbd mode on --session %s", e.session, e.session)
	}
	if t, err := e.st.LatestTurn(); err != nil {
		return err
	} else if t != nil && t.State.Live() {
		return fmt.Errorf("%s is %s; end it with abbd turn end before starting another", t.ID, t.State)
	}
	if e.seeds == "" || e.request == "" {
		return fmt.Errorf("turn start needs --seeds and --request")
	}
	text := e.request
	if text == "-" {
		b, err := io.ReadAll(e.stdin)
		if err != nil {
			return err
		}
		text = string(b)
	}
	if err := e.refresh(); err != nil {
		return err
	}
	v, err := e.load()
	if err != nil {
		return err
	}
	if v.phase != topology.Aligned {
		fmt.Fprintf(e.stdout, "%s: the project must be aligned before a turn opens\n", v.phase)
		e.printItems(v.items)
		return errNeedHuman
	}
	seeds, err := knownUnits(v, splitList(e.seeds))
	if err != nil {
		return err
	}
	tree, err := e.tree()
	if err != nil {
		return err
	}
	script, err := e.extractorHash()
	if err != nil {
		return err
	}
	req, err := e.st.NextID("REQ")
	if err != nil {
		return err
	}
	chg, err := e.st.NextID("CHG")
	if err != nil {
		return err
	}
	t := turn.Turn{
		ID: string(chg), Requirement: string(req), Session: e.session, State: turn.Open,
		Baseline: turn.Baseline{Revision: gitx.Head(e.root), Fingerprint: tree, Extractor: script},
		Seeds:    seeds, Impact: turn.ComputeImpact(*v.intended, seeds),
	}
	if err := e.st.WriteRequirement(store.Requirement{ID: string(req), Origin: model.UserStated, Turn: t.ID}, text); err != nil {
		return err
	}
	if err := e.st.SaveTurn(t); err != nil {
		return err
	}
	fmt.Fprintf(e.stdout, "%s OPEN, request %s\n", t.ID, req)
	e.printImpact(t)
	return nil
}

func (e env) printImpact(t turn.Turn) {
	for _, i := range t.Impact {
		fmt.Fprintf(e.stdout, "impact %s: %s (%s)\n", i.Unit, i.Why, i.By)
	}
}

func (e env) turnImpact() error {
	t, err := e.current(turn.State.Live, "change")
	if err != nil {
		return err
	}
	v, err := e.impact(t)
	if err != nil {
		return err
	}
	add, err := knownUnits(v, splitList(e.add))
	if err != nil {
		return err
	}
	for _, id := range add {
		t.AddImpact(id)
	}
	if err := e.st.SaveTurn(*t); err != nil {
		return err
	}
	e.printImpact(*t)
	return nil
}

func (e env) turnDeclare() error {
	t, err := e.current(turn.State.Live, "declare")
	if err != nil {
		return err
	}
	v, err := e.impact(t)
	if err != nil {
		return err
	}
	units, err := knownUnits(v, splitList(e.units))
	if err != nil {
		return err
	}
	adrs, err := e.st.LoadADRs()
	if err != nil {
		return err
	}
	answered := func(q string) string {
		for _, a := range adrs {
			if a.Question == q && a.Decision == "approved" {
				return a.ID
			}
		}
		return ""
	}
	t.Delta = turn.Delta{Units: units}
	var lines []string
	waiting := false
	for _, key := range splitList(e.changes) {
		it, grade, err := turn.Declared(*v.intended, v.units, key)
		if err != nil {
			return err
		}
		t.Delta.Changes = append(t.Delta.Changes, key)
		switch {
		case grade == turn.B:
			lines = append(lines, fmt.Sprintf("B PROCEED %s: proceeding by default; recorded as an UNCONFIRMED ADR when the turn syncs", key))
		case grade != turn.C:
			lines = append(lines, "A "+key)
		case t.Approved(key) != "":
			lines = append(lines, fmt.Sprintf("C APPROVED %s by %s", key, t.Approved(key)))
		case t.Rejected(key) != nil:
			waiting = true
			lines = append(lines, fmt.Sprintf("C %s; undo it or declare without it", t.Rejected(key)))
		case answered(turn.Question(*v.intended, it)) != "":
			adr := answered(turn.Question(*v.intended, it))
			t.Approvals = append(t.Approvals, turn.Approval{Item: key, ADR: adr})
			lines = append(lines, fmt.Sprintf("C APPROVED %s by %s, answered before", key, adr))
		default:
			if !slices.Contains(t.Questions, key) {
				t.Questions = append(t.Questions, key)
			}
			waiting = true
			lines = append(lines, fmt.Sprintf("C QUESTION %s: a human must run abbd approve %s before it is made", key, key))
		}
	}
	if len(t.Questions) > turn.MaxQuestions {
		return fmt.Errorf("this turn would ask %d questions; at most %d are allowed, so split the work into smaller turns", len(t.Questions), turn.MaxQuestions)
	}
	for _, id := range units {
		if !t.Impacts(id) {
			lines = append(lines, fmt.Sprintf("note: %s is outside the impact; add it with abbd turn impact --add %s", id, id))
		}
	}
	if err := e.st.SaveTurn(*t); err != nil {
		return err
	}
	fmt.Fprintln(e.stdout, t.ID, "declares", len(t.Delta.Units), "unit(s) and", len(t.Delta.Changes), "change(s)")
	for _, l := range lines {
		fmt.Fprintln(e.stdout, l)
	}
	if waiting {
		return errNeedHuman
	}
	return nil
}

// pending finds the grade C item key names in a turn: a question it asked or
// a blocked change the gate sees.
func pending(t *turn.Turn, v view, key string) (*topology.Item, error) {
	var item *topology.Item
	if slices.Contains(t.Questions, key) {
		it, _, err := turn.Declared(*v.intended, v.units, key)
		if err != nil {
			return nil, err
		}
		item = &it
	}
	for _, vd := range turn.Unallowed(turn.Judge(*v.intended, v.units, t.Approvals)) {
		if vd.Item.Key() == key {
			item = &vd.Item
		}
	}
	if item == nil {
		return nil, fmt.Errorf("%s is neither a question of %s nor a blocked change", key, t.ID)
	}
	return item, nil
}

// approveInTurn allows one grade C item for this turn only. It leaves an ADR
// now and enters the intended topology only when the turn syncs.
func (e env) approveInTurn(t *turn.Turn, key string) error {
	v, err := e.impact(t)
	if err != nil {
		return err
	}
	if adr := t.Approved(key); adr != "" {
		fmt.Fprintf(e.stdout, "%s is already approved for %s by %s\n", key, t.ID, adr)
		return nil
	}
	item, err := pending(t, v, key)
	if err != nil {
		return err
	}
	id, err := e.st.NextID("ADR")
	if err != nil {
		return err
	}
	adr := store.ADR{
		ID: string(id), Grade: "C", Subject: key, Decision: "approved", Origin: model.UserStated,
		Question: turn.Question(*v.intended, *item), Turn: t.ID,
	}
	if err := e.st.WriteADR(adr, fmt.Sprintf("Approved %s for %s. It enters the intended topology only if the turn syncs.", key, t.ID)); err != nil {
		return err
	}
	t.Approvals = append(t.Approvals, turn.Approval{Item: key, ADR: adr.ID})
	if err := e.st.SaveTurn(*t); err != nil {
		return err
	}
	fmt.Fprintf(e.stdout, "approved %s for %s as %s\n", key, t.ID, adr.ID)
	return nil
}

// rejectInTurn refuses one grade C item of the turn. The turn stays as it
// is, and every later turn command shows the reason to the agent.
func (e env) rejectInTurn(t *turn.Turn, key string) error {
	v, err := e.impact(t)
	if err != nil {
		return err
	}
	if adr := t.Approved(key); adr != "" {
		return fmt.Errorf("%s is already approved for %s by %s", key, t.ID, adr)
	}
	item, err := pending(t, v, key)
	if err != nil {
		return err
	}
	adr, err := e.writeRejection(key, turn.Question(*v.intended, *item), t.ID)
	if err != nil {
		return err
	}
	t.Rejections = append(t.Rejections, turn.Rejection{Item: key, ADR: adr, Reason: e.reason})
	if err := e.st.SaveTurn(*t); err != nil {
		return err
	}
	fmt.Fprintf(e.stdout, "rejected %s for %s as %s; %s stays %s\n", key, t.ID, adr, t.ID, t.State)
	return nil
}

func (e env) printRejected(t turn.Turn) {
	seen := map[string]bool{}
	for _, r := range t.Rejections {
		if !seen[r.Item] {
			seen[r.Item] = true
			if s := t.Rejected(r.Item); s != nil {
				fmt.Fprintln(e.stdout, s)
			}
		}
	}
}

func (e env) turnDiff() error {
	t, err := e.current(turn.State.Resumable, "compare")
	if err != nil {
		return err
	}
	if err := e.reextract(); err != nil {
		return err
	}
	v, err := e.impact(t)
	if err != nil {
		return err
	}
	if err := e.st.SaveTurn(*t); err != nil {
		return err
	}
	vs := turn.Judge(*v.intended, v.units, t.Approvals)
	for _, vd := range vs {
		fmt.Fprintln(e.stdout, vd)
	}
	for _, key := range turn.Unexpected(t.Delta, vs) {
		fmt.Fprintln(e.stdout, "UNEXPECTED", key)
	}
	e.printRejected(*t)
	if len(turn.Unallowed(vs)) > 0 {
		fmt.Fprintln(e.stdout, "stop: a change crosses the intended boundary; a human must approve it or the work must be undone")
		return errNeedHuman
	}
	return nil
}

func (e env) turnVerify() error {
	t, err := e.current(turn.State.Resumable, "verify")
	if err != nil {
		return err
	}
	p, err := e.st.LoadProject()
	if err != nil {
		return err
	}
	switch {
	case len(e.argv) > 0 && len(p.Verify) == 0:
		p.Verify = e.argv
		if err := e.st.SaveProject(p); err != nil {
			return err
		}
		fmt.Fprintf(e.stdout, "registered the verify command %q\n", p.Verify)
	case len(e.argv) > 0 && !slices.Equal(e.argv, p.Verify):
		return fmt.Errorf("the verify command is fixed: project.yaml records %q; run abbd turn verify without --", p.Verify)
	case len(p.Verify) == 0:
		return fmt.Errorf("project.yaml names no verify command; register it once with abbd turn verify -- VERIFY-ARGV")
	}
	tree, err := e.tree()
	if err != nil {
		return err
	}
	var out bytes.Buffer
	cmd := exec.Command(p.Verify[0], p.Verify[1:]...)
	cmd.Dir, cmd.Stdout, cmd.Stderr = e.root, &out, &out
	code := 0
	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			return fmt.Errorf("running the verify command: %w", err)
		}
		code = exit.ExitCode()
	}
	after, err := e.tree()
	if err != nil {
		return err
	}
	n, err := e.st.NextID("RUN")
	if err != nil {
		return err
	}
	id := "run-" + strings.TrimPrefix(string(n), "RUN-")
	run := turn.Run{ID: id, Turn: t.ID, Command: p.Verify, Tree: tree, ExitCode: code, Result: model.Passed, Output: "verification/runs/" + id + ".log"}
	if after != tree {
		paths, _, _ := gitx.Changed(e.root, tree, after)
		run.Why = turn.EditedTree(paths)
	}
	if code != 0 || run.Why != "" {
		run.Result = model.Failed
	}
	if err := e.st.SaveRun(run, out.Bytes()); err != nil {
		return err
	}
	t.Run = id
	if err := e.st.SaveTurn(*t); err != nil {
		return err
	}
	if run.Result != model.Passed {
		return errors.New(turn.Evidence(&run, tree))
	}
	fmt.Fprintf(e.stdout, "%s PASSED on the current tree; gitignored files are not part of it\n", id)
	return nil
}

// turnEnd is the only way a turn completes. It gates against the intended
// topology plus this turn's approvals, never against the declaration.
func (e env) turnEnd() error {
	t, err := e.current(turn.State.Resumable, "end")
	if err != nil {
		return err
	}
	tree, err := e.tree()
	if err != nil {
		return err
	}
	if tree == t.Baseline.Fingerprint {
		declared, _ := turn.Contrast(t.Delta, nil, nil)
		t.State, t.Report = turn.Closed, &turn.Report{DeclaredNotDone: declared, Why: "the working tree never changed, so there was nothing to gate"}
		if err := e.st.SaveTurn(*t); err != nil {
			return err
		}
		fmt.Fprintf(e.stdout, "%s CLOSED: the working tree never changed\n", t.ID)
		return nil
	}
	if t.Baseline.Extractor != "" {
		script, err := e.extractorHash()
		if err != nil {
			return err
		}
		if script != t.Baseline.Extractor {
			return fmt.Errorf("the extraction script changed during %s (.abbd/extractor/ or a file project.yaml extract names), so the gate cannot trust it; restore it and end the turn again. Changing the script is a human's step outside the mode", t.ID)
		}
	}
	if err := e.reextract(); err != nil {
		return err
	}
	v, err := e.impact(t)
	if err != nil {
		return err
	}
	vs := turn.Judge(*v.intended, v.units, t.Approvals)
	touched, known, err := e.touched(t.Baseline.Fingerprint, tree, v.units)
	if err != nil {
		return fmt.Errorf("listing the files the turn changed: %w", err)
	}
	rep := &turn.Report{}
	for _, vd := range vs {
		rep.Gate = append(rep.Gate, vd.String())
	}
	declared := t.Delta
	if !known {
		declared.Units = nil
		rep.Notes = append(rep.Notes, fmt.Sprintf("the baseline snapshot %s is gone, so which units the turn touched is unknown", t.Baseline.Fingerprint))
	}
	rep.DeclaredNotDone, rep.DoneNotDeclared = turn.Contrast(declared, vs, touched)
	t.Report = rep

	run, err := e.st.LoadRun(t.Run)
	if err != nil {
		return err
	}
	switch {
	case len(turn.Unallowed(vs)) > 0:
		t.State, rep.Why = turn.Blocked, "a change crosses the intended boundary without approval"
		if slices.ContainsFunc(turn.Unallowed(vs), func(vd turn.Verdict) bool { return t.Rejected(vd.Item.Key()) != nil }) {
			rep.Why += "; a human rejected it, so undo it as the reason says"
		}
	case turn.Evidence(run, tree) != "":
		t.State, rep.Why = turn.Draft, turn.Evidence(run, tree)
	default:
		if err := e.sync(t, *v.intended, vs); err != nil {
			return err
		}
		t.State = turn.Synced
	}
	if err := e.st.SaveTurn(*t); err != nil {
		return err
	}
	e.printReport(*t)
	if t.State != turn.Synced {
		return errNeedHuman
	}
	return nil
}

// sync commits this turn's allowed changes into the intended topology, then
// rereads it: the turn completes only if the project is aligned again.
func (e env) sync(t *turn.Turn, in topology.Intended, vs []turn.Verdict) error {
	for _, vd := range vs {
		if vd.Grade != turn.B {
			continue
		}
		id, err := e.st.NextID("ADR")
		if err != nil {
			return err
		}
		adr := store.ADR{
			ID: string(id), Grade: "B", Subject: vd.Item.Key(), Decision: "proceeded by default",
			Origin: model.Inferred, Confirmation: model.Unconfirmed, Question: turn.Question(in, vd.Item), Turn: t.ID,
		}
		if err := e.st.WriteADR(adr, fmt.Sprintf("%s proceeded by default in %s: %s. A human can still reverse it.", vd.Item.Key(), t.ID, vd)); err != nil {
			return err
		}
	}
	if err := e.st.SaveIntended(turn.Fold(in, vs)); err != nil {
		return err
	}
	after, err := e.load()
	if err != nil {
		return err
	}
	if after.phase != topology.Aligned {
		return fmt.Errorf("%s did not leave the project aligned (%s)", t.ID, after.phase)
	}
	return nil
}

// touched lists the units whose anchored files changed. known is false when
// the baseline snapshot is gone and the changed files cannot be listed.
func (e env) touched(from, to string, units []model.Unit) (ids []model.ID, known bool, err error) {
	paths, ok, err := gitx.Changed(e.root, from, to)
	if err != nil || !ok {
		return nil, false, err
	}
	for _, u := range units {
		if slices.ContainsFunc(u.Anchors, func(a model.Anchor) bool {
			return slices.ContainsFunc(paths, func(p string) bool { return turn.Touches(a, p) })
		}) {
			ids = append(ids, u.ID)
		}
	}
	return ids, true, nil
}

func (e env) printReport(t turn.Turn) {
	fmt.Fprintf(e.stdout, "%s %s\n", t.ID, t.State)
	r := t.Report
	if r == nil {
		return
	}
	for _, g := range r.Gate {
		fmt.Fprintln(e.stdout, g)
	}
	for _, s := range r.DeclaredNotDone {
		fmt.Fprintln(e.stdout, "declared but not done:", s)
	}
	for _, s := range r.DoneNotDeclared {
		fmt.Fprintln(e.stdout, "done but not declared:", s)
	}
	for _, s := range r.Notes {
		fmt.Fprintln(e.stdout, "note:", s)
	}
	if r.Why != "" {
		fmt.Fprintln(e.stdout, r.Why)
	}
	e.printRejected(t)
}

func (e env) turnShow() error {
	t, err := e.st.LatestTurn()
	if err != nil {
		return err
	}
	if t == nil {
		fmt.Fprintln(e.stdout, "no turn yet")
		return nil
	}
	if t.State.Live() {
		if _, err := e.impact(t); err != nil {
			return err
		}
		if err := e.st.SaveTurn(*t); err != nil {
			return err
		}
	}
	e.printImpact(*t)
	for _, c := range t.Delta.Changes {
		fmt.Fprintln(e.stdout, "declared", c)
	}
	for _, a := range t.Approvals {
		fmt.Fprintf(e.stdout, "approved %s by %s\n", a.Item, a.ADR)
	}
	e.printReport(*t)
	if t.Report == nil {
		e.printRejected(*t)
	}
	return nil
}
