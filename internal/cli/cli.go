// Package cli is the For My Harness Core CLI (PRD ch.09). Every command is a thin adapter
// over internal/workflow; the MCP server exposes the same functions.
package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/whyjp/for-my-harness/internal/bootstrap"
	"github.com/whyjp/for-my-harness/internal/intent"
	"github.com/whyjp/for-my-harness/internal/mcp"
	"github.com/whyjp/for-my-harness/internal/model"
	"github.com/whyjp/for-my-harness/internal/ssot"
	"github.com/whyjp/for-my-harness/internal/workflow"
	"gopkg.in/yaml.v3"
)

type multi []string

func (m *multi) String() string     { return strings.Join(*m, ",") }
func (m *multi) Set(v string) error { *m = append(*m, v); return nil }

type command struct {
	usage string
	run   func(e *workflow.Engine, args []string) (any, error)
}

// Commands is the registry of top-level commands; tests check that skills only name these.
var Commands = map[string]command{
	"init":      {"init [DIR] [--mode M] [--from PATH] [--verify-available] [--preview]", runInit},
	"bootstrap": {"bootstrap --mode M [--scan] [--verify-available] [--from PATH] [--preview]", runBootstrap},
	"status":    {"status [--knowledge]", runStatus},
	"reconcile": {"reconcile", func(e *workflow.Engine, a []string) (any, error) { return e.Reconcile() }},
	"run":       {`run "REQUEST" [--type T] [--tier T0..T3]`, runRun},
	"plan":      {`plan "REQUEST"`, func(e *workflow.Engine, a []string) (any, error) { return e.Plan(strings.Join(a, " ")) }},
	"inspect":   {"inspect ID", runInspect},
	"intent":    {"intent feature|invariant|module|contract|delta ... (module: --id MOD-x | --path DIR for a module this turn creates)", runIntent},
	"decide":    {"decide --question Q --option key=label[=effect]... --default KEY [--answer KEY] [--subject ID]... [--tier B|C] [--title T] [--proceeds-without SCOPE] | decide --adr ADR-x --answer KEY", runDecide},
	"decisions": {"decisions [QUERY]", func(e *workflow.Engine, a []string) (any, error) { return e.Decisions(strings.Join(a, " ")) }},
	"impact":    {"impact", func(e *workflow.Engine, a []string) (any, error) { return e.Impact() }},
	"contracts": {"contracts list [--module MOD-x] | contracts impact [--diff]", runContracts},
	"graph":     {"graph trace --from ID --to ID | graph neighbors ID [--depth N]", runGraph},
	"policy":    {"policy classify --action ACTION", runPolicy},
	"verify":    {`verify [--affected|--all] [--cmd "CMD"]... [--unverifiable "CHECK" --reason TEXT]`, runVerify},
	"sync":      {"sync [--commit]", runSync},
	"turn":      {"turn show | turn note --model-calls N | turn abort [--reason TEXT]", runTurn},
	"confirm":   {`confirm [--list] [ID [--value TEXT|--purpose TEXT] [--reject] [--external-consumers true|false]] [--merge CAND-x|--split CAND-x] [--all]`, runConfirm},
	"learn":     {`learn --kind architecture|type|lint|test|doc|skill --proposal TEXT | learn --preference key=value`, runLearn},
	"render":    {"render", func(e *workflow.Engine, a []string) (any, error) { return e.Render() }},
	"check":     {"check", runCheck},
	"open-docs": {"open-docs", func(e *workflow.Engine, a []string) (any, error) {
		return ssot.Path(e.Root, "generated", "explorer.html"), nil
	}},
	"verification-plan": {"verification-plan", func(e *workflow.Engine, a []string) (any, error) { return e.VerificationPlan() }},
	"mcp":               {"mcp (stdio MCP server)", runMCP},
	"domain":            {"domain ... (reserved for Phase 3)", reserved("domain state layer (DOM/STATE/TRN/INV) is reserved for Phase 3; Phase 1 only reserves the ID prefixes")},
	"eval":              {"eval run (optional appendix A benchmark)", reserved("appendix A benchmark is optional and runs after completion; Phase 1 keeps only the ChangeReceipt telemetry hooks")},
}

var errUsage = errors.New("usage")

func reserved(msg string) func(*workflow.Engine, []string) (any, error) {
	return func(*workflow.Engine, []string) (any, error) { return nil, errors.New(msg) }
}

// Main is the testable entry point. Exit codes: 0 ok, 1 error, 2 completion gate refused.
func Main(args []string, stdout, stderr io.Writer) int {
	root, asJSON := ".", false
	var rest []string
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--json":
			asJSON = true
		case args[i] == "--root" && i+1 < len(args):
			root = args[i+1]
			i++
		case strings.HasPrefix(args[i], "--root="):
			root = strings.TrimPrefix(args[i], "--root=")
		default:
			rest = append(rest, args[i])
		}
	}
	if len(rest) == 0 || rest[0] == "help" || rest[0] == "--help" || rest[0] == "-h" {
		printHelp(stdout)
		return 0
	}
	cmd, ok := Commands[rest[0]]
	if !ok {
		fmt.Fprintf(stderr, "unknown command %q\n", rest[0])
		printHelp(stderr)
		return 1
	}
	if rest[0] == "init" && len(rest) > 1 && !strings.HasPrefix(rest[1], "-") {
		root = rest[1]
		rest = append(rest[:1], rest[2:]...)
	}
	e, err := workflow.New(root)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	out, err := cmd.run(e, rest[1:])
	// A refused gate or a failed verification still returns the receipt that explains it;
	// any other error returns nothing worth printing.
	gate := errors.Is(err, workflow.ErrGate) || errors.Is(err, workflow.ErrFailed)
	if out != nil && (err == nil || gate) {
		emit(stdout, rest[0], out, asJSON)
	}
	switch {
	case errors.Is(err, errUsage):
		fmt.Fprintf(stderr, "usage: myharness %s\n", cmd.usage)
		return 1
	case gate:
		fmt.Fprintf(stderr, "BLOCKED: %v\n", err)
		return 2
	case err != nil:
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	return 0
}

func printHelp(w io.Writer) {
	fmt.Fprintln(w, "myharness — For My Harness Core CLI (Phase 1 draft)\n\nglobal flags: --root DIR, --json\n\ncommands:")
	for _, name := range model.SortedKeys(Commands) {
		fmt.Fprintf(w, "  myharness %s\n", Commands[name].usage)
	}
}

func emit(w io.Writer, name string, v any, asJSON bool) {
	if s, ok := v.(string); ok && !asJSON {
		fmt.Fprintln(w, s)
		return
	}
	if asJSON {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		enc.Encode(v)
		return
	}
	b, err := yaml.Marshal(toPlain(v))
	if err != nil {
		fmt.Fprintf(w, "%v\n", v)
		return
	}
	fmt.Fprintf(w, "# myharness %s\n%s", name, b)
}

// toPlain routes text output through JSON so YAML uses the same field names as --json and MCP.
func toPlain(v any) any {
	b, err := json.Marshal(v)
	if err != nil {
		return v
	}
	var out any
	if json.Unmarshal(b, &out) != nil {
		return v
	}
	return out
}

func newFlags(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return fs
}

// parse lets flags and positional arguments appear in any order.
func parse(fs *flag.FlagSet, args []string) ([]string, error) {
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, fmt.Errorf("%w: %v", errUsage, err)
		}
		args = fs.Args()
		if len(args) == 0 {
			return pos, nil
		}
		pos = append(pos, args[0])
		args = args[1:]
	}
}

func bootFlags(name string, args []string) (bootstrap.Options, error) {
	fs := newFlags(name)
	var o bootstrap.Options
	fs.StringVar(&o.Mode, "mode", "", "greenfield|code-only|adopt")
	fs.StringVar(&o.From, "from", "", "adopt source")
	fs.BoolVar(&o.VerifyAvailable, "verify-available", false, "run detected build/test commands")
	fs.BoolVar(&o.Preview, "preview", false, "report without writing")
	fs.Bool("scan", true, "scan sources (always on)")
	_, err := parse(fs, args)
	return o, err
}

func runInit(e *workflow.Engine, args []string) (any, error) {
	o, err := bootFlags("init", args)
	if err != nil {
		return nil, err
	}
	return e.Init(o)
}

func runBootstrap(e *workflow.Engine, args []string) (any, error) {
	o, err := bootFlags("bootstrap", args)
	if err != nil {
		return nil, err
	}
	return bootstrap.Run(e.Root, o)
}

func runStatus(e *workflow.Engine, args []string) (any, error) {
	fs := newFlags("status")
	knowledge := fs.Bool("knowledge", false, "include knowledge counts")
	if _, err := parse(fs, args); err != nil {
		return nil, err
	}
	st, err := e.Status()
	if err == nil && !*knowledge {
		st.Knowledge = nil
	}
	return st, err
}

func runRun(e *workflow.Engine, args []string) (any, error) {
	fs := newFlags("run")
	var o workflow.RunOptions
	fs.StringVar(&o.TaskType, "type", "", "task type override")
	fs.StringVar(&o.Tier, "tier", "", "verification tier override")
	pos, err := parse(fs, args)
	if err != nil || len(pos) == 0 {
		return nil, errUsage
	}
	if o.TaskType != "" && !knownType(o.TaskType) {
		return nil, fmt.Errorf("%w: --type must be one of %v", errUsage, typeNames())
	}
	if o.Tier != "" && !strings.Contains("T0 T1 T2 T3", o.Tier) {
		return nil, fmt.Errorf("%w: --tier must be T0..T3", errUsage)
	}
	return e.Run(strings.Join(pos, " "), o)
}

func runInspect(e *workflow.Engine, args []string) (any, error) {
	if len(args) != 1 {
		return nil, errUsage
	}
	return e.Inspect(args[0])
}

func originFlag(s string) model.Origin { return model.Origin(strings.ToUpper(s)) }

func runIntent(e *workflow.Engine, args []string) (any, error) {
	if len(args) == 0 {
		return nil, errUsage
	}
	fs := newFlags("intent " + args[0])
	origin := fs.String("origin", "", "USER_STATED|ADOPTED|INFERRED (default INFERRED)")
	switch args[0] {
	case "feature":
		var in workflow.IntentFeatureInput
		var anchors multi
		fs.StringVar(&in.ID, "id", "", "")
		fs.StringVar(&in.Name, "name", "", "")
		fs.StringVar(&in.Purpose, "purpose", "", "")
		fs.Var(&anchors, "anchor", "kind:ref")
		if _, err := parse(fs, args[1:]); err != nil {
			return nil, err
		}
		for _, a := range anchors {
			an, err := model.ParseAnchor(a)
			if err != nil {
				return nil, err
			}
			in.Anchors = append(in.Anchors, an)
		}
		in.Origin = originFlag(*origin)
		return e.IntentFeature(in)
	case "invariant":
		var in workflow.IntentInvariantInput
		fs.StringVar(&in.Feature, "feature", "", "")
		fs.StringVar(&in.Claim, "claim", "", "")
		fs.StringVar(&in.Test, "test", "", "")
		if _, err := parse(fs, args[1:]); err != nil || in.Feature == "" || in.Claim == "" {
			return nil, errUsage
		}
		in.Origin = originFlag(*origin)
		return e.IntentInvariant(in)
	case "module":
		var in workflow.IntentModuleInput
		var allowed, forbidden multi
		fs.StringVar(&in.ID, "id", "", "")
		fs.StringVar(&in.Path, "path", "", "directory of a module this turn creates")
		fs.StringVar(&in.Responsibility, "responsibility", "", "")
		fs.Var(&allowed, "allow", "MOD-x")
		fs.Var(&forbidden, "forbid", "MOD-x")
		if _, err := parse(fs, args[1:]); err != nil || (in.ID == "" && in.Path == "") {
			return nil, errUsage
		}
		in.Allowed, in.Forbidden, in.Origin = allowed, forbidden, originFlag(*origin)
		return e.IntentModule(in)
	case "contract":
		var in workflow.IntentContractInput
		fs.StringVar(&in.ID, "id", "", "")
		fs.StringVar(&in.Compatibility, "compatibility", "", "")
		fs.StringVar(&in.ExternalConsumers, "external-consumers", "", "true|false")
		if _, err := parse(fs, args[1:]); err != nil || in.ID == "" {
			return nil, errUsage
		}
		in.Origin = originFlag(*origin)
		return e.IntentContract(in)
	case "delta":
		var add, remove, preserve multi
		fs.Var(&add, "add", "")
		fs.Var(&remove, "remove", "")
		fs.Var(&preserve, "preserve", "FEAT-x: invariant")
		if _, err := parse(fs, args[1:]); err != nil {
			return nil, err
		}
		return e.IntentDelta(add, remove, preserve)
	}
	return nil, errUsage
}

func runDecide(e *workflow.Engine, args []string) (any, error) {
	fs := newFlags("decide")
	var in workflow.DecideInput
	var options, subjects multi
	tier := fs.String("tier", "B", "B|C")
	fs.StringVar(&in.Question, "question", "", "")
	fs.StringVar(&in.Title, "title", "", "")
	fs.StringVar(&in.Default, "default", "", "")
	fs.StringVar(&in.Answer, "answer", "", "")
	fs.StringVar(&in.ProceedsWithout, "proceeds-without", "", "scope that proceeds without an answer")
	fs.StringVar(&in.ADR, "adr", "", "answer an existing ADR")
	fs.Var(&options, "option", "key=label[=effect]")
	fs.Var(&subjects, "subject", "ID")
	if _, err := parse(fs, args); err != nil || (in.Question == "" && in.ADR == "") {
		return nil, errUsage
	}
	if in.ADR != "" {
		return e.Decide(in)
	}
	for _, o := range options {
		parts := strings.SplitN(o, "=", 3)
		op := model.Option{Key: parts[0], Label: parts[0]}
		if len(parts) > 1 {
			op.Label = parts[1]
		}
		if len(parts) > 2 {
			op.Effect = parts[2]
		}
		in.Options = append(in.Options, op)
	}
	in.Subjects, in.Tier = subjects, model.Tier(strings.ToUpper(*tier))
	if in.Tier == model.TierB && len(in.Options) < 2 {
		return nil, fmt.Errorf("%w: a tier B question needs 2-3 options", errUsage)
	}
	res, err := e.Decide(in)
	if err == nil && res.Blocked {
		// An unapproved tier C stop is not success: the caller must not proceed.
		return res, fmt.Errorf("%w: tier C stop %s waits for explicit approval", workflow.ErrGate, res.Decision.ID)
	}
	return res, err
}

func runContracts(e *workflow.Engine, args []string) (any, error) {
	if len(args) == 0 {
		return nil, errUsage
	}
	fs := newFlags("contracts")
	module := fs.String("module", "", "")
	fs.Bool("diff", true, "compare the working tree (default)")
	if _, err := parse(fs, args[1:]); err != nil {
		return nil, err
	}
	switch args[0] {
	case "list":
		return e.Contracts(*module)
	case "impact":
		return e.ContractsImpact()
	}
	return nil, errUsage
}

func runGraph(e *workflow.Engine, args []string) (any, error) {
	if len(args) == 0 {
		return nil, errUsage
	}
	fs := newFlags("graph")
	from := fs.String("from", "", "")
	to := fs.String("to", "", "")
	depth := fs.Int("depth", 2, "")
	pos, err := parse(fs, args[1:])
	if err != nil {
		return nil, err
	}
	switch args[0] {
	case "trace":
		if *from == "" || *to == "" {
			return nil, errUsage
		}
		return e.Trace(*from, *to)
	case "neighbors":
		if len(pos) != 1 {
			return nil, errUsage
		}
		return e.Neighbors(pos[0], *depth)
	}
	return nil, errUsage
}

func runPolicy(e *workflow.Engine, args []string) (any, error) {
	if len(args) == 0 || args[0] != "classify" {
		return nil, errUsage
	}
	fs := newFlags("policy")
	action := fs.String("action", "", "")
	if _, err := parse(fs, args[1:]); err != nil || *action == "" {
		return nil, errUsage
	}
	return e.Classify(*action)
}

func runVerify(e *workflow.Engine, args []string) (any, error) {
	fs := newFlags("verify")
	var o workflow.VerifyOptions
	var cmds multi
	fs.Bool("affected", true, "verify what the change affects (default)")
	fs.BoolVar(&o.All, "all", false, "")
	fs.Var(&cmds, "cmd", "command")
	fs.StringVar(&o.Unverifiable, "unverifiable", "", "check that cannot run here")
	fs.StringVar(&o.Reason, "reason", "", "")
	if _, err := parse(fs, args); err != nil {
		return nil, err
	}
	if o.Unverifiable != "" && o.Reason == "" {
		return nil, fmt.Errorf("%w: --unverifiable needs --reason", errUsage)
	}
	o.Commands = cmds
	return e.Verify(o)
}

func runSync(e *workflow.Engine, args []string) (any, error) {
	fs := newFlags("sync")
	var o workflow.SyncOptions
	fs.BoolVar(&o.Commit, "commit", false, "local commit when policy allows (tier A)")
	if _, err := parse(fs, args); err != nil {
		return nil, err
	}
	return e.Sync(o)
}

func runTurn(e *workflow.Engine, args []string) (any, error) {
	if len(args) == 0 {
		return nil, errUsage
	}
	fs := newFlags("turn")
	calls := fs.Int("model-calls", 0, "")
	reason := fs.String("reason", "aborted", "")
	if _, err := parse(fs, args[1:]); err != nil {
		return nil, err
	}
	switch args[0] {
	case "show":
		return e.TurnShow()
	case "note":
		return e.TurnNote(*calls)
	case "abort":
		return e.TurnAbort(*reason)
	}
	return nil, errUsage
}

func runConfirm(e *workflow.Engine, args []string) (any, error) {
	fs := newFlags("confirm")
	var in workflow.ConfirmInput
	list := fs.Bool("list", false, "")
	purpose := fs.String("purpose", "", "alias of --value")
	fs.StringVar(&in.Value, "value", "", "")
	fs.BoolVar(&in.Reject, "reject", false, "")
	fs.StringVar(&in.Merge, "merge", "", "")
	fs.StringVar(&in.Split, "split", "", "")
	fs.StringVar(&in.ExternalConsumers, "external-consumers", "", "true|false")
	fs.BoolVar(&in.All, "all", false, "")
	pos, err := parse(fs, args)
	if err != nil {
		return nil, err
	}
	if *purpose != "" {
		in.Value = *purpose
	}
	if *list || (len(pos) == 0 && in.Merge == "" && in.Split == "" && !in.All) {
		return e.Pending()
	}
	if len(pos) > 0 {
		in.ID = pos[0]
	}
	return e.Confirm(in)
}

func runLearn(e *workflow.Engine, args []string) (any, error) {
	fs := newFlags("learn")
	var in workflow.LearnInput
	fs.StringVar(&in.Kind, "kind", "", "")
	fs.StringVar(&in.Proposal, "proposal", "", "")
	fs.StringVar(&in.Preference, "preference", "", "key=value")
	if _, err := parse(fs, args); err != nil || (in.Preference == "" && in.Proposal == "") {
		return nil, errUsage
	}
	return e.Learn(in)
}

func runCheck(e *workflow.Engine, args []string) (any, error) {
	r, err := e.Check()
	if err == nil && !r.OK {
		return r, fmt.Errorf("%d consistency errors", len(r.Errors))
	}
	return r, err
}

func knownType(t string) bool {
	for _, n := range typeNames() {
		if n == t {
			return true
		}
	}
	return false
}

func typeNames() []string {
	var out []string
	for _, t := range intent.Types {
		out = append(out, t.Name)
	}
	return out
}

func runMCP(e *workflow.Engine, args []string) (any, error) {
	return nil, mcp.Serve(e, os.Stdin, os.Stdout)
}
