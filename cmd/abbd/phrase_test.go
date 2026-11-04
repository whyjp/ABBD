package main

import (
	"strings"
	"testing"
)

// The script reads that the ledger calls billing. A reader names what
// crosses that edge with --phrase, through the same record command, and the
// blueprint draws the phrase beside the line.
func TestAReaderNamesWhatCrossesAnEdgeTheScriptRead(t *testing.T) {
	p := semanticProject(t)
	p.extract(strings.Replace(ledgerTopo, "\n]}", `],"relations":[{"from":"ledger","to":"billing","kind":"depends_on"}]}`, 1))
	p.resync()
	edge := []string{"record", "relation", "MOD-0002", "depends_on", "MOD-0001", "--via", "call", "--evidence", "ledger/book.go:6:func Post()"}
	p.want(1, "already EXTRACTED from the code; name what crosses it with --phrase", edge...)
	p.want(1, "at most 24 characters", append(edge, "--phrase", "posts every charge to the ledger book")...)
	p.want(1, "at most 4 words", append(edge, "--phrase", "a b c d e")...)
	p.want(1, "only repeats the via", append(edge, "--phrase", "호출")...)
	p.want(0, `"charge post" names what crosses it`, append(edge, "--phrase", "charge post")...)

	contains(t, p.want(0, "", "explain", "MOD-0002"), `MOD-0001 billing (extracted; phrase "charge post", inferred)`)
	if page := p.read("blueprint/index.html"); !strings.Contains(page, ">charge post</text>") {
		t.Errorf("the overview does not draw the phrase beside the line")
	} else if !strings.Contains(page, "건너는 것: charge post (판독)") {
		t.Errorf("the line's tooltip does not say the phrase is a reader's")
	}
	if g := p.read("blueprint/graph.json"); !strings.Contains(g, `"Phrase": "charge post",`) || !strings.Contains(g, `"PhraseOrigin": "INFERRED"`) {
		t.Errorf("graph.json does not give the phrase its own origin:\n%s", g)
	}
	contains(t, p.want(0, "", "evidence"), `live MOD-0002.depends_on.MOD-0001.phrase "charge post"`)

	p.want(0, "retracted", "retract", "MOD-0002.depends_on.MOD-0001", "--reason", "names the wrong call")
	if page := p.read("blueprint/index.html"); strings.Contains(page, "charge post") {
		t.Errorf("a retracted phrase is still drawn")
	}
	contains(t, p.want(0, "", "explain", "MOD-0002"), "MOD-0001 billing (extracted)\n")
}

// The supplement may give a phrase, even on an edge the script reads, and
// the extraction script may not.
func TestOnlyTheSupplementGivesAPhrase(t *testing.T) {
	p := semanticProject(t)
	topo := strings.Replace(ledgerTopo, "\n]}", `],"relations":[{"from":"ledger","to":"billing","kind":"depends_on"}]}`, 1)
	p.extract(strings.Replace(topo, `"kind":"depends_on"}`, `"kind":"depends_on","phrase":"charge post"}`, 1))
	p.want(1, "a phrase is a reader's words", "sync")
	p.extract(topo)
	p.resync()
	sup := strings.Replace(chargesTable, `"relations":[`, `"relations":[
 {"from":"l","to":"b","kind":"depends_on","via":"call","phrase":"charge post","evidence":[{"path":"ledger/book.go","lines":"6","quote":"func Post()"}]},`, 1)
	p.setup(0, "synced", "--supplement", p.file(t, "phrased.json", sup))
	contains(t, p.want(0, "", "explain", "MOD-0002"), `MOD-0001 billing (extracted; phrase "charge post", inferred)`)
	contains(t, p.want(0, "", "explain", "MOD-0002"), `consumes DATA-0001 charges table`)
}

// A human retracts a reader's coupling. The reader records it again with a
// phrase, and the coupling stays retracted, because a phrase only relabels
// a line.
func TestAPhraseDoesNotReviveARetractedCoupling(t *testing.T) {
	p := semanticProject(t)
	edge := []string{"record", "relation", "MOD-0002", "depends_on", "MOD-0001", "--via", "call", "--evidence", "ledger/book.go:6:func Post()"}
	p.want(0, "INFERRED", edge...)
	p.want(0, "retracted", "retract", "MOD-0002.depends_on.MOD-0001", "--reason", "ledger never calls billing", "--source", "conversation 2026-11-04")
	p.want(1, "a human retracted it", append(edge, "--phrase", "charge post")...)
	if out := p.want(0, "", "explain", "MOD-0002"); strings.Contains(out, "MOD-0001 billing") {
		t.Errorf("the retracted coupling is back:\n%s", out)
	}
}

// The script reads that the ledger calls billing, and a reader names what
// crosses it. When the script stops reading the edge, the phrase does not
// become a coupling. It names no line, and the check says so until a reader
// retracts it.
func TestAPhraseOnAnEdgeTheScriptDropsDangles(t *testing.T) {
	p := semanticProject(t)
	p.extract(strings.Replace(ledgerTopo, "\n]}", `],"relations":[{"from":"ledger","to":"billing","kind":"depends_on"}]}`, 1))
	p.resync()
	p.want(0, "names what crosses it", "record", "relation", "MOD-0002", "depends_on", "MOD-0001", "--via", "call", "--phrase", "charge post", "--evidence", "ledger/book.go:6:func Post()")
	p.extract(ledgerTopo)
	p.resync()
	if out := p.want(0, "", "explain", "MOD-0002"); strings.Contains(out, "MOD-0001 billing") {
		t.Errorf("the phrase became a coupling:\n%s", out)
	}
	p.want(1, "dangling: MOD-0002.depends_on.MOD-0001.phrase", "status", "--check")
	p.want(0, "retracted", "retract", "MOD-0002.depends_on.MOD-0001.phrase", "--reason", "the call is gone")
	if out := p.want(0, "", "status"); strings.Contains(out, "dangling") {
		t.Errorf("a retracted phrase still dangles:\n%s", out)
	}
}

// While the code a phrase quotes has drifted, explain still lists the
// phrase, marked drifted, and the blueprint does not draw it.
func TestADriftedPhraseShowsAsDrifted(t *testing.T) {
	p := semanticProject(t)
	p.extract(strings.Replace(ledgerTopo, "\n]}", `],"relations":[{"from":"ledger","to":"billing","kind":"depends_on"}]}`, 1))
	p.resync()
	p.want(0, "", "record", "relation", "MOD-0002", "depends_on", "MOD-0001", "--via", "call", "--phrase", "charge post", "--evidence", "ledger/book.go:6:func Post()")
	p.code("ledger/book.go", "package ledger\n\nconst q = `SELECT amount\n    FROM charges`\n\nfunc PostAll() {}\n")
	p.resync()
	contains(t, p.want(0, "", "explain", "MOD-0002"), `MOD-0001 billing (extracted; phrase "charge post", inferred, drifted)`)
	if page := p.read("blueprint/index.html"); strings.Contains(page, ">charge post</text>") {
		t.Errorf("a drifted phrase is drawn")
	}
}
