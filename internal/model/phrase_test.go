package model

import (
	"strings"
	"testing"
)

// A reader's phrase names what crosses an edge the script read. It is a
// note beside the script's claim, never a claim, so the script's claim
// still shows and the note's evidence is checked while it names the line.
func TestAReadersPhraseNamesAnEdgeTheScriptRead(t *testing.T) {
	u := mod("MOD-0001")
	u.PutRelation(Relation{To: "MOD-0002", Kind: DependsOn, Writer: Script})
	u.PutNote(PhraseNote{To: "MOD-0002", Kind: DependsOn, Writer: Reader, Text: "schema fetch", Evidence: cite})
	if len(u.Claims()) != 1 || u.Relations[0].Writer != Script {
		t.Fatalf("the phrase made a claim: %+v", u.Claims())
	}
	if n, ok := u.Note(DependsOn, "MOD-0002"); !ok || n.Text != "schema fetch" || n.Writer.Origin() != Inferred {
		t.Errorf("note %+v, want the reader's INFERRED phrase", n)
	}
	var seen []Writer
	u.EachShownEvidence(func(w Writer, _ *Evidence) { seen = append(seen, w) })
	if len(seen) != 1 || seen[0] != Reader {
		t.Errorf("the phrase's evidence is not checked: %v", seen)
	}
	if err := u.Validate(); err != nil {
		t.Error(err)
	}
}

// When nothing shows on the edge any more, its note does not take the
// edge's place. It dangles, and its evidence is not checked.
func TestAPhraseWithoutALineDangles(t *testing.T) {
	u := mod("MOD-0001")
	u.PutNote(PhraseNote{To: "MOD-0002", Kind: DependsOn, Writer: Reader, Text: "schema fetch", Evidence: cite})
	units := []Unit{u, mod("MOD-0002")}
	Resolve(units)
	if len(units[0].Relations) != 0 {
		t.Errorf("the note became a claim: %+v", units[0].Relations)
	}
	if d := Dangling(units); len(d) != 1 || d[0] != "MOD-0001.depends_on.MOD-0002" {
		t.Errorf("dangling %v", d)
	}
	n := 0
	units[0].EachShownEvidence(func(Writer, *Evidence) { n++ })
	if n != 0 {
		t.Errorf("a dangling phrase's evidence counts as shown")
	}
	units[0].PutRelation(Relation{To: "MOD-0002", Kind: DependsOn, Writer: Script})
	if d := Dangling(units); len(d) != 0 {
		t.Errorf("a phrase on a shown line dangles: %v", d)
	}
}

// A mask binds a relation by its kind, its far end and its via only, so a
// mask stored before phrases existed still hides the claim.
func TestARelationMaskIgnoresPhrases(t *testing.T) {
	r := Relation{To: "MOD-0002", Kind: DependsOn, Writer: Supplement, Via: "call", Evidence: cite}
	if RelationDigest(r) != Digest("depends_on", "MOD-0002", "call") {
		t.Fatalf("the relation digest changed")
	}
	u := mod("MOD-0001")
	u.PutRelation(r)
	u.PutNote(PhraseNote{To: "MOD-0002", Kind: DependsOn, Writer: Supplement, Text: "Cypher", Evidence: cite})
	u.PutMask(Mask{Target: RelationTarget(r.Kind, r.To), Writer: Human, Digest: RelationDigest(r), Reason: "wrong", Source: "conversation"})
	u.resolve()
	if len(u.Relations) != 0 {
		t.Errorf("a phrase on the edge lifts the mask: %+v", u.Relations)
	}
}

// The note that names an edge is the first by status and then the table. A
// drifted one is still found, so explain can say it drifted. A dead or a
// masked one names nothing, and a changed one shows again.
func TestWhichPhraseNamesAnEdge(t *testing.T) {
	u := mod("MOD-0001")
	u.PutRelation(Relation{To: "MOD-0002", Kind: DependsOn, Writer: Script})
	u.PutNote(PhraseNote{To: "MOD-0002", Kind: DependsOn, Writer: Supplement, Text: "Cypher", Evidence: cite})
	u.PutNote(PhraseNote{To: "MOD-0002", Kind: DependsOn, Writer: Human, Text: "sample SQL", Source: "conversation"})
	if n, _ := u.Note(DependsOn, "MOD-0002"); n.Writer != Human {
		t.Errorf("the %s's phrase outranks the human's", n.Writer)
	}
	u.DropNotes(func(n PhraseNote) bool { return n.Writer == Human })
	u.PutNote(PhraseNote{To: "MOD-0002", Kind: DependsOn, Writer: Reader, Text: "graph query", Evidence: []Evidence{{Path: "a.go", Quote: "x", Status: Drifted, Since: "r1"}}})
	if n, _ := u.Note(DependsOn, "MOD-0002"); n.Writer != Supplement {
		t.Errorf("a drifted reader's phrase outranks a live supplement's")
	}
	u.DropNotes(func(n PhraseNote) bool { return n.Writer == Supplement })
	if n, ok := u.Note(DependsOn, "MOD-0002"); !ok || n.Status() != Drifted {
		t.Errorf("a drifted phrase is not found")
	}
	u.Phrases[0].Evidence[0].Status = Dead
	if _, ok := u.Note(DependsOn, "MOD-0002"); ok {
		t.Error("a dead phrase names the edge")
	}
	n := PhraseNote{To: "MOD-0002", Kind: DependsOn, Writer: Supplement, Text: "Cypher", Evidence: cite}
	u.PutNote(n)
	u.PutMask(Mask{Target: PhraseTarget(n.Kind, n.To), Writer: Reader, Digest: PhraseDigest(n), Reason: "wrong"})
	if _, ok := u.Note(DependsOn, "MOD-0002"); ok {
		t.Error("a masked phrase names the edge")
	}
	n.Text = "샘플 조회 SQL"
	u.PutNote(n)
	if got, _ := u.Note(DependsOn, "MOD-0002"); got.Text != "샘플 조회 SQL" {
		t.Errorf("a changed phrase stays masked: %q", got.Text)
	}
}

// A phrase is a few words on one line, says more than its via, and is
// never the script's.
func TestAPhraseIsShortAndNotTheScripts(t *testing.T) {
	for _, tc := range []struct {
		phrase string
		via    Via
		bad    string
	}{
		{"NDJSON /ingest push", "data", ""},
		{"IsReady 조회 (health)", "call", ""},
		{"Unit, Relation", "import", ""},
		{strings.Repeat("x", PhraseMax), "", ""},
		{strings.Repeat("x", PhraseMax+1), "", "characters"},
		{"a b c d e", "", "words"},
		{" Cypher", "", "one line"},
		{"two\nlines", "", "one line"},
		{"call", "call", "repeats the via"},
		{"호출", "call", "repeats the via"},
		{"model types", "import", "repeats the via"},
		{"Model Types", "import", "repeats the via"},
		{"function call", "call", "repeats the via"},
	} {
		err := ValidatePhrase(tc.phrase, tc.via)
		switch {
		case tc.bad == "" && err != nil:
			t.Errorf("%q refused: %v", tc.phrase, err)
		case tc.bad != "" && (err == nil || !strings.Contains(err.Error(), tc.bad)):
			t.Errorf("%q: got %v, want it refused for %s", tc.phrase, err, tc.bad)
		}
	}
	u := mod("MOD-0001")
	u.Phrases = []PhraseNote{{To: "MOD-0002", Kind: DependsOn, Writer: Script, Text: "HTTP /api/v1"}}
	if err := u.Validate(); err == nil || !strings.Contains(err.Error(), "phrase") {
		t.Errorf("the script's phrase is accepted: %v", err)
	}
}
