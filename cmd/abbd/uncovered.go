package main

import (
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/whyjp/abbd/internal/gitx"
	"github.com/whyjp/abbd/internal/model"
)

// manifests are the files that declare a module or package of their own.
var manifests = map[string]bool{
	"go.mod": true, "package.json": true, "Cargo.toml": true, "pyproject.toml": true,
	"setup.py": true, "pom.xml": true, "build.gradle": true, "build.gradle.kts": true,
	"Gemfile": true, "composer.json": true, "mix.exs": true, "Package.swift": true,
}

// notCode are directories whose manifests describe test data or someone
// else's code, never a module of the project.
var notCode = map[string]bool{"testdata": true, "node_modules": true, "vendor": true}

// uncovered names each manifest in the working tree that no present unit
// anchors code under or holds by a directory anchor: a module the extraction
// script does not know, which
// a hand-kept list in the script would hide without a word.
// The root's manifest is skipped, since every unit sits under the root.
func uncovered(root string, units []model.Unit) []string {
	var anchors []string
	for _, u := range model.Present(units) {
		for _, a := range u.Anchors {
			anchors = append(anchors, a.Path)
		}
	}
	var out []string
	for _, f := range gitx.WorkingFiles(root) {
		dir, base := path.Split(f)
		dir = strings.TrimSuffix(dir, "/")
		if !manifests[base] || dir == "" || slices.ContainsFunc(strings.Split(dir, "/"), func(s string) bool { return notCode[s] }) {
			continue
		}
		// An anchor under the manifest's directory covers it, and so does a
		// directory anchor that holds it, the module the manifest sits in.
		if !slices.ContainsFunc(anchors, func(a string) bool {
			return strings.HasPrefix(a+"/", dir+"/") || strings.HasPrefix(dir+"/", a+"/")
		}) {
			out = append(out, fmt.Sprintf("uncovered: %s: no unit anchors code under %s; teach the extraction script the module, or find modules from their manifests instead of a list", f, dir))
		}
	}
	slices.Sort(out)
	return out
}
