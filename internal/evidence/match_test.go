package evidence

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/whyjp/abbd/internal/model"
)

func reviewTree(t *testing.T, content string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

// A quote matches inside a longer identifier, so the code can stop holding
// the coupling while its evidence stays live.
func TestQuoteMustEndWhereTheWordEnds(t *testing.T) {
	c := New(reviewTree(t, "package a\n\nconst q = `SELECT amount FROM charges_archive`\n"))
	r := c.Check(model.Evidence{Path: "a.go", Lines: "3", Quote: "SELECT amount FROM charges"}, nil)
	if r.Live {
		t.Errorf("quote of table charges is live against charges_archive (At=%s)", r.At)
	}
}

// Whitespace-insensitive only between tokens that already had whitespace: a
// formatter that adds spaces around an operator kills the evidence.
func TestFormattingNeverKillsAQuote(t *testing.T) {
	c := New(reviewTree(t, "package a\n\nfunc f() { if x != y { g() } }\n"))
	r := c.Check(model.Evidence{Path: "a.go", Lines: "3", Quote: "if x!=y {"}, nil)
	if !r.Live {
		t.Errorf("quote %q is dead after gofmt-style spacing", "if x!=y {")
	}
}

// Apply never moves Lines to where the quote was found, so drift within the
// slack accumulates across syncs until the evidence dies.
func TestApplyMovesLinesToWhereTheQuoteIs(t *testing.T) {
	root := reviewTree(t, "package a\n\n\n\n\nfunc Charge() {}\n")
	rel := model.Relation{To: "MOD-0002", Kind: model.DependsOn, Origin: model.Inferred,
		Evidence: []model.Evidence{{Path: "a.go", Lines: "3", Quote: "func Charge() {}"}}}
	u := model.Unit{ID: "MOD-0001", Kind: model.Module, Name: model.Fact{Value: "a", Origin: model.Extracted}, Relations: []model.Relation{rel}}
	out := New(root).Apply([]model.Unit{u})[0]
	if out.Relations[0].Dead {
		t.Fatal("quote at line 6 should be live for lines 3 within slack 3")
	}
	if got := out.Relations[0].Evidence[0].Lines; got != "6" {
		t.Errorf("Apply kept Lines %q though the quote is at 6; the next 1-line drift kills it", got)
	}
}
