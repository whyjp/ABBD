package main

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/whyjp/abbd/internal/model"
)

func say(unit, name, text string) wording {
	return wording{model.ID(unit), name, sentences(text)}
}

func flow(steps ...wording) written {
	return written{once: map[string]wording{}, flows: map[string][]wording{"pay": steps}}
}

func wantLines(t *testing.T, got []wordChange, want ...string) {
	t.Helper()
	if l := lines(got); !slices.Equal(l, want) {
		t.Errorf("words =\n%s\nwant\n%s", strings.Join(l, "\n"), strings.Join(want, "\n"))
	}
}

// Diff says, sentence by sentence, what a text or note now says
// differently, keys a text by its module so a rename keeps it, and keeps
// to the units --unit names.
func TestWordsDiffNamesTheSentencesThatChanged(t *testing.T) {
	was := written{once: map[string]wording{
		"text MOD-0001":         say("MOD-0001", "billing", "## Overview\n\nBilling charges cards, e.g. visa. It retries `once. twice` never.\n\n## Key Concepts\n\n- **Charge**: posts one charge."),
		"text MOD-0002":         say("MOD-0002", "ledger", "Ledger posts entries."),
		"note MOD-0001.failure": say("MOD-0001", "", "A failed charge is logged."),
	}}
	now := written{once: map[string]wording{
		"text MOD-0001": say("MOD-0001", "payments", "## Overview\n\nBilling charges cards, e.g. visa. It retries three times.\n\n## Key Concepts\n\n- **Charge**: posts one charge."),
		"text MOD-0002": say("MOD-0002", "books", "Ledger posts entries."),
	}}
	wantLines(t, wordsDiff(was, now, nil),
		"- note MOD-0001.failure (1 sentences)",
		"~ text MOD-0001 payments",
		`    - "It retries `+"`once. twice`"+` never."`,
		`    + "It retries three times."`,
	)
	wantLines(t, wordsDiff(was, now, func(id model.ID) bool { return id == "MOD-0002" }))
}

// One step put into a flow is one added step: the steps after it keep
// their pairs, a reworded step lists only its own sentences, and steps
// number as a reader counts them.
func TestWordsDiffAlignsFlowSteps(t *testing.T) {
	var steps []wording
	for i := range 11 {
		steps = append(steps, say(fmt.Sprintf("MOD-%04d", i%3), "", fmt.Sprintf("Step %d runs.", i)))
	}
	was := flow(steps...)
	moved := slices.Clone(steps)
	moved[9] = wording{steps[9].unit, "", []string{"Step 9 runs.", "It may fail."}}
	now := flow(slices.Insert(moved, 0, say("MOD-0007", "", "A check runs first."))...)
	wantLines(t, wordsDiff(was, now, nil),
		"+ flow pay step 1 (MOD-0007) (1 sentences)",
		"~ flow pay step 11 (MOD-0000)",
		`    + "It may fail."`,
	)
	wantLines(t, wordsDiff(now, was, nil),
		"- flow pay step 1 (MOD-0007) (1 sentences)",
		"~ flow pay step 10 (MOD-0000)",
		`    - "It may fail."`,
	)
}

// --json carries every sentence a change lost and gained; the text stops
// at wordsShown each way and counts the rest.
func TestWordsDiffKeepsEverySentenceForATool(t *testing.T) {
	var many []string
	for i := range 6 {
		many = append(many, strings.Repeat("x", i+1)+".")
	}
	was := written{once: map[string]wording{"text MOD-0003": say("MOD-0003", "x", "")}}
	now := written{once: map[string]wording{"text MOD-0003": say("MOD-0003", "x", strings.Join(many, " "))}}
	got := wordsDiff(was, now, nil)
	if len(got) != 1 || !slices.Equal(got[0].Gained, many) || got[0].Change != "changed" || got[0].Kind != "text" || got[0].Unit != "MOD-0003" {
		t.Errorf("change = %+v, want all six sentences gained", got)
	}
	if l := lines(got); l[len(l)-1] != "    + and 2 more" {
		t.Errorf("the text ends %q, want the count of the rest", l[len(l)-1])
	}
}

// A text reads as its sentences: Korean ones split at 다. and 요., a list
// item past its bullet or number, a bold lead kept, a rule or table ruling
// dropped, a code line whole.
func TestSentencesReadProseAndCode(t *testing.T) {
	got := sentences("검사는 두 가지다. 둘 다 3.5초 안에 끝나요. 왜?\n\n---\n\n1. first step. then more\n- **Bold** lead.\n\n| a | b |\n|---|:-:|\n\n```go\nx := 1. y := 2\n# not a heading\n```\n")
	want := []string{"검사는 두 가지다.", "둘 다 3.5초 안에 끝나요.", "왜?", "first step.", "then more", "**Bold** lead.", "| a | b |", "x := 1. y := 2", "# not a heading"}
	if !slices.Equal(got, want) {
		t.Errorf("sentences = %q\nwant %q", got, want)
	}
}
