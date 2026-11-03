package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// statesCommented is billing's retry rule with a comment two lines above the
// states quote reworded: the note's words still hold, but its lines moved.
func statesCommented() string {
	return strings.Replace(retryCode("const tries = 3", "var states = []string{\"pending\", \"charged\"}"), "// line 23\n", "// line twenty-three\n", 1)
}

// A comment edit near a note's quote stales it. A reader who read the code
// again and found the note still right stamps it again, naming who reread it, and the note
// keeps its words, its quotes and the human's why, and says who re-read it
// and when, until a reader stores it with new words.
func TestAReaderKeepsANoteAfterACosmeticEdit(t *testing.T) {
	p := notesProject(t)
	p.wantNote(0, "understood", "MOD-0001.decisions.k", "The states run pending then charged.", statesQuote)
	p.want(0, "recorded", "record", "fact", "MOD-0001.decisions.k", "--value", "refunds come later", "--source", "https://example.com/pr/7")
	p.want(1, "MOD-0001.decisions.k is current; nothing to stamp again", "understand", "MOD-0001.decisions.k", "--reread-by", "claude")

	p.code("billing/retry.go", statesCommented())
	p.resync()
	p.want(1, "1 note(s) STALE", "status", "--check")
	p.wantNote(1, "read it again", "MOD-0001.decisions.k", "The states run pending then charged.", statesQuote)
	p.want(1, "pass neither a FILE nor --evidence", "understand", "MOD-0001.decisions.k", p.file(t, "n.md", "x"), "--reread-by", "claude")
	p.want(1, "pass neither a FILE nor --evidence", "understand", "MOD-0001.decisions.k", "--evidence", statesQuote, "--reread-by", "claude")
	p.want(1, "names who read the code", "understand", "MOD-0001.decisions.k", "--reread-by", " ")
	p.want(1, "is for a slot note", "understand", "MOD-0001", "--reread-by", "claude")
	p.want(1, "there is no note MOD-0001.failure", "understand", "MOD-0001.failure", "--reread-by", "claude")

	p.want(0, "stamped MOD-0001.decisions.k billing again against the code it cites", "understand", "MOD-0001.decisions.k", "--reread-by", "claude")
	p.want(0, "notes: 1 slot note(s), 0 STALE", "status", "--check")
	out := p.want(0, "decisions.k (INFERRED, CURRENT)", "explain", "MOD-0001")
	for _, want := range []string{"The states run pending then charged.", "why (USER_STATED, https://example.com/pr/7): refunds come later"} {
		if !strings.Contains(out, want) {
			t.Errorf("explain must still show %q:\n%s", want, out)
		}
	}
	if !regexp.MustCompile(`reread by claude on \d{4}-\d{2}-\d{2}, after billing/retry.go \(the lines around it changed\) moved`).MatchString(out) {
		t.Errorf("explain must say who reread the note, when and after what:\n%s", out)
	}
	if page := p.blueprint(); !strings.Contains(page, "claude이(가)") {
		t.Error("the blueprint must say who reread the note")
	}
	p.want(1, "is current; nothing to stamp again", "understand", "MOD-0001.decisions.k", "--reread-by", "claude")

	p.wantNote(0, "understood", "MOD-0001.decisions.k", "The states run pending, then charged.", statesQuote)
	if out := p.want(0, "decisions.k (INFERRED, CURRENT)", "explain", "MOD-0001"); strings.Contains(out, "reread by") {
		t.Errorf("a note stored with new words is a new reading, not a reread one:\n%s", out)
	}
}

// Stamping a note again keeps the quotes, so it is refused once a quote is not in
// its file exactly once, word for word: the note no longer rests on code
// that is there as it quotes it.
func TestARereadIsRefusedWhenAQuoteNoLongerHolds(t *testing.T) {
	p := notesProject(t)
	p.wantNote(0, "understood", "MOD-0001.failure", "A charge is tried three times.", failureQuote)
	p.wantNote(0, "understood", "MOD-0001.states", "pending then charged.", statesQuote)
	p.code("billing/retry.go", retryCode("const tries = 5", "var states = []string{\"pending\", \"charged\"}")+"var states = []string{\"pending\", \"charged\"}\n")
	p.resync()
	p.want(1, `billing/retry.go no longer holds "const tries = 3" exactly once`, "understand", "MOD-0001.failure", "--reread-by", "claude")
	p.want(1, `no longer holds "var states = []string{\"pending\", \"charged\"}" exactly once`, "understand", "MOD-0001.states", "--reread-by", "claude")
	p.want(1, "2 note(s) STALE", "status", "--check")
}

// abbd forget removes a note by the name status gives it, whatever state it
// is in, says what it removed, and names the human's why that went with it.
func TestForgetRemovesANoteAndSaysWhatItWas(t *testing.T) {
	p := notesProject(t)
	p.wantNote(0, "understood", "MOD-0001.failure", "A charge is tried three times.", failureQuote)
	p.wantNote(0, "understood", "MOD-0001.states", "states text", statesQuote)
	p.wantNote(0, "understood", "MOD-0001.decisions.k", "A charge is tried three times.", failureQuote)
	p.want(0, "recorded", "record", "fact", "MOD-0001.decisions.k", "--value", "the card network bans a fourth try", "--source", "https://example.com/pr/7")
	failure := p.read("understanding/MOD-0001/failure.md")
	write(t, filepath.Join(p.root, ".abbd", "understanding", "MOD-0099", "failure.md"), strings.Replace(failure, "unit: MOD-0001", "unit: MOD-0099", 1))
	write(t, filepath.Join(p.root, ".abbd", "understanding", "MOD-0001", "a.md"), p.read("understanding/MOD-0001/states.md"))
	p.code("billing/retry.go", statesCommented())
	p.resync()
	p.want(1, "notes: 5 slot note(s), 1 STALE, 2 orphaned", "status", "--check")

	p.want(1, "there is no note at .abbd/understanding/MOD-0001/semantics.md", "forget", "MOD-0001.semantics")
	p.want(1, "name the note as ID.SLOT[.KEY]", "forget", "MOD-0001")
	p.want(0, "forgot MOD-0001.a (misfiled, it says it is MOD-0001.states): removed .abbd/understanding/MOD-0001/a.md", "forget", "MOD-0001.a")
	p.want(0, "forgot MOD-0099.failure (orphaned): removed .abbd/understanding/MOD-0099/failure.md", "forget", "MOD-0099.failure")
	p.want(0, "forgot MOD-0001.states (STALE): removed .abbd/understanding/MOD-0001/states.md", "forget", "MOD-0001.states")
	out := p.want(0, "forgot MOD-0001.decisions.k (CURRENT): removed .abbd/understanding/MOD-0001/decisions.k.md", "forget", "MOD-0001.decisions.k")
	if !strings.Contains(out, "the why stated at https://example.com/pr/7 went with it: the card network bans a fourth try") {
		t.Errorf("forget must name the human's why it removed:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(p.root, ".abbd", "understanding", "MOD-0099")); !os.IsNotExist(err) {
		t.Errorf("forget must remove a unit's directory once it holds no note: %v", err)
	}
	p.want(0, "notes: 1 slot note(s), 0 STALE, 0 orphaned", "status", "--check")
	if strings.Contains(p.blueprint(), "fourth try") {
		t.Error("the blueprint must drop a forgotten note")
	}
	p.want(0, "forgot MOD-0001.failure (CURRENT)", "forget", "MOD-0001.failure")
	p.want(1, "there is no note at", "forget", "MOD-0001.failure")
}

// A copy of a note in a directory that is no unit ID, such as
// MOD-0001.decisions/k.md, has no ref that names only its file: status names
// the file to delete instead of a forget that would remove the current note.
func TestForgetNeverNamesTwoFilesByOneRef(t *testing.T) {
	p := notesProject(t)
	p.wantNote(0, "understood", "MOD-0001.decisions.k", "A charge is tried three times.", failureQuote)
	write(t, filepath.Join(p.root, ".abbd", "understanding", "MOD-0001.decisions", "k.md"), p.read("understanding/MOD-0001/decisions.k.md"))
	p.resync()
	out := p.want(0, "delete .abbd/understanding/MOD-0001.decisions/k.md to drop it", "status")
	if strings.Contains(out, "abbd forget MOD-0001.decisions.k") {
		t.Errorf("status must not name a forget that also matches the current note:\n%s", out)
	}
	p.want(0, "forgot MOD-0001.decisions.k (CURRENT): removed .abbd/understanding/MOD-0001/decisions.k.md", "forget", "MOD-0001.decisions.k")
	if _, err := os.Stat(filepath.Join(p.root, ".abbd", "understanding", "MOD-0001.decisions", "k.md")); err != nil {
		t.Errorf("forget must leave the copy it was not named for: %v", err)
	}
}

// A note whose evidence was edited by hand reads STALE because its stored
// code was stamped against other quotes. A reread would bless words and a
// human's why given about the old quote, so it is refused.
func TestARereadIsRefusedWhenTheEvidenceWasEditedByHand(t *testing.T) {
	p := notesProject(t)
	p.wantNote(0, "understood", "MOD-0001.decisions.k", "The states run pending then charged.", statesQuote)
	p.want(0, "recorded", "record", "fact", "MOD-0001.decisions.k", "--value", "refunds come later", "--source", "https://example.com/pr/7")
	f := filepath.Join(p.root, ".abbd", "understanding", "MOD-0001", "decisions.k.md")
	note := p.read("understanding/MOD-0001/decisions.k.md")
	edited := strings.Replace(note, `var states = []string{"pending", "charged"}`, "const tries = 3", 1)
	if edited == note {
		t.Fatalf("the note must quote the states line:\n%s", note)
	}
	write(t, f, edited)
	p.resync()
	p.want(1, "1 note(s) STALE", "status", "--check")
	p.want(1, "store it again with its words", "understand", "MOD-0001.decisions.k", "--reread-by", "claude")
	p.want(1, "1 note(s) STALE", "status", "--check")
}

// A reread records where each quote sits now, as a store with fresh
// --evidence would.
func TestARereadMovesEachQuotesLinesToWhereItIsNow(t *testing.T) {
	p := notesProject(t)
	p.wantNote(0, "understood", "MOD-0001.states", "pending then charged.", statesQuote)
	p.code("billing/retry.go", "// new 1\n// new 2\n"+statesCommented())
	p.resync()
	p.want(1, "1 note(s) STALE", "status", "--check")
	p.want(0, "stamped MOD-0001.states", "understand", "MOD-0001.states", "--reread-by", "claude")
	if note := p.read("understanding/MOD-0001/states.md"); !regexp.MustCompile(`lines: "?27"?\n`).MatchString(note) {
		t.Errorf("the reread note must cite the states line where it sits now, line 27:\n%s", note)
	}
	p.want(0, "notes: 1 slot note(s), 0 STALE", "status", "--check")
}

// The skill and the check's summary send a reader to the commands, not to
// deleting a note's file by hand.
func TestTheSkillAndTheCheckNameTheNoteCommands(t *testing.T) {
	if text := readPlugin(t, "skills", "abbd", "SKILL.md"); strings.Contains(text, "Delete `.abbd/understanding/<ID>/") {
		t.Error("the abbd skill must remove a note with abbd forget, not by deleting its file")
	}
	p := notesProject(t)
	p.wantNote(0, "understood", "MOD-0001.states", "pending then charged.", statesQuote)
	p.code("billing/retry.go", statesCommented())
	p.resync()
	p.want(1, "1 note(s) STALE, read the code each cites again and store it with abbd understand ID.SLOT FILE --evidence, or with abbd understand ID.SLOT --reread-by WHO when it still holds as written, or drop it with abbd forget ID.SLOT", "status", "--check")
}
