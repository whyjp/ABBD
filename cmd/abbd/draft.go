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
// where a package says what it is.
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
	slices.Sort(out)
	return out
}

// declares matches a line that declares symbol in the common languages,
// a Go method's receiver included.
func declares(symbol string) *regexp.Regexp {
	return regexp.MustCompile(`^\s*((export|pub(\([^)]*\))?|async|static|public|private|protected|default)\s+)*(func|def|fn|class|type|struct|enum|trait|interface|const|var|let|function|impl|mod)\s+(\([^)]*\)\s*)?\*?` + regexp.QuoteMeta(symbol) + `\b`)
}

var commentMark = regexp.MustCompile(`^\s*(///?!?|#!?|--|/\*\*?|\*/?|"""|''')\s?`)

// docComment is the comment block above the line that declares symbol, or
// with no symbol the file's first comment block that is no licence. A
// Markdown file gives its first paragraph instead.
func docComment(text, symbol string, md bool) string {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	if md {
		return firstParagraph(lines)
	}
	if symbol != "" {
		decl, word := declares(symbol), regexp.MustCompile(`\b`+regexp.QuoteMeta(symbol)+`\b`)
		for _, re := range []*regexp.Regexp{decl, word} {
			for i, l := range lines {
				if isComment(l) || !re.MatchString(l) {
					continue
				}
				j := i
				for j > 0 && isComment(lines[j-1]) {
					j--
				}
				return joinComment(lines[j:i])
			}
		}
		return ""
	}
	for i := 0; i < len(lines); i++ {
		if !isComment(lines[i]) || strings.HasPrefix(lines[i], "#!") {
			continue
		}
		j := i
		for j < len(lines) && isComment(lines[j]) {
			j++
		}
		block := joinComment(lines[i:j])
		low := strings.ToLower(block)
		if block != "" && !strings.Contains(low, "copyright") && !strings.Contains(low, "license") && !strings.HasPrefix(low, "go:build") {
			return block
		}
		i = j
	}
	return ""
}

func isComment(l string) bool {
	t := strings.TrimSpace(l)
	return t != "" && commentMark.MatchString(t) && !strings.HasPrefix(t, "#include") && !strings.HasPrefix(t, "#[")
}

func joinComment(lines []string) string {
	var words []string
	for _, l := range lines {
		if s := strings.TrimSpace(commentMark.ReplaceAllString(strings.TrimSpace(l), "")); s != "" {
			words = append(words, s)
		}
	}
	return strings.Join(words, " ")
}

func firstParagraph(lines []string) string {
	var para []string
	front := len(lines) > 0 && strings.TrimSpace(lines[0]) == "---"
	for i, l := range lines {
		t := strings.TrimSpace(l)
		if front {
			if i > 0 && t == "---" {
				front = false
			}
			continue
		}
		switch {
		case t == "" && len(para) > 0:
			return strings.Join(para, " ")
		case t == "", strings.HasPrefix(t, "#"), strings.HasPrefix(t, "<"), strings.HasPrefix(t, "!["), strings.HasPrefix(t, "```"):
			continue
		}
		para = append(para, t)
	}
	return strings.Join(para, " ")
}

// firstSentence cuts a comment to its first sentence, within draftMax.
func firstSentence(s string) string {
	if i := strings.Index(s, ". "); i >= 0 {
		s = s[:i+1]
	}
	if r := []rune(s); len(r) > draftMax {
		s = strings.TrimSpace(string(r[:draftMax-1])) + "…"
	}
	return s
}
