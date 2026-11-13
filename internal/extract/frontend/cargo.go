package frontend

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/whyjp/abbd/internal/extract"
	"github.com/whyjp/abbd/internal/model"
)

type cargoMetadata struct {
	Packages []struct {
		Name         string `json:"name"`
		ManifestPath string `json:"manifest_path"`
		Dependencies []struct {
			Name     string  `json:"name"`
			Path     string  `json:"path"`
			Kind     *string `json:"kind"`
			Optional bool    `json:"optional"`
		} `json:"dependencies"`
		Targets []struct {
			Kind    []string `json:"kind"`
			SrcPath string   `json:"src_path"`
		} `json:"targets"`
	} `json:"packages"`
}

// crateArtifacts maps the cargo target kinds a crate ships to what the
// build makes. A plain library or a proc-macro is code other crates compile
// in, so it stays a module that builds no artifact of its own. Tests,
// examples, benches and build scripts are left out.
var crateArtifacts = map[string]model.Artifact{
	"bin": "exe", "staticlib": "static", "cdylib": "cdylib", "dylib": "shared",
	"lib": "", "rlib": "", "proc-macro": "",
}

// cargo makes each workspace member a module and each dependency of one
// member on another a relation. --no-deps reads only the workspace, so it
// needs no lockfile, registry or network, and prints the same on every
// machine.
func cargo(t tree) (extract.Doc, error) {
	out, err := t.run("cargo", "metadata", "--format-version", "1", "--no-deps", "--offline")
	if err != nil {
		return extract.Doc{}, err
	}
	var md cargoMetadata
	if err := json.Unmarshal(out, &md); err != nil {
		return extract.Doc{}, fmt.Errorf("cargo metadata output: %w", err)
	}
	var doc extract.Doc
	procMacro := map[string]bool{}
	// A dependency names a member by its path, since a registry crate can
	// share a member's name.
	member := map[string]string{}
	for _, p := range md.Packages {
		member[filepath.Clean(filepath.Dir(p.ManifestPath))] = p.Name
		manifest, ok := t.rel(p.ManifestPath)
		if !ok {
			continue
		}
		u := extract.DocUnit{Ref: "cargo:" + p.Name, Kind: model.Module, Name: p.Name, Anchors: []model.Anchor{{Path: manifest}}}
		dir := filepath.Dir(p.ManifestPath)
		seen := map[string]bool{}
		for _, tg := range p.Targets {
			for _, k := range tg.Kind {
				a, ok := crateArtifacts[k]
				if !ok {
					continue
				}
				if k == "proc-macro" {
					procMacro[p.Name] = true
				}
				if a != "" && !slices.Contains(u.Artifacts, a) {
					u.Artifacts = append(u.Artifacts, a)
				}
				// The crate's top source directory, never the crate's own
				// directory, which is the whole tree for a root crate.
				if r, err := filepath.Rel(dir, tg.SrcPath); err == nil && !strings.HasPrefix(r, "..") {
					top, _, _ := strings.Cut(filepath.ToSlash(r), "/")
					if a, ok := t.rel(filepath.Join(dir, top)); ok && !seen[a] {
						seen[a] = true
						u.Anchors = append(u.Anchors, model.Anchor{Path: a})
					}
				}
			}
		}
		doc.Units = append(doc.Units, u)
	}
	for _, p := range md.Packages {
		vias := map[string]model.Via{}
		for _, d := range p.Dependencies {
			to, ok := member[filepath.Clean(d.Path)]
			if d.Path == "" || !ok || to == p.Name || d.Kind != nil && *d.Kind == "dev" {
				continue
			}
			v := model.Via("import")
			switch {
			case d.Optional || d.Kind != nil && *d.Kind == "build":
				v = "build-feature"
			case procMacro[to]:
				v = "macro"
			}
			if had, ok := vias[to]; !ok || rank(v) < rank(had) {
				vias[to] = v
			}
		}
		for to, v := range vias {
			doc.Relations = append(doc.Relations, extract.DocRelation{From: "cargo:" + p.Name, To: "cargo:" + to, Kind: model.DependsOn, Via: v})
		}
	}
	return doc, nil
}

// rank orders the vias of one crate's dependencies on another: a plain
// dependency, or one on a proc-macro crate, outranks one only a feature or
// the build script enables.
func rank(v model.Via) int {
	return map[model.Via]int{"macro": 0, "import": 1, "build-feature": 2}[v]
}
