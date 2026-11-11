package screen

import (
	"strings"
	"testing"

	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/store"
)

// A module's page says what the module is responsible for and why it is
// there, and the overview's box and the contract index carry the sentence.
func TestPagesSayWhatEachModuleIsFor(t *testing.T) {
	units := append(ripgrep(), inside("MOD-0006", "MOD-0003", rel("IF-0002", model.Consumes)))
	for i, u := range units {
		switch u.ID {
		case "MOD-0003":
			units[i].Responsibility = &model.Fact{Value: "runs one search over a file", Origin: model.Inferred}
			units[i].Purpose = &model.Fact{Value: "keeps reading apart from matching", Origin: model.Inferred}
		case "IF-0002":
			units[i].Responsibility = &model.Fact{Value: "carries matches to a sink", Origin: model.Inferred}
		case "MOD-0006":
			units[i].Responsibility = &model.Fact{Value: "writes one match", Origin: model.Inferred}
		}
	}
	ps := pages(t, Build(Input{Units: units}))
	contains := func(file string, wants ...string) {
		t.Helper()
		for _, w := range wants {
			if !strings.Contains(ps[file], w) {
				t.Errorf("%s should hold %q", file, w)
			}
		}
	}
	contains("MOD-0003.html", "<strong>맡은 것</strong> runs one search over a file", "<strong>왜 있는가</strong> keeps reading apart from matching")
	contains("index.html", "<title>MOD-0003 grep-searcher: runs one search over a file")
	contains("MOD-0003.html", `class="read" title="판독: `, "기울인 문장은 리더가 읽은 것입니다")
	contains("contracts.html", "carries matches to a sink")
}

// A module's page counts what the module provides and uses, the units it
// reaches with no contract, and its STALE text, over its whole subtree.
func TestAModulePageSaysItsStanding(t *testing.T) {
	gone := model.Relation{To: "MOD-0004", Kind: model.DependsOn, Origin: model.Inferred, Writer: model.Reader, Via: "call", Dead: true,
		Evidence: []model.Evidence{{Path: "src/a.rs", Lines: "1", Quote: "gone", Status: model.Dead}}}
	units := append(ripgrep(), inside("MOD-0006", "MOD-0003", rel("IF-0002", model.Consumes), gone))
	in := Input{Units: units, Understandings: map[model.ID]store.Understanding{"MOD-0003": {Unit: "MOD-0003", Digest: "old", Text: "### Overview\nIt searches.\n"}}, Digests: map[model.ID]string{"MOD-0003": "new"}}
	ps := pages(t, Build(in))
	if want := `<p class="muted standing">계약 1개를 내놓고 2개를 씁니다. 계약 없이 1곳에 직접 의존합니다. 설명은 코드가 바뀐 뒤 다시 읽히지 않았습니다 <span class="pill STALE">STALE</span>. 근거 1개가 코드에서 사라졌습니다.</p>`; !strings.Contains(ps["MOD-0003.html"], want) {
		t.Errorf("MOD-0003.html should say %s", want)
	}
}

// A reader's coupling says whether its evidence still holds, and no page
// offers a 확인 tier.
func TestAReaderClaimSaysWhetherItsEvidenceHolds(t *testing.T) {
	held := model.Relation{To: "MOD-0004", Kind: model.DependsOn, Origin: model.Inferred, Writer: model.Reader, Via: "call",
		Evidence: []model.Evidence{{Path: "src/a.rs", Lines: "1", Quote: "held", Status: model.Live}}}
	moved := model.Relation{To: "MOD-0005", Kind: model.DependsOn, Origin: model.Inferred, Writer: model.Reader, Via: "call",
		Evidence: []model.Evidence{{Path: "src/a.rs", Lines: "2", Quote: "moved", Status: model.Drifted}}}
	ps := pages(t, Build(Input{Units: append(ripgrep(), inside("MOD-0006", "MOD-0003", held, moved))}))
	page := ps["MOD-0003.html"]
	if !strings.Contains(page, `<span class="muted">근거 유지</span>`) || !strings.Contains(page, `근거 어긋남</span>`) {
		t.Errorf("each reader's coupling should say whether its evidence holds")
	}
	for f, p := range ps {
		if strings.Contains(p, "확인:") || strings.Contains(p, ">확인<") {
			t.Errorf("%s still offers a 확인 tier", f)
		}
	}
}

// A module's text shows its first section and folds the rest, and a Key
// Concept that names a unit's file stands by that unit's entry.
func TestATextLeadsWithItsSummaryAndTiesConceptsToBoxes(t *testing.T) {
	units := append(ripgrep(), inside("MOD-0006", "MOD-0003"))
	for i := range units {
		if units[i].ID == "MOD-0006" {
			units[i].Anchors = []model.Anchor{{Path: "crates/searcher/sink.rs"}}
		}
	}
	text := "## Overview\nThe searcher runs one search.\n\n## Key Concepts\n- **Sink** (`sink.rs`): takes each match.\n- **Other** (`none.rs`): names no box.\n"
	in := Input{Units: units, Understandings: map[model.ID]store.Understanding{"MOD-0003": {Unit: "MOD-0003", Digest: "d", Text: text}}, Digests: map[model.ID]string{"MOD-0003": "d"}}
	page := pages(t, Build(in))["MOD-0003.html"]
	lead, rest, ok := strings.Cut(page, `<details class="more">`)
	if !ok || !strings.Contains(lead, "The searcher runs one search.") || !strings.Contains(rest, "takes each match.") {
		t.Fatalf("the summary should come first and the Key Concepts fold")
	}
	if !strings.Contains(page, "핵심 개념 1개") {
		t.Errorf("the concept naming sink.rs should stand by MOD-0006's entry")
	}
}

// The lead ends where the first section ends: at the first heading when
// prose comes before it. A file name two units share ties a concept to
// neither.
func TestALeadEndsWithTheFirstSection(t *testing.T) {
	lead, rest := split(blocks("Intro prose.\n\n## Overview\nMore.\n"))
	if len(lead) != 1 || len(rest) != 2 {
		t.Errorf("an intro before the first heading is the whole lead: %d, %d", len(lead), len(rest))
	}
	if lead, rest := split(blocks("No heading at all.\n")); len(lead) != 1 || rest != nil {
		t.Errorf("a text with no heading is all lead")
	}
	units := []InsideUnit{{ID: "MOD-0001", Anchors: []string{"a/mod.rs"}}, {ID: "MOD-0002", Anchors: []string{"b/mod.rs"}}}
	if got := concepts(blocks("- **Mod** (`mod.rs`): shared.\n"), units); len(got) != 0 {
		t.Errorf("a shared file name tied a concept: %v", got)
	}
}

// A reader's note on why a relation exists stands by that coupling, and
// every decision note is listed on decisions.html.
func TestARelationSaysWhyAndDecisionsHaveAnIndex(t *testing.T) {
	coupled := model.Relation{To: "MOD-0004", Kind: model.DependsOn, Origin: model.Inferred, Writer: model.Reader, Via: "call",
		Evidence: []model.Evidence{{Path: "src/a.rs", Lines: "1", Quote: "held", Status: model.Live}}}
	units := append(ripgrep(), inside("MOD-0006", "MOD-0003", coupled))
	note := func(ref, text string) store.Note {
		r, err := model.ParseNoteRef(ref)
		if err != nil {
			t.Fatal(err)
		}
		return store.Note{Ref: r, Known: true, Text: text, Slot: r.Name()}
	}
	in := Input{Units: units, Notes: map[model.ID][]store.Note{
		"MOD-0006": {note("MOD-0006.relations."+model.RelationKey(model.DependsOn, "MOD-0004"), "The sink hands each match to the matcher.")},
		"MOD-0003": {note("MOD-0003.decisions.mmap", "Searches read files through mmap rather than a buffer.")},
	}}
	ps := pages(t, Build(in))
	if !strings.Contains(ps["MOD-0003.html"], `<div class="why">`) || !strings.Contains(ps["MOD-0003.html"], "The sink hands each match to the matcher.") {
		t.Errorf("the coupling should carry the note on why it exists")
	}
	if d := ps[DecisionsFile]; !strings.Contains(d, "Searches read files through mmap rather than a buffer.") || !strings.Contains(d, `href="MOD-0003.html#n-MOD-0003.decisions.mmap"`) {
		t.Errorf("decisions.html should list the decision and link it:\n%s", d)
	}
}
