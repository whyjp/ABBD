package main

import (
	"strings"
	"testing"
)

// A step of the charge flow that the charges table, inside billing, runs.
const tableStepFlow = `---
question: How does a charge reach the ledger?
steps:
  - unit: DATA-0001
    does: the charge row lands in the charges table
    evidence:
      - {path: billing/charge.go, lines: "3", quote: "INSERT INTO charges"}
  - unit: MOD-0002
    does: Post() reads the charge into the book
    evidence:
      - {path: ledger/book.go, lines: "6", symbol: Post, quote: "func Post() {}"}
---
`

func (p *project) toEnglish() {
	p.t.Helper()
	p.code(".abbd/project.yaml", strings.Replace(p.read("project.yaml"), "language: ko", "language: en", 1))
}

func TestExplainSaysWhereAUnitSitsInAStoredFlow(t *testing.T) {
	t.Parallel()
	p := notesProject(t)
	p.flow(0, "stored flow charge", chargeFlow)
	p.setLanguage("ko")
	p.want(0, "\n흐름: charge 3/3단계; 앞: MOD-0001 billing\n", "explain", "MOD-0002")
	p.want(0, "\n- 흐름: charge 1·2/3단계; 뒤: MOD-0002 ledger\n", "brief", "MOD-0001")

	p.toEnglish()
	p.want(0, "\nflow: charge steps 1, 2 of 3; after: MOD-0002 ledger\n", "explain", "MOD-0001")
}

func TestAModuleSaysWhichStepsItsInsideUnitsRun(t *testing.T) {
	t.Parallel()
	p := notesProject(t)
	p.flow(0, "stored flow charge", tableStepFlow)
	p.setLanguage("ko")
	p.want(0, "\n흐름: charge 1/2단계 (1단계 DATA-0001 charges table); 뒤: MOD-0002 ledger\n", "explain", "MOD-0001")
	p.want(0, "\n흐름: charge 1/2단계; 뒤: MOD-0002 ledger\n", "explain", "DATA-0001")
}

// With no stored flow, the data relations place a unit: billing writes the
// charges table and the ledger reads it.
func TestExplainReadsAFlowFromDataRelationsWhenNoneIsStored(t *testing.T) {
	t.Parallel()
	p := semanticProject(t)
	p.setLanguage("ko")
	p.want(0, "\n데이터 흐름: 앞: MOD-0001 billing; 뒤: MOD-0002 ledger\n", "explain", "DATA-0001")
	p.want(0, "\n데이터 흐름: 앞: DATA-0001 charges table\n", "explain", "MOD-0002")
	p.toEnglish()
	p.want(0, "\ndata flow: before: DATA-0001 charges table\n", "explain", "ledger")
}

func TestAUnitInNoFlowPrintsNoFlowLine(t *testing.T) {
	t.Parallel()
	p, exe := bare(t)
	p.code("billing/charge.go", "package billing\n\nfunc Charge() {}\n")
	p.code("ledger/book.go", "package ledger\n\nfunc Post() {}\n")
	p.extract(ledgerTopo)
	p.setup(0, "synced", "--", exe)
	p.setLanguage("ko")
	for _, args := range [][]string{{"explain", "MOD-0001"}, {"brief", "MOD-0001"}} {
		if out := p.want(0, "", args...); strings.Contains(out, "흐름") || strings.Contains(out, "primary flow") {
			t.Errorf("abbd %v names a flow for a unit in none:\n%s", args, out)
		}
	}
}
