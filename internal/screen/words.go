package screen

import (
	"regexp"
	"strings"

	"github.com/whyjp/abbd/internal/narrative"
)

// Understanding is a module's text from a reader, split into blocks the
// page renders without trusting any markup in it.
type Understanding struct {
	// Reader names who read the code and wrote the text, when it was named.
	Reader string `json:",omitempty"`
	Stale  bool
	Blocks []Block
}

type Block struct {
	// Kind is "h" for a heading, "li" for a list item, "p" for a paragraph,
	// "table" for a pipe table, whose Rows start with its header, and "pre"
	// for fenced code, whose Code is its lines as written.
	Kind  string
	Parts []Span     `json:",omitempty"`
	Rows  [][][]Span `json:",omitempty"`
	Code  string     `json:",omitempty"`
}

// tableRule is a table's line under its header, such as |---|:--:|.
var tableRule = regexp.MustCompile(`^\|?(\s*:?-+:?\s*\|)*\s*:?-+:?\s*\|?$`)

// cells splits a table row at its pipes, keeping an escaped \| in its cell.
func cells(row string) [][]Span {
	row = strings.ReplaceAll(row, `\|`, "\x00")
	row = strings.TrimSuffix(strings.TrimPrefix(row, "|"), "|")
	var out [][]Span
	for _, c := range strings.Split(row, "|") {
		out = append(out, spans(strings.ReplaceAll(strings.TrimSpace(c), "\x00", "|")))
	}
	return out
}

type Span struct {
	Text string
	Code bool
	Bold bool `json:",omitempty"`
}

// spans reads the inline marks how's text uses: `code` and **bold**.
func spans(s string) []Span {
	var out []Span
	for i, part := range strings.Split(s, "`") {
		if i%2 == 1 {
			if part != "" {
				out = append(out, Span{Text: part, Code: true})
			}
			continue
		}
		for j, piece := range strings.Split(part, "**") {
			if piece != "" {
				out = append(out, Span{Text: piece, Bold: j%2 == 1})
			}
		}
	}
	return out
}

// blocks reads a text's lines as narrative.Lines does, so the page and the
// lint agree on its headings and its code.
func blocks(text string) []Block {
	var out []Block
	var para, code []string
	inCode := false
	flush := func() {
		if len(para) > 0 {
			out = append(out, Block{Kind: "p", Parts: spans(strings.Join(para, " "))})
			para = nil
		}
		if inCode {
			out = append(out, Block{Kind: "pre", Code: strings.Join(code, "\n")})
			code, inCode = nil, false
		}
	}
	row := false
	for _, l := range narrative.Lines(text) {
		t := l.Text
		wasRow := row
		row = l.Kind == narrative.Prose && strings.HasPrefix(t, "|")
		switch {
		case l.Kind == narrative.Code:
			code = append(code, t)
		case l.Kind == narrative.Fence:
			opens := !inCode
			flush()
			inCode = opens
		case l.Kind == narrative.Heading:
			flush()
			out = append(out, Block{Kind: "h", Parts: spans(t)})
		case row && wasRow:
			if !tableRule.MatchString(t) {
				out[len(out)-1].Rows = append(out[len(out)-1].Rows, cells(t))
			}
		case row:
			flush()
			out = append(out, Block{Kind: "table", Rows: [][][]Span{cells(t)}})
		case t == "":
			flush()
		case strings.HasPrefix(t, "- "), strings.HasPrefix(t, "* "):
			flush()
			out = append(out, Block{Kind: "li", Parts: spans(t[2:])})
		case len(t) > 2 && t[0] >= '0' && t[0] <= '9' && strings.Contains(t[:min(4, len(t))], ". "):
			flush()
			_, rest, _ := strings.Cut(t, ". ")
			out = append(out, Block{Kind: "li", Parts: spans(rest)})
		default:
			para = append(para, t)
		}
	}
	flush()
	return out
}
