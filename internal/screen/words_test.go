package screen

import (
	"reflect"
	"slices"
	"testing"

	"github.com/whyjp/abbd/internal/narrative"
)

func cellTexts(b Block) [][]string {
	var out [][]string
	for _, r := range b.Rows {
		var row []string
		for _, c := range r {
			s := ""
			for _, sp := range c {
				s += sp.Text
			}
			row = append(row, s)
		}
		out = append(out, row)
	}
	return out
}

// A states matrix reads as a table, a blank line ends it, and fenced code
// keeps its lines and indentation as written, a # inside it included.
func TestATextDrawsTablesAndCodeAsWritten(t *testing.T) {
	got := blocks("| state | next |\n|:---|---:|\n| `pending` | charged |\n\n| a |\n|---|\n| b |\n```go\nfunc F() {\n\t# not a heading\n}\n```\nafter")
	var kinds []string
	for _, b := range got {
		kinds = append(kinds, b.Kind)
	}
	if want := []string{"table", "table", "pre", "p"}; !reflect.DeepEqual(kinds, want) {
		t.Fatalf("blocks: %v, want %v", kinds, want)
	}
	if want := [][]string{{"state", "next"}, {"pending", "charged"}}; !reflect.DeepEqual(cellTexts(got[0]), want) {
		t.Errorf("first table %v, want %v", cellTexts(got[0]), want)
	}
	if !got[0].Rows[1][0][0].Code {
		t.Error("a cell keeps its `code` mark")
	}
	if want := [][]string{{"a"}, {"b"}}; !reflect.DeepEqual(cellTexts(got[1]), want) {
		t.Errorf("second table %v, want %v", cellTexts(got[1]), want)
	}
	if want := "func F() {\n\t# not a heading\n}"; got[2].Code != want {
		t.Errorf("code %q, want %q", got[2].Code, want)
	}
}

// An escaped pipe stays in its cell, and a tilde fence is code.
func TestATableKeepsEscapedPipesAndTildesFenceCode(t *testing.T) {
	got := blocks("| state | key |\n|---|---|\n| pending | `a \\| b` |\n\n~~~json\n{\"a\": 1}\n~~~")
	if want := [][]string{{"state", "key"}, {"pending", "a | b"}}; !reflect.DeepEqual(cellTexts(got[0]), want) {
		t.Errorf("table %v, want %v", cellTexts(got[0]), want)
	}
	if len(got) != 2 || got[1].Kind != "pre" || got[1].Code != `{"a": 1}` {
		t.Errorf("tilde fence: %+v", got[1:])
	}
}

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
			code = append(code, b.Code)
		}
	}
	if !slices.Equal(page, lint) {
		t.Errorf("blueprint headings = %v, lint sections = %v", page, lint)
	}
	if want := []string{"# build it\nmake"}; !slices.Equal(code, want) {
		t.Errorf("code blocks = %q, want %q", code, want)
	}
}
