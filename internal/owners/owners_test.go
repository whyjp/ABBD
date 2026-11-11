package owners

import (
	"slices"
	"testing"
)

// The last matching rule names a path's owners, as GitHub reads the file.
func TestTheLastMatchingRuleOwnsAPath(t *testing.T) {
	o := Parse("# owners\n* @all\n/billing/ @pay @ops\n*.md @docs\ninternal/store/ @store\n")
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
	more := Parse("* @all\n/billing/ @pay\ndocs/* @docs\n**/logs @logs\na/**/b @ab\n")
	for path, want := range map[string][]string{
		"billing":       {"@pay"},
		"docs/x.md":     {"@docs"},
		"docs/sub/y.md": {"@all"},
		"logs/z":        {"@logs"},
		"deep/logs/z":   {"@logs"},
		"a/b":           {"@ab"},
		"a/x/y/b":       {"@ab"},
	} {
		if got := more.For(path); !slices.Equal(got, want) {
			t.Errorf("%s: got %v, want %v", path, got, want)
		}
	}
	if Parse("").For("a.go") != nil {
		t.Errorf("no CODEOWNERS owns nothing")
	}
}
