package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/whyjp/abbd/internal/screen"
)

func lintAt(t *testing.T, root string, args ...string) (int, string) {
	t.Helper()
	var out, errs bytes.Buffer
	code := run(append([]string{"lint", "--root", root}, args...), strings.NewReader(""), &out, &errs)
	return code, out.String() + errs.String()
}

func fixtureRoot(name string) string {
	return filepath.Join("..", "..", "internal", "screen", "testdata", name)
}

// copyFixture copies a fixture's .abbd into a new root, with the lint debt
// file given in place of its own, or none.
func copyFixture(t *testing.T, name, debt string) string {
	t.Helper()
	root := t.TempDir()
	from := filepath.Join(fixtureRoot(name), ".abbd")
	err := filepath.WalkDir(from, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Name() == "lint-debt.yaml" {
			return err
		}
		rel, _ := filepath.Rel(from, p)
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		to := filepath.Join(root, ".abbd", rel)
		if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
			return err
		}
		return os.WriteFile(to, b, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	if debt != "" {
		if err := os.WriteFile(filepath.Join(root, ".abbd", "lint-debt.yaml"), []byte(debt), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// Orca's src/ keeps every hard criterion, so lint passes, and it names the
// three spots the user circled: the mobile/ line into the Orca runtime, down
// from a staircase of 8 turns to 2, the native/ chip's two turns in its
// clear column, which count as excess turns though they are no step, and
// the preload bridge line, which now drops straight from its box to its
// chip and so breaks nothing. Each breach names its boxes by unit ID too.
func TestLintNamesTheStaircasesOfSrc(t *testing.T) {
	code, out := lintAt(t, fixtureRoot("orca"), "MOD-0098 src/", "--json")
	if code != exitOK {
		t.Fatalf("exit %d, want 0:\n%s", code, out)
	}
	var r lintReport
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{
		`B1 the line from chip "← mobile/" to box "Orca runtime (RPC host)" (MOD-0056) turns 2 times`:                            false,
		`B1 the line from chip "← native/" to box "desktop main process" (MOD-0042) turns 2 times, allowed 0, 2 turns in excess`: false,
	}
	for _, v := range r.Views {
		for _, b := range v.Breaches {
			if strings.Contains(b.Text, `"desktop preload bridge" (MOD-0071) to chip "preload bridge"`) {
				t.Errorf("the preload bridge line breaks %s: %s", b.Criterion, b.Text)
			}
			for w := range want {
				if strings.HasPrefix(b.Criterion+" "+b.Text, w) {
					want[w] = true
					if !slices.ContainsFunc(b.Items, func(it screen.Item) bool {
						return it.Kind == "box" && it.ID != "" && strings.Contains(b.Text, "("+it.ID+")")
					}) {
						t.Errorf("%s: no box among %+v names its unit ID", w, b.Items)
					}
				}
			}
		}
	}
	for w, seen := range want {
		if !seen {
			t.Errorf("lint does not report %q", w)
		}
	}
	if r.Schema != lintSchema || r.Totals.ExcessTurns != 66 || r.Totals.Steps != 44 || r.Totals.Hard != 0 || len(r.Failures) != 0 {
		t.Errorf("schema %d, excess turns %d, steps %d, hard %d, failures %v; want %d, 66, 44, 0 and none", r.Schema, r.Totals.ExcessTurns, r.Totals.Steps, r.Totals.Hard, r.Failures, lintSchema)
	}
}

// A hard breach the view has recorded as debt passes, and is marked so.
func TestLintPassesARecordedDebt(t *testing.T) {
	code, out := lintAt(t, fixtureRoot("self"), "MOD-0047", "--json")
	if code != exitOK {
		t.Fatalf("exit %d, want 0:\n%s", code, out)
	}
	var r lintReport
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatal(err)
	}
	debts := 0
	for _, v := range r.Views {
		for _, b := range v.Breaches {
			if b.Debt != (b.Hard && v.View == "MOD-0047 internal/topology") {
				t.Errorf("%s: %s: debt %v", v.View, b.Text, b.Debt)
			}
			if b.Debt {
				debts++
			}
		}
	}
	if debts != 1 {
		t.Errorf("%d breaches marked debt, want rule 13's 1", debts)
	}
}

// A hard breach the view has not recorded fails lint, and the report says
// why.
func TestLintFailsOnAHardBreachPastItsDebt(t *testing.T) {
	code, out := lintAt(t, copyFixture(t, "self", ""), "MOD-0047")
	if code != exitError || !strings.Contains(out, "R hard  rule 13:") || !strings.Contains(out, "MOD-0047 internal/topology: 1 hard breach of R, 0 recorded as debt") {
		t.Fatalf("exit %d, want 1 with rule 13 named:\n%s", code, out)
	}
}

// Over every view, a total past its ceiling fails lint.
func TestLintHoldsTheTotalsToTheirCeilings(t *testing.T) {
	debt := "ceilings:\n  weighed: 100\n  placement: 50\ndebt:\n  MOD-0030 internal/model: {R: 1}\n  MOD-0047 internal/topology: {R: 1}\n"
	code, out := lintAt(t, copyFixture(t, "self", debt))
	if code != exitError || !strings.Contains(out, ", past its ceiling 100 by more than 0") || strings.Contains(out, "placement 50, past") {
		t.Fatalf("exit %d, want 1 on the weighed sum alone:\n%s", code, out)
	}
}

// --record lowers a ceiling above what the drawings measure and keeps one
// below it, so the slack the gate allows never ratchets a ceiling up.
func TestLintRecordNeverRaisesACeiling(t *testing.T) {
	root := copyFixture(t, "self", "ceilings:\n  weighed: 999999\n  placement: 1\n")
	if code, out := lintAt(t, root, "--record"); code != exitOK {
		t.Fatalf("exit %d:\n%s", code, out)
	}
	var r lintReport
	code, out := lintAt(t, root, "--json")
	if err := json.NewDecoder(strings.NewReader(out)).Decode(&r); err != nil {
		t.Fatalf("exit %d: %v\n%s", code, err, out)
	}
	if r.Ceilings["placement"] != 1 || r.Totals.Placement <= 1 {
		t.Errorf("placement ceiling %d with %d measured, want the ceiling kept at 1", r.Ceilings["placement"], r.Totals.Placement)
	}
	if r.Ceilings["weighed"] != r.Totals.Weighed {
		t.Errorf("weighed ceiling %d, want it lowered to the %d measured", r.Ceilings["weighed"], r.Totals.Weighed)
	}
}

// This repository's own blueprint keeps the gate its debt file sets.
func TestLintPassesThisRepository(t *testing.T) {
	if code, out := lintAt(t, filepath.Join("..", "..")); code != exitOK {
		t.Fatalf("exit %d, want 0:\n%s", code, out)
	}
}

func TestLintListsTheViews(t *testing.T) {
	code, out := lintAt(t, fixtureRoot("self"), "--list")
	if code != exitOK || !strings.HasPrefix(out, "overview\n") || !strings.Contains(out, "\nMOD-0047 internal/topology\n") {
		t.Fatalf("exit %d:\n%s", code, out)
	}
}

func TestLintRefusesAnUnknownView(t *testing.T) {
	if code, out := lintAt(t, fixtureRoot("self"), "no such view"); code != exitError || !strings.Contains(out, `no view is named "no such view"`) {
		t.Fatalf("exit %d:\n%s", code, out)
	}
}
