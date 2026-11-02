package screen

import (
	"regexp"
	"strings"
)

// Understanding is a module's text from pstack's how, split into blocks the
// page renders without trusting any markup in it.
type Understanding struct {
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

// cells splits a table row at its pipes.
func cells(row string) [][]Span {
	row = strings.TrimSuffix(strings.TrimPrefix(row, "|"), "|")
	var out [][]Span
	for _, c := range strings.Split(row, "|") {
		out = append(out, spans(strings.TrimSpace(c)))
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

func blocks(text string) []Block {
	var out []Block
	var para []string
	flush := func() {
		if len(para) > 0 {
			out = append(out, Block{Kind: "p", Parts: spans(strings.Join(para, " "))})
			para = nil
		}
	}
	var fence *Block
	row := false
	for _, line := range strings.Split(text, "\n") {
		t := strings.TrimSpace(line)
		wasRow := row
		row = fence == nil && strings.HasPrefix(t, "|")
		switch {
		case fence != nil && strings.HasPrefix(t, "```"):
			fence.Code = strings.TrimSuffix(fence.Code, "\n")
			out, fence = append(out, *fence), nil
		case fence != nil:
			fence.Code += strings.TrimRight(line, " \t") + "\n"
		case strings.HasPrefix(t, "```"):
			flush()
			fence = &Block{Kind: "pre"}
		case row && wasRow:
			if !tableRule.MatchString(t) {
				out[len(out)-1].Rows = append(out[len(out)-1].Rows, cells(t))
			}
		case row:
			flush()
			out = append(out, Block{Kind: "table", Rows: [][][]Span{cells(t)}})
		case t == "":
			flush()
		case strings.HasPrefix(t, "#"):
			flush()
			out = append(out, Block{Kind: "h", Parts: spans(strings.TrimSpace(strings.TrimLeft(t, "#")))})
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
	if fence != nil {
		fence.Code = strings.TrimSuffix(fence.Code, "\n")
		out = append(out, *fence)
	}
	return out
}
