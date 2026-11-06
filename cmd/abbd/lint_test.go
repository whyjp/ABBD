package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
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
// one spot the user circled on the top edge that still turns: the mobile/
// line into the Orca runtime, down from a staircase of 8 turns to 2. The
// top native/ line and the preload bridge line now drop straight between
// chip and box and break nothing, and so does orca CLI's line down to the
// bottom chip of the Orca runtime RPC methods. On the bottom edge main drops
// desktop main's lines to the connector contract and to native/ straight;
// here each turns 4 times round WSL guest agent, and native/ stands off
// desktop main, so a change to either shows. Each breach names the box at
// its line's box end by unit ID: the last item of a line into a box, the
// first of a line out of one.
func TestLintNamesTheStaircasesOfSrc(t *testing.T) {
	code, out := lintAt(t, fixtureRoot("orca"), "MOD-0098 src/", "--json")
	if code != exitOK {
		t.Fatalf("exit %d, want 0:\n%s", code, out)
	}
	var r lintReport
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatal(err)
	}
	// box is the item that names the box end: -1 the last item, 0 the first.
	type spot struct {
		box  int
		seen bool
	}
	want := map[string]*spot{
		`B1 the line from chip "← mobile/" to box "Orca runtime (RPC host)" (MOD-0056) turns 2 times`:                               {box: -1},
		`B1 the line from box "desktop main process" (MOD-0042) to chip "@orca-cloud/connector- contract" turns 4 times, allowed 0`: {box: 0},
		`B1 the line from box "desktop main process" (MOD-0042) to chip "→ native/" turns 4 times, allowed 2`:                       {box: 0},
		`F1 chip "→ native/" stands 79px left of box "desktop main process" (MOD-0042)`:                                             {box: 1},
	}
	straight := []string{
		`"desktop preload bridge" (MOD-0071) to chip "preload bridge"`,
		`chip "← native/" to box "desktop main process" (MOD-0042)`,
		`box "orca CLI" (MOD-0038) to chip "Orca runtime RPC methods"`,
	}
	for _, v := range r.Views {
		for _, b := range v.Breaches {
			for _, s := range straight {
				if strings.Contains(b.Text, s) {
					t.Errorf("a line that drops straight breaks %s: %s", b.Criterion, b.Text)
				}
			}
			for w, sp := range want {
				if !strings.HasPrefix(b.Criterion+" "+b.Text, w) {
					continue
				}
				sp.seen = true
				i := sp.box
				if i < 0 {
					i += len(b.Items)
				}
				if it := b.Items[i]; it.Kind != "box" || it.ID == "" || !strings.Contains(b.Text, "("+it.ID+")") {
					t.Errorf("%s: item %d %+v names no unit ID", w, i, it)
				}
			}
		}
	}
	for w, sp := range want {
		if !sp.seen {
			t.Errorf("lint does not report %q", w)
		}
	}
	if r.Schema != lintSchema || r.Totals.ExcessTurns != 24 || r.Totals.Steps != 12 || r.Totals.Hard != 0 || len(r.Failures) != 0 {
		t.Errorf("schema %d, excess turns %d, steps %d, hard %d, failures %v; want %d, 24, 12, 0 and none", r.Schema, r.Totals.ExcessTurns, r.Totals.Steps, r.Totals.Hard, r.Failures, lintSchema)
	}
}

// A hard breach the view has recorded as debt passes, and is marked so.
func TestLintPassesARecordedDebt(t *testing.T) {
	code, out := lintAt(t, fixtureRoot("self"), "MOD-0030", "--json")
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
			if b.Debt != (b.Hard && v.View == "MOD-0030 internal/model") {
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
	code, out := lintAt(t, copyFixture(t, "self", ""), "MOD-0030")
	if code != exitError || !strings.Contains(out, "R hard  rule 13:") || !strings.Contains(out, "MOD-0030 internal/model: 1 hard breach of R, 0 recorded as debt") {
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

// The first --record of a repository refuses while a view with hard
// breaches stands at the wrong level, since the units, not the routing,
// owe those breaches. Orca's renderer page draws 21 lines between 9 parts,
// and its shared types page 22 parts.
func TestLintFirstRecordWaitsForTheLevels(t *testing.T) {
	root := copyFixture(t, "orca", "")
	code, out := lintAt(t, root, "--record")
	if code != exitError || !strings.Contains(out, "will not take the first debt") || !strings.Contains(out, "MOD-0072 desktop renderer UI (the view draws 21 lines between 9 boxes") || !strings.Contains(out, "--keep-level VIEW=REASON") {
		t.Fatalf("exit %d, want 1 naming the renderer page's level and the way to keep it:\n%s", code, out)
	}
	if _, err := os.Stat(filepath.Join(root, ".abbd", "lint-debt.yaml")); err == nil {
		t.Error("the refused record wrote a debt file")
	}
}

// A level the agent can justify stays: --keep-level names the view and the
// reason, the record goes through and keeps the reason in the debt file,
// and lint then prints it beside the level line and under levels in JSON.
// A view kept without a reason, or one with no level line, is refused.
func TestLintRecordKeepsAJustifiedLevel(t *testing.T) {
	root := copyFixture(t, "orca", "")
	for _, bad := range []string{"MOD-0072", "overview=it reads", "MOD-0072=  "} {
		if code, out := lintAt(t, root, "--record", "--keep-level", bad); code != exitError || !strings.Contains(out, "--keep-level") {
			t.Errorf("--keep-level %q: exit %d, want 1:\n%s", bad, code, out)
		}
	}
	if code, out := lintAt(t, root, "--keep-level", "MOD-0072=x"); code != exitError || !strings.Contains(out, "with --record") {
		t.Errorf("--keep-level without --record: exit %d:\n%s", code, out)
	}
	if code, out := lintAt(t, root, "--record", "--keep-level", "MOD-0072=renderer UI parts are the app's screens", "--keep-level", "MOD-0081=one schema per wire format"); code != exitOK || !strings.Contains(out, "kept the level of MOD-0072 desktop renderer UI: renderer UI parts are the app's screens") {
		t.Fatalf("exit %d, want the record to keep both levels:\n%s", code, out)
	}
	b, err := os.ReadFile(filepath.Join(root, ".abbd", "lint-debt.yaml"))
	if err != nil || !strings.Contains(string(b), "levels:") || !strings.Contains(string(b), "MOD-0081 shared types and schemas: one schema per wire format") {
		t.Fatalf("the debt file does not keep the reasons (%v):\n%s", err, b)
	}
	code, out := lintAt(t, root)
	if code != exitOK || !strings.Contains(out, "level MOD-0072 desktop renderer UI: kept, renderer UI parts are the app's screens") {
		t.Fatalf("exit %d, want lint to pass and print the kept reason:\n%s", code, out)
	}
	var r lintReport
	_, out = lintAt(t, root, "--json")
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatal(err)
	}
	kept := map[string]string{}
	for _, l := range r.Levels {
		kept[l.View] = l.Kept
	}
	if kept["MOD-0072 desktop renderer UI"] == "" || kept["MOD-0081 shared types and schemas"] == "" || kept["overview"] != "" {
		t.Errorf("levels kept %v, want the two kept views alone", kept)
	}
}

// A repository that already keeps a debt file records again whatever its
// levels say, and the reasons it kept stay while their level lines do.
func TestLintRecordOverAnExistingDebtFile(t *testing.T) {
	debt := "ceilings:\n  weighed: 999999\n  placement: 999999\nlevels:\n  MOD-0072 desktop renderer UI: screens\n  MOD-0001 gone: a view that no longer has a level line\n"
	root := copyFixture(t, "orca", debt)
	if code, out := lintAt(t, root, "--record"); code != exitOK || !strings.Contains(out, "kept the level of MOD-0072 desktop renderer UI: screens") {
		t.Fatalf("exit %d, want the record to go through and keep the prior reason:\n%s", code, out)
	}
	b, _ := os.ReadFile(filepath.Join(root, ".abbd", "lint-debt.yaml"))
	if !strings.Contains(string(b), "MOD-0072 desktop renderer UI: screens") || strings.Contains(string(b), "MOD-0001 gone") {
		t.Errorf("the debt file must keep the reason of a level line that stays and drop the rest:\n%s", b)
	}
}

// lint --json lists every module's page and the overview under levels, with
// the parts and the lines between them, and leaves features' pages out:
// they draw the units a feature reaches, not a module's parts.
func TestLintJSONListsTheLevels(t *testing.T) {
	_, out := lintAt(t, fixtureRoot("self"), "--json")
	var r lintReport
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatal(err)
	}
	by := map[string]screen.Level{}
	for _, l := range r.Levels {
		by[l.View] = l
		if strings.HasPrefix(l.View, "FEAT-") {
			t.Errorf("levels lists the feature page %s", l.View)
		}
	}
	ov, ok := by["overview"]
	if !ok || ov.Boxes != 14 || len(ov.Advice) != 1 || strings.Join(ov.Hubs, ",") != "internal/model" {
		t.Errorf("overview level %+v, want 14 boxes, internal/model as its hub and one level line", ov)
	}
	if _, ok := by["MOD-0034 internal/screen"]; !ok {
		t.Errorf("levels has no MOD-0034 internal/screen: %v", r.Levels)
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
