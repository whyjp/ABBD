package screen

import (
	"testing"

	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/store"
)

func TestABoxSaysWhatItsModuleDoesInAShortPhrase(t *testing.T) {
	fact := func(s string) *model.Fact { return &model.Fact{Value: s} }
	for _, c := range []struct {
		unit model.Unit
		want string
	}{
		{model.Unit{Responsibility: fact("Answers read-only queries over stored units: one unit at a glance.")}, "Answers read-only queries"},
		{model.Unit{Responsibility: fact("Shells out to git for HEAD, files and blobs at a revision.")}, "Shells out to git"},
		{model.Unit{Purpose: fact("so a design unit keeps its ID")}, ""},
		{model.Unit{Responsibility: fact("The pstack plugin: skills that shape how an agent works.")}, ""},
		{model.Unit{Responsibility: fact("Tells the assistant to read the blueprint before it greps the code.")}, ""},
		{model.Unit{Responsibility: fact("Tells the assistant about the blueprint")}, "Tells the assistant"},
		{model.Unit{Responsibility: fact("Decodes the extraction JSON with unknown fields refused")}, ""},
		{model.Unit{Responsibility: fact("Talks")}, ""},
		{model.Unit{}, ""},
	} {
		if got := gist(c.unit, 180); got != c.want {
			t.Errorf("gist of %+v = %q, want %q", c.unit, got, c.want)
		}
	}
}

// With overview_sentences on, a phrase is cut to sentenceRunes characters,
// and an overview box widens for its phrase, never past boxMax, so more
// boxes say what they do; with it off the overview is as it was.
func TestOverviewSentencesWidenBoxesForTheirPhrase(t *testing.T) {
	fact := func(s string) *model.Fact { return &model.Fact{Value: s} }
	long := model.Unit{Responsibility: fact("Settles all open charges for the ledger, nightly.")}
	if got := gistWithin(long, boxMax-16, sentenceRunes); len([]rune(got)) > sentenceRunes || got == "" {
		t.Errorf("gist within %d characters = %q", sentenceRunes, got)
	}
	units := []model.Unit{
		called("MOD-0001", "api", rel("MOD-0002", model.DependsOn)),
		called("MOD-0002", "core", rel("MOD-0003", model.DependsOn)),
		called("MOD-0003", "store"),
		called("MOD-0004", "cli", rel("MOD-0001", model.DependsOn)),
	}
	units[1].Responsibility = fact("Settles all open charges for the ledger, nightly.")
	said := func(m Model) string {
		for _, b := range m.Overview.Boxes {
			if b.ID == "MOD-0002" {
				if b.W > boxMax {
					t.Errorf("box %s is %dpx, past %d", b.ID, b.W, boxMax)
				}
				if b.Sub != nil {
					return b.Sub.S
				}
			}
		}
		return ""
	}
	off := said(Build(Input{Units: units}))
	on := said(Build(Input{Units: units, Project: store.Project{OverviewSentences: true}}))
	if off == on || on != "Settles all open charges" {
		t.Errorf("core's last line reads %q off and %q on", off, on)
	}
}
