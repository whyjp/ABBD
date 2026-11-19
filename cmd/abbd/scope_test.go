package main

import (
	"strings"
	"testing"

	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/store"
)

// A scope mark says a unit is a sample, a build artifact or a tool, not the
// product. It is stored with the overview, cites code like the other parts,
// goes STALE with that code, and is refused when it is not one of the scopes
// or its quotes are not in the unit's code.
func TestAScopeMarkIsStoredCheckedAndGoesStale(t *testing.T) {
	t.Parallel()
	p := notesProject(t)
	p.code(".abbd/project.yaml", p.read("project.yaml")+"language: ko\n")
	marked := strings.Replace(chargeOverview, "\n---\n\n장부는", "\nscope:\n  - unit: MOD-0002\n    scope: sample\n    evidence:\n      - {path: ledger/book.go, lines: \"6\", symbol: Post, quote: \"func Post() {}\"}\n---\n\n장부는", 1)
	p.overview(0, "stored the overview: 1 zones, 1 lines", marked)
	if got := p.read("overview.md"); !strings.Contains(got, "scope:") || !strings.Contains(got, "scope: sample") {
		t.Errorf("the stored overview holds the scope mark:\n%s", got)
	}
	p.want(0, "overview: 1 zones, 1 lines, 1 scope marks, 0 STALE", "status", "--check")
	if out := p.want(0, "scope: sample", "overview"); !strings.Contains(out, "unit: MOD-0002") || strings.Contains(out, "code:") {
		t.Errorf("abbd overview prints the scope mark in the draft, without what abbd sets:\n%s", out)
	}
	p.want(0, "live overview scope 1 (MOD-0002 sample): ledger/book.go", "evidence")

	p.overview(1, "one of sample, artifact, tool", strings.Replace(marked, "scope: sample", "scope: product", 1))
	p.overview(1, "none of the quotes of the scope of MOD-0002 is in its code", strings.Replace(marked, `{path: ledger/book.go, lines: "6", symbol: Post, quote: "func Post() {}"}`, `{path: billing/charge.go, lines: "4", symbol: Charge, quote: "func Charge() {}"}`, 1))

	p.code("ledger/book.go", "package ledger\n\nconst q = `SELECT amount\n    FROM charges`\n\nfunc Post(x int) {}\n")
	p.resync()
	p.want(1, "overview scope 1 (MOD-0002 sample) is STALE", "status", "--check")
}

func TestUnscopedNamesBoxesThatLookLikeSamplesAndTestsUntilTheyAreMarked(t *testing.T) {
	t.Parallel()
	unit := func(id, name, parent string, paths ...string) model.Unit {
		u := model.Unit{ID: model.ID(id), Kind: model.Module, Name: model.Said(model.Script, name), Parent: model.ID(parent)}
		for _, p := range paths {
			u.Anchors = append(u.Anchors, model.Anchor{Path: p})
		}
		return u
	}
	units := []model.Unit{
		unit("MOD-0001", "core", "", "src/core/a.go"),
		unit("MOD-0002", "샘플 요청과 핸들러 samples", "", "x/y.cs"),
		unit("MOD-0003", "billing", "", "examples/billing/a.go"),
		unit("MOD-0004", "testing", "", "src/testing/a.go"),
		unit("MOD-0005", "tests helpers", "MOD-0001", "src/core/tests/a.go"),
		unit("MOD-0006", "generator", "", "tools/generator/a.go"),
		unit("MOD-0007", "api", "", "src/api/server.go", "src/api/testdata/x.json"),
		unit("MOD-0008", "web", "", "web/__tests__/a.js", "web/__tests__/b.js"),
	}
	got := unscoped(units, store.Overview{})
	want := []string{
		`MOD-0002 샘플 요청과 핸들러 samples (its name says "samples")`,
		`MOD-0003 billing (its anchor examples/billing/a.go lies under "examples")`,
		`MOD-0008 web (its anchor web/__tests__/a.js lies under "__tests__")`,
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("unscoped =\n%q\nwant\n%q", got, want)
	}
	marks := store.Overview{Scopes: []store.Part[model.ScopeMark]{
		{Claim: model.ScopeMark{Unit: "MOD-0002", Scope: model.ScopeSample}},
		{Claim: model.ScopeMark{Unit: "MOD-0003", Scope: model.ScopeSample}},
		{Claim: model.ScopeMark{Unit: "MOD-0008", Scope: model.ScopeTool}},
	}}
	if got := unscoped(units, marks); len(got) != 0 {
		t.Errorf("marked boxes are still named: %q", got)
	}
}

// A scope entry the record cannot read is refused with a message that names
// the entry, never a panic: an empty unit, a unit the state does not hold, a
// scope outside the three and an entry with no code.
func TestABadScopeEntryIsNamedNotPanicked(t *testing.T) {
	t.Parallel()
	p := notesProject(t)
	p.code(".abbd/project.yaml", p.read("project.yaml")+"language: ko\n")
	quote := `evidence:
      - {path: ledger/book.go, lines: "6", symbol: Post, quote: "func Post() {}"}`
	with := func(entry string) string {
		return strings.Replace(chargeOverview, "\n---\n\n장부는", "\nscope:\n"+entry+"\n---\n\n장부는", 1)
	}
	p.overview(1, "scope", with("  - unit: \"\"\n    scope: sample\n    "+quote))
	p.overview(1, "scope", with("  - {}"))
	p.overview(1, "scope", with("  - unit:\n    scope:\n    evidence:"))
	p.overview(1, "MOD-0099", with("  - unit: MOD-0099\n    scope: sample\n    "+quote))
	p.overview(1, "one of sample, artifact, tool", with("  - unit: MOD-0002\n    scope: \"\"\n    "+quote))
	p.overview(1, "cites no code", with("  - unit: MOD-0002\n    scope: tool\n    evidence: []"))
	p.overview(1, "MOD-0002", with("  - unit: MOD-0002\n    scope: sample\n    "+quote+"\n  - unit: MOD-0002\n    scope: tool\n    "+quote))
}

// A stored overview whose scope entry went bad by hand reads as a warning
// in status and lint, not a crash.
func TestAHandBrokenStoredScopeEntryDoesNotCrashStatus(t *testing.T) {
	t.Parallel()
	p := notesProject(t)
	p.code(".abbd/project.yaml", p.read("project.yaml")+"language: ko\n")
	p.overview(0, "stored the overview", chargeOverview)
	stored := p.read("overview.md")
	p.code(".abbd/overview.md", strings.Replace(stored, "\n---\n", "\nscope:\n  - unit: \"\"\n    scope: sample\n---\n", 1))
	p.want(1, "scope 1", "status", "--check")
	p.want(0, "", "lint")
}

// MediatR keeps its library in one box and the samples, the benchmarks and
// the tests in boxes of their own. The three are named until the overview
// marks them, so a drawer can tell them from the product's parts.
func TestAMediatRLikeRepositoryNamesItsSampleBoxesUntilMarked(t *testing.T) {
	t.Parallel()
	unit := func(id, name string, paths ...string) model.Unit {
		u := model.Unit{ID: model.ID(id), Kind: model.Module, Name: model.Said(model.Script, name)}
		for _, p := range paths {
			u.Anchors = append(u.Anchors, model.Anchor{Path: p})
		}
		return u
	}
	units := []model.Unit{
		unit("MOD-0001", "MediatR", "src/MediatR/Mediator.cs"),
		unit("MOD-0002", "MediatR.Examples", "samples/MediatR.Examples/Pinged.cs"),
		unit("MOD-0003", "MediatR.Benchmarks", "samples/MediatR.Benchmarks/Program.cs"),
		unit("MOD-0004", "MediatR.Tests", "test/MediatR.Tests/PublishTests.cs"),
		unit("MOD-0005", "MediatR.Extensions", "src/MediatR.Extensions/Registry.cs"),
	}
	got := unscoped(units, store.Overview{})
	if len(got) != 3 || !strings.HasPrefix(got[0], "MOD-0002 ") || !strings.HasPrefix(got[1], "MOD-0003 ") || !strings.HasPrefix(got[2], "MOD-0004 ") {
		t.Fatalf("the three sample boxes are named, the library and its extension are not: %q", got)
	}
	var marks store.Overview
	for _, id := range []model.ID{"MOD-0002", "MOD-0003"} {
		marks.Scopes = append(marks.Scopes, store.Part[model.ScopeMark]{Claim: model.ScopeMark{Unit: id, Scope: model.ScopeSample}})
	}
	marks.Scopes = append(marks.Scopes, store.Part[model.ScopeMark]{Claim: model.ScopeMark{Unit: "MOD-0004", Scope: model.ScopeTool}})
	if got := unscoped(units, marks); len(got) != 0 {
		t.Errorf("marked boxes are still named: %q", got)
	}
}

// Lint prints the boxes named or placed like samples and asks for a mark.
func TestLintAsksAScopeMarkOfAnExamplesBox(t *testing.T) {
	t.Parallel()
	var out strings.Builder
	printLint(env{stdout: &out}, lintReport{Unscoped: []string{`MOD-0003 billing (its anchor examples/billing/a.go lies under "examples")`}})
	for _, want := range []string{"1 box named or placed like samples", "carry no scope mark", "MOD-0003 billing"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("lint output lacks %q: %s", want, out.String())
		}
	}
	out.Reset()
	printLint(env{stdout: &out}, lintReport{})
	if strings.Contains(out.String(), "scope") {
		t.Errorf("a clean report names no scope: %s", out.String())
	}
}

// An overview that holds only scope marks still prints them in its draft.
func TestAScopeOnlyOverviewKeepsItsMarksInTheDraft(t *testing.T) {
	t.Parallel()
	p := notesProject(t)
	p.code(".abbd/project.yaml", p.read("project.yaml")+"language: ko\n")
	only := "---\nscope:\n  - unit: MOD-0002\n    scope: sample\n    evidence:\n      - {path: ledger/book.go, lines: \"6\", symbol: Post, quote: \"func Post() {}\"}\n---\n"
	p.overview(0, "1 scope marks", only)
	if out := p.want(0, "scope: sample", "overview"); !strings.Contains(out, "unit: MOD-0002") {
		t.Errorf("the draft keeps the mark: %s", out)
	}
}
