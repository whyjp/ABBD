package screen

import (
	"testing"

	"github.com/whyjp/abbd/internal/model"
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
