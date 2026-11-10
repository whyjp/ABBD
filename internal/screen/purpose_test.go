package screen

import (
	"strings"
	"testing"

	"github.com/whyjp/abbd/internal/model"
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
