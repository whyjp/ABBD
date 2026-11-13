package main

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

func (p *project) setLanguage(tag string) {
	p.t.Helper()
	p.code(".abbd/project.yaml", p.read("project.yaml")+"language: "+tag+"\n")
}

func TestBriefNamesTheLanguageToWriteIn(t *testing.T) {
	p := semanticProject(t)
	p.setLanguage("ko")
	out := p.want(0, "## Read first", "brief", "MOD-0001")
	if !strings.Contains(out, "## Read first\n\nWrite all prose in ko; keep code identifiers, paths and quotes verbatim.\n") {
		t.Errorf("brief does not name the language under Read first:\n%s", out)
	}
	p.want(0, "language: MOD-0001's text does not read as ko", "understand", "MOD-0001", p.file(t, "en.md", "## Overview\n\nbilling writes the charges table.\n"))
}

func proseOf(t *testing.T, p *project) (lintReport, string) {
	t.Helper()
	_, out := p.cmd("", "lint", "--json")
	var r lintReport
	if err := json.NewDecoder(strings.NewReader(out)).Decode(&r); err != nil {
		t.Fatalf("%v:\n%s", err, out)
	}
	return r, out
}

func TestLintFlagsProseNotInTheProjectLanguage(t *testing.T) {
	p := semanticProject(t)
	p.want(0, "understood", "understand", "MOD-0001", p.file(t, "en.md", "## Overview\n\nbilling writes the charges table.\n"))
	if r, _ := proseOf(t, p); len(r.Prose) != 0 {
		t.Fatalf("lint judged prose with no language in project.yaml: %v", r.Prose)
	}

	p.setLanguage("ko")
	r, out := proseOf(t, p)
	if !slices.Contains(r.Prose, "MOD-0001 text") || !slices.ContainsFunc(r.Failures, func(f string) bool { return strings.HasPrefix(f, "prose: MOD-0001 text does not read as ko") }) {
		t.Fatalf("lint did not fail the English text under ko:\n%s", out)
	}

	p.want(0, "understood", "understand", "MOD-0001", p.file(t, "ko.md", "## Overview\n\n`billing/charge.go`의 Charge가 \"INSERT INTO charges\" 쿼리로 charges table에 금액을 기록한다.\n"))
	if r, out := proseOf(t, p); slices.Contains(r.Prose, "MOD-0001 text") {
		t.Fatalf("lint flagged a Korean text with code spans, paths and quotes:\n%s", out)
	}

	p.want(0, "understood", "understand", "MOD-0001", p.file(t, "en.md", "## Overview\n\nbilling writes the charges table.\n"))
	p.want(0, "", "lint", "--record")
	if !strings.Contains(p.read("lint-debt.yaml"), "prose:\n    language: ko\n    reason: written before project.yaml named ko") {
		t.Fatalf("record wrote no prose debt:\n%s", p.read("lint-debt.yaml"))
	}
	if r, out := proseOf(t, p); slices.ContainsFunc(r.Failures, func(f string) bool { return strings.HasPrefix(f, "prose:") }) {
		t.Fatalf("lint failed prose the debt carries:\n%s", out)
	}
	p.want(0, "understood", "understand", "MOD-0001", p.file(t, "ko.md", "## Overview\n\nbilling 모듈은 charges table에 금액을 기록한다.\n"))
	if r, out := proseOf(t, p); !slices.Contains(r.Lowerable, "prose: MOD-0001 text reads as ko now; drop it from the debt") {
		t.Fatalf("lint did not offer to drop a text written again in ko:\n%s", out)
	}

	p.want(0, "responsibility of MOD-0001", "understand", "MOD-0001", p.file(t, "ko.md", "## Overview\n\nbilling 모듈은 charges table에 금액을 기록한다.\n"), "--sentence", "billing writes the charges table.")
	p.want(1, "prose: 1 text not in ko left out of the debt", "lint", "--record")
	debt := p.read("lint-debt.yaml")
	if strings.Contains(debt, "MOD-0001 responsibility") || strings.Contains(debt, "MOD-0001 text") {
		t.Fatalf("record must keep only owed texts still foreign, never take in one written since:\n%s", debt)
	}
	if r, out := proseOf(t, p); !slices.Contains(r.Failures, "prose: MOD-0001 responsibility does not read as ko; write it in ko, keeping identifiers, paths and quotes verbatim") {
		t.Fatalf("an English text written after the first record must still fail after --record:\n%s", out)
	}
}

func TestLintJudgesEveryTablePartAndColumn(t *testing.T) {
	t.Parallel()
	p := notesProject(t)
	ko := strings.NewReplacer(
		"question: Which part of the charge path touches the charges table?", "question: charge 경로의 어느 부분이 charges table에 닿는가?",
		"The ledger reads what billing writes.", "ledger는 billing이 쓴 것을 읽는다.",
	).Replace(chargeTable)
	p.table(0, "stored table charge", ko)
	p.setLanguage("ko")
	r, out := proseOf(t, p)
	for _, k := range []string{"table charge columns", "table charge row MOD-0001", "table charge row MOD-0002"} {
		if !slices.Contains(r.Prose, k) {
			t.Errorf("lint must judge %s:\n%s", k, out)
		}
	}
	if slices.Contains(r.Prose, "table charge") {
		t.Errorf("lint failed a Korean question and caption:\n%s", out)
	}
}

// The debt owes the words a text held when it was taken, not its slot: an
// owed text written again in English fails until it reads in the language.
func TestTheProseDebtOwesWordsNotSlots(t *testing.T) {
	p := semanticProject(t)
	p.setLanguage("ko")
	sentence := "billing writes the charges table."
	p.want(0, "understood", "understand", "MOD-0001", p.file(t, "en.md", "## Overview\n\nbilling writes the charges table.\n"), "--sentence", sentence)
	p.want(0, "", "lint", "--record")
	p.want(0, "", "lint")

	p.want(0, "understood", "understand", "MOD-0001", p.file(t, "en.md", "## Overview\n\nbilling stores every charge in the charges table.\n"), "--sentence", sentence)
	r, out := proseOf(t, p)
	if !slices.Contains(r.Failures, "prose: MOD-0001 text was written again since the debt took it and does not read as ko; write it in ko, keeping identifiers, paths and quotes verbatim") {
		t.Fatalf("an owed text written again in English must fail lint:\n%s", out)
	}
	if slices.ContainsFunc(r.Failures, func(f string) bool { return strings.HasPrefix(f, "prose: MOD-0001 responsibility") }) {
		t.Fatalf("an owed text left as it was must still pass:\n%s", out)
	}
	p.want(1, "prose: 1 text not in ko left out of the debt", "lint", "--record")
	p.want(1, "prose: MOD-0001 text does not read as ko", "lint")

	p.want(0, "understood", "understand", "MOD-0001", p.file(t, "ko.md", "## Overview\n\nbilling 모듈은 charges table에 금액을 기록한다.\n"), "--sentence", sentence)
	p.want(0, "", "lint", "--record")
	debt := p.read("lint-debt.yaml")
	if strings.Contains(debt, "MOD-0001 text") || !strings.Contains(debt, "MOD-0001 responsibility") {
		t.Fatalf("record must drop the text written in ko and keep the untouched one:\n%s", debt)
	}
	p.want(0, "", "lint")
}
