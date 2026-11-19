package main

import (
	"strings"
	"testing"

	"github.com/whyjp/abbd/internal/model"
)

func TestSystemLookNamesTheStandardLibraryAndTheSystemEnvironment(t *testing.T) {
	t.Parallel()
	unit := func(kind model.Kind, name string, paths ...string) model.Unit {
		u := model.Unit{Kind: kind, Name: model.Said(model.Script, name)}
		for _, p := range paths {
			u.Anchors = append(u.Anchors, model.Anchor{Path: p})
		}
		return u
	}
	env := func(names ...string) model.Unit {
		u := unit(model.Config, "environment variables", "src/cli.rs")
		for _, n := range names {
			u.Entries = append(u.Entries, model.Entry{Name: n})
		}
		return u
	}
	for name, tc := range map[string]struct {
		u    model.Unit
		want bool
	}{
		"std module":                     {unit(model.Module, "std", "src/std.rs"), true},
		"libc by a long name":            {unit(model.Module, "Standard Library", "x.c"), true},
		"header outside the repo":        {unit(model.Module, "headers", "/usr/include/stdio.h"), true},
		"a library's code is no system":  {unit(model.Module, "serde", "web/node_modules/serde/index.js"), false},
		"an external library named libc": {unit(model.External, "libc", "Cargo.toml"), false},
		"system variables":               {env("HOME", "PATH", "XDG_DATA_HOME", "OBS_DATA_PATH"), true},
		"toolchain variables":            {env("CARGO_MANIFEST_DIR", "OUT_DIR"), true},
		"the project's own":              {env("VECTOR_CONFIG", "VECTOR_LOG", "HOME"), false},
		"one variable is no signal":      {env("PATH"), false},
		"one of two is no majority":      {env("HOME", "VECTOR_LOG"), false},
		"own module named runtime":       {unit(model.Module, "runtime", "src/runtime/mod.rs"), false},
		"own module named core":          {unit(model.Module, "core", "src/core/a.go"), false},
		"anchor named like a lib":        {unit(model.Module, "vendorish", "src/node_modules_cache/a.go"), false},
	} {
		if got := systemLook(tc.u) != ""; got != tc.want {
			t.Errorf("%s: systemLook says %q, want a report = %v", name, systemLook(tc.u), tc.want)
		}
	}
}

// A unit that draws a third-party library by the code a package manager
// keeps is reported apart from the system: the library is one external unit
// its manifest declares. An external unit is never reported.
func TestLibraryLookNamesALibraryDrawnByItsCode(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		u    model.Unit
		want bool
	}{
		"node_modules":      {model.Unit{Kind: model.Module, Anchors: []model.Anchor{{Path: "web/node_modules/serde/index.js"}}}, true},
		"site-packages":     {model.Unit{Kind: model.Module, Anchors: []model.Anchor{{Path: "venv/lib/site-packages/requests/api.py"}}}, true},
		"cargo registry":    {model.Unit{Kind: model.Module, Anchors: []model.Anchor{{Path: "home/.cargo/registry/src/serde/lib.rs"}}}, true},
		"the project's own": {model.Unit{Kind: model.Module, Anchors: []model.Anchor{{Path: "src/node_modules_cache/a.go"}}}, false},
		"an external unit":  {model.Unit{Kind: model.External, Anchors: []model.Anchor{{Path: "package.json", Symbol: "react"}}}, false},
		"the standard lib":  {model.Unit{Kind: model.Module, Anchors: []model.Anchor{{Path: "x/include/c++/vector"}}}, false},
	} {
		if got := libraryLook(tc.u) != ""; got != tc.want {
			t.Errorf("%s: libraryLook says %q, want a report = %v", name, libraryLook(tc.u), tc.want)
		}
	}
}

func TestStatusNamesAUnitThatStandsForTheSystem(t *testing.T) {
	t.Parallel()
	p, exe := bare(t)
	p.code("billing/charge.go", "package billing\n\nfunc Charge() {}\n")
	p.extract(`{"units":[
 {"key":"billing","kind":"MOD","name":"billing","anchors":[{"path":"billing/charge.go","symbol":"Charge"}]},
 {"key":"env","kind":"CFG","name":"environment variables","anchors":[{"path":"billing/charge.go","symbol":"Charge"}],
  "entries":[{"name":"HOME"},{"name":"PATH"},{"name":"XDG_CONFIG_HOME"},{"name":"BILLING_URL"}]}
]}`)
	p.setup(0, "synced:", "--", exe)
	out := p.want(0, "system: CFG-", "status")
	if !strings.Contains(out, "3 of its 4 entries are variables the system or the toolchain defines") {
		t.Errorf("status did not say why the CFG is the system's:\n%s", out)
	}
	if strings.Contains(out, `system: MOD-`) {
		t.Errorf("status named a project module as the system's:\n%s", out)
	}
}
