package store

import (
	"slices"
	"testing"
)

// The last matching rule names a path's owners, as GitHub reads the file.
func TestTheLastMatchingRuleOwnsAPath(t *testing.T) {
	o := ParseOwners("# owners\n* @all\n/billing/ @pay @ops\n*.md @docs\ninternal/store/ @store\n")
	for path, want := range map[string][]string{
		"main.go":                   {"@all"},
		"billing/charge.go":         {"@pay", "@ops"},
		"billing/README.md":         {"@docs"},
		"internal/store/store.go":   {"@store"},
		"x/internal/store/store.go": {"@all"},
	} {
		if got := o.For(path); !slices.Equal(got, want) {
			t.Errorf("%s: got %v, want %v", path, got, want)
		}
	}
	if ParseOwners("").For("a.go") != nil {
		t.Errorf("no CODEOWNERS owns nothing")
	}
}
