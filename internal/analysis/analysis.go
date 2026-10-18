// Package analysis is the Code Intelligence and External Contract Extractor (PRD 01A, 08, 08A).
// It reads only deterministic structure: Go ASTs, regex over other languages, SQL migrations
// and deployment manifests. Anything it cannot resolve is recorded, never dropped.
package analysis

import (
	"cmp"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/whyjp/for-my-harness/internal/model"
)

type Result struct {
	Root          string
	Languages     []string
	Modules       []ModuleFacts
	Tests         []TestFacts
	Contracts     []ContractFacts
	Serving       []ServingFacts
	TestCommands  []string
	BuildCommands []string
	Docs          []string
	Unknowns      []string
}

type ModuleFacts struct {
	Path       string
	Name       string
	Language   string
	ImportPath string
	IsMain     bool
	Symbols    []model.Symbol
	Imports    []string
	Calls      []CallEdge
	Analyzer   string
	Files      []string
}

type CallEdge struct {
	From       string
	To         string
	Confidence string
	Location   string
}

const (
	ConfidenceStatic   = "static"
	ConfidencePossible = "possible"
)

type TestFacts struct {
	Name       string
	ModulePath string
	Location   string
	Command    string
}

type ContractFacts struct {
	Kind      string
	Direction string
	Selector  string
	OwnerPath string
	Location  string
	Symbol    string
	Origin    model.Origin
	Note      string
}

const (
	KindHTTPRoute  = "http-route"
	KindKafka      = "kafka-topic"
	KindDBTable    = "db-table"
	KindEnvVar     = "env-var"
	KindCLICommand = "cli-command"

	DirExposes  = "exposes"
	DirConsumes = "consumes"
	DirProduces = "produces"
	DirOwns     = "owns"
	DirReads    = "reads"
	DirUnknown  = "unknown"
)

type ServingFacts struct {
	DeployUnit  string
	Source      string
	ModulePaths []string
	Origin      model.Origin
}

// repo is the file listing every extractor works from. Paths are root-relative with forward slashes.
type repo struct {
	root  string
	files []string
	set   map[string]bool
}

func (r *repo) read(rel string) ([]byte, error) {
	return os.ReadFile(filepath.Join(r.root, filepath.FromSlash(rel)))
}

// dirName names a directory, using the root's own folder name for ".".
func (r *repo) dirName(dir string) string {
	if dir == "." {
		return filepath.Base(r.root)
	}
	return path.Base(dir)
}

func Analyze(root string) (*Result, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("analysis root %s is not a directory", abs)
	}
	rp, err := listFiles(abs)
	if err != nil {
		return nil, err
	}

	g := analyzeGo(rp)
	res := &Result{Root: abs, Modules: g.modules, Tests: g.tests, Contracts: g.contracts}
	analyzeRegexLanguages(rp, res)
	res.Contracts = append(res.Contracts, sqlContracts(rp, g, res.Modules)...)
	res.Unknowns = append(res.Unknowns, g.unknowns...)
	res.Serving = servingFacts(rp, res)
	res.TestCommands, res.BuildCommands = commands(rp, res)
	res.Docs = docs(rp)
	res.finalize()
	return res, nil
}

func skipDir(name string) bool {
	switch name {
	case "vendor", "node_modules", "testdata":
		return true
	}
	return strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_")
}

func listFiles(abs string) (*repo, error) {
	rp := &repo{root: abs, set: map[string]bool{}}
	err := filepath.WalkDir(abs, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == abs {
			return nil
		}
		if d.IsDir() {
			if skipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(abs, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		rp.files = append(rp.files, rel)
		rp.set[rel] = true
		return nil
	})
	slices.Sort(rp.files)
	return rp, err
}

func (r *Result) finalize() {
	langs := map[string]bool{}
	for i := range r.Modules {
		m := &r.Modules[i]
		langs[m.Language] = true
		slices.SortFunc(m.Symbols, func(a, b model.Symbol) int {
			return cmp.Or(compareLoc(a.Location, b.Location), cmp.Compare(a.Name, b.Name))
		})
		slices.SortFunc(m.Calls, func(a, b CallEdge) int {
			return cmp.Or(cmp.Compare(a.From, b.From), cmp.Compare(a.To, b.To), cmp.Compare(a.Location, b.Location))
		})
		m.Calls = slices.Compact(m.Calls)
		m.Imports = sortedUnique(m.Imports)
		m.Files = sortedUnique(m.Files)
	}
	r.Languages = model.SortedKeys(langs)
	slices.SortFunc(r.Modules, func(a, b ModuleFacts) int {
		return cmp.Or(cmp.Compare(a.Path, b.Path), cmp.Compare(a.Language, b.Language))
	})
	slices.SortFunc(r.Tests, func(a, b TestFacts) int {
		return cmp.Or(cmp.Compare(a.ModulePath, b.ModulePath), cmp.Compare(a.Location, b.Location), cmp.Compare(a.Name, b.Name))
	})
	slices.SortFunc(r.Contracts, func(a, b ContractFacts) int {
		return cmp.Or(cmp.Compare(a.OwnerPath, b.OwnerPath), cmp.Compare(a.Kind, b.Kind),
			cmp.Compare(a.Selector, b.Selector), cmp.Compare(a.Direction, b.Direction), cmp.Compare(a.Location, b.Location))
	})
	r.Contracts = slices.CompactFunc(r.Contracts, func(a, b ContractFacts) bool {
		return a.OwnerPath == b.OwnerPath && a.Kind == b.Kind && a.Selector == b.Selector &&
			a.Direction == b.Direction && a.Location == b.Location
	})
	slices.SortFunc(r.Serving, func(a, b ServingFacts) int {
		return cmp.Or(cmp.Compare(a.DeployUnit, b.DeployUnit), cmp.Compare(a.Source, b.Source))
	})
	r.TestCommands = sortedUnique(r.TestCommands)
	r.BuildCommands = sortedUnique(r.BuildCommands)
	r.Docs = sortedUnique(r.Docs)
	r.Unknowns = sortedUnique(r.Unknowns)
}

func sortedUnique(s []string) []string {
	slices.Sort(s)
	return slices.Compact(s)
}

func loc(file string, line int) string { return fmt.Sprintf("%s:%d", file, line) }

// compareLoc orders "file:line" by file, then numerically by line.
func compareLoc(a, b string) int {
	af, al, _ := strings.Cut(a, ":")
	bf, bl, _ := strings.Cut(b, ":")
	ai, _ := strconv.Atoi(al)
	bi, _ := strconv.Atoi(bl)
	return cmp.Or(cmp.Compare(af, bf), cmp.Compare(ai, bi))
}

// moduleFor returns the Path of the module that owns dir: the module at dir or its nearest ancestor.
func moduleFor(dir string, modules []ModuleFacts) (string, bool) {
	paths := map[string]bool{}
	for _, m := range modules {
		paths[m.Path] = true
	}
	for d := dir; ; d = path.Dir(d) {
		if paths[d] {
			return d, true
		}
		if d == "." || d == "/" {
			return "", false
		}
	}
}

func commands(rp *repo, res *Result) (tests, builds []string) {
	inDir := func(dir, cmd string) string {
		if dir == "." {
			return cmd
		}
		return "cd " + dir + " && " + cmd
	}
	hasPyTests := false
	for _, t := range res.Tests {
		if strings.HasPrefix(t.Command, "pytest") {
			hasPyTests = true
		}
	}
	for _, f := range rp.files {
		dir := path.Dir(f)
		switch path.Base(f) {
		case "go.mod":
			tests = append(tests, inDir(dir, "go test ./..."))
			// go vet compiles every package; go build ./... would write a binary into the
			// worktree for a single main package and invalidate the evidence it produced.
			builds = append(builds, inDir(dir, "go vet ./..."))
		case "package.json":
			scripts := packageScripts(rp, f)
			if scripts["test"] {
				tests = append(tests, inDir(dir, "npm test"))
			}
			if scripts["build"] {
				builds = append(builds, inDir(dir, "npm run build"))
			}
		}
	}
	if hasPyTests {
		tests = append(tests, "pytest")
	}
	return tests, builds
}

func docs(rp *repo) []string {
	var out []string
	for _, f := range rp.files {
		base := path.Base(f)
		ext := strings.ToLower(path.Ext(f))
		segs := strings.Split(path.Dir(f), "/")
		switch {
		case strings.HasPrefix(strings.ToUpper(base), "README"):
			out = append(out, f)
		case (ext == ".md" || ext == ".html") && segs[0] == "docs":
			out = append(out, f)
		case ext == ".md" && (adrFile.MatchString(base) || slices.ContainsFunc(segs, isADRDir)):
			out = append(out, f)
		}
	}
	return out
}

func isADRDir(s string) bool {
	s = strings.ToLower(s)
	return s == "adr" || s == "adrs" || s == "decisions"
}
