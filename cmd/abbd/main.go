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

	"github.com/whyjp/abbd/internal/evidence"
	"github.com/whyjp/abbd/internal/extract"
	"github.com/whyjp/abbd/internal/gitx"
	"github.com/whyjp/abbd/internal/group"
	"github.com/whyjp/abbd/internal/identity"
	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/store"
)

const usage = `abbd is a lever: it extracts, checks evidence, keeps IDs, stores the topology
and renders the blueprint. It never judges, blocks, asks or records a decision.

abbd setup [--supplement FILE] [--root DIR] [-- EXTRACT-ARGV...]
                                     register the extraction script, prove it deterministic,
                                     fold in the semantic supplement, and sync
abbd install [--root DIR]            write or replace the abbd block in the project's CLAUDE.md,
                                     which tells any assistant to ask the blueprint first
abbd sync [--verbose] [--root DIR]   run the registered script, check every quote, keep the
                                     IDs, write the state and render the blueprint; prints
                                     counts and what needs attention, --verbose every unit
abbd record relation FROM KIND TO --via VIA [--phrase TEXT] --evidence PATH:LINES:QUOTE [--evidence ...] [--root DIR]
                                     a reader's coupling, INFERRED while its evidence is live;
                                     --phrase names in at most 4 words and 24 characters what
                                     crosses it, such as a route, a message or a query, and the
                                     blueprint draws it beside the line, as a note with its own
                                     origin; on an edge that already shows, from any writer, the
                                     note is all it stores and every claim stays as it was;
                                     it never brings back a coupling a human retracted
abbd record relation FROM KIND TO --source REF [--via VIA] [--phrase TEXT] [--evidence ...] [--root DIR]
                                     a coupling a human stated, USER_STATED
abbd record unit KIND NAME --evidence PATH:LINES:QUOTE [--evidence ...] [--parent ID] [--responsibility TEXT] [--root DIR]
                                     a unit the script cannot see, INFERRED
abbd record fact ID.purpose|ID.responsibility --value TEXT --source REF [--root DIR]
                                     what a human stated, USER_STATED with where they said it
abbd record fact ID.decisions.KEY --value TEXT --source REF [--root DIR]
                                     a human's reason for a decision a reader stored, USER_STATED
abbd record evidence TARGET --evidence PATH:LINES:QUOTE [--evidence ...] [--root DIR]
                                     refresh a reader's claim with live evidence; TARGET is an ID,
                                     ID.parent, an edge FROM.KIND.TO or its phrase
                                     FROM.KIND.TO.phrase; a supplement claim gets a reader's
                                     claim above it
abbd retract TARGET --reason TEXT [--source REF] [--root DIR]
                                     remove your own claim on TARGET (ID, ID.parent, ID.purpose,
                                     ID.responsibility, an edge with its phrases, or an edge's
                                     phrase FROM.KIND.TO.phrase) and hide the lower INFERRED
                                     claims on it until their content changes; a human's with --source
abbd migrate [--supplement FILE] [--root DIR]
                                     convert an old-format state once, telling the supplement's
                                     claims from a reader's by the supplement (default
                                     .abbd/extractor/supplement.json)
abbd understand MOD-ID|FEAT-ID FILE|- [--reader WHO] [--sentence TEXT] [--root DIR]
                                     store a reader's Overview and Key Concepts for a module
                                     or feature, stamped with its code: every file of it and
                                     the units under it, a submodule by its recorded commit (so
                                     edits inside one count once committed there and staged), an
                                     ignored directory by its files on disk, and the lines around
                                     a quote of a file only a quote cites; the text records a
                                     hash per anchor and per quote; WHO names the reader, such as
                                     'abbd reader' or 'pstack how 0.15.36'
abbd understand MOD-ID --restamp [--root DIR]
                                     stamp the stored text again after the extraction moved
                                     what the module holds; refused when the code of an anchor
                                     or a quote changed, is gone or is one the text was not
                                     written against
abbd understand ID.SLOT[.KEY] FILE|- --evidence PATH:LINES:QUOTE [--evidence ...] [--reader WHO] [--root DIR]
                                     store what a reader found for one slot of a module or a
                                     contract: semantics, failure, states, decisions (one note
                                     per KEY) or wire_sample; the note is stamped with the lines
                                     around each quote it cites, so it goes STALE when that code
                                     changes and not when other code of the unit does; a human's
                                     why is kept when the decision is stored again on the same
                                     quotes, and dropped when it cites other code
abbd understand ID.SLOT[.KEY] --reread-by WHO [--root DIR]
                                     stamp a STALE note again, keeping its words and quotes, once
                                     WHO read the code it cites again and found it still holds;
                                     the note records WHO, the date and what had moved; refused
                                     when a quote is not in its file exactly once, word for word
abbd forget ID.SLOT[.KEY] [--root DIR]
                                     remove a note, with the human's why it holds, and say what
                                     it removed; ID.SLOT[.KEY] names the file under
                                     .abbd/understanding, so a STALE, orphaned or misfiled note
                                     goes by the name abbd status gives it
abbd flow KEY FILE|- [--source REF] [--root DIR]
                                     store an ordered flow: a question, and two to 16 steps, each a
                                     unit, what it does, an optional note and fails line, the
                                     units it calls through stored relations, and the code it
                                     cites; each step is stamped with the lines around its quotes,
                                     so it goes STALE when that code changes; a reader's is
                                     INFERRED, a human's with --source USER_STATED
abbd flow KEY [--root DIR]           print the stored flow as a draft to edit and store again
abbd stack KEY FILE|- [--source REF] [--root DIR]
                                     store a layer stack: a question, and two to 8 layers from the
                                     top down, each a name, a one-line role, the stored units it
                                     holds with every unit inside them, and the code it cites; a
                                     unit sits in one layer, 16 units at most; each layer is
                                     stamped with the lines around its quotes, so it goes STALE
                                     when that code changes; a reader's is INFERRED, a human's
                                     with --source USER_STATED; it says how many stored relations
                                     run up the stack, from a lower layer to a higher one
abbd stack KEY [--root DIR]          print the stored stack as a draft to edit and store again
abbd table KEY FILE|- [--source REF] [--root DIR]
                                     store a table: a question, two to 8 columns, each a short
                                     label and optionally the stored unit it stands for, and two
                                     to 16 rows, each a stored unit, a one-line statement under
                                     each column it fills, and the code it cites; a unit heads
                                     one row; each row is stamped with the lines around its
                                     quotes, so it goes STALE when that code changes; a reader's
                                     is INFERRED, a human's with --source USER_STATED; it says
                                     how many cells under a unit's column the stored relations
                                     do not bear out
abbd table KEY [--root DIR]          print the stored table as a draft to edit and store again
abbd explain QUERY [--history] [--root DIR]
                                     one unit at a glance: its contracts, neighbours, inside
                                     units, a reader's couplings with their quotes, a reader's text;
                                     QUERY is a unit ID or name, a file or directory path, a
                                     symbol, or a phrase, and a QUERY that is no unit lists
                                     the units it matches, best first, and explains the best
                                     --history names the commit and pull request that recorded
                                     each reader's coupling
abbd path A B [--root DIR]           the shortest path between two units, each hop with its
                                     origin, and the path's weakest origin; A and B are QUERYs
abbd impact QUERIES [--root DIR]     what a change to the comma-separated units reaches, in a
                                     direct and a far tier; read before the work
abbd diff [--base REV] [--unit ID] [--page] [--json] [--check] [--root DIR]
                                     the topology difference since REV (default: the merge base
                                     with the default branch), for the PR; --check exits 1 when
                                     a contract's signature changed, a contract was added or
                                     removed, or a relation between top-level modules appeared
                                     or disappeared; a contract's code change under the same
                                     signature is reported but does not fail it; --unit keeps
                                     the changes inside one module or feature, and --check then
                                     reads only those, a relation counting at its top-level ends;
                                     --page writes .abbd/compare/index.html, the blueprint
                                     committed at REV beside the current one, never committed
                                     REV prev is the copy of .abbd a write keeps from before its
                                     first change since the last commit
abbd review [--base REV] [--features] [--root DIR]
                                     one surface for a reviewer: the change since REV, who uses
                                     each contract or entry it touched with origin and feature,
                                     what it reaches directly, and what is worth a look (a
                                     reader's new coupling, a STALE text or note, a decision
                                     with no why, drifted evidence); always exits 0
abbd evidence [--root DIR]           check every quote against the working tree: live, drifted or dead
abbd lint [VIEW] [--json] [--list] [--record [--keep-level VIEW=REASON ...]] [--root DIR]
                                     hold every drawing of the blueprint, or the views whose name
                                     starts with VIEW or holds it as a word, such as a unit ID, to
                                     the drawing criteria of docs/blueprint-svg.md; prints each
                                     breach with the boxes, lines or chips it involves, what it
                                     measures against what is allowed and why, then the totals per
                                     view and in all; exits 1 on a hard breach past the debt
                                     .abbd/lint-debt.yaml records for its view, or, over every
                                     view, on a total past its ceiling there; --list names the
                                     views; --record writes what the drawings measure now as
                                     their debt and ceilings; the first --record refuses while a
                                     view with hard breaches has a level line, unless
                                     --keep-level names that view and why it stays
abbd status [--check] [--verbose] [--root DIR]
                                     whether the state and the blueprint are CURRENT, counts,
                                     drifted and dead evidence, retractions and parent conflicts;
                                     --check exits 1 when the state, the blueprint, a module
                                     text, a note, a flow's step, a stack's layer or a table's row
                                     is STALE, an evidence item drifted or a phrase names no
                                     line, for a hook or CI;
                                     it also exits 1 when the state or the blueprint is UNKNOWN,
                                     because it could not compare them (no git work tree, a git
                                     error, or no graph.json), and says which; dead evidence, an
                                     orphaned text, one whose module is gone or no longer
                                     top-level, a relation that runs up a stack, and a table's
                                     cell the stored relations do not bear out are reported but
                                     do not fail it; long: and unbacked: lines fold into one count
                                     each unless --verbose lists them`

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
	phrase           string
	responsibility   string
	value, src       string
	reason           string
	base             string
	unit             string
	history          bool
	sentence         string
	page             bool
	asJSON, check    bool
	list, record     bool
	features         bool
	verbose, restamp bool
	rereadBy         string
	reader           string
	evidence         listFlag
	keepLevel        listFlag
	argv             []string
}

var flagDefs = map[string]func(*flag.FlagSet, *opts){
	"root": func(fs *flag.FlagSet, o *opts) { fs.StringVar(&o.root, "root", ".", "project root") },
	"supplement": func(fs *flag.FlagSet, o *opts) {
		fs.StringVar(&o.supplement, "supplement", "", "semantic supplement from the setup readers")
	},
	"via": func(fs *flag.FlagSet, o *opts) {
		fs.StringVar(&o.via, "via", "", "how the coupling arises: import, trait, callback, macro, config, build-feature, data, convention or call")
	},
	"phrase": func(fs *flag.FlagSet, o *opts) {
		fs.StringVar(&o.phrase, "phrase", "", "what crosses the coupling in a few words, such as a route, a message or a query")
	},
	"evidence": func(fs *flag.FlagSet, o *opts) {
		fs.Var(&o.evidence, "evidence", "PATH:LINES:QUOTE, a verbatim excerpt of the code; repeat for more")
	},
	"parent": func(fs *flag.FlagSet, o *opts) {
		fs.StringVar(&o.parent, "parent", "", "the module or feature that contains the unit")
	},
	"responsibility": func(fs *flag.FlagSet, o *opts) {
		fs.StringVar(&o.responsibility, "responsibility", "", "the one sentence a reader says the unit is responsible for")
	},
	"base": func(fs *flag.FlagSet, o *opts) {
		fs.StringVar(&o.base, "base", "", "the revision to compare with; default: the merge base with the default branch")
	},
	"unit": func(fs *flag.FlagSet, o *opts) {
		fs.StringVar(&o.unit, "unit", "", "keep only the changes inside this module or feature")
	},
	"page": func(fs *flag.FlagSet, o *opts) {
		fs.BoolVar(&o.page, "page", false, "write a compare view to .abbd/compare: the blueprint at the base beside the current one")
	},
	"json": func(fs *flag.FlagSet, o *opts) { fs.BoolVar(&o.asJSON, "json", false, "print JSON") },
	"features": func(fs *flag.FlagSet, o *opts) {
		fs.BoolVar(&o.features, "features", false, "group the result by the features it reaches, with the contracts and entries each uses")
	},
	"history": func(fs *flag.FlagSet, o *opts) {
		fs.BoolVar(&o.history, "history", false, "name the commit and pull request that recorded each reader's coupling")
	},
	"verbose": func(fs *flag.FlagSet, o *opts) {
		fs.BoolVar(&o.verbose, "verbose", false, "list every unit, not only what needs attention")
	},
	"list": func(fs *flag.FlagSet, o *opts) {
		fs.BoolVar(&o.list, "list", false, "list the names of the views and check nothing")
	},
	"record": func(fs *flag.FlagSet, o *opts) {
		fs.BoolVar(&o.record, "record", false, "write what the drawings measure now as their debt and ceilings")
	},
	"keep-level": func(fs *flag.FlagSet, o *opts) {
		fs.Var(&o.keepLevel, "keep-level", "VIEW=REASON, a view --record keeps at the level lint would change, and why; repeat for more")
	},
	"check": func(fs *flag.FlagSet, o *opts) {
		fs.BoolVar(&o.check, "check", false, "exit 1 on what the command's usage line names")
	},
	"restamp": func(fs *flag.FlagSet, o *opts) {
		fs.BoolVar(&o.restamp, "restamp", false, "stamp the stored text against the module's code again, when only what the module holds moved")
	},
	"sentence": func(fs *flag.FlagSet, o *opts) {
		fs.StringVar(&o.sentence, "sentence", "", "the one line the reader says the module is responsible for, stamped with its code")
	},
	"reader": func(fs *flag.FlagSet, o *opts) {
		fs.StringVar(&o.reader, "reader", "", "who read the module and wrote its text, such as 'abbd reader' or 'pstack how 0.15.36'")
	},
	"reread-by": func(fs *flag.FlagSet, o *opts) {
		fs.StringVar(&o.rereadBy, "reread-by", "", "who read the code a STALE note cites again and found the note still holds")
	},
	"value": func(fs *flag.FlagSet, o *opts) { fs.StringVar(&o.value, "value", "", "the text the human stated") },
	"reason": func(fs *flag.FlagSet, o *opts) {
		fs.StringVar(&o.reason, "reason", "", "why the claim is wrong")
	},
	"source": func(fs *flag.FlagSet, o *opts) {
		fs.StringVar(&o.src, "source", "", `where the human stated it, such as a PR URL or "conversation 2026-11-01"`)
	},
}

// command is one CLI verb. pos is the allowed number of positional arguments.
// writes says the command changes the state, so the blueprint is rendered after it.
type command struct {
	flags    []string
	pos      []int
	dashArgv bool
	writes   bool
	run      func(e env, pos []string) error
}

var commands = map[string]command{
	"setup":           {flags: []string{"root", "supplement"}, pos: []int{0}, dashArgv: true, writes: true, run: func(e env, _ []string) error { return e.setup() }},
	"sync":            {flags: []string{"root", "verbose"}, pos: []int{0}, writes: true, run: func(e env, _ []string) error { return e.sync() }},
	"record relation": {flags: []string{"root", "via", "phrase", "evidence", "source"}, pos: []int{3}, writes: true, run: func(e env, pos []string) error { return e.recordRelation(pos) }},
	"record unit":     {flags: []string{"root", "evidence", "parent", "responsibility"}, pos: []int{2}, writes: true, run: func(e env, pos []string) error { return e.recordUnit(pos) }},
	"record parent":   {flags: []string{"root", "evidence", "source"}, pos: []int{2}, writes: true, run: func(e env, pos []string) error { return e.recordParent(pos) }},
	"record fact":     {flags: []string{"root", "value", "source"}, pos: []int{1}, writes: true, run: func(e env, pos []string) error { return e.recordFact(pos[0]) }},
	"record evidence": {flags: []string{"root", "evidence"}, pos: []int{1}, writes: true, run: func(e env, pos []string) error { return e.refresh(pos[0]) }},
	"retract":         {flags: []string{"root", "reason", "source"}, pos: []int{1}, writes: true, run: func(e env, pos []string) error { return e.retract(pos[0]) }},
	"migrate":         {flags: []string{"root", "supplement"}, pos: []int{0}, writes: true, run: func(e env, _ []string) error { return e.migrate() }},
	"understand":      {flags: []string{"root", "restamp", "evidence", "reread-by", "reader", "sentence"}, pos: []int{1, 2}, writes: true, run: func(e env, pos []string) error { return e.understand(pos[0], pos[1:]) }},
	"forget":          {flags: []string{"root"}, pos: []int{1}, writes: true, run: func(e env, pos []string) error { return e.forget(pos[0]) }},
	"flow":            {flags: []string{"root", "source"}, pos: []int{1, 2}, writes: true, run: func(e env, pos []string) error { return e.flow(pos[0], pos[1:]) }},
	"stack":           {flags: []string{"root", "source"}, pos: []int{1, 2}, writes: true, run: func(e env, pos []string) error { return e.stack(pos[0], pos[1:]) }},
	"table":           {flags: []string{"root", "source"}, pos: []int{1, 2}, writes: true, run: func(e env, pos []string) error { return e.table(pos[0], pos[1:]) }},
	"install":         {flags: []string{"root"}, pos: []int{0}, run: func(e env, _ []string) error { return e.install() }},
	"explain":         {flags: []string{"root", "history"}, pos: []int{1}, run: func(e env, pos []string) error { return e.explain(pos[0]) }},
	"path":            {flags: []string{"root"}, pos: []int{2}, run: func(e env, pos []string) error { return e.path(pos[0], pos[1]) }},
	"impact":          {flags: []string{"root", "features"}, pos: []int{1}, run: func(e env, pos []string) error { return e.impact(pos[0]) }},
	"diff":            {flags: []string{"root", "base", "json", "check", "unit", "page"}, pos: []int{0}, run: func(e env, _ []string) error { return e.diff() }},
	"review":          {flags: []string{"root", "base", "features"}, pos: []int{0}, run: func(e env, _ []string) error { return e.review() }},
	"evidence":        {flags: []string{"root"}, pos: []int{0}, run: func(e env, _ []string) error { return e.checkEvidence() }},
	"status":          {flags: []string{"root", "check", "verbose"}, pos: []int{0}, run: func(e env, _ []string) error { return e.status() }},
	"lint":            {flags: []string{"root", "json", "list", "record", "keep-level"}, pos: []int{0, 1}, run: func(e env, pos []string) error { return e.lint(pos) }},
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
	c := commands[inv.name]
	e.keepPrevious(c)
	ran := c.run(e, inv.pos)
	if ran != nil {
		fmt.Fprintln(stderr, "abbd:", ran)
	}
	// A partial refusal saved part of the state, so it is rendered too.
	if ran != nil && !errors.As(ran, new(partial)) {
		return exitError
	}
	if c.writes {
		if err := e.publish(); err != nil {
			fmt.Fprintln(stderr, "abbd: rendering the blueprint:", err)
			return exitError
		}
	}
	if ran != nil {
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

// sync brings the state up to the working tree. It always runs the registered
// script, so the state rests on the deterministic extraction alone.
func (e env) sync() error {
	p, err := e.st.LoadProject()
	if err != nil {
		return err
	}
	if len(p.Extract) == 0 {
		return fmt.Errorf("project.yaml names no extraction script; run abbd setup -- EXTRACT-ARGV first")
	}
	out, err := e.runScript(p.Extract)
	if err != nil {
		return err
	}
	ex, err := extract.Parse(bytes.NewReader(out))
	if err != nil {
		return fmt.Errorf("extraction script output: %w", err)
	}
	res, err := e.syncActual(ex, identity.Sync)
	if err != nil {
		return err
	}
	n := map[identity.Outcome]int{}
	for _, ev := range res.Events {
		n[ev.Outcome]++
	}
	fmt.Fprintf(e.stdout, "synced: %d unit(s), %d matched, %d new, %d unmatched\n", len(model.Present(res.Units)), n[identity.Matched], n[identity.Created], len(res.Unmatched))
	for _, ev := range res.Events {
		if e.verbose || ev.Outcome != identity.Matched {
			fmt.Fprintf(e.stdout, "%s %s\n", ev.ID, ev.Outcome)
		}
	}
	for _, c := range res.Candidates {
		fmt.Fprintf(e.stdout, "%s candidate-of %s via %s\n", c.New, c.Existing, c.Shared.Key())
	}
	for _, id := range res.Unmatched {
		fmt.Fprintf(e.stdout, "%s unmatched\n", id)
	}
	e.attention(res.Units)
	return nil
}

// attention prints what a reader or a human must look at after a write:
// drifted and dead evidence, a human's parent the script moved under, and
// a module the extraction does not cover.
func (e env) attention(units []model.Unit) {
	n := evidenceCounts(units)
	if n[model.Drifted]+n[model.Dead] > 0 {
		fmt.Fprintf(e.stdout, "evidence: %d drifted, %d dead; abbd evidence lists them, abbd record evidence refreshes and abbd retract removes\n", n[model.Drifted], n[model.Dead])
	}
	for _, c := range model.ParentConflicts(units) {
		fmt.Fprintln(e.stdout, "conflict:", c)
	}
	for _, w := range uncovered(e.root, units) {
		fmt.Fprintln(e.stdout, w)
	}
}

// evidenceCounts counts the items of the claims that show on present units,
// by status.
func evidenceCounts(units []model.Unit) map[model.Status]int {
	n := map[model.Status]int{}
	for _, u := range model.Present(units) {
		u.EachShownEvidence(func(w model.Writer, ev *model.Evidence) {
			if w != model.Script || ev.Status != model.Live {
				n[ev.Status]++
			}
		})
	}
	return n
}

// syncActual writes the state from one extraction. It only ever replaces the
// claims of the writers the extraction speaks for.
func (e env) syncActual(ex extract.Result, ws identity.Writers) (identity.Result, error) {
	st := e.st
	stored, err := st.Writable()
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
	// The revision is where this tree was first synced. Committing .abbd/
	// moves HEAD without changing the tree, and must not dirty .abbd/ again.
	// A drifted item dies at a sync of a later revision.
	rev := state.Revision
	if state.Fingerprint != tree || rev == "" {
		rev = gitx.Head(e.root)
	}
	res := identity.Reconcile(stored, renames, ex, prior, state.HighWater, ws)
	res.Units = evidence.New(e.root).Apply(res.Units, rev)
	res.Units, res.HighWater = group.Fallback(res.Units, res.HighWater)
	stampSentences(st, stored, res.Units)
	if err := st.SaveUnits(res.Units); err != nil {
		return res, err
	}
	if err := st.SaveCandidates(res.Candidates); err != nil {
		return res, err
	}
	current, widened, err := st.UpgradeUnderstandings(res.Units)
	if err != nil {
		return res, err
	}
	for _, id := range current {
		fmt.Fprintf(e.stdout, "understanding: %s's digest now covers all of its code, which is unchanged since the text was written; it stays current\n", id)
	}
	for _, id := range widened {
		fmt.Fprintf(e.stdout, "understanding: %s's digest now covers all of its code, and its text was checked against only part of it; it is STALE until a reader reads the module again\n", id)
	}
	state = store.State{Revision: rev, Fingerprint: tree, HighWater: res.HighWater}
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
