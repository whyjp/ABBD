package frontend

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/whyjp/abbd/internal/extract"
	"github.com/whyjp/abbd/internal/model"
)

type cmakeIndex struct {
	Objects []struct {
		Kind     string `json:"kind"`
		JSONFile string `json:"jsonFile"`
	} `json:"objects"`
}

type cmakeCodemodel struct {
	Version struct {
		Major int `json:"major"`
		Minor int `json:"minor"`
	} `json:"version"`
	Paths          struct{ Source string } `json:"paths"`
	Configurations []struct {
		Name            string            `json:"name"`
		Targets         []cmakeTargetFile `json:"targets"`
		AbstractTargets []cmakeTargetFile `json:"abstractTargets"`
	} `json:"configurations"`
}

type cmakeTargetRef struct {
	ID string `json:"id"`
}

type cmakeTargetFile struct {
	ID       string `json:"id"`
	JSONFile string `json:"jsonFile"`
}

type cmakeTarget struct {
	Name                string                  `json:"name"`
	ID                  string                  `json:"id"`
	Type                string                  `json:"type"`
	IsGeneratorProvided bool                    `json:"isGeneratorProvided"`
	Paths               struct{ Source string } `json:"paths"`
	Sources             []struct {
		Path              string `json:"path"`
		IsGenerated       bool   `json:"isGenerated"`
		CompileGroupIndex *int   `json:"compileGroupIndex"`
	} `json:"sources"`
	CompileGroups          []struct{ Language string } `json:"compileGroups"`
	Dependencies           []cmakeTargetRef            `json:"dependencies"`
	LinkLibraries          []cmakeTargetRef            `json:"linkLibraries"`
	InterfaceLinkLibraries []cmakeTargetRef            `json:"interfaceLinkLibraries"`
	Backtrace              *int                        `json:"backtrace"`
	Graph                  struct {
		Nodes []struct {
			File int `json:"file"`
		} `json:"nodes"`
		Files []string `json:"files"`
	} `json:"backtraceGraph"`
}

// targetArtifacts maps a CMake target type to what the build makes. Object,
// interface and utility targets make nothing a host or a user runs, and are
// left out.
var targetArtifacts = map[string]model.Artifact{
	"EXECUTABLE": "exe", "STATIC_LIBRARY": "static", "SHARED_LIBRARY": "shared", "MODULE_LIBRARY": "module",
}

// notBuilt are directories whose targets serve tests and samples, the
// counterpart of cargo's test, example and bench targets.
var notBuilt = map[string]bool{
	"test": true, "tests": true, "example": true, "examples": true,
	"bench": true, "benches": true, "benchmark": true, "benchmarks": true, "fuzz": true,
}

// cmake asks CMake's file API for the code model: it writes the query into
// a build tree outside the repository, configures, and makes each target a
// module that lists what it builds and each target it depends on a link.
// Targets compiling the same sources, such as a static and a shared build of
// one library, are one module.
func cmake(t tree) (extract.Doc, error) {
	build := t.buildDir("cmake")
	query := filepath.Join(build, ".cmake", "api", "v1", "query")
	if err := os.MkdirAll(query, 0o755); err != nil {
		return extract.Doc{}, err
	}
	if err := os.WriteFile(filepath.Join(query, "codemodel-v2"), nil, 0o644); err != nil {
		return extract.Doc{}, err
	}
	if _, err := t.run("cmake", "-S", t.dir, "-B", build); err != nil {
		return extract.Doc{}, err
	}
	reply := filepath.Join(build, ".cmake", "api", "v1", "reply")
	var cm cmakeCodemodel
	if err := readCodemodel(reply, &cm); err != nil {
		return extract.Doc{}, err
	}
	if len(cm.Configurations) == 0 {
		return extract.Doc{}, fmt.Errorf("the CMake code model has no configuration")
	}
	type group struct {
		names     []string
		sources   []string
		artifacts []model.Artifact
		anchors   []model.Anchor
		deps      []string
	}
	var groups []*group
	byID := map[string]*group{}
	bySources := map[string]*group{}
	// interfaces are the INTERFACE libraries' own links. Such a library
	// builds nothing and is no module, so a target linking it links them.
	interfaces := map[string][]cmakeTargetRef{}
	conf := cm.Configurations[0]
	for _, ref := range slices.Concat(conf.Targets, conf.AbstractTargets) {
		var tg cmakeTarget
		if err := readJSON(filepath.Join(reply, ref.JSONFile), &tg); err != nil {
			return extract.Doc{}, err
		}
		if tg.Type == "INTERFACE_LIBRARY" {
			interfaces[tg.ID] = tg.InterfaceLinkLibraries
		}
		a, ok := targetArtifacts[tg.Type]
		if !ok || tg.IsGeneratorProvided || testDir(tg.Paths.Source) {
			continue
		}
		var sources []string
		for _, s := range tg.Sources {
			p := filepath.FromSlash(s.Path)
			if !filepath.IsAbs(p) {
				p = filepath.Join(cm.Paths.Source, p)
			}
			if s.IsGenerated || !compiled(tg, s.CompileGroupIndex) {
				continue
			}
			if r, ok := t.rel(p); ok {
				sources = append(sources, r)
			}
		}
		if len(sources) == 0 {
			continue
		}
		slices.Sort(sources)
		sources = slices.Compact(sources)
		key := strings.Join(sources, "\n")
		g := bySources[key]
		if g == nil {
			g = &group{sources: sources}
			for _, s := range sources {
				g.anchors = append(g.anchors, model.Anchor{Path: path.Dir(s)})
			}
			bySources[key] = g
			groups = append(groups, g)
		}
		g.names = append(g.names, tg.Name)
		g.artifacts = append(g.artifacts, a)
		if f, ok := t.defined(tg); ok {
			g.anchors = append(g.anchors, model.Anchor{Path: f, Symbol: tg.Name})
		}
		for _, d := range links(cm, tg) {
			g.deps = append(g.deps, d.ID)
		}
		byID[tg.ID] = g
	}
	var names [][]string
	for _, g := range groups {
		names = append(names, g.names)
	}
	refs := map[*group]string{}
	for i, r := range moduleRefs(names) {
		refs[groups[i]] = r
	}
	var doc extract.Doc
	for _, g := range groups {
		slices.Sort(g.artifacts)
		doc.Units = append(doc.Units, extract.DocUnit{Ref: refs[g], Kind: model.Module, Name: strings.TrimPrefix(refs[g], "cmake:"), Anchors: model.SortAnchors(g.anchors), Artifacts: slices.Compact(g.artifacts)})
		seen := map[string]bool{}
		for _, id := range throughInterfaces(g.deps, interfaces) {
			to, ok := byID[id]
			if !ok || to == g || seen[refs[to]] {
				continue
			}
			seen[refs[to]] = true
			doc.Relations = append(doc.Relations, extract.DocRelation{From: refs[g], To: refs[to], Kind: model.DependsOn, Via: "link"})
		}
	}
	return doc, nil
}

// throughInterfaces replaces each INTERFACE library among ids with what it
// links, at any depth, and visits each library once, so a cycle of them
// ends.
func throughInterfaces(ids []string, interfaces map[string][]cmakeTargetRef) []string {
	var out []string
	visited := map[string]bool{}
	var walk func(id string)
	walk = func(id string) {
		next, ok := interfaces[id]
		if !ok {
			out = append(out, id)
			return
		}
		if visited[id] {
			return
		}
		visited[id] = true
		for _, r := range next {
			walk(r.ID)
		}
	}
	for _, id := range ids {
		walk(id)
	}
	return out
}

// readCodemodel reads the codemodel the newest reply index names.
func readCodemodel(reply string, cm *cmakeCodemodel) error {
	idx, _ := filepath.Glob(filepath.Join(reply, "index-*.json"))
	if len(idx) == 0 {
		return fmt.Errorf("CMake wrote no file API reply under %s; it needs CMake 3.14 or later", reply)
	}
	slices.Sort(idx)
	var in cmakeIndex
	if err := readJSON(idx[len(idx)-1], &in); err != nil {
		return err
	}
	for _, o := range in.Objects {
		if o.Kind == "codemodel" {
			return readJSON(filepath.Join(reply, o.JSONFile), cm)
		}
	}
	return fmt.Errorf("the CMake file API reply has no codemodel")
}

// defined is the root-relative file whose command defined the target.
func (t tree) defined(tg cmakeTarget) (string, bool) {
	if tg.Backtrace == nil || *tg.Backtrace >= len(tg.Graph.Nodes) {
		return "", false
	}
	f := tg.Graph.Nodes[*tg.Backtrace].File
	if f >= len(tg.Graph.Files) {
		return "", false
	}
	p := filepath.FromSlash(tg.Graph.Files[f])
	if !filepath.IsAbs(p) {
		p = filepath.Join(t.dir, p)
	}
	return t.rel(p)
}

// links are the targets a target links. Code model 2.9 (CMake 4.2) lists
// them apart from targets that add_dependencies only builds first; an older
// one lists both together, and each stays a link.
func links(cm cmakeCodemodel, tg cmakeTarget) []cmakeTargetRef {
	if cm.Version.Major > 2 || cm.Version.Minor >= 9 {
		return tg.LinkLibraries
	}
	return tg.Dependencies
}

// moduleRefs names each module by its target, or a module of several
// targets by the words their names share, such as libzstd for
// libzstd_static and libzstd_shared. CMake keeps target names unique, so a
// shared name that is also a target's, or another module's, falls back to
// the module's first target name. The result depends only on the names,
// never on the order CMake lists the targets in.
func moduleRefs(names [][]string) []string {
	targets := map[string]bool{}
	order := make([]int, len(names))
	for i, ns := range names {
		slices.Sort(ns)
		for _, n := range ns {
			targets[n] = true
		}
		order[i] = i
	}
	slices.SortFunc(order, func(a, b int) int { return strings.Compare(names[a][0], names[b][0]) })
	refs := make([]string, len(names))
	taken := map[string]bool{}
	for _, i := range order {
		name := names[i][0]
		if c := commonName(names[i]); c != name && !targets[c] && !taken[c] {
			name = c
		}
		taken[name] = true
		refs[i] = "cmake:" + name
	}
	return refs
}

// commonName is the words the names share at their start, spelled as the
// first name spells them, or the first name when they share too little.
func commonName(names []string) string {
	sep := func(c byte) bool { return c == '_' || c == '-' || c == '.' }
	edge := func(s string, i int) bool { return i == len(s) || sep(s[i]) }
	end := len(names[0])
	for _, n := range names[1:] {
		i := 0
		for i < min(end, len(n)) && names[0][i] == n[i] {
			i++
		}
		for i > 0 && !(edge(names[0], i) && edge(n, i)) {
			i--
		}
		end = i
	}
	if s := strings.TrimRight(names[0][:end], "_-."); len(names) > 1 && len(s) >= 3 {
		return s
	}
	return names[0]
}

// compiled reports whether a source is code the target compiles, so a
// Windows resource file or a header does not tell two builds of one
// library apart.
func compiled(tg cmakeTarget, group *int) bool {
	return group != nil && *group < len(tg.CompileGroups) && tg.CompileGroups[*group].Language != "RC"
}

func testDir(p string) bool {
	return slices.ContainsFunc(strings.Split(filepath.ToSlash(p), "/"), func(s string) bool { return notBuilt[strings.ToLower(s)] })
}

func readJSON(p string, v any) error {
	b, err := os.ReadFile(p)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, v); err != nil {
		return fmt.Errorf("%s: %w", filepath.Base(p), err)
	}
	return nil
}
