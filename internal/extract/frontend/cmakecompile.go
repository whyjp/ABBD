package frontend

import (
	"cmp"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

type cmakeCompileGroup struct {
	Language  string `json:"language"`
	Fragments []struct {
		Fragment string `json:"fragment"`
	} `json:"compileCommandFragments"`
	Includes []struct {
		Path string `json:"path"`
	} `json:"includes"`
	Defines []struct {
		Define string `json:"define"`
	} `json:"defines"`
}

type cmakeToolchains struct {
	Toolchains []struct {
		Language string `json:"language"`
		Compiler struct {
			Path string `json:"path"`
		} `json:"compiler"`
	} `json:"toolchains"`
}

// cmakeQueries are the file API objects the front-end asks CMake for: the
// code model, and the compilers a relater rebuilds each compile command
// with.
var cmakeQueries = []string{"codemodel-v2", "toolchains-v1"}

// cmakeTargets configures the CMake project in its build tree under the
// cache and reads every target of the code model's first configuration,
// with the file API reply directory it read them from.
func cmakeTargets(t tree) ([]cmakeTarget, string, error) {
	reply, err := configure(t)
	if err != nil {
		return nil, "", err
	}
	var cm cmakeCodemodel
	if err := readCodemodel(reply, &cm); err != nil {
		return nil, "", err
	}
	if len(cm.Configurations) == 0 {
		return nil, "", fmt.Errorf("the CMake code model has no configuration")
	}
	conf := cm.Configurations[0]
	var out []cmakeTarget
	for _, ref := range slices.Concat(conf.Targets, conf.AbstractTargets) {
		var tg cmakeTarget
		if err := readJSON(filepath.Join(reply, ref.JSONFile), &tg); err != nil {
			return nil, "", err
		}
		tg.root = cm.Paths.Source
		out = append(out, tg)
	}
	return out, reply, nil
}

// configure writes the file API queries into the build tree, configures,
// and returns the reply directory.
func configure(t tree) (string, error) {
	version, err := cmakeVersion(t)
	if err != nil {
		return "", err
	}
	build := t.buildDir(filepath.Join("cmake", version))
	query := filepath.Join(build, ".cmake", "api", "v1", "query")
	if err := os.MkdirAll(query, 0o755); err != nil {
		return "", err
	}
	for _, q := range cmakeQueries {
		if err := os.WriteFile(filepath.Join(query, q), nil, 0o644); err != nil {
			return "", err
		}
	}
	if _, err := t.run("cmake", "-S", t.dir, "-B", build); err != nil {
		return "", err
	}
	return filepath.Join(build, ".cmake", "api", "v1", "reply"), nil
}

// Compile is how the build compiles one source file: the file and the
// directory the command runs in, both absolute, and the command's
// arguments, compiler first.
type Compile struct {
	File, Dir string
	Args      []string
}

// CMakeCompiles configures the CMake project whose CMakeLists.txt sits in
// dir, in the same build tree the cmake front-end uses, and returns how
// each C or C++ source of a target the front-end keeps is compiled. It
// rebuilds the commands from the file API, which every generator writes,
// rather than from compile_commands.json, which only the Makefile and Ninja
// generators do. A source two targets compile takes the command of the
// target whose name sorts first.
func CMakeCompiles(root, dir, cache string) ([]Compile, error) {
	t := tree{root: root, dir: dir, cache: cache}
	targets, reply, err := cmakeTargets(t)
	if err != nil {
		return nil, err
	}
	// The commands run in the build tree, four levels above the reply.
	build := filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(reply))))
	compilers, err := cmakeCompilers(reply)
	if err != nil {
		return nil, err
	}
	slices.SortFunc(targets, func(a, b cmakeTarget) int { return cmp.Compare(a.Name, b.Name) })
	seen := map[string]bool{}
	var out []Compile
	for _, tg := range targets {
		if !tg.built() {
			continue
		}
		for _, s := range tg.Sources {
			if s.IsGenerated || s.CompileGroupIndex == nil || *s.CompileGroupIndex >= len(tg.CompileGroups) {
				continue
			}
			g := tg.CompileGroups[*s.CompileGroupIndex]
			if g.Language != "C" && g.Language != "CXX" {
				continue
			}
			file := tg.source(s.Path)
			if seen[file] {
				continue
			}
			seen[file] = true
			var flags []string
			for _, f := range g.Fragments {
				flags = append(flags, splitFragment(f.Fragment)...)
			}
			args := append([]string{compiler(compilers[g.Language], g.Language, flags)}, flags...)
			for _, i := range g.Includes {
				args = append(args, "-I"+filepath.FromSlash(i.Path))
			}
			for _, d := range g.Defines {
				args = append(args, "-D"+d.Define)
			}
			out = append(out, Compile{File: file, Dir: build, Args: append(args, "-c", file)})
		}
	}
	slices.SortFunc(out, func(a, b Compile) int { return cmp.Compare(a.File, b.File) })
	return out, nil
}

// compiler is the compiler a command runs. The Visual Studio generators
// name none, and their flags are cl's, so a reader that picks its driver by
// the compiler's name reads them as cl's.
func compiler(path, language string, flags []string) string {
	switch {
	case path != "":
		return path
	case slices.ContainsFunc(flags, func(f string) bool { return strings.HasPrefix(f, "/") }):
		return "cl.exe"
	case language == "CXX":
		return "c++"
	}
	return "cc"
}

// cmakeCompilers maps each language to its compiler's path.
func cmakeCompilers(reply string) (map[string]string, error) {
	idx, _ := filepath.Glob(filepath.Join(reply, "index-*.json"))
	if len(idx) == 0 {
		return nil, fmt.Errorf("CMake wrote no file API reply under %s", reply)
	}
	slices.Sort(idx)
	var in cmakeIndex
	if err := readJSON(idx[len(idx)-1], &in); err != nil {
		return nil, err
	}
	for _, o := range in.Objects {
		if o.Kind != "toolchains" {
			continue
		}
		var tc cmakeToolchains
		if err := readJSON(filepath.Join(reply, o.JSONFile), &tc); err != nil {
			return nil, err
		}
		out := map[string]string{}
		for _, c := range tc.Toolchains {
			out[c.Language] = c.Compiler.Path
		}
		return out, nil
	}
	return nil, fmt.Errorf("the CMake file API reply has no toolchains")
}

// splitFragment splits a compile command fragment into arguments at blanks
// outside double quotes, as CMake writes a fragment for a shell.
func splitFragment(s string) []string {
	var out []string
	var b strings.Builder
	quoted, any := false, false
	for _, r := range s {
		switch {
		case r == '"':
			quoted, any = !quoted, true
		case (r == ' ' || r == '\t') && !quoted:
			if any {
				out = append(out, b.String())
				b.Reset()
				any = false
			}
		default:
			b.WriteRune(r)
			any = true
		}
	}
	if any {
		out = append(out, b.String())
	}
	return out
}
