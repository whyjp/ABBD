package model

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Kind is an ID prefix. The registry below is the single list of object kinds.
type Kind string

const (
	KindFeature     Kind = "FEAT"
	KindModule      Kind = "MOD"
	KindInterface   Kind = "IF"
	KindEvent       Kind = "EVT"
	KindData        Kind = "DATA"
	KindConfig      Kind = "CFG"
	KindRequirement Kind = "REQ"
	KindDecision    Kind = "ADR"
	KindChange      Kind = "CHG"
	KindTest        Kind = "TEST"
	KindCandidate   Kind = "CAND"
	// Reserved for Phase 3+ (PRD 06, 08A). Prefixes only; nothing creates them.
	KindDomain     Kind = "DOM"
	KindState      Kind = "STATE"
	KindTransition Kind = "TRN"
	KindInvariant  Kind = "INV"
)

var ReservedKinds = map[Kind]bool{KindDomain: true, KindState: true, KindTransition: true, KindInvariant: true}

func KindOf(id string) Kind {
	i := strings.LastIndex(id, "-")
	if i <= 0 {
		return ""
	}
	return Kind(id[:i])
}

func FormatID(k Kind, n int) string { return fmt.Sprintf("%s-%03d", k, n) }

// NextID returns the next free numeric ID of kind k given existing IDs.
func NextID(k Kind, existing []string) string {
	max := 0
	for _, id := range existing {
		if KindOf(id) != k {
			continue
		}
		n, err := strconv.Atoi(id[len(k)+1:])
		if err == nil && n > max {
			max = n
		}
	}
	return FormatID(k, max+1)
}

// Anchor kinds used by stable identity (PRD 06).
const (
	AnchorSymbol = "symbol"
	AnchorPath   = "path"
	AnchorTest   = "test"
	AnchorEntry  = "entry"
	AnchorName   = "name"
)

type Anchor struct {
	Kind string `yaml:"kind" json:"kind"`
	Ref  string `yaml:"ref" json:"ref"`
}

func (a Anchor) String() string { return a.Kind + ":" + a.Ref }

func ParseAnchor(s string) (Anchor, error) {
	i := strings.Index(s, ":")
	if i <= 0 {
		return Anchor{}, fmt.Errorf("anchor %q must be kind:ref", s)
	}
	return Anchor{Kind: s[:i], Ref: s[i+1:]}, nil
}

type Observation struct {
	Behavior     string       `yaml:"behavior" json:"behavior"`
	Origin       Origin       `yaml:"origin" json:"origin"`
	Verification Verification `yaml:"verification" json:"verification"`
	Evidence     []string     `yaml:"evidence,omitempty" json:"evidence,omitempty"`
	Location     string       `yaml:"location,omitempty" json:"location,omitempty"`
}

type Invariant struct {
	Claim        string       `yaml:"claim" json:"claim"`
	Origin       Origin       `yaml:"origin" json:"origin"`
	Confirmation Confirmation `yaml:"confirmation" json:"confirmation"`
	Verification Verification `yaml:"verification" json:"verification"`
	Evidence     []string     `yaml:"evidence,omitempty" json:"evidence,omitempty"`
}

type Intent struct {
	Purpose    Semantic    `yaml:"purpose" json:"purpose"`
	Invariants []Invariant `yaml:"invariants,omitempty" json:"invariants,omitempty"`
}

type EntryPoint struct {
	Kind     string `yaml:"kind" json:"kind"`
	Selector string `yaml:"selector" json:"selector"`
	Contract string `yaml:"contract,omitempty" json:"contract,omitempty"`
}

type TestRef struct {
	TestID       string       `yaml:"test_id" json:"test_id"`
	Command      string       `yaml:"command,omitempty" json:"command,omitempty"`
	Expected     string       `yaml:"expected,omitempty" json:"expected,omitempty"`
	Verification Verification `yaml:"verification" json:"verification"`
	Evidence     string       `yaml:"evidence,omitempty" json:"evidence,omitempty"`
}

type SyncMark struct {
	Revision    string `yaml:"revision" json:"revision"`
	Fingerprint string `yaml:"fingerprint,omitempty" json:"fingerprint,omitempty"`
	Evidence    string `yaml:"evidence,omitempty" json:"evidence,omitempty"`
	Change      string `yaml:"change,omitempty" json:"change,omitempty"`
}

// Lifecycle of a map item. A draft item was produced by an unverified turn and is not
// the last confirmed version (PRD 03 "HTML distinguishes last confirmed and draft").
type ItemStatus string

const (
	ItemCurrent ItemStatus = "current"
	ItemDraft   ItemStatus = "draft"
	ItemRemoved ItemStatus = "removed"
)

type Feature struct {
	ID                string        `yaml:"id" json:"id"`
	Name              string        `yaml:"name" json:"name"`
	Status            ItemStatus    `yaml:"status" json:"status"`
	Intent            Intent        `yaml:"intent" json:"intent"`
	Observations      []Observation `yaml:"observations,omitempty" json:"observations,omitempty"`
	EntryPoints       []EntryPoint  `yaml:"entry_points,omitempty" json:"entry_points,omitempty"`
	ExternalContracts []string      `yaml:"external_contracts,omitempty" json:"external_contracts,omitempty"`
	OwnedBy           []string      `yaml:"owned_by,omitempty" json:"owned_by,omitempty"`
	Requirements      []string      `yaml:"requirements,omitempty" json:"requirements,omitempty"`
	Verification      []TestRef     `yaml:"verification,omitempty" json:"verification,omitempty"`
	Anchors           []Anchor      `yaml:"anchors" json:"anchors"`
	Changes           []string      `yaml:"changes,omitempty" json:"changes,omitempty"`
	LastSynced        SyncMark      `yaml:"last_synced" json:"last_synced"`
}

type DependencyPolicy struct {
	Allowed   []string `yaml:"allowed,omitempty" json:"allowed,omitempty"`
	Forbidden []string `yaml:"forbidden,omitempty" json:"forbidden,omitempty"`
	Origin    Origin   `yaml:"origin,omitempty" json:"origin,omitempty"`
}

// Contract is one external contract (PRD 08A). It is stored once, in its owning module.
type Contract struct {
	ID                string       `yaml:"id" json:"id"`
	Kind              string       `yaml:"kind" json:"kind"`
	Direction         string       `yaml:"direction" json:"direction"`
	Selector          string       `yaml:"selector" json:"selector"`
	Location          string       `yaml:"location,omitempty" json:"location,omitempty"`
	Symbol            string       `yaml:"symbol,omitempty" json:"symbol,omitempty"`
	Origin            Origin       `yaml:"origin" json:"origin"`
	Note              string       `yaml:"note,omitempty" json:"note,omitempty"`
	Verification      Verification `yaml:"verification" json:"verification"`
	VerificationNote  string       `yaml:"verification_note,omitempty" json:"verification_note,omitempty"`
	Compatibility     Semantic     `yaml:"compatibility" json:"compatibility"`
	ExternalConsumers *bool        `yaml:"external_consumers,omitempty" json:"external_consumers,omitempty"`
	ConsumersOrigin   Origin       `yaml:"external_consumers_origin,omitempty" json:"external_consumers_origin,omitempty"`
	Status            ItemStatus   `yaml:"status" json:"status"`
	Anchors           []Anchor     `yaml:"anchors" json:"anchors"`
}

type Serving struct {
	DeployUnit string `yaml:"deploy_unit" json:"deploy_unit"`
	Source     string `yaml:"source,omitempty" json:"source,omitempty"`
	Origin     Origin `yaml:"origin" json:"origin"`
}

type Symbol struct {
	Name     string `yaml:"name" json:"name"`
	Kind     string `yaml:"kind" json:"kind"`
	Location string `yaml:"location" json:"location"`
}

type Module struct {
	ID                 string           `yaml:"id" json:"id"`
	Name               string           `yaml:"name" json:"name"`
	Path               string           `yaml:"path" json:"path"`
	Language           string           `yaml:"language" json:"language"`
	Status             ItemStatus       `yaml:"status" json:"status"`
	Responsibility     Semantic         `yaml:"responsibility" json:"responsibility"`
	DependsOn          []string         `yaml:"depends_on,omitempty" json:"depends_on,omitempty"`
	DependencyPolicy   DependencyPolicy `yaml:"dependency_policy,omitempty" json:"dependency_policy,omitempty"`
	ExternalContracts  []Contract       `yaml:"external_contracts,omitempty" json:"external_contracts,omitempty"`
	Serving            []Serving        `yaml:"serving,omitempty" json:"serving,omitempty"`
	Symbols            []Symbol         `yaml:"symbols,omitempty" json:"symbols,omitempty"`
	Tests              []string         `yaml:"tests,omitempty" json:"tests,omitempty"`
	Analyzer           string           `yaml:"analyzer" json:"analyzer"`
	Anchors            []Anchor         `yaml:"anchors" json:"anchors"`
	LastSyncedRevision string           `yaml:"last_synced_revision" json:"last_synced_revision"`
}

type Test struct {
	ID           string       `yaml:"id" json:"id"`
	Name         string       `yaml:"name" json:"name"`
	Module       string       `yaml:"module" json:"module"`
	Location     string       `yaml:"location" json:"location"`
	Command      string       `yaml:"command" json:"command"`
	Origin       Origin       `yaml:"origin" json:"origin"`
	Verification Verification `yaml:"verification" json:"verification"`
	Evidence     string       `yaml:"evidence,omitempty" json:"evidence,omitempty"`
	Anchors      []Anchor     `yaml:"anchors" json:"anchors"`
	Status       ItemStatus   `yaml:"status" json:"status"`
}

// RequirementScope separates a request for one change from a rule the user wants kept on every
// path from now on (PRD ch.03 Current Intent). Project-scope requirements are standing rules.
type RequirementScope string

const (
	ScopeChange  RequirementScope = "change"
	ScopeProject RequirementScope = "project"
)

type Requirement struct {
	ID        string           `yaml:"id" json:"id"`
	Statement Semantic         `yaml:"statement" json:"statement"`
	Change    string           `yaml:"change" json:"change"`
	Scope     RequirementScope `yaml:"scope,omitempty" json:"scope,omitempty"`
	// Auto marks a rule detected from the wording of a request, not recorded on purpose; it is
	// retired with the request if that turn is abandoned.
	Auto bool `yaml:"auto,omitempty" json:"auto,omitempty"`
}

// Tier is the approval grade of PRD ch.10.
type Tier string

const (
	TierA Tier = "A"
	TierB Tier = "B"
	TierC Tier = "C"
)

type Decision struct {
	ID              string       `yaml:"id" json:"id"`
	Title           string       `yaml:"title" json:"title"`
	Tier            Tier         `yaml:"tier" json:"tier"`
	Question        string       `yaml:"question" json:"question"`
	Options         []Option     `yaml:"options" json:"options"`
	Default         string       `yaml:"default" json:"default"`
	Chosen          string       `yaml:"chosen" json:"chosen"`
	Answered        bool         `yaml:"answered" json:"answered"`
	Asked           bool         `yaml:"asked" json:"asked"`
	ProceedsWithout string       `yaml:"proceeds_without,omitempty" json:"proceeds_without,omitempty"`
	Rationale       Semantic     `yaml:"rationale" json:"rationale"`
	Change          string       `yaml:"change" json:"change"`
	Subjects        []string     `yaml:"subjects,omitempty" json:"subjects,omitempty"`
	Supersedes      string       `yaml:"supersedes,omitempty" json:"supersedes,omitempty"`
	Confirmation    Confirmation `yaml:"confirmation" json:"confirmation"`
}

type Option struct {
	Key    string `yaml:"key" json:"key"`
	Label  string `yaml:"label" json:"label"`
	Effect string `yaml:"effect,omitempty" json:"effect,omitempty"`
}

// TurnState is the workflow state machine of PRD ch.03.
type TurnState string

const (
	StateDiscovering    TurnState = "DISCOVERING"
	StateIntentResolved TurnState = "INTENT_RESOLVED"
	StateImplementing   TurnState = "IMPLEMENTING"
	StateVerifying      TurnState = "VERIFYING"
	StateSynchronizing  TurnState = "SYNCHRONIZING"
	StateCompleted      TurnState = "COMPLETED"
	StateBlocked        TurnState = "BLOCKED"
	StateNeedsUser      TurnState = "NEEDS_USER"
)

type IntentDelta struct {
	Additions           []string `yaml:"additions,omitempty" json:"additions,omitempty"`
	Removals            []string `yaml:"removals,omitempty" json:"removals,omitempty"`
	PreservedInvariants []string `yaml:"preserved_invariants,omitempty" json:"preserved_invariants,omitempty"`
}

type IntentFidelity struct {
	DeltaItems           int `yaml:"delta_items" json:"delta_items"`
	CoveredByPassedTests int `yaml:"covered_by_passed_tests" json:"covered_by_passed_tests"`
	UserConfirmed        int `yaml:"user_confirmed" json:"user_confirmed"`
}

type Telemetry struct {
	ModelCalls     int    `yaml:"model_calls" json:"model_calls"`
	ModelCallsNote string `yaml:"model_calls_note,omitempty" json:"model_calls_note,omitempty"`
	GraphQueries   int    `yaml:"graph_queries" json:"graph_queries"`
	TestRuns       int    `yaml:"test_runs" json:"test_runs"`
	QuestionsAsked int    `yaml:"questions_asked" json:"questions_asked"`
	DurationSec    int64  `yaml:"duration_sec" json:"duration_sec"`
	StartedAt      string `yaml:"started_at" json:"started_at"`
	FinishedAt     string `yaml:"finished_at,omitempty" json:"finished_at,omitempty"`
}

type SyncState struct {
	FeatureMap      Freshness `yaml:"feature_map" json:"feature_map"`
	ArchitectureMap Freshness `yaml:"architecture_map" json:"architecture_map"`
	Graph           Freshness `yaml:"graph" json:"graph"`
	HTML            Freshness `yaml:"html" json:"html"`
}

type DecisionRef struct {
	ADR      string `yaml:"adr" json:"adr"`
	Tier     Tier   `yaml:"tier" json:"tier"`
	Answered bool   `yaml:"answered" json:"answered"`
}

type Plan struct {
	TaskType         string   `yaml:"task_type" json:"task_type"`
	VerificationTier string   `yaml:"verification_tier" json:"verification_tier"`
	Steps            []string `yaml:"steps" json:"steps"`
	ContextStrategy  string   `yaml:"context_strategy" json:"context_strategy"`
	ContextReason    string   `yaml:"context_reason" json:"context_reason"`
	Targets          []string `yaml:"targets,omitempty" json:"targets,omitempty"`
	Ambiguous        bool     `yaml:"ambiguous" json:"ambiguous"`
	// TierReason is required when the verification tier is set below the routed one.
	TierReason string `yaml:"tier_reason,omitempty" json:"tier_reason,omitempty"`
}

// Change is the CHG-* record: intent delta, turn state and ChangeReceipt in one place.
type Change struct {
	ID                   string         `yaml:"id" json:"id"`
	RequestSummary       string         `yaml:"request_summary" json:"request_summary"`
	IntentSource         string         `yaml:"intent_source" json:"intent_source"`
	ChangedBy            string         `yaml:"changed_by" json:"changed_by"`
	State                TurnState      `yaml:"state" json:"state"`
	BaseRevision         string         `yaml:"base_revision" json:"base_revision"`
	BaseFingerprint      string         `yaml:"base_fingerprint" json:"base_fingerprint"`
	ResultRevision       string         `yaml:"result_revision,omitempty" json:"result_revision,omitempty"`
	ResultFingerprint    string         `yaml:"result_fingerprint,omitempty" json:"result_fingerprint,omitempty"`
	Plan                 Plan           `yaml:"plan" json:"plan"`
	IntentDelta          IntentDelta    `yaml:"intent_delta" json:"intent_delta"`
	Intends              []string       `yaml:"intends,omitempty" json:"intends,omitempty"`
	Delivers             []string       `yaml:"delivers,omitempty" json:"delivers,omitempty"`
	Impacted             []string       `yaml:"impacted,omitempty" json:"impacted,omitempty"`
	FilesChanged         []string       `yaml:"files_changed,omitempty" json:"files_changed,omitempty"`
	VerificationEvidence []string       `yaml:"verification_evidence,omitempty" json:"verification_evidence,omitempty"`
	Verification         Verification   `yaml:"verification" json:"verification"`
	IntentFidelity       IntentFidelity `yaml:"intent_fidelity" json:"intent_fidelity"`
	Decisions            []DecisionRef  `yaml:"decisions,omitempty" json:"decisions,omitempty"`
	Blocked              []string       `yaml:"blocked,omitempty" json:"blocked,omitempty"`
	Telemetry            Telemetry      `yaml:"telemetry" json:"telemetry"`
	SyncState            SyncState      `yaml:"sync_state" json:"sync_state"`
	Risks                []string       `yaml:"risks,omitempty" json:"risks,omitempty"`
	NextSteps            []string       `yaml:"next_steps,omitempty" json:"next_steps,omitempty"`
	NewUnconfirmed       int            `yaml:"new_unconfirmed" json:"new_unconfirmed"`
	// ModuleIntents holds meaning for modules this turn creates, keyed by path until sync gives them an ID.
	ModuleIntents map[string]Semantic `yaml:"module_intents,omitempty" json:"module_intents,omitempty"`
	// StructuralDeltas are the turn's structural changes, each with its own tier and decision (v1.4 CR-3).
	StructuralDeltas []StructuralDelta `yaml:"structural_deltas,omitempty" json:"structural_deltas,omitempty"`
	Commit           string            `yaml:"commit,omitempty" json:"commit,omitempty"`
	// ReproWaiver is why a bug-fix turn has no failing-first evidence; empty means it must have one.
	ReproWaiver string `yaml:"repro_waiver,omitempty" json:"repro_waiver,omitempty"`
	Time        string `yaml:"time" json:"time"`
}

// Evidence is one verification run (PRD FR-07).
// EvidenceLevel separates unit/integration checks from a run of the product the way a user
// reaches it (PRD ch.04 T3 "사용자 수준 실행 증거").
const LevelUser = "user"

type Evidence struct {
	ID           string       `yaml:"id" json:"id"`
	Level        string       `yaml:"level,omitempty" json:"level,omitempty"`
	Command      string       `yaml:"command" json:"command"`
	Dir          string       `yaml:"dir" json:"dir"`
	Revision     string       `yaml:"revision" json:"revision"`
	Fingerprint  string       `yaml:"fingerprint" json:"fingerprint"`
	ExitCode     int          `yaml:"exit_code" json:"exit_code"`
	Verification Verification `yaml:"verification" json:"verification"`
	Reason       string       `yaml:"reason,omitempty" json:"reason,omitempty"`
	Change       string       `yaml:"change,omitempty" json:"change,omitempty"`
	Tests        []TestResult `yaml:"tests,omitempty" json:"tests,omitempty"`
	OutputTail   string       `yaml:"output_tail,omitempty" json:"output_tail,omitempty"`
	StartedAt    string       `yaml:"started_at" json:"started_at"`
	DurationMs   int64        `yaml:"duration_ms" json:"duration_ms"`
}

type TestResult struct {
	Package string       `yaml:"package" json:"package"`
	Name    string       `yaml:"name" json:"name"`
	Result  Verification `yaml:"result" json:"result"`
	// Output is the failing test's own output: the evidence of what disagreed, not just that
	// something did ("red is not a measurement").
	Output string `yaml:"output,omitempty" json:"output,omitempty"`
}

// Candidate is a merge candidate produced when exactly one anchor matched (AC-BOOT-03).
type Candidate struct {
	ID           string       `yaml:"id" json:"id"`
	Kind         Kind         `yaml:"kind" json:"kind"`
	CandidateOf  string       `yaml:"candidate_of" json:"candidate_of"`
	Name         string       `yaml:"name" json:"name"`
	Anchors      []Anchor     `yaml:"anchors" json:"anchors"`
	Matched      []Anchor     `yaml:"matched" json:"matched"`
	Confirmation Confirmation `yaml:"confirmation" json:"confirmation"`
	// Target holds the fresh facts while the candidate is open: the new module or contract,
	// or, for a feature, the entry contract it would absorb.
	Target string `yaml:"target" json:"target"`
}

func SortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// DeltaKind is a structural change kind of PRD v1.4 08B. Internal code edits are not deltas.
type DeltaKind string

const (
	ModuleAdded          DeltaKind = "module_added"
	ModuleRemoved        DeltaKind = "module_removed"
	ModuleSplit          DeltaKind = "module_split"
	ModuleMerged         DeltaKind = "module_merged"
	DependencyAdded      DeltaKind = "dependency_added"
	DependencyRemoved    DeltaKind = "dependency_removed"
	ForbiddenDependency  DeltaKind = "forbidden_dependency"
	ContractAdded        DeltaKind = "contract_added"
	ContractRemoved      DeltaKind = "contract_removed"
	ContractCompatChange DeltaKind = "contract_compat_changed"
	OwnershipMoved       DeltaKind = "ownership_moved"
	DeployMappingChanged DeltaKind = "deploy_mapping_changed"
	FeaturePathChanged   DeltaKind = "feature_path_changed"
)

// DeltaStatus is the card state of PRD v1.4 08B section 5.
type DeltaStatus string

const (
	DeltaApplied        DeltaStatus = "applied"         // tier A: applied, can be reverted
	DeltaDefaultApplied DeltaStatus = "default_applied" // tier B: default taken, decision open
	DeltaPending        DeltaStatus = "pending"         // tier C: not executed until approved
	DeltaApproved       DeltaStatus = "approved"
	DeltaRejected       DeltaStatus = "rejected"
	DeltaRevertAsked    DeltaStatus = "revert_requested"
	DeltaSuperseded     DeltaStatus = "superseded" // the item changed again after the decision
)

type StructuralDelta struct {
	ID       string        `yaml:"id" json:"id"` // CHG-x#n
	Kind     DeltaKind     `yaml:"kind" json:"kind"`
	Subject  string        `yaml:"subject" json:"subject"` // node or edge key, e.g. MOD-002, MOD-001->MOD-003
	Nodes    []string      `yaml:"nodes" json:"nodes"`     // nodes to highlight on the topology
	Before   string        `yaml:"before,omitempty" json:"before,omitempty"`
	After    string        `yaml:"after,omitempty" json:"after,omitempty"`
	Tier     Tier          `yaml:"tier" json:"tier"`
	Reason   string        `yaml:"reason" json:"reason"`
	Inferred bool          `yaml:"inferred,omitempty" json:"inferred,omitempty"` // new INFERRED item: drawn dashed
	Status   DeltaStatus   `yaml:"status" json:"status"`
	ADR      string        `yaml:"adr,omitempty" json:"adr,omitempty"` // tier B/C decision record
	Decision *ItemDecision `yaml:"decision,omitempty" json:"decision,omitempty"`
}

// ItemDecision is the one decision record every channel (serve, decision file, MCP) writes.
type ItemDecision struct {
	Decision    string `yaml:"decision" json:"decision"` // approve | reject | option:<key> | revert
	Reason      string `yaml:"reason,omitempty" json:"reason,omitempty"`
	At          string `yaml:"at" json:"at"`
	Revision    string `yaml:"revision" json:"revision"` // workspace fingerprint the decision saw
	Channel     string `yaml:"channel" json:"channel"`   // serve | file | mcp | cli
	ADR         string `yaml:"adr,omitempty" json:"adr,omitempty"`
	Consumed    string `yaml:"consumed_by,omitempty" json:"consumed_by,omitempty"` // turn that took a rejection as IntentDelta
	Invalidated string `yaml:"invalidated_by,omitempty" json:"invalidated_by,omitempty"`
}
