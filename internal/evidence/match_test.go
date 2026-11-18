package evidence

import (
	"os"
	"path/filepath"
	"strings"
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
	r := c.Check(model.Evidence{Path: "a.go", Lines: "3", Quote: "SELECT amount FROM charges"})
	if r.Live {
		t.Errorf("quote of table charges is live against charges_archive (At=%s)", r.At)
	}
}

// Whitespace-insensitive only between tokens that already had whitespace: a
// formatter that adds spaces around an operator kills the evidence.
func TestFormattingNeverKillsAQuote(t *testing.T) {
	c := New(reviewTree(t, "package a\n\nfunc f() { if x != y { g() } }\n"))
	r := c.Check(model.Evidence{Path: "a.go", Lines: "3", Quote: "if x!=y {"})
	if !r.Live {
		t.Errorf("quote %q is dead after gofmt-style spacing", "if x!=y {")
	}
}

// Apply never moves Lines to where the quote was found, so drift within the
// slack accumulates across syncs until the evidence dies.
func TestApplyMovesLinesToWhereTheQuoteIs(t *testing.T) {
	root := reviewTree(t, "package a\n\n\n\n\nfunc Charge() {}\n")
	rel := model.Relation{To: "MOD-0002", Kind: model.DependsOn, Writer: model.Reader,
		Evidence: []model.Evidence{{Path: "a.go", Lines: "3", Quote: "func Charge() {}"}}}
	u := model.Unit{ID: "MOD-0001", Kind: model.Module, Name: model.Said(model.Script, "a"), Relations: []model.Relation{rel}}
	out := New(root).Apply([]model.Unit{u}, "r1")[0]
	if out.Relations[0].Dead {
		t.Fatal("quote at line 6 should be live for lines 3 within slack 3")
	}
	if got := out.Relations[0].Evidence[0].Lines; got != "6" {
		t.Errorf("Apply kept Lines %q though the quote is at 6; the next 1-line drift kills it", got)
	}
}

// A quote cut in the middle of a word is dead, and its reason says so, so the
// reader knows to cut at a word boundary instead of looking for a missing line.
func TestAQuoteCutInsideAWordSaysSo(t *testing.T) {
	c := New(reviewTree(t, "package a\n\nfunc ChargeCustomer(amount int) {}\n"))
	cut := model.Evidence{Path: "a.go", Lines: "3", Quote: "func ChargeCust"}
	now, _ := c.Status(cut, "r1", false)
	if now.Status != model.Dead || !strings.Contains(now.Reason, "inside a word") {
		t.Errorf("a quote cut inside a word is %q with reason %q", now.Status, now.Reason)
	}
	gone := model.Evidence{Path: "a.go", Lines: "3", Quote: "func Refund("}
	now, _ = c.Status(gone, "r1", false)
	if now.Status != model.Dead || now.Reason != "the script's quote is not in the file" {
		t.Errorf("an absent quote is %q with reason %q", now.Status, now.Reason)
	}
	whole := model.Evidence{Path: "a.go", Lines: "3", Quote: "ChargeCustomer(amount int)"}
	if now, _ = c.Status(whole, "r1", false); now.Status != model.Live {
		t.Errorf("a quote cut at word boundaries is %q: %s", now.Status, now.Reason)
	}
}
