package main

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func lintFixture(t *testing.T, args ...string) (int, string) {
	t.Helper()
	var out, errs bytes.Buffer
	root := filepath.Join("..", "..", "internal", "screen", "testdata")
	code := run(append([]string{"lint", "--root", filepath.Join(root, args[0])}, args[1:]...), strings.NewReader(""), &out, &errs)
	return code, out.String() + errs.String()
}

// Orca's src/ keeps every hard criterion, so lint passes, and it names the
// three spots the user circled: the mobile/ staircase, the native/ chip's
// step and the preload bridge chip that stands past its box.
func TestLintNamesTheStaircasesOfSrc(t *testing.T) {
	code, out := lintFixture(t, "orca", "MOD-0098 src/", "--json")
	if code != exitOK {
		t.Fatalf("exit %d, want 0:\n%s", code, out)
	}
	var r lintReport
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{
		`B1 the line from chip "← mobile/" to box "Orca runtime (RPC host)" (MOD-0056) turns 8 times`:         false,
		`B1 the line from chip "← native/" to box "desktop main process" (MOD-0042) turns 2 times, allowed 0`: false,
		`F1 chip "preload bridge" stands 5px right of box "desktop preload bridge" (MOD-0071)`:                false,
	}
	for _, v := range r.Views {
		for _, b := range v.Breaches {
			for w := range want {
				if strings.HasPrefix(b.Criterion+" "+b.Text, w) {
					want[w] = true
				}
			}
		}
	}
	for w, seen := range want {
		if !seen {
			t.Errorf("lint does not report %q", w)
		}
	}
	if r.Totals.Steps != 57 || r.Totals.Hard != 0 {
		t.Errorf("steps %d, hard %d; want 57 and 0", r.Totals.Steps, r.Totals.Hard)
	}
}

// A view that breaks a hard criterion fails lint, and the report says why.
func TestLintFailsOnAHardBreach(t *testing.T) {
	code, out := lintFixture(t, "self", "MOD-0047")
	if code != exitError || !strings.Contains(out, "R hard  rule 13:") || !strings.Contains(out, "hard breaches") {
		t.Fatalf("exit %d, want 1 with rule 13 named:\n%s", code, out)
	}
}

func TestLintRefusesAnUnknownView(t *testing.T) {
	if code, out := lintFixture(t, "self", "no such view"); code != exitError || !strings.Contains(out, `no view is named "no such view"`) {
		t.Fatalf("exit %d:\n%s", code, out)
	}
}
