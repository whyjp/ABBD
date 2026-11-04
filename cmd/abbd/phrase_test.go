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

	contains(t, p.want(0, "", "explain", "MOD-0002"), `MOD-0001 billing (extracted, "charge post")`)
	if page := p.read("blueprint/index.html"); !strings.Contains(page, ">charge post</text>") {
		t.Errorf("the overview does not draw the phrase beside the line")
	}
	contains(t, p.want(0, "", "evidence"), "live MOD-0002.depends_on.MOD-0001 via call")

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
	contains(t, p.want(0, "", "explain", "MOD-0002"), `MOD-0001 billing (extracted, "charge post")`)
	contains(t, p.want(0, "", "explain", "MOD-0002"), `consumes DATA-0001 charges table`)
}
