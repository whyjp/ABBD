package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
	"strings"

	"github.com/whyjp/abbd/internal/confirm"
	"github.com/whyjp/abbd/internal/extract"
	"github.com/whyjp/abbd/internal/gitx"
	"github.com/whyjp/abbd/internal/identity"
	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/store"
	"github.com/whyjp/abbd/internal/topology"
	"github.com/whyjp/abbd/internal/turn"
)

const usage = `abbd setup [--supplement FILE] [--adopt FILE] [--root DIR] [-- EXTRACT-ARGV...]
                                     bootstrap from the project extraction script; never asks
abbd setup --greenfield FILE [--root DIR]
                                     a human states the design units of an empty project
abbd align (--all | --confirm IDS | --exclude IDS) [--root DIR]
                                     first alignment: confirm design units only
abbd align [--root DIR]              short realignment of a drifted project
abbd confirm --list [--root DIR]     the confirm queue, design units first
abbd confirm ITEM [--value TEXT] [--root DIR]
abbd confirm IF-0001.external_consumers --value true|false [--root DIR]
                                     a human says a contract has consumers the code does not show
abbd approve [--root DIR] ITEM       approve one grade C change; inside a turn, for that turn only
abbd ingest [--root DIR] FILE|-     reconcile an extraction result into .abbd
abbd mode on|off --session ID [--root DIR]
abbd status [--root DIR] [--session ID]
abbd hook EVENT [--root DIR]         hook entry point; reads the hook JSON on stdin
abbd turn start --session ID --seeds IDS --request TEXT|- [--root DIR]
                                     open a turn: record the request, compute the impact
abbd turn impact --session ID [--add IDS] [--root DIR]
                                     list the impact; the agent may only add to it
abbd turn declare --session ID [--units IDS] [--changes KEYS] [--root DIR]
                                     the IntentDelta; exit 2 while a grade C change waits for a human
abbd turn diff --session ID [--root DIR]
                                     re-extract and compare; exit 2 on a boundary-crossing change
abbd turn verify --session ID [--root DIR] [-- VERIFY-ARGV...]
                                     run the project verification and record the evidence;
                                     gitignored files are outside the evidence, so the
                                     verification must not depend on them
abbd turn end --session ID [--root DIR]
                                     gate and conclude: SYNCED, CLOSED, or exit 2 on DRAFT and BLOCKED
abbd turn show [--root DIR] [--session ID]
                                     any session may read the latest turn`

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
	opts
	stdin          io.Reader
	stdout, stderr io.Writer
	st             store.Store
}

type opts struct {
	root, session                 string
	supplement, adopt, greenfield string
	confirm, exclude, value       string
	seeds, request, units         string
	changes, add                  string
	all, list                     bool
	argv                          []string
}

var flagDefs = map[string]func(*flag.FlagSet, *opts){
	"root":    func(fs *flag.FlagSet, o *opts) { fs.StringVar(&o.root, "root", ".", "project root") },
	"session": func(fs *flag.FlagSet, o *opts) { fs.StringVar(&o.session, "session", "", "Claude Code session id") },
	"supplement": func(fs *flag.FlagSet, o *opts) {
		fs.StringVar(&o.supplement, "supplement", "", "semantic supplement from the pstack swarm")
	},
	"adopt": func(fs *flag.FlagSet, o *opts) {
		fs.StringVar(&o.adopt, "adopt", "", "existing design document, reconstructed as an extraction result")
	},
	"greenfield": func(fs *flag.FlagSet, o *opts) {
		fs.StringVar(&o.greenfield, "greenfield", "", "design units a human stated")
	},
	"all": func(fs *flag.FlagSet, o *opts) { fs.BoolVar(&o.all, "all", false, "confirm every candidate") },
	"confirm": func(fs *flag.FlagSet, o *opts) {
		fs.StringVar(&o.confirm, "confirm", "", "comma-separated IDs to confirm")
	},
	"exclude": func(fs *flag.FlagSet, o *opts) {
		fs.StringVar(&o.exclude, "exclude", "", "comma-separated IDs to leave below the line")
	},
	"list":  func(fs *flag.FlagSet, o *opts) { fs.BoolVar(&o.list, "list", false, "list the confirm queue") },
	"value": func(fs *flag.FlagSet, o *opts) { fs.StringVar(&o.value, "value", "", "the value a human states") },
	"seeds": func(fs *flag.FlagSet, o *opts) {
		fs.StringVar(&o.seeds, "seeds", "", "comma-separated design units the request touches")
	},
	"request": func(fs *flag.FlagSet, o *opts) {
		fs.StringVar(&o.request, "request", "", "the user's request as stated, or - for stdin")
	},
	"units": func(fs *flag.FlagSet, o *opts) {
		fs.StringVar(&o.units, "units", "", "comma-separated design units the turn will change")
	},
	"changes": func(fs *flag.FlagSet, o *opts) {
		fs.StringVar(&o.changes, "changes", "", "comma-separated topology changes, as item keys")
	},
	"add": func(fs *flag.FlagSet, o *opts) { fs.StringVar(&o.add, "add", "", "comma-separated units to add") },
}

// command is one CLI verb. pos is the allowed number of positional arguments.
type command struct {
	flags    []string
	subs     []string
	pos      []int
	dashArgv bool
	session  bool
	run      func(e env, sub string, pos []string) error
}

var commands = map[string]command{
	"setup":   {flags: []string{"root", "supplement", "adopt", "greenfield"}, pos: []int{0}, dashArgv: true, run: env.setup},
	"align":   {flags: []string{"root", "all", "confirm", "exclude"}, pos: []int{0}, run: func(e env, _ string, _ []string) error { return e.align() }},
	"confirm": {flags: []string{"root", "list", "value"}, pos: []int{0, 1}, run: func(e env, _ string, pos []string) error { return e.confirmItem(pos) }},
	"approve": {flags: []string{"root"}, pos: []int{1}, run: func(e env, _ string, pos []string) error { return e.approve(pos[0]) }},
	"ingest":  {flags: []string{"root"}, pos: []int{1}, run: func(e env, _ string, pos []string) error { return e.ingest(pos[0]) }},
	"mode":    {flags: []string{"root", "session"}, subs: []string{"on", "off"}, pos: []int{0}, session: true, run: func(e env, sub string, _ []string) error { return e.mode(sub == "on") }},
	"status":  {flags: []string{"root", "session"}, pos: []int{0}, run: func(e env, _ string, _ []string) error { return e.status() }},
	"hook":    {flags: []string{"root"}, subs: []string{"*"}, pos: []int{0}},

	"turn start":   {flags: []string{"root", "session", "seeds", "request"}, pos: []int{0}, session: true, run: func(e env, _ string, _ []string) error { return e.turnStart() }},
	"turn impact":  {flags: []string{"root", "session", "add"}, pos: []int{0}, session: true, run: func(e env, _ string, _ []string) error { return e.turnImpact() }},
	"turn declare": {flags: []string{"root", "session", "units", "changes"}, pos: []int{0}, session: true, run: func(e env, _ string, _ []string) error { return e.turnDeclare() }},
	"turn diff":    {flags: []string{"root", "session"}, pos: []int{0}, session: true, run: func(e env, _ string, _ []string) error { return e.turnDiff() }},
	"turn verify":  {flags: []string{"root", "session"}, pos: []int{0}, dashArgv: true, session: true, run: func(e env, _ string, _ []string) error { return e.turnVerify() }},
	"turn end":     {flags: []string{"root", "session"}, pos: []int{0}, session: true, run: func(e env, _ string, _ []string) error { return e.turnEnd() }},
	"turn show":    {flags: []string{"root", "session"}, pos: []int{0}, run: func(e env, _ string, _ []string) error { return e.turnShow() }},
}

type invocation struct {
	name, sub string
	pos       []string
	o         opts
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
	if c.subs != nil {
		if len(args) == 0 || (c.subs[0] != "*" && !slices.Contains(c.subs, args[0])) {
			return inv, fmt.Errorf("%s needs one of %v", inv.name, c.subs)
		}
		inv.sub, args = args[0], args[1:]
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
	if c.session && inv.o.session == "" {
		return inv, fmt.Errorf("%s needs --session", inv.name)
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
	if inv.name == "hook" {
		e.hook(inv.sub)
		return exitOK
	}
	err = commands[inv.name].run(e, inv.sub, inv.pos)
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
	res, err := e.syncActual(ex, identity.KeepGuesses)
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
	selecting := e.all || e.confirm != "" || e.exclude != ""
	var next topology.Intended
	switch {
	case v.phase == topology.Uninitialized:
		return fmt.Errorf("nothing to align: run setup first")
	case v.phase == topology.Bootstrapped:
		chosen, err := choose(topology.Present(v.units), e.opts)
		if err != nil {
			return err
		}
		if chosen == nil {
			fmt.Fprintln(e.stdout, "design unit candidates; confirm with --all, --confirm IDS or --exclude IDS:")
			for _, u := range topology.Present(v.units) {
				fmt.Fprintf(e.stdout, "  %s %s\n", u.ID, u.Name.Value)
			}
			return errNeedHuman
		}
		next = topology.Select(v.units, chosen)
	case selecting:
		return fmt.Errorf("the project is %s; --all, --confirm and --exclude choose the first design units only", v.phase)
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
	fmt.Fprintf(e.stdout, "aligned: %d design unit(s)\n", len(next.Units))
	return nil
}

// choose returns nil when the human has not said which units to confirm.
func choose(present []model.Unit, o opts) (map[model.ID]bool, error) {
	set := 0
	for _, on := range []bool{o.all, o.confirm != "", o.exclude != ""} {
		if on {
			set++
		}
	}
	if set == 0 {
		return nil, nil
	}
	if set > 1 {
		return nil, fmt.Errorf("give one of --all, --confirm and --exclude")
	}
	known := map[model.ID]bool{}
	for _, u := range present {
		known[u.ID] = true
	}
	ids := func(list string) (map[model.ID]bool, error) {
		out := map[model.ID]bool{}
		for _, s := range strings.Split(list, ",") {
			id := model.ID(strings.TrimSpace(s))
			if !known[id] {
				return nil, fmt.Errorf("%q is not a design unit candidate", id)
			}
			out[id] = true
		}
		return out, nil
	}
	switch {
	case o.confirm != "":
		return ids(o.confirm)
	case o.exclude != "":
		drop, err := ids(o.exclude)
		if err != nil {
			return nil, err
		}
		for id := range drop {
			delete(known, id)
		}
	}
	if len(known) == 0 {
		return nil, fmt.Errorf("confirm at least one design unit")
	}
	return known, nil
}

func (e env) confirmItem(pos []string) error {
	v, err := e.load()
	if err != nil {
		return err
	}
	if e.list == (len(pos) == 1) {
		return fmt.Errorf("give either --list or one ITEM")
	}
	if e.list {
		for _, it := range confirm.Queue(v.units, v.designIDs()) {
			fmt.Fprintln(e.stdout, it)
		}
		return nil
	}
	if id, ok := strings.CutSuffix(pos[0], ".external_consumers"); ok {
		return e.setExternalConsumers(v, model.ID(id))
	}
	u, err := confirm.Apply(v.units, pos[0], e.value)
	if err != nil {
		return err
	}
	if err := e.st.SaveUnits([]model.Unit{u}); err != nil {
		return err
	}
	fmt.Fprintln(e.stdout, "confirmed", pos[0])
	if v.intended == nil {
		return nil
	}
	// After alignment a confirmed coupling is a new edge in the actual
	// topology. The human who confirmed it has approved it, so it must not
	// surface as drift.
	after, err := e.load()
	if err != nil {
		return err
	}
	for _, it := range after.items {
		if it.Change != topology.NewRelation || it.Edge.String() != pos[0] {
			continue
		}
		if it.Effect == topology.Blocked {
			return e.approve(it.Key())
		}
		fmt.Fprintf(e.stdout, "%s: run abbd align to acknowledge it\n", it)
	}
	return nil
}

func (e env) setExternalConsumers(v view, id model.ID) error {
	if v.intended == nil {
		return fmt.Errorf("nothing is aligned yet")
	}
	if e.value != "true" && e.value != "false" {
		return fmt.Errorf("%s.external_consumers needs --value true or --value false", id)
	}
	next := *v.intended
	next.Units = slices.Clone(next.Units)
	i := slices.IndexFunc(next.Units, func(u topology.IntendedUnit) bool { return u.ID == id })
	if i < 0 || !next.Units[i].Kind.IsContract() {
		return fmt.Errorf("%s is not an intended contract", id)
	}
	next.Units[i].ExternalConsumers = e.value == "true"
	if err := e.st.SaveIntended(next); err != nil {
		return err
	}
	fmt.Fprintln(e.stdout, "confirmed", string(id)+".external_consumers", e.value)
	return nil
}

func (v view) designIDs() map[model.ID]bool {
	ids := map[model.ID]bool{}
	if v.intended != nil {
		for _, u := range v.intended.Units {
			ids[u.ID] = true
		}
	}
	return ids
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
	t, err := e.st.LatestTurn()
	if err != nil {
		return err
	}
	// A DRAFT turn can still sync, so its approvals must wait for it as well.
	if t != nil && t.State.Resumable() {
		return e.approveInTurn(t, key)
	}
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
	if err := e.refresh(); err != nil {
		return err
	}
	if t, err := e.st.LatestTurn(); err != nil {
		return err
	} else if t != nil && t.State.Resumable() {
		return e.resume(t)
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

// resume turns the mode on for a session that picks up an unfinished turn. The
// turn's own changes drift the project until it ends, so they are judged as the
// turn's work, which its end gate still decides, rather than refused as drift.
// One turn runs at a time, so the new session takes it over, but only from a
// session whose mode a human has turned off: a live session keeps its turn.
func (e env) resume(t *turn.Turn) error {
	if t.Session != e.session {
		if e.st.ModeOn(t.Session) {
			return fmt.Errorf("%s belongs to session %s, whose mode is still on; a human turns it off with abbd mode off --session %s before another session takes the turn", t.ID, t.Session, t.Session)
		}
		fmt.Fprintf(e.stdout, "%s moves from session %s to %s\n", t.ID, t.Session, e.session)
		t.Session = e.session
	}
	v, err := e.impact(t)
	if err != nil {
		return err
	}
	if err := e.st.SaveTurn(*t); err != nil {
		return err
	}
	if err := e.st.SetMode(e.session, true); err != nil {
		return err
	}
	fmt.Fprintf(e.stdout, "active: resuming %s (%s)\n", t.ID, t.State)
	for _, vd := range turn.Judge(*v.intended, v.units, t.Approvals) {
		fmt.Fprintln(e.stdout, vd)
	}
	return nil
}

// refresh brings the actual topology up to the working tree. A greenfield
// project has no script until its first turn adds code, so it may go without
// one only while there is no code to read.
func (e env) refresh() error {
	p, err := e.st.LoadProject()
	if err != nil {
		return err
	}
	if len(p.Extract) > 0 {
		return e.reextract()
	}
	in, err := e.st.LoadIntended()
	if err != nil {
		return err
	}
	f, err := firstFile(e.root)
	if err != nil {
		return err
	}
	if in == nil || f != "" {
		return fmt.Errorf("project.yaml names no extraction script; register one with abbd setup -- EXTRACT-ARGV")
	}
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
	out, err := e.runScript(p.Extract)
	if err != nil {
		return err
	}
	ex, err := extract.Parse(bytes.NewReader(out))
	if err != nil {
		return err
	}
	_, err = e.syncActual(ex, identity.KeepGuesses)
	return err
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

func (e env) status() error {
	v, err := e.load()
	if err != nil {
		return err
	}
	fmt.Fprintln(e.stdout, "phase:", v.phase)
	fmt.Fprintln(e.stdout, "confirm queue:", len(confirm.Queue(v.units, v.designIDs())))
	if e.session != "" {
		fmt.Fprintln(e.stdout, "mode:", map[bool]string{true: "on", false: "off"}[e.st.ModeOn(e.session)])
	}
	e.printItems(v.items)
	return nil
}
