// Package turn is one unit of work under abbd mode: what it may touch, what it
// says it will change, what a human allowed, and how it ended. It is pure; the
// CLI reads and writes the files.
package turn

import (
	"fmt"
	"slices"
	"strings"

	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/topology"
)

type State string

const (
	Open    State = "OPEN"
	Blocked State = "BLOCKED"
	// Draft ended without completing. Verify and end may run again on it.
	Draft  State = "DRAFT"
	Synced State = "SYNCED"
	// Closed is a turn that never changed the working tree, so nothing was gated.
	Closed State = "CLOSED"
)

// Live turns take declarations and approvals.
func (s State) Live() bool { return s == Open || s == Blocked }

// Resumable turns can be verified and ended again.
func (s State) Resumable() bool { return s.Live() || s == Draft }

type Grade string

const (
	// A proceeds and is only recorded in the turn.
	A Grade = "A"
	// B is reported, proceeds by default, and leaves an UNCONFIRMED ADR.
	B Grade = "B"
	// C stops until a human approves it for this turn.
	C Grade = "C"
)

// MaxQuestions is how many human decisions one turn may ask for.
const MaxQuestions = 3

type Baseline struct {
	Revision    string `yaml:"revision,omitempty"`
	Fingerprint string `yaml:"fingerprint"`
}

const (
	Computed = "computed"
	ByAgent  = "agent"
)

type ImpactItem struct {
	Unit model.ID `yaml:"unit"`
	Why  string   `yaml:"why"`
	By   string   `yaml:"by"`
}

// Delta is the IntentDelta: what the agent says it will change. The gate never
// reads it; only the declared-versus-actual report does.
type Delta struct {
	Units   []model.ID `yaml:"units,omitempty"`
	Changes []string   `yaml:"changes,omitempty"`
}

// Approval allows one grade C item for this turn only.
type Approval struct {
	Item string `yaml:"item"`
	ADR  string `yaml:"adr"`
}

type Report struct {
	Gate            []string `yaml:"gate,omitempty"`
	DeclaredNotDone []string `yaml:"declared_not_done,omitempty"`
	DoneNotDeclared []string `yaml:"done_not_declared,omitempty"`
	// Why says what kept the turn from completing.
	Why string `yaml:"why,omitempty"`
}

type Turn struct {
	ID          string       `yaml:"id"`
	Requirement string       `yaml:"requirement"`
	Session     string       `yaml:"session"`
	State       State        `yaml:"state"`
	Baseline    Baseline     `yaml:"baseline"`
	Seeds       []model.ID   `yaml:"seeds"`
	Impact      []ImpactItem `yaml:"impact"`
	Delta       Delta        `yaml:"intent_delta"`
	Questions   []string     `yaml:"questions,omitempty"`
	Approvals   []Approval   `yaml:"approvals,omitempty"`
	Run         string       `yaml:"verification,omitempty"`
	Report      *Report      `yaml:"report,omitempty"`
}

// ComputeImpact follows the intended graph from the seeds: whatever depends
// on or consumes an impacted unit, the providers of an impacted contract, and
// the contracts an impacted unit provides. A provider is impacted because it
// implements the contract, but what it does elsewhere is not followed.
func ComputeImpact(in topology.Intended, seeds []model.ID) []ImpactItem {
	var out []ImpactItem
	seen := map[model.ID]bool{}
	var queue []model.ID
	add := func(id model.ID, why string, follow bool) {
		if seen[id] {
			return
		}
		seen[id] = true
		out = append(out, ImpactItem{Unit: id, Why: why, By: Computed})
		if follow {
			queue = append(queue, id)
		}
	}
	for _, s := range seeds {
		add(s, "seed", true)
	}
	for len(queue) > 0 {
		u := queue[0]
		queue = queue[1:]
		for _, e := range in.Relations {
			switch {
			case e.To == u && (e.Kind == model.DependsOn || e.Kind == model.Consumes):
				add(e.From, fmt.Sprintf("%s %s", e.Kind, u), true)
			case e.To == u && e.Kind == model.Provides && isContract(u):
				add(e.From, "provides "+string(u), false)
			case e.From == u && e.Kind == model.Provides:
				add(e.To, "provided by "+string(u), true)
			}
		}
	}
	return out
}

func isContract(id model.ID) bool {
	k, _, err := model.ParseID(string(id))
	return err == nil && k.IsContract()
}

// EnsureImpact puts back any computed item missing from the turn. The agent
// may add to the impact but never shrink it, whoever edited the file.
func (t *Turn) EnsureImpact(computed []ImpactItem) (restored []model.ID) {
	for _, c := range computed {
		if !t.Impacts(c.Unit) {
			t.Impact = append(t.Impact, c)
			restored = append(restored, c.Unit)
		}
	}
	return restored
}

func (t *Turn) Impacts(id model.ID) bool {
	return slices.ContainsFunc(t.Impact, func(i ImpactItem) bool { return i.Unit == id })
}

func (t *Turn) AddImpact(id model.ID) bool {
	if t.Impacts(id) {
		return false
	}
	t.Impact = append(t.Impact, ImpactItem{Unit: id, Why: "added by the agent", By: ByAgent})
	return true
}

func (t *Turn) Approved(key string) string {
	for _, a := range t.Approvals {
		if a.Item == key {
			return a.ADR
		}
	}
	return ""
}

// GradeOf adapts the stage 2 effect to a turn. A contract change no one
// consumes is grade B; any other change that would block outside a turn is
// grade C, and a realignment is grade B.
func GradeOf(in topology.Intended, units []model.Unit, it topology.Item) Grade {
	switch it.Effect {
	case topology.Blocked:
		if it.Change == topology.ContractChanged && !in.HasConsumers(units, it.Unit) {
			return B
		}
		return C
	case topology.Realign:
		return B
	}
	return A
}

// Declared parses and grades one declared change against the intended topology.
func Declared(in topology.Intended, units []model.Unit, key string) (topology.Item, Grade, error) {
	it, err := topology.ParseKey(key)
	if err != nil {
		return it, "", err
	}
	effect, ok := in.Effect(it)
	if !ok {
		return it, "", fmt.Errorf("%s does not change the intended topology", key)
	}
	it.Effect = effect
	if it.Change == topology.ContractChanged {
		u, _ := in.Unit(it.Unit)
		it.Old = u.Signature
	}
	return it, GradeOf(in, units, it), nil
}

// Question names the human decision an item asks for. A contract change asks
// about the signature it starts from, so approving one change of a contract
// never answers the next.
func Question(it topology.Item) string {
	if it.Change == topology.ContractChanged {
		return fmt.Sprintf("%s from %q", it.Key(), it.Old)
	}
	return it.Key()
}

// Verdict is the gate's judgment of one difference between intended and actual.
type Verdict struct {
	Item  topology.Item
	Grade Grade
	// ADR is the turn approval that allows a grade C item.
	ADR string
}

func (v Verdict) Allowed() bool { return v.Grade != C || v.ADR != "" }

func (v Verdict) String() string {
	detail := v.Item.Key()
	if v.Item.Change == topology.ContractChanged {
		detail += fmt.Sprintf(" (%q -> %q)", v.Item.Old, v.Item.New)
	}
	switch {
	case v.Grade == C && v.ADR != "":
		return fmt.Sprintf("C APPROVED %s by %s", detail, v.ADR)
	case v.Grade == C:
		return "C BLOCKED " + detail
	case v.Grade == B:
		return "B PROCEED " + detail
	}
	return "A " + string(v.Item.Effect) + " " + detail
}

// Judge compares the actual topology with the intended one plus this turn's
// approvals. The declaration plays no part.
func Judge(in topology.Intended, units []model.Unit, approvals []Approval) []Verdict {
	t := Turn{Approvals: approvals}
	var out []Verdict
	for _, it := range topology.Compare(in, units) {
		v := Verdict{Item: it, Grade: GradeOf(in, units, it)}
		if v.Grade == C {
			v.ADR = t.Approved(it.Key())
		}
		out = append(out, v)
	}
	return out
}

func Unallowed(vs []Verdict) []Verdict {
	var out []Verdict
	for _, v := range vs {
		if !v.Allowed() {
			out = append(out, v)
		}
	}
	return out
}

// Fold is the intended topology after a turn syncs: every grade B change and
// every approved grade C change enters it. A new unit stays a promotion candidate.
func Fold(in topology.Intended, vs []Verdict) topology.Intended {
	for _, v := range vs {
		if v.Grade != A && v.Allowed() {
			in = topology.Accept(in, v.Item)
		}
	}
	return in
}

// Contrast is the declared-versus-actual report. touched are the units whose
// anchored files changed during the turn.
func Contrast(d Delta, vs []Verdict, touched []model.ID) (declaredNotDone, doneNotDeclared []string) {
	var done []string
	for _, v := range vs {
		if v.Grade != A {
			done = append(done, v.Item.Key())
		}
	}
	for _, id := range touched {
		done = append(done, string(id))
	}
	declared := append([]string(nil), d.Changes...)
	for _, id := range d.Units {
		declared = append(declared, string(id))
	}
	for _, s := range declared {
		if !slices.Contains(done, s) {
			declaredNotDone = append(declaredNotDone, s)
		}
	}
	for _, s := range done {
		if !slices.Contains(declared, s) {
			doneNotDeclared = append(doneNotDeclared, s)
		}
	}
	slices.Sort(declaredNotDone)
	slices.Sort(doneNotDeclared)
	return declaredNotDone, doneNotDeclared
}

// Unexpected lists the relation changes the declaration did not name.
func Unexpected(d Delta, vs []Verdict) []string {
	var out []string
	for _, v := range vs {
		c := v.Item.Change
		if (c == topology.NewRelation || c == topology.RemovedRelation) && !slices.Contains(d.Changes, v.Item.Key()) {
			out = append(out, v.Item.Key())
		}
	}
	return out
}

// Run is the evidence one verification left.
type Run struct {
	ID       string             `yaml:"id"`
	Turn     string             `yaml:"turn"`
	Command  []string           `yaml:"command"`
	Tree     string             `yaml:"tree"`
	ExitCode int                `yaml:"exit_code"`
	Result   model.Verification `yaml:"result"`
	Output   string             `yaml:"output"`
}

// Evidence says why run does not let a turn complete on tree, or "" when it does.
// PASSED counts only for the tree it ran on.
func Evidence(run *Run, tree string) string {
	switch {
	case run == nil:
		return "no verification ran in this turn; run abbd turn verify"
	case run.Result != model.Passed:
		return fmt.Sprintf("%s %s (exit %d), output in .abbd/%s", run.ID, run.Result, run.ExitCode, run.Output)
	case run.Tree != tree:
		return fmt.Sprintf("the working tree changed after %s passed; run abbd turn verify again", run.ID)
	}
	return ""
}

// Touches reports whether a changed path falls under an anchor, which may name
// a file or a directory.
func Touches(a model.Anchor, path string) bool {
	p := strings.TrimSuffix(a.Path, "/")
	return path == p || strings.HasPrefix(path, p+"/")
}
