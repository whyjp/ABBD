package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// bindingShown bounds the binding sites a brief lists of each kind, so a
// long registration table does not hide the one load beside it.
const bindingShown = 5

// bindings are the patterns of code that ties units together at run time,
// which no import shows: a library loaded by name, a process started, a
// component registered with a registry, a bundle compiled in.
var bindings = []struct {
	kind string
	re   *regexp.Regexp
}{
	{"load", regexp.MustCompile(`\b(\w*dlopen|dlsym|LoadLibrary\w*|GetProcAddress|plugin\.Open|importlib\.import_module|__import__|System\.loadLibrary|libloading)\b`)},
	{"spawn", regexp.MustCompile(`\b(exec\.Command(Context)?|CreateProcess\w*|ShellExecute\w*|posix_spawnp?|exec[lv]p?e?|_?popen|subprocess\.(run|Popen|call|check_call|check_output)|os\.system|ProcessBuilder|child_process|spawnSync|Command::new)\b`)},
	{"registry", regexp.MustCompile(`(\bregister_\w*\s*\(|_register_\w*\s*\(|\b(Must)?Register\w*\s*\(|\.register\s*\(|@register\b|inventory::submit|typetag::|configurable_component)`)},
	{"bundle", regexp.MustCompile(`\b(include_str!|include_bytes!|go:embed)`)},
}

// bindingSites lists, for the files of a module, each line that binds at run
// time, as PATH:LINE: CODE (KIND), up to bindingShown of each kind. Tests,
// test data, prose and binary files are skipped, since they bind nothing
// the module does.
func bindingSites(root string, files []string) []string {
	var out []string
	more, shown := 0, map[string]int{}
	for _, f := range files {
		if noQuoteFile(f) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(f)))
		if err != nil || bytes.IndexByte(data[:min(len(data), 8000)], 0) >= 0 {
			continue
		}
		s := bufio.NewScanner(bytes.NewReader(data))
		s.Buffer(make([]byte, 1<<20), 1<<20)
		for n := 1; s.Scan(); n++ {
			line := strings.TrimSpace(s.Text())
			for _, b := range bindings {
				if !b.re.MatchString(line) {
					continue
				}
				if shown[b.kind] == bindingShown {
					more++
					break
				}
				shown[b.kind]++
				if r := []rune(line); len(r) > 100 {
					line = string(r[:99]) + "…"
				}
				out = append(out, fmt.Sprintf("`%s:%d`: `%s` (%s)", f, n, line, b.kind))
				break
			}
		}
	}
	if more > 0 {
		out = append(out, fmt.Sprintf("and %d more", more))
	}
	return out
}
