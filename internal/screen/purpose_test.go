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
	if want := `<p class="muted standing">제공 1 · 사용 2 · 계약 없는 직접 의존 1 · <span class="pill STALE">설명 STALE</span> · 죽은 근거 1개</p>`; !strings.Contains(ps["MOD-0003.html"], want) {
		t.Errorf("MOD-0003.html should say %s", want)
	}
}
