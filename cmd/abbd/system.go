package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/whyjp/abbd/internal/model"
)

// systemNames are the names of units that stand for the standard library, the
// language runtime or the system's own libraries, which no diagram draws.
var systemNames = map[string]bool{
	"std": true, "stdlib": true, "standard library": true, "libc": true, "glibc": true,
	"libstdc++": true, "libc++": true, "system headers": true, "system libraries": true,
}

// systemDirs are path segments of code that lies outside the repository's own,
// where a unit would anchor the standard library or a dependency.
var systemDirs = []string{"include/c++", "site-packages", "node_modules", ".cargo/registry"}

// systemVars are environment variables the operating system, the shell or the
// toolchain defines, by exact name, and systemVarPrefixes by prefix.
var systemVars = map[string]bool{
	"HOME": true, "HOMEDRIVE": true, "HOMEPATH": true, "PATH": true, "USER": true, "USERNAME": true,
	"SHELL": true, "TERM": true, "TMPDIR": true, "TEMP": true, "TMP": true, "LANG": true, "LC_ALL": true,
	"PWD": true, "LOCALAPPDATA": true, "APPDATA": true, "PROGRAMFILES": true, "SYSTEMROOT": true,
	"OUT_DIR": true, "JAVA_HOME": true, "NODE_PATH": true,
}

var systemVarPrefixes = []string{"XDG_", "CARGO_", "RUSTUP_", "DOCKER_"}

func systemVar(name string) bool {
	n := strings.ToUpper(strings.TrimSpace(name))
	if systemVars[n] {
		return true
	}
	for _, p := range systemVarPrefixes {
		if strings.HasPrefix(n, p) {
			return true
		}
	}
	return false
}

// systemLook says why a unit looks like the standard library, a system
// header, the system environment or a dependency outside the repository, or
// "". The diagram draws only the project's own units, so such a unit has no
// place in it. It is a report, never a refusal: the name and the paths are
// all it can judge.
func systemLook(u model.Unit) string {
	if systemNames[strings.ToLower(strings.TrimSpace(u.Name.Value))] {
		return "its name is the standard library or a system library"
	}
	for _, a := range u.Anchors {
		p := strings.ToLower(strings.ReplaceAll(a.Path, "\\", "/"))
		if strings.HasPrefix(p, "/usr/") || strings.HasPrefix(p, "/opt/") {
			return "its anchor " + a.Path + " lies outside the repository"
		}
		for _, d := range systemDirs {
			if strings.Contains("/"+p, "/"+d+"/") {
				return "its anchor " + a.Path + " lies in code outside the repository"
			}
		}
	}
	if u.Kind == model.Config && len(u.Entries) >= 2 {
		sys := 0
		for _, e := range u.Entries {
			if systemVar(e.Name) {
				sys++
			}
		}
		if sys*2 > len(u.Entries) {
			return fmt.Sprintf("%d of its %d entries are variables the system or the toolchain defines", sys, len(u.Entries))
		}
	}
	return ""
}

// statusSystem names each unit that looks like the standard library, a
// system header or the system environment, so the reader leaves it out. It
// is a report; the check ignores it.
func statusSystem(w io.Writer, units []model.Unit) {
	for _, u := range model.Present(units) {
		if why := systemLook(u); why != "" {
			fmt.Fprintf(w, "system: %s %q is not the project's own, so no diagram draws it: %s; leave it out, and say in one line of the module's text that the module uses it\n", u.ID, u.Name.Value, why)
		}
	}
}
