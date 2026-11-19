package model

import (
	"strings"
	"testing"
)

// An external library is only ever the far end of a depends_on: it uses,
// provides and runs nothing the diagram draws.
func TestAnExternalLibraryIsOnlyTheFarEndOfADependsOn(t *testing.T) {
	for _, c := range []struct {
		kind     RelationKind
		from, to Kind
		ok       bool
	}{
		{DependsOn, Module, External, true},
		{DependsOn, Feature, External, true},
		{DependsOn, External, Module, false},
		{DependsOn, External, External, false},
		{Consumes, External, Interface, false},
		{Provides, External, Interface, false},
		{RunsOn, External, Exec, false},
		{PostsTo, External, Exec, false},
		{Consumes, Module, External, false},
	} {
		if err := c.kind.Joins(c.from, c.to); (err == nil) != c.ok {
			t.Errorf("%s %s %s: %v, want ok=%v", c.from, c.kind, c.to, err, c.ok)
		}
	}
}

// An external library anchors the manifest line that declares it, and no
// module or feature contains it.
func TestAnExternalLibraryHasNoParent(t *testing.T) {
	u := Unit{ID: "EXT-0001", Kind: External, Name: Said(Script, "serde"), Anchors: []Anchor{{Path: "Cargo.toml", Symbol: "serde"}}}
	if err := u.Validate(); err != nil {
		t.Fatalf("a library anchored at its declaration: %v", err)
	}
	u.Parents = []ParentClaim{{To: "MOD-0001", Writer: Script}}
	if err := u.Validate(); err == nil || !strings.Contains(err.Error(), "outside the repository") {
		t.Errorf("a library inside a module: %v", err)
	}
}
