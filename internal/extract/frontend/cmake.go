package frontend

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/whyjp/abbd/internal/extract"
	"github.com/whyjp/abbd/internal/model"
)

type cmakeIndex struct {
	CMake struct {
		Version struct {
			String string `json:"string"`
		} `json:"version"`
	} `json:"cmake"`
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
// counterpart of cargo's test, example and bench targets, or hold someone
// else's code a repository vendors.
var notBuilt = map[string]bool{
	"test": true, "tests": true, "example": true, "examples": true,
	"bench": true, "benches": true, "benchmark": true, "benchmarks": true, "fuzz": true,
	"third_party": true, "thirdparty": true, "3rdparty": true, "vendor": true, "external": true,
}

// notBuiltWords are the words of a target's name that say it is a test, a
// bench or an example, as c_test, db_bench and leveldb_tests do where a
// repository's top CMakeLists.txt defines them, so no directory tells.
var notBuiltWords = map[string]bool{
	"test": true, "tests": true, "unittest": true, "unittests": true, "bench": true, "benches": true,
	"benchmark": true, "benchmarks": true, "example": true, "examples": true, "fuzz": true, "fuzzer": true,
}

// headerExt are the files a target lists that it does not compile but that
// belong to it: its headers and templates.
var headerExt = map[string]bool{".h": true, ".hh": true, ".hpp": true, ".hxx": true, ".inc": true, ".inl": true, ".ipp": true, ".tcc": true, ".in": true}

// cmake asks CMake's file API for the code model: it writes the query into
// a build tree outside the repository, configures, and makes each target a
// module that lists what it builds and each target it depends on a link.
// Targets compiling the same sources, such as a static and a shared build of
// one library, are one module.
func cmake(t tree) (extract.Doc, error) {
	version, err := cmakeVersion(t)
	if err != nil {
		return extract.Doc{}, err
	}
	build := t.buildDir(filepath.Join("cmake", version))
	query := filepath.Join(build, ".cmake", "api", "v1", "query")
	if err := os.MkdirAll(query, 0o755); err != nil {
		return extract.Doc{}, err
	}
	if err := os.WriteFile(filepath.Join(query, "codemodel-v2"), nil, 0o644); err != nil {
		return extract.Doc{}, err
	}
	if _, err := t.run(append([]string{"cmake", "-S", t.dir, "-B", build}, t.defs...)...); err != nil {
		return extract.Doc{}, fmt.Errorf("%w\nA configure that fails on a missing subdirectory, as git submodules not checked out leave, or on a missing package, needs the option that turns that part off: pass it as abbd extract cmake=%s -D<NAME>=OFF, or check the submodules out%s", err, relDir(t), optionHint(t.dir))
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
		if !ok || tg.IsGeneratorProvided || testDir(tg.Paths.Source) || notBuiltName(tg.Name) {
			continue
		}
		var sources, headers []string
		for _, s := range tg.Sources {
			p := filepath.FromSlash(s.Path)
			if !filepath.IsAbs(p) {
				p = filepath.Join(cm.Paths.Source, p)
			}
			if s.IsGenerated {
				continue
			}
			r, ok := t.rel(p)
			if !ok {
				continue
			}
			if compiled(tg, s.CompileGroupIndex) {
				sources = append(sources, r)
			} else if headerExt[strings.ToLower(path.Ext(r))] {
				headers = append(headers, r)
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
			bySources[key] = g
			groups = append(groups, g)
		}
		for _, s := range slices.Concat(sources, headers) {
			g.anchors = append(g.anchors, model.Anchor{Path: path.Dir(s)})
		}
		g.names = append(g.names, tg.Name)
		g.artifacts = append(g.artifacts, a)
		if f, ok := t.defined(tg); ok {
			g.anchors = append(g.anchors, model.Anchor{Path: f, Symbol: tg.Name})
		}
		for _, d := range tg.LinkLibraries {
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

// cmakeVersion asks the cmake on PATH for its version before anything is
// configured, so a CMake older than 4.2 gets the refusal even where its
// configure fails first. The version is in the name of the build tree, so a
// tree an older CMake made never meets a newer one that cannot reuse it.
func cmakeVersion(t tree) (string, error) {
	out, err := t.run("cmake", "-E", "capabilities")
	if err != nil {
		return "", err
	}
	var caps struct {
		Version struct {
			String string `json:"string"`
			Major  int    `json:"major"`
			Minor  int    `json:"minor"`
		} `json:"version"`
	}
	if err := json.Unmarshal(out, &caps); err != nil {
		return "", fmt.Errorf("cmake -E capabilities printed no JSON: %w", err)
	}
	if v := caps.Version; v.Major < 4 || v.Major == 4 && v.Minor < 2 {
		return "", oldCMake{v.String}
	}
	return strings.Map(func(r rune) rune {
		if r == '.' || r == '-' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' {
			return r
		}
		return '_'
	}, caps.Version.String), nil
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

// readCodemodel reads the codemodel the newest reply index names, and
// refuses one older than 2.9, the first to list a target's direct links
// apart from targets add_dependencies only builds first.
func readCodemodel(reply string, cm *cmakeCodemodel) error {
	idx, _ := filepath.Glob(filepath.Join(reply, "index-*.json"))
	if len(idx) == 0 {
		return fmt.Errorf("CMake wrote no file API reply under %s; %w", reply, oldCMake{"older than 3.14"})
	}
	slices.Sort(idx)
	var in cmakeIndex
	if err := readJSON(idx[len(idx)-1], &in); err != nil {
		return err
	}
	for _, o := range in.Objects {
		if o.Kind != "codemodel" {
			continue
		}
		if err := readJSON(filepath.Join(reply, o.JSONFile), cm); err != nil {
			return err
		}
		if cm.Version.Major == 2 && cm.Version.Minor < 9 {
			return oldCMake{in.CMake.Version.String}
		}
		return nil
	}
	return fmt.Errorf("the CMake file API reply has no codemodel")
}

// oldCMake is the refusal of a CMake older than 4.2.
type oldCMake struct{ found string }

func (e oldCMake) Error() string {
	return "found CMake " + e.found + ", but the cmake front-end needs CMake 4.2 or later: " +
		"older CMake does not record which links a target makes directly, apart from the targets add_dependencies only builds first. " +
		"Get CMake 4.2 or later and put it first on PATH: the official release from https://cmake.org/download/ on any OS, " +
		"`pip install cmake` on any OS, the Kitware apt repository (https://apt.kitware.com) on Debian or Ubuntu, " +
		"`brew install cmake` on macOS, or `winget install Kitware.CMake` on Windows. " +
		"Or leave this front-end out and describe the modules in a supplement during setup."
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

// notBuiltName reports whether a target's name says it is a test, a bench or
// an example: a word of it, split at _, - and ., is one of notBuiltWords.
func notBuiltName(name string) bool {
	return slices.ContainsFunc(strings.FieldsFunc(strings.ToLower(name), func(r rune) bool { return r == '_' || r == '-' || r == '.' }), func(w string) bool { return notBuiltWords[w] })
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

var optionLine = regexp.MustCompile(`(?im)^\s*(?:cmake_dependent_option|option)\s*\(\s*([A-Za-z_][\w]*)`)

// optionHint lists the option( names the project's CMake files declare,
// those that name a build or a test first, so a failed configure says which
// -D to pass instead of leaving the reader to guess it.
func optionHint(dir string) string {
	var names []string
	filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if p != dir && (strings.HasPrefix(d.Name(), ".") || slices.Contains([]string{"third_party", "vendor", "external", "node_modules"}, d.Name())) {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() != "CMakeLists.txt" && !strings.HasSuffix(d.Name(), ".cmake") {
			return nil
		}
		text, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		for _, m := range optionLine.FindAllStringSubmatch(string(text), -1) {
			if !slices.Contains(names, m[1]) {
				names = append(names, m[1])
			}
		}
		return nil
	})
	if len(names) == 0 {
		return ", or read the option names from `cmake -LH`"
	}
	like := func(n string) bool {
		u := strings.ToUpper(n)
		return strings.Contains(u, "BUILD") || strings.Contains(u, "TEST")
	}
	slices.SortStableFunc(names, func(a, b string) int {
		switch {
		case like(a) && !like(b):
			return -1
		case !like(a) && like(b):
			return 1
		}
		return 0
	})
	more := ""
	if len(names) > 12 {
		more = fmt.Sprintf(" and %d more (`cmake -LH`)", len(names)-12)
		names = names[:12]
	}
	return "\nThe project's options: " + strings.Join(names, ", ") + more
}
