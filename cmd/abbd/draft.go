package main

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/whyjp/abbd/internal/model"
)

// draftMax bounds a drafted responsibility to one line of a box.
const draftMax = 160

// draft is a one-line responsibility for a unit that records none, taken
// from the comment at its first anchor that carries one, with the file it
// came from. A reader keeps or rewrites it; nothing stores it unread.
func (e env) draft(u model.Unit) (string, string) {
	for _, a := range u.Anchors {
		for _, f := range e.anchorFiles(a.Path) {
			b, err := os.ReadFile(filepath.Join(e.root, filepath.FromSlash(f)))
			if err != nil {
				continue
			}
			if s := firstSentence(docComment(string(b), a.Symbol, filepath.Ext(f) == ".md")); s != "" {
				return s, f
			}
		}
	}
	return "", ""
}

// anchorFiles is the anchor's file, or for a directory its entry files,
// code first, where a package says what it is.
func (e env) anchorFiles(p string) []string {
	fi, err := os.Stat(filepath.Join(e.root, filepath.FromSlash(p)))
	if err != nil {
		return nil
	}
	if !fi.IsDir() {
		return []string{p}
	}
	ents, err := os.ReadDir(filepath.Join(e.root, filepath.FromSlash(p)))
	if err != nil {
		return nil
	}
	var out []string
	for _, d := range ents {
		f := strings.TrimSuffix(p, "/") + "/" + d.Name()
		if !d.IsDir() && entryRank(f) == 0 {
			out = append(out, f)
		}
	}
	slices.SortFunc(out, func(a, b string) int {
		if am, bm := filepath.Ext(a) == ".md", filepath.Ext(b) == ".md"; am != bm {
			if am {
				return 1
			}
			return -1
		}
		return strings.Compare(a, b)
	})
	return out
}

// declares matches a line that declares symbol: after a keyword in the
// common languages, a Go method's receiver included, or as a C, C++, Java
// or C# function or method that a body follows.
func declares(symbol string) *regexp.Regexp {
	q := regexp.QuoteMeta(symbol)
	return regexp.MustCompile(`^\s*(((export|pub(\([^)]*\))?|async|static|public|private|protected|default|abstract|final|unsafe)\s+)*(func|def|fn|class|type|struct|enum|trait|interface|const|var|let|function|impl|mod|record|object)\s+(\([^)]*\)\s*)?\*?` + q + `\b|([\w:<>\[\],*&]+\s+)+[*&]?` + q + `\s*\([^;]*$)`)
}

// preprocessor are C lines that start with # and are code, not comments.
var preprocessor = regexp.MustCompile(`^#\s*(include|define|undef|if|ifdef|ifndef|elif|else|endif|pragma|import|error|line)\b|^#\[|^#!`)

// lineMark is a comment's opening on one line; blockOpen and blockClose a
// comment that runs over lines, or a Python docstring.
var (
	lineMark   = regexp.MustCompile(`^(///?!?|#|-- )\s?`)
	blockOpen  = regexp.MustCompile(`^(/\*\*?|"""|''')\s?`)
	blockClose = regexp.MustCompile(`\s*(\*/|"""|''')\s*$`)
	blockInner = regexp.MustCompile(`^\*\s?`)
)

// comments marks each line that is a comment, tracking the comments that
// run over lines, so a C line that starts with * or # is not one.
func comments(lines []string) []bool {
	out := make([]bool, len(lines))
	closer := ""
	for i, l := range lines {
		t := strings.TrimSpace(l)
		if closer != "" {
			out[i] = true
			if strings.Contains(t, closer) {
				closer = ""
			}
			continue
		}
		if m := blockOpen.FindString(t); m != "" {
			out[i] = true
			c := strings.TrimSpace(m)
			if strings.HasPrefix(c, "/*") {
				c = "*/"
			}
			if !strings.Contains(t[len(m):], c) {
				closer = c
			}
			continue
		}
		out[i] = t != "" && lineMark.MatchString(t) && !preprocessor.MatchString(t)
	}
	return out
}

// docComment is the comment that documents symbol: the block above the line
// that declares it, past attributes and decorators, or the docstring below
// it. With no symbol it is the file's header comment, before any code, that
// is no licence and no generated-file mark. A Markdown file gives its first
// paragraph of prose instead.
func docComment(text, symbol string, md bool) string {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	if md {
		return firstParagraph(lines)
	}
	isC := comments(lines)
	if symbol == "" {
		for i := 0; i < len(lines) && (isC[i] || strings.TrimSpace(lines[i]) == "" || strings.HasPrefix(lines[i], "#!")); i++ {
			if !isC[i] {
				continue
			}
			j := i
			for j < len(lines) && isC[j] {
				j++
			}
			if block := joinComment(lines[i:j]); block != "" && !boilerplate(block) {
				return block
			}
			i = j - 1
		}
		return ""
	}
	decl := declares(symbol)
	for i, l := range lines {
		if isC[i] || !decl.MatchString(l) {
			continue
		}
		j := i
		for j > 0 && attribute(lines[j-1]) {
			j--
		}
		k := j
		for k > 0 && isC[k-1] {
			k--
		}
		if k < j {
			return joinComment(lines[k:j])
		}
		// A Python docstring sits below what it documents.
		k = i + 1
		for k < len(lines) && k-i <= 20 && isC[k] {
			k++
		}
		if k > i+1 && blockOpen.MatchString(strings.TrimSpace(lines[i+1])) {
			return joinComment(lines[i+1 : k])
		}
		return ""
	}
	return ""
}

// attribute is a line between a doc comment and what it documents: a Rust
// attribute, a decorator or an annotation.
func attribute(l string) bool {
	t := strings.TrimSpace(l)
	return strings.HasPrefix(t, "#[") || strings.HasPrefix(t, "@")
}

// boilerplate is a header comment that says nothing of the code: a licence
// or a generated-file mark.
func boilerplate(block string) bool {
	low := strings.ToLower(block)
	for _, w := range []string{"copyright", "spdx-license", "licensed under", "license, version", "code generated", "do not edit", "auto-generated", "autogenerated", "go:build", "+build"} {
		if strings.Contains(low, w) {
			return true
		}
	}
	return false
}

func joinComment(lines []string) string {
	var words []string
	for _, l := range lines {
		t := strings.TrimSpace(l)
		t = blockOpen.ReplaceAllString(t, "")
		t = blockClose.ReplaceAllString(t, "")
		t = lineMark.ReplaceAllString(t, "")
		t = blockInner.ReplaceAllString(t, "")
		if t = strings.TrimSpace(t); t != "" {
			words = append(words, t)
		}
	}
	return strings.Join(words, " ")
}

// firstParagraph is a Markdown file's first paragraph of prose, past its
// frontmatter, headings, badges, HTML, images, tables and fenced code.
func firstParagraph(lines []string) string {
	var para []string
	front := len(lines) > 0 && strings.TrimSpace(lines[0]) == "---"
	fence := false
	for i, l := range lines {
		t := strings.TrimSpace(l)
		if front {
			if i > 0 && t == "---" {
				front = false
			}
			continue
		}
		if strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~") {
			fence = !fence
			continue
		}
		switch {
		case fence:
			continue
		case t == "" && len(para) > 0:
			return strings.Join(para, " ")
		case t == "", strings.HasPrefix(t, "#"), strings.HasPrefix(t, "<"), strings.HasPrefix(t, "!["), strings.HasPrefix(t, "[!["), strings.HasPrefix(t, "|"):
			continue
		}
		para = append(para, t)
	}
	return strings.Join(para, " ")
}

// abbreviations end in a period that ends no sentence.
var abbreviations = []string{"e.g.", "i.e.", "etc.", "vs.", "cf."}

// firstSentence cuts a comment to its first sentence, within draftMax.
func firstSentence(s string) string {
	for from := 0; ; {
		i := strings.Index(s[from:], ". ")
		if i < 0 {
			break
		}
		end := from + i + 1
		if !slices.ContainsFunc(abbreviations, func(a string) bool { return strings.HasSuffix(strings.ToLower(s[:end]), a) }) {
			s = s[:end]
			break
		}
		from = end
	}
	if r := []rune(s); len(r) > draftMax {
		s = strings.TrimSpace(string(r[:draftMax-1])) + "…"
	}
	return s
}
