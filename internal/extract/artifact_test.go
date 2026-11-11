package extract

import (
	"strings"
	"testing"
)

// A module may say the set of what the build makes of it, kept sorted; a
// contract may not, and an artifact ABBD does not know or one listed twice
// is refused.
func TestOnlyAModuleIsABuildArtifact(t *testing.T) {
	unit := func(kind, artifacts string) string {
		return `{"units":[{"ref":"z","kind":"` + kind + `","name":"libzstd","artifacts":[` + artifacts + `],"anchors":[{"path":"lib"},{"path":"lib/zstd.h"}]}]}`
	}
	r, err := Parse(strings.NewReader(unit("MOD", `"static","shared"`)))
	if err != nil || r.Units[0].Artifact == nil || r.Units[0].Artifact.Value != "shared,static" {
		t.Fatalf("a static and shared module: %v, %+v", err, r.Units)
	}
	for _, c := range [][2]string{{"IF", `"shared"`}, {"MOD", `"dll"`}, {"MOD", `"static","static"`}} {
		if _, err := Parse(strings.NewReader(unit(c[0], c[1]))); err == nil {
			t.Errorf("%s with artifacts %s was accepted", c[0], c[1])
		}
	}
}
