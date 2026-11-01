package screen

import (
	"strings"
)

// Understanding is a module's text from pstack's how, split into blocks the
// page renders without trusting any markup in it.
type Understanding struct {
	Stale  bool
	Blocks []Block
}

type Block struct {
	// Kind is "h" for a heading, "li" for a list item, "p" for a paragraph.
	Kind  string
	Parts []Span
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
	for _, line := range strings.Split(text, "\n") {
		t := strings.TrimSpace(line)
		switch {
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
	return out
}
