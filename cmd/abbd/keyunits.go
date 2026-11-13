package main

import (
	"bufio"
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/whyjp/abbd/internal/model"
)

// keyUnit is a type or function a module declares, with how many places
// across the repository name it: what a reader of the module must cover.
type keyUnit struct {
	Name, File string
	Uses       int
}

// types match a type declaration in most languages: a type, struct, class,
// enum, trait, interface or union. The name must be followed by what a
// declaration has there: a brace, a parenthesis, a generic list, a colon, an
// equals sign, a semicolon, a base or a type expression such as struct or
// map. A word of prose after "type", as in a help text's "type names", has
// none of these, so it is no declaration.
var types = []*regexp.Regexp{
	regexp.MustCompile(`^\s*(?:export\s+)?(?:pub(?:\([^)]*\))?\s+)?(?:type|struct|class|enum|trait|interface|union)\s+([A-Za-z_]\w{2,})` + afterName),
	// A brace on the next line, as in Allman-style C and C++, leaves the name last.
	regexp.MustCompile(`^\s*(?:export\s+)?(?:pub(?:\([^)]*\))?\s+)?(?:struct|class|enum|trait|interface|union)\s+([A-Za-z_]\w{2,})\s*$`),
	// A Go defined type: the name, then one type expression to the line's end.
	// A lone lower-case word counts only when it is a builtin type, so prose
	// such as "type that the" is no declaration.
	regexp.MustCompile(`^\s*type\s+([A-Za-z_]\w{2,})\s+(?:[*\[\]]+[A-Za-z_][\w.]*|[A-Za-z_]\w*\.[\w.]+|[A-Z]\w*|string|bool|byte|rune|any|error|u?int(?:8|16|32|64|ptr)?|float(?:32|64)|complex(?:64|128))\s*(?://.*)?$`),
	regexp.MustCompile(`^\s*(?:typedef\s+)?struct\s+([A-Za-z_]\w{2,})\s*\{`),
}

// afterName is what follows a declared type's name: a delimiter, or one of
// the words that start a type expression or a base list.
const afterName = `(?:\s*[{(<:=;\[*]|\s+(?:struct|interface|map|func|chan|extends|implements|where|enum|union|trait)\b)`

// funcs match a function at the left margin in most languages.
var funcs = []*regexp.Regexp{
	regexp.MustCompile(`^func\s+(?:\([^)]*\)\s*)?([A-Za-z_]\w{2,})\s*\(`),
	regexp.MustCompile(`^\s*(?:pub(?:\([^)]*\))?\s+)?(?:async\s+)?fn\s+([A-Za-z_]\w{2,})`),
	regexp.MustCompile(`^\s*(?:export\s+)?(?:async\s+)?(?:def|function)\s+([A-Za-z_]\w{2,})`),
	regexp.MustCompile(`^(?:static\s+|inline\s+|extern\s+)*[A-Za-z_][\w\s\*]*\b([A-Za-z_]\w{2,})\s*\([^;{]*\)?\s*\{?\s*$`),
}

// distinctive is a function name a repository-wide search can count: a
// plain lower-case word such as from or lock names many unrelated things.
func distinctive(name string) bool {
	if strings.HasPrefix(name, "__") || strings.HasPrefix(name, "_") && strings.HasSuffix(name, "_") {
		return false // a Python dunder such as __init__
	}
	return len(name) >= 5 && (strings.Contains(name, "_") || strings.ToLower(name) != name)
}

// plainWord is a lower-case name with no underscore, such as validate or
// names: it names many unrelated things in a repository, so a search for it
// finds none of them in particular.
func plainWord(name string) bool {
	return strings.ToLower(name) == name && !strings.Contains(name, "_")
}

// shouting is a name in capitals, a macro or a fixed-width typedef such as
// U32, which says nothing of the module's design.
func shouting(name string) bool { return strings.ToUpper(name) == name }

// searched is how many of a module's names keyUnits counts across the
// repository, and searchFor how long it may take.
const (
	searched  = 40
	searchFor = 20 * time.Second
)

// keyUnits ranks what a module's files declare by fan-in: how many matches
// of the name the repository's tracked files in the same languages hold
// outside the module, read through one git grep. The first n are the units
// a text about the module covers. A search past searchFor, outside a git
// repository or without git yields none.
func keyUnits(root string, files []string, n int) []keyUnit {
	decl := map[string]string{}
	own := map[string]bool{}
	for _, f := range files {
		own[f] = true
	}
	inside := map[string]int{}
	var words []string
	for _, f := range files {
		if noQuoteFile(f) {
			continue
		}
		file, err := os.Open(filepath.Join(root, filepath.FromSlash(f)))
		if err != nil {
			continue
		}
		s := bufio.NewScanner(file)
		s.Buffer(make([]byte, 1<<20), 1<<20)
		for s.Scan() {
			line := s.Text()
			words = append(words, ident.FindAllString(line, -1)...)
			if name := declaration(line); name != "" {
				if _, seen := decl[name]; !seen {
					decl[name] = f
				}
			}
		}
		file.Close()
	}
	if len(decl) == 0 {
		return nil
	}
	for _, w := range words {
		if _, ok := decl[w]; ok {
			inside[w]++
		}
	}
	var names []string
	for k := range decl {
		names = append(names, k)
	}
	slices.Sort(names)
	// The names the module itself uses most are the ones searched.
	slices.SortStableFunc(names, func(a, b string) int { return inside[b] - inside[a] })
	names = names[:min(len(names), searched)]
	args := []string{"-C", root, "-c", "core.quotePath=false", "grep", "-I", "-o", "-w", "-F"}
	for _, nm := range names {
		args = append(args, "-e", nm)
	}
	args = append(args, "--")
	exts := map[string]bool{}
	for _, f := range files {
		if x := filepath.Ext(f); x != "" && !exts[x] {
			exts[x] = true
			args = append(args, "*"+x)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), searchFor)
	defer cancel()
	out, _ := exec.CommandContext(ctx, "git", args...).Output()
	if ctx.Err() != nil {
		return nil
	}
	uses := map[string]int{}
	for _, line := range bytes.Split(out, []byte("\n")) {
		path, match, ok := bytes.Cut(line, []byte(":"))
		if !ok {
			continue
		}
		if decl[string(match)] != "" && !own[string(path)] {
			uses[string(match)]++
		}
	}
	var ranked []keyUnit
	for _, nm := range names {
		if uses[nm] > 0 {
			ranked = append(ranked, keyUnit{Name: nm, File: decl[nm], Uses: uses[nm]})
		}
	}
	slices.SortStableFunc(ranked, func(a, b keyUnit) int { return b.Uses - a.Uses })
	return ranked[:min(n, len(ranked))]
}

var ident = regexp.MustCompile(`[A-Za-z_]\w*`)

// common are names a language or its standard library uses everywhere,
// which a module may declare again but which say nothing of it: a count of
// them across the repository counts every use of the library's own. Names a
// module often declares as its own, such as Writer or Path, stay out.
var common = func() map[string]bool {
	m := map[string]bool{}
	for _, n := range strings.Fields(`Result Error Err Value Option String class
		Debug Display Clone Default Copy Drop Send Sync Into From Vec Box
		File Self None Some
		try_from as_ref as_str is_empty to_string into_iter from_str as_bytes into_bytes as_slice to_vec to_owned
		from_utf8 as_mut into_inner with_capacity is_some is_none unwrap_or as_path to_str as_ptr from_bytes to_bytes
		get_mut iter_mut toString valueOf hashCode`) {
		m[n] = true
	}
	return m
}()

// declaration is the type or distinctive function a line declares, "" else.
func declaration(line string) string {
	for _, re := range types {
		if m := re.FindStringSubmatch(line); m != nil && !shouting(m[1]) && !common[m[1]] {
			return m[1]
		}
	}
	for _, re := range funcs {
		if m := re.FindStringSubmatch(line); m != nil && distinctive(m[1]) && !shouting(m[1]) && !common[m[1]] && !strings.HasSuffix(m[1], "_t") {
			return m[1]
		}
	}
	return ""
}

// keyUnitsOf ranks the key units of a module's own files.
func (e env) keyUnitsOf(units []model.Unit, id model.ID, n int) []keyUnit {
	files, _ := e.st.Read(units, id)
	return keyUnits(e.root, files, n)
}

// unnamed are the key units a text never names.
func unnamed(text string, keys []keyUnit) []keyUnit {
	var out []keyUnit
	lower := strings.ToLower(text)
	for _, k := range keys {
		if !strings.Contains(lower, strings.ToLower(k.Name)) {
			out = append(out, k)
		}
	}
	return out
}

// testFile is a test, whose helpers are no unit of the module's design.
func testFile(f string) bool {
	base := strings.ToLower(filepath.Base(f))
	return strings.Contains(base, "_test.") || strings.Contains(base, ".test.") || strings.Contains(base, ".spec.") || strings.HasPrefix(base, "test_") ||
		strings.Contains("/"+filepath.ToSlash(f), "/tests/") || strings.Contains("/"+filepath.ToSlash(f), "/test/")
}

// noQuoteFile is a file that is no code of its module: a test, a test fixture or
// a page of prose. Its lines are no design of the module and no use it makes
// of another.
func noQuoteFile(f string) bool { return testFile(f) || fixtureFile(f) || proseFile(f) }

// fixtureFile lies in a directory of test data, such as testdata or fixtures.
func fixtureFile(f string) bool {
	for _, d := range strings.Split(strings.ToLower(filepath.ToSlash(filepath.Dir(f))), "/") {
		switch d {
		case "testdata", "test_data", "fixtures", "fixture", "__fixtures__", "__snapshots__", "snapshots", "golden", "testfiles":
			return true
		}
	}
	return false
}

// proseFile is a page of prose, whose lines name things a text talks about.
func proseFile(f string) bool {
	switch strings.ToLower(filepath.Ext(f)) {
	case ".md", ".markdown", ".rst", ".adoc", ".txt":
		return filepath.Base(f) != "CMakeLists.txt"
	}
	return false
}

// dataFile holds data or configuration, not code of a language.
func dataFile(f string) bool {
	switch strings.ToLower(filepath.Ext(f)) {
	case ".json", ".yaml", ".yml", ".toml", ".xml", ".csv", ".svg", ".html", ".htm", ".lock", ".sum", ".snap", ".ini", ".cfg":
		return true
	}
	return false
}
