package screen

import (
	"slices"
	"testing"

	"github.com/whyjp/abbd/internal/narrative"
)

// The blueprint and the lint split a text at the same headings, so a #
// line inside a fence is code on both.
func TestTheBlueprintSplitsATextWhereTheLintDoes(t *testing.T) {
	text := "## Overview\n\nIt builds.\n\n```sh\n# build it\nmake\n```\n\n## Key Concepts\n\n- **x**: y\n"
	var lint, page []string
	for _, s := range narrative.Sections(text) {
		lint = append(lint, s.Heading)
	}
	var code []string
	for _, b := range blocks(text) {
		switch b.Kind {
		case "h":
			page = append(page, b.Parts[0].Text)
		case "pre":
			code = append(code, b.Parts[0].Text)
		}
	}
	if !slices.Equal(page, lint) {
		t.Errorf("blueprint headings = %v, lint sections = %v", page, lint)
	}
	if want := []string{"# build it\nmake"}; !slices.Equal(code, want) {
		t.Errorf("code blocks = %q, want %q", code, want)
	}
}
