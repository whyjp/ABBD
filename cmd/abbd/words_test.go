package main

import (
	"slices"
	"strings"
	"testing"

	"github.com/whyjp/abbd/internal/model"
)

// Diff says, sentence by sentence, what a text, note or flow step now says
// differently, past headings, bullets, abbreviations and code spans, and
// keeps to the units --unit names.
func TestWordsDiffNamesTheSentencesThatChanged(t *testing.T) {
	mk := func(kv ...string) written {
		w := written{text: map[string]string{}, unit: map[string]model.ID{}}
		for i := 0; i < len(kv); i += 3 {
			w.put(kv[i], model.ID(kv[i+1]), kv[i+2])
		}
		return w
	}
	was := mk(
		"text MOD-0001 billing", "MOD-0001", "## Overview\n\nBilling charges cards, e.g. visa. It retries `once. twice` never.\n\n## Key Concepts\n\n- **Charge**: posts one charge.",
		"note MOD-0001.failure", "MOD-0001", "A failed charge is logged.",
		"flow pay step 1 (MOD-0002)", "MOD-0002", "ledger posts the entry",
	)
	now := mk(
		"text MOD-0001 billing", "MOD-0001", "## Overview\n\nBilling charges cards, e.g. visa. It retries three times.\n\n## Key Concepts\n\n- **Charge**: posts one charge.",
		"flow pay step 1 (MOD-0002)", "MOD-0002", "ledger posts the entry",
		"flow pay step 2 (MOD-0002)", "MOD-0002", "ledger answers the caller",
	)
	got := wordsDiff(was, now, nil)
	want := []string{
		"+ flow pay step 2 (MOD-0002) (1 sentences)",
		"- note MOD-0001.failure (1 sentences)",
		"~ text MOD-0001 billing",
		`    - "It retries ` + "`once. twice`" + ` never."`,
		`    + "It retries three times."`,
	}
	if !slices.Equal(got, want) {
		t.Errorf("words =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	only := wordsDiff(was, now, func(id model.ID) bool { return id == "MOD-0002" })
	if len(only) != 1 || !strings.HasPrefix(only[0], "+ flow pay step 2") {
		t.Errorf("--unit MOD-0002 kept %q", only)
	}
	var many []string
	for i := range 6 {
		many = append(many, strings.Repeat("x", i+1)+".")
	}
	long := wordsDiff(mk("text MOD-0003 x", "MOD-0003", ""), mk("text MOD-0003 x", "MOD-0003", strings.Join(many, " ")), nil)
	if got := long[len(long)-1]; got != "    + and 2 more" {
		t.Errorf("a long change ends %q, want the count of the rest", got)
	}
}
