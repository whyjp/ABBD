package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"

	"github.com/whyjp/abbd/internal/evidence"
	"github.com/whyjp/abbd/internal/extract"
	"github.com/whyjp/abbd/internal/gitx"
	"github.com/whyjp/abbd/internal/identity"
	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/store"
)

const usage = `abbd setup [--supplement FILE] [--root DIR] [-- EXTRACT-ARGV...]
                                     register the extraction script, prove it deterministic,
                                     fold in the semantic supplement, and sync
abbd ingest [--root DIR] FILE|-     reconcile an extraction result into .abbd
abbd propose relation FROM KIND TO --via VIA --evidence PATH:LINES:QUOTE [--evidence ...] [--root DIR]
abbd propose unit KIND NAME --evidence PATH:LINES:QUOTE [--evidence ...] [--parent ID] [--responsibility TEXT] [--root DIR]
abbd understand MOD-ID FILE|- [--root DIR]
                                     store pstack how's Overview and Key Concepts for a module,
                                     stamped with the module's code
abbd evidence [--root DIR]           check every quote against the working tree
abbd status [--root DIR]             whether the state and the blueprint are CURRENT or STALE`

const (
	exitOK    = 0
	exitError = 1
	exitUsage = 64
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

type env struct {
	opts
	stdin          io.Reader
	stdout, stderr io.Writer
	st             store.Store
}

type opts struct {
	root, supplement string
	via, parent      string
	responsibility   string
	evidence         evidenceFlags
	argv             []string
}

var flagDefs = map[string]func(*flag.FlagSet, *opts){
	"root": func(fs *flag.FlagSet, o *opts) { fs.StringVar(&o.root, "root", ".", "project root") },
	"supplement": func(fs *flag.FlagSet, o *opts) {
		fs.StringVar(&o.supplement, "supplement", "", "semantic supplement from the pstack swarm")
	},
	"via": func(fs *flag.FlagSet, o *opts) {
		fs.StringVar(&o.via, "via", "", "how the coupling arises: import, trait, callback, macro, config, build-feature, data, convention or call")
	},
	"evidence": func(fs *flag.FlagSet, o *opts) {
		fs.Var(&o.evidence, "evidence", "PATH:LINES:QUOTE, a verbatim excerpt of the code; repeat for more")
	},
	"parent": func(fs *flag.FlagSet, o *opts) {
		fs.StringVar(&o.parent, "parent", "", "the module that contains the unit")
	},
	"responsibility": func(fs *flag.FlagSet, o *opts) {
		fs.StringVar(&o.responsibility, "responsibility", "", "the one sentence a reader says the unit is responsible for")
	},
}

// command is one CLI verb. pos is the allowed number of positional arguments.
type command struct {
	flags    []string
	pos      []int
	dashArgv bool
	run      func(e env, pos []string) error
}

var commands = map[string]command{
	"setup":            {flags: []string{"root", "supplement"}, pos: []int{0}, dashArgv: true, run: func(e env, _ []string) error { return e.setup() }},
	"ingest":           {flags: []string{"root"}, pos: []int{1}, run: func(e env, pos []string) error { return e.ingest(pos[0]) }},
	"propose relation": {flags: []string{"root", "via", "evidence"}, pos: []int{3}, run: func(e env, pos []string) error { return e.proposeRelation(pos) }},
	"propose unit":     {flags: []string{"root", "evidence", "parent", "responsibility"}, pos: []int{2}, run: func(e env, pos []string) error { return e.proposeUnit(pos) }},
	"understand":       {flags: []string{"root"}, pos: []int{2}, run: func(e env, pos []string) error { return e.understand(model.ID(pos[0]), pos[1]) }},
	"evidence":         {flags: []string{"root"}, pos: []int{0}, run: func(e env, _ []string) error { return e.checkEvidence() }},
	"status":           {flags: []string{"root"}, pos: []int{0}, run: func(e env, _ []string) error { return e.status() }},
}

type invocation struct {
	name string
	pos  []string
	o    opts
}

// parse checks a command line against the command table without running it.
func parse(args []string, stderr io.Writer) (invocation, error) {
	if len(args) == 0 {
		return invocation{}, fmt.Errorf("no command")
	}
	inv := invocation{name: args[0]}
	args = args[1:]
	if len(args) > 0 {
		if _, ok := commands[inv.name+" "+args[0]]; ok {
			inv.name, args = inv.name+" "+args[0], args[1:]
		}
	}
	c, ok := commands[inv.name]
	if !ok {
		return inv, fmt.Errorf("unknown command %q", inv.name)
	}
	if i := slices.Index(args, "--"); i >= 0 {
		if !c.dashArgv {
			return inv, fmt.Errorf("%s takes no -- arguments", inv.name)
		}
		args, inv.o.argv = args[:i], args[i+1:]
		if len(inv.o.argv) == 0 {
			return inv, fmt.Errorf("nothing after --")
		}
	}
	fs := flag.NewFlagSet(inv.name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	for _, f := range c.flags {
		flagDefs[f](fs, &inv.o)
	}
	pos, err := parseInterspersed(fs, args)
	if err != nil {
		return inv, err
	}
	if !slices.Contains(c.pos, len(pos)) {
		return inv, fmt.Errorf("%s takes %v positional arguments, got %d", inv.name, c.pos, len(pos))
	}
	inv.pos = pos
	return inv, nil
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	inv, err := parse(args, stderr)
	if err != nil {
		fmt.Fprintln(stderr, "abbd:", err)
		fmt.Fprintln(stderr, usage)
		return exitUsage
	}
	e := env{opts: inv.o, stdin: stdin, stdout: stdout, stderr: stderr, st: store.Open(inv.o.root)}
	// Only a command that changed what the blueprint renders publishes, so a
	// hand edit stays STALE until a command writes over it.
	before, _ := e.st.InputsHash()
	if err := commands[inv.name].run(e, inv.pos); err != nil {
		fmt.Fprintln(stderr, "abbd:", err)
		return exitError
	}
	if after, _ := e.st.InputsHash(); after != before {
		if err := e.publish(); err != nil {
			fmt.Fprintln(stderr, "abbd: warning: rendering the blueprint:", err)
		}
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
	res, err := e.syncActual(ex, identity.KeepGuesses)
	if err != nil {
		return err
	}
	e.printEvents(res)
	return nil
}

func (e env) printEvents(res identity.Result) {
	for _, ev := range res.Events {
		fmt.Fprintf(e.stdout, "%s %s\n", ev.ID, ev.Outcome)
	}
	for _, c := range res.Candidates {
		fmt.Fprintf(e.stdout, "%s candidate-of %s via %s\n", c.New, c.Existing, c.Shared.Key())
	}
	for _, id := range res.Unmatched {
		fmt.Fprintf(e.stdout, "%s unmatched\n", id)
	}
}

// syncActual writes the state from one extraction. It only ever touches EXTRACTED facts.
func (e env) syncActual(ex extract.Result, guesses identity.Guesses) (identity.Result, error) {
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
	res := identity.Reconcile(stored, renames, ex, prior, state.HighWater, guesses)
	res.Units = evidence.New(e.root).Apply(res.Units)
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
	// The revision is where this tree was first synced. Committing .abbd/
	// moves HEAD without changing the tree, and must not dirty .abbd/ again.
	rev := state.Revision
	if state.Fingerprint != tree || rev == "" {
		rev = gitx.Head(e.root)
	}
	state = store.State{Revision: rev, Fingerprint: tree, MDHash: hash, HighWater: res.HighWater}
	return res, st.SaveState(state)
}

func (e env) runScript(argv []string, env ...string) ([]byte, error) {
	var out, errb bytes.Buffer
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir, cmd.Stdout, cmd.Stderr = e.root, &out, &errb
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("extraction script failed: %w\n%s", err, errb.String())
	}
	return out.Bytes(), nil
}

func presentUnit(units []model.Unit, id model.ID) (model.Unit, bool) {
	i := slices.IndexFunc(units, func(u model.Unit) bool { return u.ID == id && !u.Absent })
	if i < 0 {
		return model.Unit{}, false
	}
	return units[i], true
}

func (e env) status() error {
	units, err := e.st.LoadUnits()
	if err != nil {
		return err
	}
	fmt.Fprintln(e.stdout, "units:", len(model.Present(units)))
	return e.printFreshness()
}
