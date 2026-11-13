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
}
