package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// bindingShown bounds the binding sites a brief lists.
const bindingShown = 15

// bindings are the patterns of code that ties units together at run time,
// which no import shows: a library loaded by name, a process started, a
// component registered with a registry, a bundle compiled in.
var bindings = []struct {
	kind string
	re   *regexp.Regexp
}{
	{"load", regexp.MustCompile(`\b(\w*dlopen|dlsym|LoadLibrary\w*|GetProcAddress|plugin\.Open|importlib\.import_module|__import__|System\.loadLibrary|libloading)\b`)},
	{"spawn", regexp.MustCompile(`\b(exec\.Command(Context)?|CreateProcess\w*|ShellExecute\w*|posix_spawnp?|execv[pe]?|popen|subprocess\.\w+|child_process|spawnSync|Command::new|process::Command)\b`)},
	{"registry", regexp.MustCompile(`(register_\w*\s*\(|\bRegister\w*\s*\(|\.register\s*\(|@register\b|inventory::submit|typetag::|configurable_component)`)},
	{"bundle", regexp.MustCompile(`\b(include_str!|include_bytes!|go:embed)`)},
}

// bindingSites lists, for the files of a module, each line that binds at run
// time, as PATH:LINE: CODE (KIND). Tests, test data and prose are skipped,
// since they bind nothing the module does.
func bindingSites(root string, files []string) []string {
	var out []string
	more := 0
	for _, f := range files {
		if testFile(f) || strings.Contains("/"+f, "/testdata/") || filepath.Ext(f) == ".md" {
			continue
		}
		file, err := os.Open(filepath.Join(root, filepath.FromSlash(f)))
		if err != nil {
			continue
		}
		s := bufio.NewScanner(file)
		s.Buffer(make([]byte, 1<<20), 1<<20)
		for n := 1; s.Scan(); n++ {
			line := strings.TrimSpace(s.Text())
			for _, b := range bindings {
				if !b.re.MatchString(line) {
					continue
				}
				if len(out) == bindingShown {
					more++
					break
				}
				if r := []rune(line); len(r) > 100 {
					line = string(r[:99]) + "…"
				}
				out = append(out, fmt.Sprintf("`%s:%d`: `%s` (%s)", f, n, line, b.kind))
				break
			}
		}
		file.Close()
	}
	if more > 0 {
		out = append(out, fmt.Sprintf("and %d more", more))
	}
	return out
}
