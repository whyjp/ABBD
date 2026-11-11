package screen

import (
	"path"
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
	// Lead is the text's first section, which the page shows first, and
	// Rest the sections after it, which it folds.
	Lead []Block `json:"-"`
	Rest []Block `json:"-"`
}

// split cuts blocks at the second heading: the first section, the
// summary a reader meets first, and the rest. A text with no second
// heading is all lead.
func split(bs []Block) (lead, rest []Block) {
	heads := 0
	for i, b := range bs {
		if b.Kind == "h" {
			if heads++; heads == 2 {
				return bs[:i], bs[i:]
			}
		}
	}
	return bs, nil
}

// concepts maps each list item of a text to the inside units it names: a
// code span equal to the base name of a file the unit is anchored at, or to
// the unit's ID. So a Key Concept stands by the box it describes.
func concepts(bs []Block, units []InsideUnit) map[string][]Block {
	out := map[string][]Block{}
	for _, b := range bs {
		if b.Kind != "li" {
			continue
		}
		for _, u := range units {
			if namesUnit(b, u) {
				out[u.ID] = append(out[u.ID], b)
			}
		}
	}
	return out
}

func namesUnit(b Block, u InsideUnit) bool {
	for _, s := range b.Parts {
		if !s.Code {
			continue
		}
		if s.Text == u.ID {
			return true
		}
		for _, a := range u.Anchors {
			file, _, _ := strings.Cut(a, " ")
			if file != "" && s.Text == path.Base(file) && strings.Contains(path.Base(file), ".") {
				return true
			}
		}
	}
	return false
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
