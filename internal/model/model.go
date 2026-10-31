package model

import (
	"fmt"
	"regexp"
	"sort"
)

type Origin string

const (
	UserStated Origin = "USER_STATED"
	Adopted    Origin = "ADOPTED"
	Extracted  Origin = "EXTRACTED"
	Observed   Origin = "OBSERVED"
	Inferred   Origin = "INFERRED"
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

var origins = map[Origin]bool{UserStated: true, Adopted: true, Extracted: true, Observed: true, Inferred: true}
var verifications = map[Verification]bool{"": true, NotRun: true, Passed: true, Failed: true, Unverifiable: true}
var confirmations = map[Confirmation]bool{"": true, Unconfirmed: true, Confirmed: true, Rejected: true}

// Fact is one field value with its evidence recorded on three independent axes.
type Fact struct {
	Value        string       `yaml:"value"`
	Origin       Origin       `yaml:"origin"`
	Verification Verification `yaml:"verification,omitempty"`
	Confirmation Confirmation `yaml:"confirmation,omitempty"`
}

// A why-field states intent, which code can never prove.
func (f Fact) validate(why bool) error {
	if !origins[f.Origin] {
		return fmt.Errorf("unknown origin %q", f.Origin)
	}
	if !verifications[f.Verification] {
		return fmt.Errorf("unknown verification %q", f.Verification)
	}
	if !confirmations[f.Confirmation] {
		return fmt.Errorf("unknown confirmation %q", f.Confirmation)
	}
	if why && f.Origin != UserStated && f.Origin != Adopted && f.Origin != Inferred {
		return fmt.Errorf("why-field origin must be USER_STATED, ADOPTED or INFERRED, got %s", f.Origin)
	}
	return nil
}

type Kind string

const (
	Module    Kind = "MOD"
	Interface Kind = "IF"
	Event     Kind = "EVT"
	Data      Kind = "DATA"
	Config    Kind = "CFG"
	Feature   Kind = "FEAT"
)

var kinds = map[Kind]string{
	Module: "architecture", Interface: "architecture", Event: "architecture",
	Data: "architecture", Config: "architecture", Feature: "features",
}

// Domain-layer prefixes are reserved and not part of this restart.
var reserved = map[Kind]bool{"DOM": true, "STATE": true, "TRN": true, "INV": true}

func (k Kind) Dir() string { return kinds[k] }

func (k Kind) Validate() error {
	if reserved[k] {
		return fmt.Errorf("kind %s is reserved for the domain layer", k)
	}
	if _, ok := kinds[k]; !ok {
		return fmt.Errorf("unknown kind %q", k)
	}
	return nil
}

type Anchor struct {
	Path   string `yaml:"path" json:"path"`
	Symbol string `yaml:"symbol,omitempty" json:"symbol,omitempty"`
}

func (a Anchor) Key() string { return a.Path + "#" + a.Symbol }

func SortAnchors(as []Anchor) []Anchor {
	seen := map[string]bool{}
	out := make([]Anchor, 0, len(as))
	for _, a := range as {
		if !seen[a.Key()] {
			seen[a.Key()] = true
			out = append(out, a)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key() < out[j].Key() })
	return out
}

type ID string

var idPattern = regexp.MustCompile(`^([A-Z]+)-(\d{4,})$`)

func ParseID(s string) (Kind, int, error) {
	m := idPattern.FindStringSubmatch(s)
	if m == nil {
		return "", 0, fmt.Errorf("malformed id %q", s)
	}
	var n int
	fmt.Sscanf(m[2], "%d", &n)
	return Kind(m[1]), n, nil
}

func FormatID(k Kind, n int) ID { return ID(fmt.Sprintf("%s-%04d", k, n)) }

// Unit is one design unit: a module, an external contract, or a feature.
type Unit struct {
	ID             ID       `yaml:"id"`
	Kind           Kind     `yaml:"kind"`
	Name           Fact     `yaml:"name"`
	Anchors        []Anchor `yaml:"anchors"`
	Purpose        *Fact    `yaml:"purpose,omitempty"`
	Responsibility *Fact    `yaml:"responsibility,omitempty"`
	// Notes is the user-owned "## Notes" section, carried through untouched.
	Notes string `yaml:"-"`
}

func (u Unit) Validate() error {
	if err := u.Kind.Validate(); err != nil {
		return err
	}
	k, _, err := ParseID(string(u.ID))
	if err != nil {
		return err
	}
	if k != u.Kind {
		return fmt.Errorf("%s: id prefix does not match kind %s", u.ID, u.Kind)
	}
	if len(u.Anchors) == 0 {
		return fmt.Errorf("%s: a unit needs at least one anchor", u.ID)
	}
	if err := u.Name.validate(false); err != nil {
		return fmt.Errorf("%s name: %w", u.ID, err)
	}
	for label, f := range map[string]*Fact{"purpose": u.Purpose, "responsibility": u.Responsibility} {
		if f != nil {
			if err := f.validate(true); err != nil {
				return fmt.Errorf("%s %s: %w", u.ID, label, err)
			}
		}
	}
	return nil
}
