package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/whyjp/abbd/internal/extract"
	"github.com/whyjp/abbd/internal/gitx"
	"github.com/whyjp/abbd/internal/identity"
	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/store"
	"github.com/whyjp/abbd/internal/topology"
)

const usage = `abbd ingest [--root DIR] FILE|-     reconcile an extraction result into .abbd
abbd align [--root DIR]              confirm design units (first alignment or short realignment)
abbd approve [--root DIR] ITEM       approve one BLOCKED (grade C) change
abbd mode on|off --session ID [--root DIR]
abbd status [--root DIR] [--session ID]
abbd hook EVENT [--root DIR]         hook entry point; reads the hook JSON on stdin`

const (
	exitOK        = 0
	exitError     = 1
	exitNeedHuman = 2
	exitUsage     = 64
)

// errNeedHuman means the command did its part and a human decision is now required.
var errNeedHuman = errors.New("needs a human decision")

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

type env struct {
	root           string
	session        string
	stdin          io.Reader
	stdout, stderr io.Writer
	st             store.Store
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, usage)
		return exitUsage
	}
	cmd, args := args[0], args[1:]
	var sub string
	if (cmd == "mode" || cmd == "hook") && len(args) > 0 {
		sub, args = args[0], args[1:]
	}
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	fs.SetOutput(stderr)
	e := env{stdin: stdin, stdout: stdout, stderr: stderr}
	fs.StringVar(&e.root, "root", ".", "project root")
	fs.StringVar(&e.session, "session", "", "Claude Code session id")
	pos, err := parseInterspersed(fs, args)
	if err != nil {
		return exitUsage
	}
	e.st = store.Open(e.root)

	switch {
	case cmd == "ingest" && len(pos) == 1:
		err = e.ingest(pos[0])
	case cmd == "align" && len(pos) == 0:
		err = e.align()
	case cmd == "approve" && len(pos) == 1:
		err = e.approve(pos[0])
	case cmd == "mode" && (sub == "on" || sub == "off") && e.session != "":
		err = e.mode(sub == "on")
	case cmd == "status" && len(pos) == 0:
		err = e.status()
	case cmd == "hook" && sub != "":
		e.hook(sub)
		return exitOK
	default:
		fmt.Fprintln(stderr, usage)
		return exitUsage
	}
	switch {
	case errors.Is(err, errNeedHuman):
		return exitNeedHuman
	case err != nil:
		fmt.Fprintln(stderr, "abbd:", err)
		return exitError
	}
	return exitOK
}

// parseInterspersed accepts flags before, between and after positional arguments.
func parseInterspersed(fs *flag.FlagSet, args []string) ([]string, error) {
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		args = fs.Args()
		if len(args) == 0 {
			return pos, nil
		}
		pos, args = append(pos, args[0]), args[1:]
	}
}

func (e env) ingest(src string) error {
	in := e.stdin
	if src != "-" {
		f, err := os.Open(src)
		if err != nil {
			return err
		}
		defer f.Close()
		in = f
	}
	ex, err := extract.Parse(in)
	if err != nil {
		return err
	}
	res, err := e.syncActual(ex)
	if err != nil {
		return err
	}
	for _, ev := range res.Events {
		fmt.Fprintf(e.stdout, "%s %s\n", ev.ID, ev.Outcome)
	}
	for _, c := range res.Candidates {
		fmt.Fprintf(e.stdout, "%s candidate-of %s via %s\n", c.New, c.Existing, c.Shared.Key())
	}
	for _, id := range res.Unmatched {
		fmt.Fprintf(e.stdout, "%s unmatched\n", id)
	}
	return nil
}

// syncActual updates the actual topology. It only ever touches EXTRACTED facts.
func (e env) syncActual(ex extract.Result) (identity.Result, error) {
	st := e.st
	stored, err := st.LoadUnits()
	if err != nil {
		return identity.Result{}, err
	}
	prior, err := st.LoadCandidates()
	if err != nil {
		return identity.Result{}, err
	}
	state, err := st.LoadState()
	if err != nil {
		return identity.Result{}, err
	}
	tree, err := gitx.Snapshot(e.root, store.Dir)
	if err != nil {
		return identity.Result{}, fmt.Errorf("snapshotting the working tree: %w", err)
	}
	renames, ok, err := gitx.Renames(e.root, state.Fingerprint, tree)
	if err != nil {
		return identity.Result{}, fmt.Errorf("reading git renames since %s: %w", state.Fingerprint, err)
	}
	if !ok {
		fmt.Fprintf(e.stderr, "abbd: warning: last sync snapshot %s is gone; file moves since then are not detected\n", state.Fingerprint)
	}

	if len(ex.Units) == 0 && len(stored) > 0 {
		return identity.Result{}, fmt.Errorf("the extraction reported no units; refusing to mark every unit absent")
	}
	res := identity.Reconcile(stored, renames, ex, prior, state.HighWater)
	if err := st.SaveUnits(res.Units); err != nil {
		return res, err
	}
	if err := st.SaveCandidates(res.Candidates); err != nil {
		return res, err
	}
	hash, err := st.MDHash()
	if err != nil {
		return res, err
	}
	state = store.State{Revision: gitx.Head(e.root), Fingerprint: tree, MDHash: hash, HighWater: res.HighWater}
	return res, st.SaveState(state)
}

type view struct {
	units    []model.Unit
	intended *topology.Intended
	phase    topology.Phase
	items    []topology.Item
}

func (e env) load() (view, error) {
	units, err := e.st.LoadUnits()
	if err != nil {
		return view{}, err
	}
	in, err := e.st.LoadIntended()
	if err != nil {
		return view{}, err
	}
	v := view{units: units, intended: in, phase: topology.DerivePhase(units, in)}
	if in != nil {
		v.items = topology.Compare(*in, units)
	}
	return v, nil
}

func (e env) printItems(items []topology.Item) {
	for _, it := range items {
		fmt.Fprintln(e.stdout, it)
	}
}

func (e env) align() error {
	v, err := e.load()
	if err != nil {
		return err
	}
	var next topology.Intended
	switch v.phase {
	case topology.Uninitialized:
		return fmt.Errorf("nothing to align: run setup first")
	case topology.Bootstrapped:
		next = topology.FromActual(v.units)
	default:
		next, err = topology.RealignTo(*v.intended, v.units)
		if err != nil {
			e.printItems(blocked(v.items))
			fmt.Fprintln(e.stderr, "abbd:", err)
			return errNeedHuman
		}
	}
	if err := e.st.SaveIntended(next); err != nil {
		return err
	}
	fmt.Fprintln(e.stdout, "aligned")
	return nil
}

func blocked(items []topology.Item) []topology.Item {
	var out []topology.Item
	for _, it := range items {
		if it.Effect == topology.Blocked {
			out = append(out, it)
		}
	}
	return out
}

func (e env) approve(key string) error {
	v, err := e.load()
	if err != nil {
		return err
	}
	if v.intended == nil {
		return fmt.Errorf("nothing is aligned yet")
	}
	for _, it := range v.items {
		if it.Key() != key {
			continue
		}
		next, err := topology.Approve(*v.intended, it)
		if err != nil {
			return err
		}
		state, err := e.st.LoadState()
		if err != nil {
			return err
		}
		if state.HighWater == nil {
			state.HighWater = map[model.Kind]int{}
		}
		state.HighWater["ADR"]++
		adr := store.ADR{
			ID: string(model.FormatID("ADR", state.HighWater["ADR"])), Grade: "C",
			Subject: key, Decision: "approved", Origin: model.UserStated,
		}
		if err := e.st.WriteADR(adr, "Approved "+it.String()+"."); err != nil {
			return err
		}
		if err := e.st.SaveState(state); err != nil {
			return err
		}
		if err := e.st.SaveIntended(next); err != nil {
			return err
		}
		fmt.Fprintf(e.stdout, "approved %s as %s\n", key, adr.ID)
		return nil
	}
	return fmt.Errorf("no pending change %q", key)
}

// mode on re-extracts whatever changed while the mode was off, then turns the
// session on only when the project is aligned.
func (e env) mode(on bool) error {
	if !on {
		if err := e.st.SetMode(e.session, false); err != nil {
			return err
		}
		fmt.Fprintln(e.stdout, "mode off")
		return nil
	}
	if err := e.st.SetMode(e.session, false); err != nil {
		return err
	}
	if err := e.reextract(); err != nil {
		return err
	}
	v, err := e.load()
	if err != nil {
		return err
	}
	switch v.phase {
	case topology.Uninitialized, topology.Bootstrapped:
		fmt.Fprintf(e.stdout, "%s: run setup and align before turning the mode on\n", v.phase)
		return errNeedHuman
	case topology.Drifted:
		fmt.Fprintln(e.stdout, "drifted: changes made outside the mode touch design units")
		e.printItems(v.items)
		return errNeedHuman
	}
	if err := e.st.SetMode(e.session, true); err != nil {
		return err
	}
	fmt.Fprintln(e.stdout, "active")
	e.printItems(v.items)
	return nil
}

// reextract always runs the project script. Gate decisions must rest on the
// deterministic script, never on whatever was ingested last.
func (e env) reextract() error {
	p, err := e.st.LoadProject()
	if err != nil {
		return err
	}
	if len(p.Extract) == 0 {
		return fmt.Errorf("project.yaml names no extraction script; run setup first")
	}
	var out, errb bytes.Buffer
	cmd := exec.Command(p.Extract[0], p.Extract[1:]...)
	cmd.Dir, cmd.Stdout, cmd.Stderr = e.root, &out, &errb
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("extraction script failed: %w\n%s", err, errb.String())
	}
	ex, err := extract.Parse(&out)
	if err != nil {
		return err
	}
	_, err = e.syncActual(ex)
	return err
}

func (e env) status() error {
	v, err := e.load()
	if err != nil {
		return err
	}
	fmt.Fprintln(e.stdout, "phase:", v.phase)
	if e.session != "" {
		fmt.Fprintln(e.stdout, "mode:", map[bool]string{true: "on", false: "off"}[e.st.ModeOn(e.session)])
	}
	e.printItems(v.items)
	return nil
}

type hookInput struct {
	SessionID string `json:"session_id"`
	ToolInput struct {
		FilePath     string `json:"file_path"`
		NotebookPath string `json:"notebook_path"`
	} `json:"tool_input"`
	Cwd string `json:"cwd"`
}

// hookRoot prefers an explicit --root, then the project dir Claude Code exports,
// then the nearest ancestor of the hook's cwd that holds .abbd.
func (e env) hookRoot(cwd string) string {
	if e.root != "." {
		return e.root
	}
	if d := os.Getenv("CLAUDE_PROJECT_DIR"); d != "" {
		return d
	}
	for d := cwd; d != ""; d = filepath.Dir(d) {
		if fi, err := os.Stat(filepath.Join(d, store.Dir)); err == nil && fi.IsDir() {
			return d
		}
		if filepath.Dir(d) == d {
			break
		}
	}
	return e.root
}

func relTo(root, p string) string {
	if !filepath.IsAbs(p) {
		return filepath.ToSlash(p)
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return filepath.ToSlash(p)
	}
	if r, err := filepath.Rel(abs, p); err == nil && !strings.HasPrefix(r, "..") {
		return filepath.ToSlash(r)
	}
	return filepath.ToSlash(p)
}

// hook never blocks the host. Outside an active session it does nothing at all.
func (e env) hook(event string) {
	var in hookInput
	if err := json.NewDecoder(e.stdin).Decode(&in); err != nil || in.SessionID == "" {
		return
	}
	e.root = e.hookRoot(in.Cwd)
	e.st = store.Open(e.root)
	if !e.st.ModeOn(in.SessionID) {
		return
	}
	v, err := e.load()
	if err != nil || v.phase != topology.Aligned {
		return
	}
	file := in.ToolInput.FilePath
	if file == "" {
		file = in.ToolInput.NotebookPath
	}
	if event == "PostToolUse" && file != "" {
		if err := e.st.AppendJournal(in.SessionID, "changed "+relTo(e.root, file)); err != nil {
			fmt.Fprintln(e.stderr, "abbd hook:", err)
		}
	}
}
