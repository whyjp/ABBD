package main

import (
	"bufio"
	"bytes"
	"context"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/whyjp/abbd/internal/model"
)

// keyUnit is a type or function a module declares, with how many files
// outside the module name it: a candidate for what a user of the module
// touches first.
type keyUnit struct {
	Name, File string
	Users      int
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

// codeExts are the extensions of source files whose declarations keyUnits
// reads. A licence, a readme or a build file holds prose that the
// declaration patterns misread as code.
var codeExts = map[string]bool{
	".go": true, ".rs": true, ".c": true, ".h": true, ".cc": true, ".cpp": true, ".cxx": true, ".hpp": true, ".hh": true, ".hxx": true,
	".m": true, ".mm": true, ".py": true, ".js": true, ".jsx": true, ".mjs": true, ".cjs": true, ".ts": true, ".tsx": true, ".mts": true,
	".java": true, ".kt": true, ".scala": true, ".swift": true, ".cs": true, ".rb": true, ".php": true, ".lua": true, ".zig": true,
	".dart": true, ".ex": true, ".exs": true, ".erl": true, ".hs": true, ".ml": true, ".fs": true, ".vue": true, ".svelte": true,
}

func codeFile(f string) bool { return codeExts[strings.ToLower(filepath.Ext(f))] }

// distinctive is a function name a repository-wide search can count: a
// plain lower-case word such as from or lock names many unrelated things.
func distinctive(name string) bool {
	if strings.HasPrefix(name, "__") || strings.HasPrefix(name, "_") && strings.HasSuffix(name, "_") {
		return false // a Python dunder such as __init__
	}
	return len(name) >= 5 && (strings.Contains(name, "_") || strings.ToLower(name) != name)
}

// shouting is a name in capitals, a macro or a fixed-width typedef such as
// U32, which says nothing of the module's design.
func shouting(name string) bool { return strings.ToUpper(name) == name }

// searchFor is how long keyUnits may read the repository.
const searchFor = 20 * time.Second

// keyUnits ranks what a module's code files declare by how many tracked
// files outside the module, in the same languages and tests left out, name
// it. The first n are the names other modules use most. A search past
// searchFor, outside a git repository or without git yields none.
func keyUnits(root string, files []string, n int) []keyUnit {
	decl := map[string]string{}
	own := map[string]bool{}
	var exts []string
	for _, f := range files {
		own[f] = true
		if testFile(f) || !codeFile(f) {
			continue
		}
		if x := "*" + filepath.Ext(f); !slices.Contains(exts, x) {
			exts = append(exts, x)
		}
		file, err := os.Open(filepath.Join(root, filepath.FromSlash(f)))
		if err != nil {
			continue
		}
		s := bufio.NewScanner(file)
		s.Buffer(make([]byte, 1<<20), 1<<20)
		for s.Scan() {
			if name := declaration(s.Text()); name != "" {
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
	ctx, cancel := context.WithTimeout(context.Background(), searchFor)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", append([]string{"-C", root, "-c", "core.quotePath=false", "ls-files", "-z", "--"}, exts...)...).Output()
	if err != nil {
		return nil
	}
	users := map[string]int{}
	for _, p := range strings.Split(string(out), "\x00") {
		if p == "" || own[p] || testFile(p) {
			continue
		}
		if ctx.Err() != nil {
			return nil
		}
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(p)))
		if err != nil || bytes.IndexByte(data[:min(len(data), 8000)], 0) >= 0 {
			continue
		}
		named := map[string]bool{}
		for _, w := range words(data) {
			if _, ok := decl[string(w)]; ok && !named[string(w)] {
				named[string(w)] = true
				users[string(w)]++
			}
		}
	}
	var ranked []keyUnit
	for _, nm := range slices.Sorted(maps.Keys(users)) {
		ranked = append(ranked, keyUnit{Name: nm, File: decl[nm], Users: users[nm]})
	}
	slices.SortStableFunc(ranked, func(a, b keyUnit) int { return b.Users - a.Users })
	return ranked[:min(n, len(ranked))]
}

// words are the identifiers in b, as a regexp of [A-Za-z_]\w* finds them,
// by hand because a regexp reads a large repository too slowly.
func words(b []byte) [][]byte {
	word := func(c byte) bool {
		return c == '_' || 'a' <= c|0x20 && c|0x20 <= 'z' || '0' <= c && c <= '9'
	}
	var out [][]byte
	for i := 0; i < len(b); {
		if !word(b[i]) {
			i++
			continue
		}
		j := i + 1
		for j < len(b) && word(b[j]) {
			j++
		}
		if c := b[i]; c < '0' || c > '9' {
			out = append(out, b[i:j])
		}
		i = j
	}
	return out
}

// generic are names, in lower case, that a language, its standard library
// or any codebase uses everywhere: a module may declare one again, but it
// says nothing of what a user of the module touches.
var generic = map[string]bool{
	"result": true, "error": true, "err": true, "value": true, "option": true, "options": true, "string": true, "config": true,
	"builder": true, "context": true, "handle": true, "state": true, "class": true, "item": true, "info": true, "param": true,
	"params": true, "input": true, "output": true, "data": true, "type": true, "kind": true, "node": true, "entry": true,
	"base": true, "list": true, "args": true, "flags": true, "settings": true, "default": true, "inner": true, "object": true,
	"license": true, "try_from": true, "as_ref": true, "as_str": true, "is_empty": true, "to_string": true, "into_iter": true,
	"from_str": true, "with_capacity": true, "to_owned": true, "into_owned": true, "into_inner": true, "from_parts": true,
	"file_name": true, "poll_next": true, "poll_ready": true, "target": true,
}

func plain(name string) bool { return shouting(name) || generic[strings.ToLower(name)] }

// declaration is the type or distinctive function a line declares, "" else.
func declaration(line string) string {
	for _, re := range types {
		if m := re.FindStringSubmatch(line); m != nil && !plain(m[1]) {
			return m[1]
		}
	}
	for _, re := range funcs {
		if m := re.FindStringSubmatch(line); m != nil && distinctive(m[1]) && !plain(m[1]) && !strings.HasSuffix(m[1], "_t") {
			return m[1]
		}
	}
	return ""
}

// keyUnitsOf ranks the names other modules use most among a module's own
// declarations.
func (e env) keyUnitsOf(units []model.Unit, id model.ID, n int) []keyUnit {
	files, _ := e.st.Read(units, id)
	return keyUnits(e.root, files, n)
}

// testFile is a test, whose helpers are no unit of the module's design.
func testFile(f string) bool {
	base := strings.ToLower(filepath.Base(f))
	return strings.Contains(base, "_test.") || strings.Contains(base, ".test.") || strings.Contains(base, ".spec.") || strings.HasPrefix(base, "test_") ||
		strings.Contains("/"+filepath.ToSlash(f), "/tests/") || strings.Contains("/"+filepath.ToSlash(f), "/test/")
}
