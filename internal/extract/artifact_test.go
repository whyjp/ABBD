package extract

import (
	"strings"
	"testing"
)

// A module may say what the build makes of it; a contract may not, and an
// artifact ABBD does not know is refused.
func TestOnlyAModuleIsABuildArtifact(t *testing.T) {
	unit := func(kind, artifact string) string {
		return `{"units":[{"ref":"z","kind":"` + kind + `","name":"libzstd","artifact":"` + artifact + `","anchors":[{"path":"lib"},{"path":"lib/zstd.h"}]}]}`
	}
	r, err := Parse(strings.NewReader(unit("MOD", "shared")))
	if err != nil || r.Units[0].Artifact != "shared" {
		t.Fatalf("a shared module: %v, %+v", err, r.Units)
	}
	for _, c := range [][2]string{{"IF", "shared"}, {"MOD", "dll"}} {
		if _, err := Parse(strings.NewReader(unit(c[0], c[1]))); err == nil {
			t.Errorf("%s with artifact %s was accepted", c[0], c[1])
		}
	}
}
