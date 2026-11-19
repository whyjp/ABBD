package relate

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/whyjp/abbd/internal/extract"
	"github.com/whyjp/abbd/internal/model"
)

func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for p, s := range files {
		f := filepath.Join(root, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(f), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(f, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func libMod(ref string, paths ...string) extract.DocUnit {
	u := extract.DocUnit{Ref: ref, Kind: model.Module, Name: ref}
	for _, p := range paths {
		u.Anchors = append(u.Anchors, model.Anchor{Path: p})
	}
	return u
}

// lines lists the doc's relations to libraries as "from -> lib quote".
func libLines(doc extract.Doc) []string {
	var out []string
	for _, r := range doc.Relations {
		if strings.HasPrefix(r.To, "lib:") {
			q := ""
			if len(r.Evidence) > 0 {
				q = r.Evidence[0].Path + ":" + r.Evidence[0].Lines
			}
			out = append(out, r.From+" -> "+r.To+" "+string(r.Kind)+" "+string(r.Via)+" "+q)
		}
	}
	slices.Sort(out)
	return out
}

func libUnits(doc extract.Doc) []string {
	var out []string
	for _, u := range doc.Units {
		if u.Kind == model.External {
			var as []string
			for _, a := range u.Anchors {
				as = append(as, a.Key())
			}
			out = append(out, u.Ref+" "+u.Name+" "+strings.Join(as, ","))
		}
	}
	slices.Sort(out)
	return out
}

// Each module that imports a library a go.mod requires gets one line to it,
// however often it imports it; an indirect require, one a replace keeps in
// the repository, the repository's own module and the standard library get
// none, and test code is not read.
func TestLibrariesReadGoModRequiresAndImports(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{
		"go.mod":            "module example.com/app\n\ngo 1.22\n\nrequire (\n\tgopkg.in/yaml.v3 v3.0.1\n\tgithub.com/spf13/cobra v1.8.0\n\tgolang.org/x/sys v0.1.0 // indirect\n\texample.com/local v0.0.0\n)\n\nrequire github.com/stretchr/testify v1.9.0\n\nreplace example.com/local => ./local\n",
		"store/store.go":    "package store\n\nimport (\n\t\"fmt\"\n\t\"gopkg.in/yaml.v3\"\n)\n\nvar _ = fmt.Sprint\nvar _ = yaml.Marshal\n",
		"store/more.go":     "package store\n\nimport \"gopkg.in/yaml.v3\"\n\nvar _ = yaml.Unmarshal\n",
		"cli/main.go":       "package main\n\nimport (\n\t\"github.com/spf13/cobra\"\n\t\"example.com/local/x\"\n\t\"example.com/app/store\"\n)\n",
		"cli/main_test.go":  "package main\n\nimport \"github.com/stretchr/testify/assert\"\n",
		"testdata/x/go.mod": "module x\n\nrequire github.com/fake/fake v1.0.0\n",
		"local/x/x.go":      "package x\n",
	})
	doc := extract.Doc{Units: []extract.DocUnit{libMod("store", "store"), libMod("cli", "cli"), libMod("local", "local")}}
	got, notes, err := Libraries(root, doc)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"cli -> lib:go:github.com/spf13/cobra depends_on import cli/main.go:4",
		"store -> lib:go:gopkg.in/yaml.v3 depends_on import store/more.go:3",
	}
	if l := libLines(got); !slices.Equal(l, want) {
		t.Errorf("lines:\n%s\nwant:\n%s", strings.Join(l, "\n"), strings.Join(want, "\n"))
	}
	wantUnits := []string{
		"lib:go:github.com/spf13/cobra github.com/spf13/cobra go.mod#github.com/spf13/cobra",
		"lib:go:gopkg.in/yaml.v3 gopkg.in/yaml.v3 go.mod#gopkg.in/yaml.v3",
	}
	if u := libUnits(got); !slices.Equal(u, wantUnits) {
		t.Errorf("units:\n%s\nwant:\n%s", strings.Join(u, "\n"), strings.Join(wantUnits, "\n"))
	}
	if len(notes) != 1 || !strings.Contains(notes[0], "github.com/stretchr/testify") {
		t.Errorf("notes %q, want one naming the library only a test imports", notes)
	}
	// Two of store's files import yaml: one line, quoting both.
	for _, r := range got.Relations {
		if r.From == "store" && len(r.Evidence) != 2 {
			t.Errorf("store's line quotes %d imports, want 2", len(r.Evidence))
		}
	}
}

// A package.json's runtime dependencies are libraries; its devDependencies,
// a workspace package and the repository's own packages are not. Each
// module that imports one by a bare specifier gets a line.
func TestLibrariesReadPackageJSONAndImports(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{
		"package.json": `{
  "name": "web",
  "dependencies": {
    "react": "^18.0.0",
    "@tanstack/react-query": "^5.0.0",
    "shared": "workspace:*",
    "lodash": "^4.0.0"
  },
  "devDependencies": {
    "vitest": "^1.0.0"
  }
}
`,
		"packages/shared/package.json":    `{"name": "shared"}`,
		"src/app/App.tsx":                 "import React from 'react';\nimport { useQuery } from \"@tanstack/react-query/build\";\nimport x from './x';\nimport fs from 'node:fs';\n",
		"src/app/App.test.tsx":            "import { test } from 'vitest';\nimport _ from 'lodash';\n",
		"src/util/u.js":                   "const _ = require('lodash/fp');\n",
		"node_modules/react/package.json": `{"name":"react","dependencies":{"loose-envify":"1"}}`,
	})
	doc := extract.Doc{Units: []extract.DocUnit{libMod("app", "src/app"), libMod("util", "src/util")}}
	got, _, err := Libraries(root, doc)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"app -> lib:npm:@tanstack/react-query depends_on import src/app/App.tsx:2",
		"app -> lib:npm:react depends_on import src/app/App.tsx:1",
		"util -> lib:npm:lodash depends_on import src/util/u.js:1",
	}
	if l := libLines(got); !slices.Equal(l, want) {
		t.Errorf("lines:\n%s\nwant:\n%s", strings.Join(l, "\n"), strings.Join(want, "\n"))
	}
	if u := libUnits(got); !slices.Contains(u, "lib:npm:react react package.json#react") {
		t.Errorf("units %q lack react anchored at its declaration", u)
	}
}

// A Cargo.toml's dependencies, in every form a crate writes them, are
// libraries; dev ones, path ones and workspace ones with a path are not. A
// crate that names one in its code gets a line; one it names nowhere is
// still used by the crate that holds the manifest, quoting the declaration.
func TestLibrariesReadCargoTomlAndPaths(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{
		"Cargo.toml": "[workspace]\nmembers = [\"core\"]\n\n[workspace.dependencies]\nshared = { path = \"shared\" }\ntokio = \"1\"\n",
		"core/Cargo.toml": `[package]
name = "core"

[dependencies]
serde = { version = "1", features = [
  "derive",
] }
tokio.workspace = true
shared = { workspace = true }
local = { path = "../local" }
json = { package = "serde_json", version = "1" }
log = "0.4"

[dependencies.regex-lite]
version = "0.1"

[target.'cfg(unix)'.dependencies]
nix = "0.27"

[dev-dependencies]
proptest = "1"
`,
		"core/src/lib.rs": "use serde::Serialize;\n// use log::info;\n#[tokio::main]\nasync fn main() { let _ = json::to_string(&1); regex_lite::Regex::new(\"x\"); std::mem::drop(1); }\n",
		"core/tests/t.rs": "use proptest::prelude::*;\n",
	})
	doc := extract.Doc{Units: []extract.DocUnit{libMod("cargo:core", "core/Cargo.toml", "core/src")}}
	got, _, err := Libraries(root, doc)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"cargo:core -> lib:cargo:log depends_on import core/Cargo.toml:12",
		"cargo:core -> lib:cargo:nix depends_on import core/Cargo.toml:18",
		"cargo:core -> lib:cargo:regex-lite depends_on import core/src/lib.rs:4",
		"cargo:core -> lib:cargo:serde depends_on import core/src/lib.rs:1",
		"cargo:core -> lib:cargo:serde_json depends_on import core/src/lib.rs:4",
		"cargo:core -> lib:cargo:tokio depends_on import core/src/lib.rs:3",
	}
	if l := libLines(got); !slices.Equal(l, want) {
		t.Errorf("lines:\n%s\nwant:\n%s", strings.Join(l, "\n"), strings.Join(want, "\n"))
	}
}

// A repository with no manifest, or one that declares no third-party
// library, leaves the extraction as it was.
func TestLibrariesLeaveAnExtractionWithoutLibrariesAlone(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{
		"go.mod":           "module example.com/app\n\ngo 1.22\n",
		"a/a.go":           "package a\n\nimport \"fmt\"\n\nvar _ = fmt.Sprint\n",
		"web/package.json": `{"name":"web","devDependencies":{"vitest":"1"}}`,
	})
	doc := extract.Doc{Units: []extract.DocUnit{libMod("a", "a")}, Relations: []extract.DocRelation{}}
	got, notes, err := Libraries(root, doc)
	if err != nil || len(notes) > 0 {
		t.Fatal(err, notes)
	}
	if len(got.Units) != 1 || len(got.Relations) != 0 {
		t.Errorf("an extraction without libraries changed: %+v", got)
	}
}

// A Rust path names its crate by its first segment only, outside strings
// and comments, and crate, self and super name the crate's own code.
func TestRustImportsReadOnlyThePathsFirstSegment(t *testing.T) {
	t.Parallel()
	for line, want := range map[string][]string{
		"use std::time::Instant;":             {"std"},
		"let c = axum::http::StatusCode::OK;": {"axum"},
		"crate::config::load();":              nil,
		`println!("see tokio::spawn");`:       nil,
		"x(); // uses log::info":              nil,
		"#[tokio::main]":                      {"tokio"},
		"extern crate serde;":                 {"serde"},
	} {
		var got []string
		for _, im := range rustImports([]byte(line)) {
			got = append(got, im.spec)
		}
		if !slices.Equal(got, want) {
			t.Errorf("%q imports %q, want %q", line, got, want)
		}
	}
}

// A package.json written on one line still declares its libraries, and an
// import inside a comment or a string is none.
func TestNpmReadsAOneLineManifestAndOnlyRealImports(t *testing.T) {
	t.Parallel()
	ds := npmDeclares([]byte(`{"name":"w","dependencies":{"react":"18","lodash":"4"}}`), localNames{modules: map[string]bool{}})
	if len(ds) != 2 || ds[0].line != 1 {
		t.Errorf("a one-line package.json declares %+v, want react and lodash on line 1", ds)
	}
	var got []string
	for _, im := range npmImports([]byte("/* import x from 'a' */\nconst s = \"copy from 'b'\";\nimport c from 'c';\n} from 'd';\n")) {
		got = append(got, im.spec)
	}
	if !slices.Equal(got, []string{"c", "d"}) {
		t.Errorf("imports %q, want c and d", got)
	}
}

// A package named build or external is the project's own code; only a
// target directory beside a Cargo.toml is build output.
func TestLibrariesReadAPackageNamedLikeBuildOutput(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{
		"go.mod":               "module example.com/app\n\nrequire github.com/spf13/cobra v1.8.0\n",
		"internal/build/b.go":  "package build\n\nimport \"github.com/spf13/cobra\"\n",
		"internal/target/t.go": "package target\n\nimport \"github.com/spf13/cobra\"\n",
	})
	doc := extract.Doc{Units: []extract.DocUnit{libMod("build", "internal/build"), libMod("target", "internal/target")}}
	got, _, err := Libraries(root, doc)
	if err != nil {
		t.Fatal(err)
	}
	if l := libLines(got); len(l) != 2 {
		t.Errorf("lines %q, want one from each package", l)
	}
}
