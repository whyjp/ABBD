package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/whyjp/abbd/internal/narrative"
)

// A text that reads as history or runs long is still stored: understand
// says what to fix, and status reports it without failing the check.
func TestLongAndHistoricalTextsAreReportedNeverRefused(t *testing.T) {
	p := semanticProject(t)
	text := "## Overview\n\nbilling inserts charges, migrated from the old service in PR #12.\n\n## Key Concepts\n\n" + strings.Repeat("charges are rows. ", 120) + "\n"
	out := p.want(0, "understood MOD-0001", "understand", "MOD-0001", p.file(t, "how.md", text))
	for _, want := range []string{
		`long: MOD-0001 "Key Concepts" holds 2159 characters, over 2000`,
		`history: MOD-0001 "Overview" says "migrated from", which reads as a history tag`,
		`history: MOD-0001 "Overview" says "PR #12", which reads as an issue or PR number`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("understand did not say %q:\n%s", want, out)
		}
	}
	if !strings.Contains(p.read("understanding/MOD-0001.md"), "migrated from") {
		t.Error("understand did not store the text it warned on")
	}
	out = p.want(0, "readability: 1 section(s) over 2000 characters, 2 phrase(s) that read as history; a report, the check ignores it", "status", "--check")
	if !strings.Contains(out, `long: MOD-0001 "Key Concepts"`) || !strings.Contains(out, `history: MOD-0001 "Overview" says "PR #12"`) {
		t.Errorf("status did not list what it counted:\n%s", out)
	}
}

func TestAPresentStateTextOfShortSectionsReportsNothing(t *testing.T) {
	p := semanticProject(t)
	out := p.want(0, "understood MOD-0001", "understand", "MOD-0001", p.file(t, "how.md", "## Overview\n\nbilling 2.1 inserts charges.\n"))
	if strings.Contains(out, "long:") || strings.Contains(out, "history:") {
		t.Errorf("understand reported a clean text:\n%s", out)
	}
	p.want(0, "readability: 0 section(s) over 2000 characters, 0 phrase(s)", "status", "--check")
}

// Both skills that store a text tell the agent what the report checks, at
// the threshold the report uses.
func TestTheSkillsAskForPresentStateTextsInShortSections(t *testing.T) {
	limit := fmt.Sprintf("%d,%03d characters", narrative.MaxSection/1000, narrative.MaxSection%1000)
	for _, skill := range []string{"abbd", "abbd-setup"} {
		text := readPlugin(t, "skills", skill, "SKILL.md")
		for _, want := range []string{"present tense", "issue and PR numbers", "migrated from X", limit, "`###` headings", "Neither refuses the text or fails the check"} {
			if !strings.Contains(text, want) {
				t.Errorf("%s must say %q", skill, want)
			}
		}
	}
}

// An orphan text shows beside no module and the check skips it, so the
// readability report skips it too.
func TestTheReadabilityReportSkipsAnOrphanText(t *testing.T) {
	p := semanticProject(t)
	p.want(0, "understood", "understand", "MOD-0001", p.file(t, "how.md", "## Overview\n\nbilling inserts charges.\n"))
	write(t, filepath.Join(p.root, ".abbd", "understanding", "MOD-0099.md"), "---\nunit: MOD-0099\norigin: INFERRED\ndigest: 0123456789abcdef\nformat: 3\n---\n\n## Overview\n\nmigrated from the old service in PR #12.\n")
	out := p.want(0, "1 orphaned", "status")
	if !strings.Contains(out, "0 phrase(s) that read as history") || strings.Contains(out, "MOD-0099 \"Overview\"") {
		t.Fatalf("status linted the orphan text:\n%s", out)
	}
}
