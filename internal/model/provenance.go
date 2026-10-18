// Package model holds the ABBD data model (PRD ch.06).
// The three provenance axes are defined here and nowhere else.
package model

import "fmt"

type Origin string

const (
	UserStated Origin = "USER_STATED"
	Adopted    Origin = "ADOPTED"
	Extracted  Origin = "EXTRACTED"
	Observed   Origin = "OBSERVED"
	Inferred   Origin = "INFERRED"
	// UnknownOrigin marks a relation the analyzer could not resolve (runtime-built topics, plugins).
	// It is never "absent": PRD 08A forbids concluding "no relation".
	UnknownOrigin Origin = "UNKNOWN"
)

type Verification string

const (
	NotRun       Verification = "NOT_RUN"
	Passed       Verification = "PASSED"
	Failed       Verification = "FAILED"
	Unverifiable Verification = "UNVERIFIABLE"
)

type Confirmation string

const (
	Unconfirmed Confirmation = "UNCONFIRMED"
	Confirmed   Confirmation = "CONFIRMED"
	Rejected    Confirmation = "REJECTED"
)

// Freshness is the artifact-level signal of PRD ch.07, a separate axis from provenance.
type Freshness string

const (
	Current      Freshness = "CURRENT"
	Stale        Freshness = "STALE"
	Draft        Freshness = "DRAFT"
	UnknownFresh Freshness = "UNKNOWN"
)

func (o Origin) Valid() bool {
	switch o {
	case UserStated, Adopted, Extracted, Observed, Inferred, UnknownOrigin:
		return true
	}
	return false
}

// SemanticAllowed reports whether a "why" field (purpose, responsibility, rationale,
// invariant claim) may carry this origin. EXTRACTED/OBSERVED only describe "what".
func (o Origin) SemanticAllowed() bool {
	return o == UserStated || o == Adopted || o == Inferred
}

func (v Verification) Valid() bool {
	switch v {
	case NotRun, Passed, Failed, Unverifiable:
		return true
	}
	return false
}

func (c Confirmation) Valid() bool {
	switch c {
	case Unconfirmed, Confirmed, Rejected:
		return true
	}
	return false
}

// Semantic is a "why" field. Its origin is restricted and it is promoted only by the user.
type Semantic struct {
	Value        string       `yaml:"value" json:"value"`
	Origin       Origin       `yaml:"origin" json:"origin"`
	Confirmation Confirmation `yaml:"confirmation" json:"confirmation"`
	ConfirmedIn  string       `yaml:"confirmed_in,omitempty" json:"confirmed_in,omitempty"`
	Replaced     string       `yaml:"rejected_value,omitempty" json:"rejected_value,omitempty"`
}

// NewSemantic builds a "why" field and enforces the origin rule at construction.
func NewSemantic(value string, origin Origin) (Semantic, error) {
	if !origin.SemanticAllowed() {
		return Semantic{}, fmt.Errorf("semantic field cannot have origin %s (only USER_STATED, ADOPTED, INFERRED)", origin)
	}
	c := Unconfirmed
	if origin == UserStated {
		c = Confirmed
	}
	return Semantic{Value: value, Origin: origin, Confirmation: c}, nil
}

func MustSemantic(value string, origin Origin) Semantic {
	s, err := NewSemantic(value, origin)
	if err != nil {
		panic(err)
	}
	return s
}

// Pending is true when the item belongs on the confirm queue: INFERRED or ADOPTED and not yet seen.
func (s Semantic) Pending() bool {
	return s.Value != "" && s.Origin != UserStated && s.Confirmation == Unconfirmed
}

// Badge is the derived display value of PRD ch.06 "rules in three lines".
type Badge string

const (
	BadgeConfirmed    Badge = "확정"
	BadgeInferred     Badge = "추정"
	BadgeAdopted      Badge = "가져옴"
	BadgeRejected     Badge = "거부됨"
	BadgeVerified     Badge = "검증됨"
	BadgeFailed       Badge = "실패"
	BadgeUnverifiable Badge = "검증불가"
	BadgeNotRun       Badge = "미실행"
	BadgeExtracted    Badge = "추출"
	BadgeUnknown      Badge = "미상"
)

func (s Semantic) Badge() Badge {
	switch {
	case s.Confirmation == Rejected:
		return BadgeRejected
	case s.Origin == UserStated || s.Confirmation == Confirmed:
		return BadgeConfirmed
	case s.Origin == Adopted:
		return BadgeAdopted
	default:
		return BadgeInferred
	}
}

func (v Verification) Badge() Badge {
	switch v {
	case Passed:
		return BadgeVerified
	case Failed:
		return BadgeFailed
	case Unverifiable:
		return BadgeUnverifiable
	default:
		return BadgeNotRun
	}
}
