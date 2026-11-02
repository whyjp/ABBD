package screen

import (
	"reflect"
	"testing"
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
