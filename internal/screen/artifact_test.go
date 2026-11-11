package screen

import (
	"strings"
	"testing"

	"github.com/whyjp/abbd/internal/model"
)

// A module the build makes into an artifact says so on its box, after
// what it does, and a module it is not leaves its box as it was.
func TestABoxSaysTheBuildArtifactItsModuleIs(t *testing.T) {
	units := ripgrep()
	exe, shared := model.Said(model.Script, "exe"), model.Said(model.Script, "shared")
	units[0].Artifact, units[3].Artifact = &exe, &shared
	m := Build(Input{Units: units})
	got := map[string]Box{}
	for _, b := range m.Overview.Boxes {
		got[b.ID] = b
	}
	for id, want := range map[string]string{"MOD-0001": "실행 파일", "MOD-0004": "공유 라이브러리", "MOD-0002": ""} {
		b := got[id]
		if b.Artifact != want {
			t.Errorf("%s says artifact %q, want %q", id, b.Artifact, want)
		}
		if want != "" && !strings.Contains(b.Title, "("+want+")") {
			t.Errorf("%s's title %q does not say %q", id, b.Title, want)
		}
		if want != "" && b.Sub != nil && !strings.Contains(b.Sub.S, want) {
			t.Errorf("%s's last line %+v does not say %q", id, b.Sub, want)
		}
	}
	for _, r := range m.Units {
		if r.ID == "MOD-0004" && r.Artifact != "shared" {
			t.Errorf("graph.json row of MOD-0004 says artifact %q, want shared", r.Artifact)
		}
	}
}
