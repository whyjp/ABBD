package main

import (
	"strings"
	"testing"

	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/topology"
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
	p.want(0, "\n흐름: charge 1/2단계 (DATA-0001 charges table 1단계); 뒤: MOD-0002 ledger\n", "explain", "MOD-0001")
	p.want(0, "\n흐름: charge 1/2단계; 뒤: MOD-0002 ledger\n", "explain", "DATA-0001")
}

// With no stored flow, the data relations place a unit: the ledger reads
// the charges table inside billing. Billing provides it, which names who
// stands behind the table and says nothing of which comes first.
func TestExplainReadsHandOffsFromDataRelationsWhenNoFlowIsStored(t *testing.T) {
	t.Parallel()
	p := semanticProject(t)
	p.setLanguage("ko")
	p.want(0, "\n주고받기: 읽는 쪽: MOD-0002 ledger; 제공: MOD-0001 billing\n", "explain", "DATA-0001")
	p.want(0, "\n주고받기: 읽음: DATA-0001 charges table\n", "explain", "MOD-0002")
	p.want(0, "\n주고받기: 읽는 쪽: MOD-0002 ledger\n", "explain", "MOD-0001")
	p.toEnglish()
	p.want(0, "\nhand-offs: reads: DATA-0001 charges table\n", "explain", "ledger")
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
		if out := p.want(0, "", args...); strings.Contains(out, "흐름") || strings.Contains(out, "주고받기") || strings.Contains(out, "primary flow") {
			t.Errorf("abbd %v names a flow for a unit in none:\n%s", args, out)
		}
	}
}

// A module whose inside units run most of a flow, as the root module of a
// Tauri app runs eleven of twelve: each inside unit once with its steps,
// runs of three steps or more as their ends, and every list cut at three.
func TestALongFlowLineGroupsAndCapsItsSteps(t *testing.T) {
	t.Parallel()
	order := []model.ID{"MOD-0057", "MOD-0040", "MOD-0040", "MOD-0046", "MOD-0045", "MOD-0055", "MOD-0055", "MOD-0055", "MOD-0049", "MOD-0045", "MOD-0045", "MOD-0040"}
	p := topology.FlowPosition{Flow: "ts-invoke-to-rust", Of: len(order), Sides: []topology.Side{
		{Role: topology.Before, Units: []model.ID{"MOD-0057", "MOD-0060", "MOD-0061", "MOD-0062", "MOD-0063"}},
	}}
	for i, u := range order[1:] {
		p.Steps = append(p.Steps, topology.FlowStep{N: i + 2, Unit: u})
	}
	name := map[model.ID]string{"MOD-0040": "js bridge", "MOD-0045": "ipc", "MOD-0046": "manager", "MOD-0057": "api"}
	for lang, want := range map[model.Language]string{
		"ko": "흐름: ts-invoke-to-rust 2–12/12단계 (MOD-0040 js bridge 2·3·12단계, MOD-0046 manager 4단계, MOD-0045 ipc 5·10·11단계 외 2개); 앞: MOD-0057 api, MOD-0060, MOD-0061 외 2개",
		"en": "flow: ts-invoke-to-rust steps 2–12 of 12 (MOD-0040 js bridge steps 2, 3, 12, MOD-0046 manager step 4, MOD-0045 ipc steps 5, 10, 11 and 2 more); before: MOD-0057 api, MOD-0060, MOD-0061 and 2 more",
	} {
		if got := flowLines([]topology.FlowPosition{p}, "MOD-0004", name, lang); len(got) != 1 || got[0] != want {
			t.Errorf("%s:\n got %q\nwant %q", lang, got, want)
		}
	}
	p.Lead = true
	if got := flowLines([]topology.FlowPosition{p}, "MOD-0004", name, "ko"); !strings.HasPrefix(got[0], "주 흐름: ts-invoke-to-rust ") {
		t.Errorf("the flow the overview draws bold heads its line as the primary flow: %q", got[0])
	}
}

// A flow that never leaves the ledger, so it draws no line on the overview.
const auditFlow = `---
question: Who reads the book?
steps:
  - unit: MOD-0002
    does: Post() opens the book
    evidence:
      - {path: ledger/book.go, lines: "6", symbol: Post, quote: "func Post() {}"}
  - unit: MOD-0002
    does: Post() posts them
    evidence:
      - {path: ledger/book.go, lines: "6", symbol: Post, quote: "func Post() {}"}
---
`

// With two stored flows, explain and brief put first, as the primary flow,
// the one the overview draws bold.
func TestTheFlowTheOverviewDrawsBoldComesFirst(t *testing.T) {
	t.Parallel()
	p := notesProject(t)
	p.flow(0, "stored flow charge", chargeFlow)
	p.want(0, "stored flow audit", "flow", "audit", p.file(t, "audit.md", auditFlow))
	if page := p.blueprint(); !strings.Contains(page, "주 흐름: 저장된 흐름 audit의 단계 순서") {
		t.Fatal("the overview no longer draws audit bold")
	}
	p.setLanguage("ko")
	p.want(0, "\n주 흐름: audit 1·2/2단계\n흐름: charge 3/3단계; 앞: MOD-0001 billing\n", "explain", "MOD-0002")
	p.want(0, "\n- 주 흐름: audit 1·2/2단계\n- 흐름: charge 3/3단계; 앞: MOD-0001 billing\n", "brief", "MOD-0002")
}
