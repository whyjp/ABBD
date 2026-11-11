package topology

import (
	"slices"
	"strings"
	"testing"

	"github.com/whyjp/abbd/internal/model"
)

// A change of what the build makes of a module is a change a reviewer
// weighs: diff names a library that turned static, one that became an
// artifact and one that stopped being one.
func TestDiffNamesAnArtifactChange(t *testing.T) {
	mod := func(id, name, artifact string) model.Unit {
		u := model.Unit{ID: model.ID(id), Kind: model.Module, Name: model.Said(model.Script, name), Anchors: []model.Anchor{{Path: name}}}
		if artifact != "" {
			f := model.Said(model.Script, artifact)
			u.Artifact = &f
		}
		return u
	}
	base := []model.Unit{mod("MOD-0001", "libzstd", "shared"), mod("MOD-0002", "cli", ""), mod("MOD-0003", "plugin", "module")}
	now := []model.Unit{mod("MOD-0001", "libzstd", "static"), mod("MOD-0002", "cli", "exe"), mod("MOD-0003", "plugin", "")}
	lines := Compare(base, now).Lines()
	for _, want := range []string{
		`~ artifact MOD-0001 libzstd: "shared" -> "static"`,
		`~ artifact MOD-0002 cli: "" -> "exe"`,
		`~ artifact MOD-0003 plugin: "module" -> ""`,
	} {
		if !slices.ContainsFunc(lines, func(l string) bool { return strings.HasPrefix(l, want) }) {
			t.Errorf("diff lacks %q:\n%s", want, strings.Join(lines, "\n"))
		}
	}
}
