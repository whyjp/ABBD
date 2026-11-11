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
// enum, trait, interface or union.
var types = []*regexp.Regexp{
	regexp.MustCompile(`^\s*(?:export\s+)?(?:pub(?:\([^)]*\))?\s+)?(?:type|struct|class|enum|trait|interface|union)\s+([A-Za-z_]\w{2,})`),
	regexp.MustCompile(`^\s*(?:typedef\s+)?struct\s+([A-Za-z_]\w{2,})\s*\{`),
}

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
	return len(name) >= 5 && (strings.Contains(name, "_") || strings.ToLower(name) != name)
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
// of the name the repository's files in the same languages hold outside its
// declaring file, read through one git grep. The first n are the units a
// text about the module covers. A search past searchFor yields none.
func keyUnits(root string, files []string, n int) []keyUnit {
	decl := map[string]string{}
	inside := map[string]int{}
	var words []string
	for _, f := range files {
		if testFile(f) {
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
	args := []string{"-C", root, "grep", "-I", "-o", "-w", "-F"}
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
		if f := decl[string(match)]; f != "" && f != string(path) {
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
// which a module may declare again but which say nothing of it.
var common = map[string]bool{"Result": true, "Error": true, "Err": true, "Value": true, "Option": true, "String": true, "Config": true, "Builder": true, "Context": true, "Handle": true, "State": true, "class": true, "Item": true, "try_from": true, "as_ref": true, "as_str": true, "is_empty": true, "to_string": true, "into_iter": true, "from_str": true}

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
	for _, k := range keys {
		if !strings.Contains(text, k.Name) {
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
