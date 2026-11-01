// Package turn is one unit of work under abbd mode: what it may touch, what it
// says it will change, what a human allowed, and how it ended. It is pure; the
// CLI reads and writes the files.
package turn

import (
	"fmt"
	"maps"
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

// Live turns take declarations.
func (s State) Live() bool { return s == Open || s == Blocked }

// Resumable turns take approvals and can be verified and ended again.
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
	// Extractor fingerprints the registered extraction script, which the tree
	// fingerprint leaves out with the rest of .abbd/. It is empty while none is registered.
	Extractor string `yaml:"extractor,omitempty"`
	// Digests are the consumed design contracts' code fingerprints at the start.
	Digests map[model.ID]string `yaml:"digests,omitempty"`
}

// ConsumedDigests fingerprints every design contract something consumes, for
// a turn's baseline. A contract the extraction gives no digest is left out.
func ConsumedDigests(in topology.Intended, units []model.Unit) map[model.ID]string {
	out := map[model.ID]string{}
	for _, u := range topology.Present(units) {
		if iu, ok := in.Unit(u.ID); ok && iu.Kind.IsContract() && u.Digest != "" && in.HasConsumers(units, u.ID) {
			out[u.ID] = u.Digest
		}
	}
	return out
}

// Touched finds the consumed design contracts whose code changed during a
// turn while their signature did not. The gate cannot read a change of
// meaning, so each is grade B: the turn proceeds, and a human sees it.
func Touched(baseline map[model.ID]string, units []model.Unit, vs []Verdict) []Verdict {
	var out []Verdict
	for _, u := range topology.Present(units) {
		was, ok := baseline[u.ID]
		if !ok || u.Digest == was || slices.ContainsFunc(vs, func(v Verdict) bool {
			return v.Item.Change == topology.ContractChanged && v.Item.Unit == u.ID
		}) {
			continue
		}
		out = append(out, Verdict{Item: topology.Item{Change: topology.ContractTouched, Unit: u.ID, Old: was, New: u.Digest}, Grade: B})
	}
	return out
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

// Rejection is a human's no to one grade C item, with the reason the next
// work in the turn reads.
type Rejection struct {
	Item   string `yaml:"item"`
	ADR    string `yaml:"adr"`
	Reason string `yaml:"reason"`
}

type Report struct {
	Gate            []string `yaml:"gate,omitempty"`
	DeclaredNotDone []string `yaml:"declared_not_done,omitempty"`
	DoneNotDeclared []string `yaml:"done_not_declared,omitempty"`
	Notes           []string `yaml:"notes,omitempty"`
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
	Rejections  []Rejection  `yaml:"rejections,omitempty"`
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

// Tier says how closely the agent reads an impact item. It is read from the
// intended graph each time, so the change record keeps its shape.
type Tier string

const (
	// Direct items are read closely.
	Direct Tier = "direct"
	// Far items are the rest of the transitive closure, checked briefly.
	Far Tier = "far"
)

// Tiers splits the impact, each tier in recorded order. Direct are the seeds,
// the units that share a relation with a seed, the consumers of a contract that
// shares one, and the agent's own additions. Far is everything else computed.
func (t Turn) Tiers(in topology.Intended) (direct, far []ImpactItem) {
	near := map[model.ID]bool{}
	for _, s := range t.Seeds {
		near[s] = true
	}
	for _, e := range in.Relations {
		if slices.Contains(t.Seeds, e.From) {
			near[e.To] = true
		}
		if slices.Contains(t.Seeds, e.To) {
			near[e.From] = true
		}
	}
	reached := maps.Clone(near)
	for _, e := range in.Relations {
		if (e.Kind == model.DependsOn || e.Kind == model.Consumes) && isContract(e.To) && near[e.To] {
			reached[e.From] = true
		}
	}
	for _, i := range t.Impact {
		if reached[i.Unit] || i.By == ByAgent {
			direct = append(direct, i)
		} else {
			far = append(far, i)
		}
	}
	return direct, far
}

// Line is how the CLI and the change record print an item of a tier.
func (i ImpactItem) Line(tier Tier) string {
	return fmt.Sprintf("%s: %s (%s, %s)", i.Unit, i.Why, i.By, tier)
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

// Waiting reports whether a key the turn still declares asked a question no
// human has answered. Questions only grows, because it counts toward the cap,
// so a key declared away no longer counts here.
func (t *Turn) Waiting() bool {
	return slices.ContainsFunc(t.Questions, func(q string) bool {
		return slices.Contains(t.Delta.Changes, q) && t.Approved(q) == "" && t.Rejected(q) == nil
	})
}

// Rejected returns the standing rejection of key. An approval given after a
// rejection replaces it.
func (t *Turn) Rejected(key string) *Rejection {
	if t.Approved(key) != "" {
		return nil
	}
	for i := len(t.Rejections) - 1; i >= 0; i-- {
		if t.Rejections[i].Item == key {
			return &t.Rejections[i]
		}
	}
	return nil
}

func (r Rejection) String() string {
	return fmt.Sprintf("REJECTED %s by %s: %s", r.Item, r.ADR, r.Reason)
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

// Question names the human decision an item asks for, against the intended
// topology it starts from. A contract change names the signature it starts
// from, so approving one change of a contract never answers the next. Any
// other change names the intended revision, because its key alone stays the
// same after the change enters the design and leaves it again.
func Question(in topology.Intended, it topology.Item) string {
	if it.Change == topology.ContractChanged {
		return fmt.Sprintf("%s from %q", it.Key(), it.Old)
	}
	return fmt.Sprintf("%s at intended revision %d", it.Key(), in.Revision)
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
	// Why says what failed a run whose exit code alone would have passed.
	Why string `yaml:"why,omitempty"`
}

// EditedTree is why a run that changed the working tree fails: what it
// checked is no longer the tree the turn would complete on. paths are the
// files it wrote, so a cache directory can be told apart from a formatter.
func EditedTree(paths []string) string {
	why := "the verify command changed the working tree, so it cannot prove the current tree; make it read-only"
	if len(paths) == 0 {
		return why
	}
	const shown = 5
	list := strings.Join(paths[:min(len(paths), shown)], ", ")
	if len(paths) > shown {
		list += fmt.Sprintf(" and %d more", len(paths)-shown)
	}
	return why + ", or gitignore what it writes if that is build or cache output (it wrote " + list + ")"
}

// Evidence says why run does not let a turn complete on tree, or "" when it does.
// PASSED counts only for the tree it ran on.
func Evidence(run *Run, tree string) string {
	switch {
	case run == nil:
		return "no verification ran in this turn; run abbd turn verify"
	case run.Result != model.Passed && run.Why != "":
		return fmt.Sprintf("%s %s: %s", run.ID, run.Result, run.Why)
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
