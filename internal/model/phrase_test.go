package model

import (
	"strings"
	"testing"
)

// A reader's phrase names what crosses an edge the script read: the
// script's claim still shows, and the reader's hidden claim gives the
// phrase while its evidence is live.
func TestAReadersPhraseNamesAnEdgeTheScriptRead(t *testing.T) {
	u := mod("MOD-0001")
	u.PutRelation(Relation{To: "MOD-0002", Kind: DependsOn, Writer: Script})
	u.PutRelation(Relation{To: "MOD-0002", Kind: DependsOn, Writer: Reader, Via: "call", Phrase: "schema fetch", Evidence: cite})
	if len(u.Relations) != 1 || u.Relations[0].Writer != Script {
		t.Fatalf("the script's claim does not show: %+v", u.Relations)
	}
	if got := u.Phrase(DependsOn, "MOD-0002"); got != "schema fetch" {
		t.Errorf("phrase %q, want the reader's", got)
	}
	if n := len(u.Shown()); n != 2 {
		t.Errorf("Shown lists %d claims, want the script's and the phrase's", n)
	}
	var seen []Writer
	u.EachShownEvidence(func(w Writer, _ *Evidence) { seen = append(seen, w) })
	if len(seen) != 1 || seen[0] != Reader {
		t.Errorf("the phrase's evidence is not checked: %v", seen)
	}
}

// A phrase whose cited code changed is not drawn until a reader refreshes
// it, a dead one names nothing, and a mask hides it.
func TestAPhraseShowsOnlyWhileItsCodeHolds(t *testing.T) {
	claim := func(es []Evidence) Unit {
		u := mod("MOD-0001")
		u.PutRelation(Relation{To: "MOD-0002", Kind: DependsOn, Writer: Script})
		u.PutRelation(Relation{To: "MOD-0002", Kind: DependsOn, Writer: Reader, Via: "call", Phrase: "Cypher", Evidence: es})
		return u
	}
	drifted := claim([]Evidence{{Path: "a.go", Quote: "x", Status: Drifted, Since: "r1"}})
	if p := drifted.Phrase(DependsOn, "MOD-0002"); p != "" {
		t.Errorf("a drifted phrase is drawn: %q", p)
	}
	if _, ok := drifted.PhraseClaim(DependsOn, "MOD-0002"); !ok {
		t.Error("a drifted phrase's claim is not found, so its evidence is not checked")
	}
	dead := claim([]Evidence{{Path: "a.go", Quote: "x", Status: Dead, Reason: "gone"}})
	if _, ok := dead.PhraseClaim(DependsOn, "MOD-0002"); ok {
		t.Error("a dead phrase names the edge")
	}
	u := mod("MOD-0001")
	r := Relation{To: "MOD-0002", Kind: DependsOn, Writer: Supplement, Via: "call", Phrase: "Cypher", Evidence: cite}
	u.PutRelation(r)
	u.PutMask(Mask{Target: RelationTarget(r.Kind, r.To), Writer: Reader, Digest: RelationDigest(r), Reason: "wrong"})
	u.resolve()
	if p := u.Phrase(DependsOn, "MOD-0002"); p != "" {
		t.Errorf("a masked phrase is drawn: %q", p)
	}
	r.Phrase = "샘플 조회 SQL"
	u.PutRelation(r)
	if p := u.Phrase(DependsOn, "MOD-0002"); p != "샘플 조회 SQL" {
		t.Errorf("a changed phrase stays masked: %q", p)
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
		{strings.Repeat("x", PhraseMax), "", ""},
		{strings.Repeat("x", PhraseMax+1), "", "characters"},
		{"a b c d e", "", "words"},
		{" Cypher", "", "one line"},
		{"two\nlines", "", "one line"},
		{"call", "call", "repeats the via"},
		{"호출", "call", "repeats the via"},
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
	u.Relations = []Relation{{To: "MOD-0002", Kind: DependsOn, Writer: Script, Phrase: "HTTP /api/v1"}}
	if err := u.Validate(); err == nil || !strings.Contains(err.Error(), "phrase") {
		t.Errorf("the script's phrase is accepted: %v", err)
	}
}
