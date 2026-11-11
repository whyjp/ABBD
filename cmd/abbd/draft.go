package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/whyjp/abbd/internal/model"
)

// draftMax bounds a drafted responsibility to one line of a box.
const draftMax = 160

// draft is a one-line responsibility for a unit that records none, with the
// file it came from. It comes from the Go doc comment of a symbol the unit
// is anchored on, else from the package doc or header comment of an anchored
// file or of a directory's entry files. A unit with neither gets no draft:
// a missing draft costs the reader a read, a misleading one a wrong box. A
// reader keeps or rewrites it; nothing stores it unread.
func (e env) draft(u model.Unit) (string, string) {
	for _, a := range u.Anchors {
		if a.Symbol == "" || filepath.Ext(a.Path) != ".go" {
			continue
		}
		if s := firstSentence(goSymbolDoc(e.read(a.Path), a.Symbol)); s != "" {
			return s, a.Path
		}
	}
	for _, a := range u.Anchors {
		for _, f := range e.anchorFiles(a.Path) {
			if s := firstSentence(header(e.read(f), filepath.Ext(f) == ".go")); s != "" {
				return s, f
			}
		}
	}
	return "", ""
}

func (e env) read(f string) string {
	b, err := os.ReadFile(filepath.Join(e.root, filepath.FromSlash(f)))
	if err != nil {
		return ""
	}
	return string(b)
}

// anchorFiles is the anchor's file, or for a directory its entry files of
// code, where a package says what it is. Prose such as a README is left
// out: its opening paragraph says what the project is, not the unit.
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
		if !d.IsDir() && entryRank(f) == 0 && filepath.Ext(f) != ".md" {
			out = append(out, f)
		}
	}
	slices.Sort(out)
	return out
}

// goSymbolDoc is the doc comment Go attaches to symbol's declaration: a
// function or method of that name, or a type, constant or variable.
func goSymbolDoc(src, symbol string) string {
	f, err := parser.ParseFile(token.NewFileSet(), "", src, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		return ""
	}
	for _, d := range f.Decls {
		switch d := d.(type) {
		case *ast.FuncDecl:
			if d.Name.Name == symbol {
				return d.Doc.Text()
			}
		case *ast.GenDecl:
			for _, s := range d.Specs {
				var names []*ast.Ident
				var spec *ast.CommentGroup
				switch s := s.(type) {
				case *ast.TypeSpec:
					names, spec = []*ast.Ident{s.Name}, s.Doc
				case *ast.ValueSpec:
					names, spec = s.Names, s.Doc
				}
				if slices.ContainsFunc(names, func(n *ast.Ident) bool { return n.Name == symbol }) {
					if spec != nil {
						return spec.Text()
					}
					return d.Doc.Text()
				}
			}
		}
	}
	return ""
}

// header is what a file says of itself: a Go file's package doc, or for
// another language its first comment before any code that is no licence,
// generated-file mark or pragma.
func header(src string, goFile bool) string {
	if goFile {
		f, err := parser.ParseFile(token.NewFileSet(), "", src, parser.PackageClauseOnly|parser.ParseComments)
		if err != nil || f.Doc == nil || boilerplate(f.Doc.Text()) {
			return ""
		}
		return f.Doc.Text()
	}
	lines := strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n")
	isC := comments(lines)
	for i := 0; i < len(lines); i++ {
		t := strings.TrimSpace(lines[i])
		if !isC[i] {
			if t == "" || strings.HasPrefix(t, "#!") {
				continue
			}
			return ""
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

// preprocessor are lines that start with # and are code, not comments.
var preprocessor = regexp.MustCompile(`^#\s*(include|define|undef|if|ifdef|ifndef|elif|else|endif|pragma|import|error|line)\b|^#\[|^#!`)

// lineMark is a comment's opening on one line; blockOpen and blockClose a
// comment that runs over lines, or a Python docstring.
var (
	lineMark   = regexp.MustCompile(`^(///?!?|#)\s?`)
	blockOpen  = regexp.MustCompile(`^(/\*\*?|"""|''')\s?`)
	blockClose = regexp.MustCompile(`\s*(\*/|"""|''')\s*$`)
	blockInner = regexp.MustCompile(`^\*\s?`)
	pragma     = regexp.MustCompile(`-\*-|\bvim?:|^#\s*(coding|encoding)[:=]`)
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

// boilerplate is a header comment that says nothing of the code: a licence
// or a generated-file mark.
func boilerplate(block string) bool {
	low := strings.ToLower(block)
	for _, w := range []string{"copyright", "spdx-license", "licensed under", "license, version", "code generated", "do not edit", "auto-generated", "autogenerated"} {
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
		if pragma.MatchString(t) {
			continue
		}
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

// abbreviations end in a period that ends no sentence.
var abbreviations = []string{"e.g.", "i.e.", "etc.", "vs.", "cf."}

// firstSentence cuts a comment to its first sentence on one line, within
// draftMax.
func firstSentence(s string) string {
	s = strings.Join(strings.Fields(s), " ")
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
