package narrative

import (
	"slices"
	"strings"
	"testing"
)

func TestSectionsRunFromOneHeadingToTheNextOfAnyLevel(t *testing.T) {
	text := "lead in\n\n## Overview\n\nabc\n\nde\n\n## Key Concepts\n\n- **x**: é\n\n### Sub\n\nfghij\n"
	got := Sections(text)
	want := []Section{{"", 7}, {"Overview", 5}, {"Key Concepts", 10}, {"Sub", 5}}
	if !slices.Equal(got, want) {
		t.Fatalf("sections = %v, want %v", got, want)
	}
}

func TestLongReportsOnlySectionsOverTheThreshold(t *testing.T) {
	text := "## Short\n\n" + strings.Repeat("a", MaxSection) + "\n\n## Long\n\n" + strings.Repeat("a", MaxSection) + "\nb\n"
	got := Long(text)
	if len(got) != 1 || got[0].Heading != "Long" || got[0].Chars != MaxSection+1 {
		t.Fatalf("long = %v, want only Long at %d", got, MaxSection+1)
	}
}

func TestHistoryFindsDatesReferencesAndTags(t *testing.T) {
	text := "## Overview\n\nSince 2026-11-02 the store keeps hashes, as of March 3, 2026 and Nov 2025.\n" +
		"## Key Concepts\n\nIt fixes #24 and PR #31, issue 7, whyjp/ABBD#45. It was migrated from the old CLI, formerly `abbd-sync`, and used to be one file, added in v0.6.\n"
	var got []string
	for _, m := range History(text) {
		got = append(got, m.Section+"|"+m.Kind+"|"+m.Match)
	}
	want := []string{
		"Overview|a date|2026-11-02",
		"Overview|a date|March 3, 2026",
		"Overview|a date|Nov 2025",
		"Key Concepts|a history tag|migrated from",
		"Key Concepts|a history tag|formerly",
		"Key Concepts|a history tag|used to be",
		"Key Concepts|a history tag|added in v0.6",
		"Key Concepts|an issue or PR number|PR #31",
		"Key Concepts|an issue or PR number|issue 7",
		"Key Concepts|an issue or PR number|whyjp/ABBD#45",
		"Key Concepts|an issue or PR number|#24",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("marks =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// What describes code as it is reads as present state: versions, names,
// anchors, and the words the code itself uses for old data.
func TestHistoryLeavesPresentStateAlone(t *testing.T) {
	text := "## Overview\n\nABBD 0.6.0 texts and format 1 stamps are upgraded, and UpgradeUnderstandings reads the old format.\n" +
		"A unit no longer cited reads dead. Links go to #overview and #m-MOD-0001, entities like &#123; stay.\n" +
		"go1.22 builds it, a key is cmd/abbd#quote-1a2b3c4d, a section holds 2000 characters, and it is used to compute digests.\n" +
		"A unit added in 2 places keeps one ID.\n"
	if got := History(text); len(got) != 0 {
		t.Fatalf("marks = %v, want none", got)
	}
}

// A code span quotes code or an example, so what reads as history inside
// one is not the text's story; the same words outside it are.
func TestHistorySkipsCodeSpans(t *testing.T) {
	got := History("It matches `PR #12` and `migrated from`, but not #7 here.\n")
	if len(got) != 1 || got[0].Match != "#7" {
		t.Fatalf("marks = %v, want only #7", got)
	}
}

// A URL names a page, so a fragment or a date in its path is not the
// text's story.
func TestHistorySkipsURLs(t *testing.T) {
	got := History("See https://x.com/a#12 and http://example.com/2026-11-02/notes, but not #7 here.\n")
	if len(got) != 1 || got[0].Match != "#7" {
		t.Fatalf("marks = %v, want only #7", got)
	}
}

// A fenced block quotes code: a # line inside it opens no section, and
// what reads as history inside it is not reported.
func TestAFencedBlockIsCodeNotProse(t *testing.T) {
	text := "## Overview\n\nIt builds.\n\n```sh\n# build it\nmake # see PR #12 from 2026-11-02\n```\n\n~~~\n#include <x.h>\n~~~\n\n## Key Concepts\n\nfixes #7\n"
	var heads []string
	for _, s := range Sections(text) {
		heads = append(heads, s.Heading)
	}
	if want := []string{"Overview", "Key Concepts"}; !slices.Equal(heads, want) {
		t.Errorf("sections = %v, want %v", heads, want)
	}
	if got := History(text); len(got) != 1 || got[0].Match != "#7" || got[0].Section != "Key Concepts" {
		t.Errorf("marks = %v, want only #7 under Key Concepts", got)
	}
}

// A backtick fence's info string holds no backtick, so a line that starts
// with an inline code span is prose, and a heading after it opens a section.
func TestALineStartingWithInlineCodeOpensNoFence(t *testing.T) {
	text := "## Overview\n\n```js``` inline\n\n## Key Concepts\n\nfixes #7\n\n~~~ a`b\n#x\n~~~\n"
	var heads []string
	for _, s := range Sections(text) {
		heads = append(heads, s.Heading)
	}
	if want := []string{"Overview", "Key Concepts"}; !slices.Equal(heads, want) {
		t.Errorf("sections = %v, want %v", heads, want)
	}
	if got := History(text); len(got) != 1 || got[0].Match != "#7" {
		t.Errorf("marks = %v, want only #7", got)
	}
}
