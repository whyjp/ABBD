package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/whyjp/abbd/internal/parts"
)

// codeExt are the extensions parts reads as source code.
var codeExt = map[string]bool{
	".go": true, ".py": true, ".js": true, ".mjs": true, ".cjs": true, ".ts": true, ".tsx": true,
	".jsx": true, ".rs": true, ".java": true, ".kt": true, ".rb": true, ".php": true, ".cs": true,
	".swift": true, ".scala": true, ".ex": true, ".c": true, ".h": true, ".cc": true, ".cpp": true, ".hpp": true,
}

var (
	testFile = regexp.MustCompile(`(^test_.*\.py$|_test\.\w+$|\.(test|spec)\.\w+$|^conftest\.py$|^tests?\.\w+$)`)
	testDir  = map[string]bool{"test": true, "tests": true, "testdata": true, "__tests__": true, "testing": true, "node_modules": true, "vendor": true}
)

// dirFiles lists the source files under dir, test files left out. A file
// directly inside dir is a node of its own; a file deeper down belongs to
// the node of the subdirectory of dir that holds it.
func dirFiles(root, dir string) ([]parts.File, error) {
	base := filepath.Join(root, filepath.FromSlash(dir))
	if st, err := os.Stat(base); err != nil || !st.IsDir() {
		return nil, fmt.Errorf("%s is no directory under %s", dir, root)
	}
	var out []parts.File
	err := filepath.WalkDir(base, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			if p != base && (strings.HasPrefix(name, ".") || testDir[name]) {
				return filepath.SkipDir
			}
			return nil
		}
		if !codeExt[path.Ext(name)] || testFile.MatchString(name) {
			return nil
		}
		rel, err := filepath.Rel(base, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		f := parts.File{Path: path.Join(dir, rel), Node: path.Join(dir, rel)}
		if i := strings.Index(rel, "/"); i > 0 {
			f.Node = path.Join(dir, rel[:i]) + "/"
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		f.Text = string(b)
		out = append(out, f)
		return nil
	})
	slices.SortFunc(out, func(a, b parts.File) int { return strings.Compare(a.Path, b.Path) })
	return out, err
}

func (e env) parts(dir string) error {
	dir = path.Clean(filepath.ToSlash(dir))
	files, err := dirFiles(e.root, dir)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("%s holds no source files", dir)
	}
	res := parts.Propose(files)
	if e.asJSON {
		enc := json.NewEncoder(e.stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(partsJSON(res))
	}
	for _, p := range res.Parts {
		fmt.Fprintf(e.stdout, "%s\n", p.Name)
		for _, n := range p.Nodes {
			fmt.Fprintf(e.stdout, "  %s\n", n)
		}
		for _, l := range p.Links {
			fmt.Fprintf(e.stdout, "    %s:%d uses %s: %s\n", l.From, l.Line, strings.Join(l.Names, ", "), l.Quote)
		}
	}
	if len(res.Loose) > 0 {
		fmt.Fprintf(e.stdout, "module itself\n")
		for _, f := range res.Loose {
			fmt.Fprintf(e.stdout, "  %s\n", f)
		}
	}
	return nil
}

type partOut struct {
	Name     string         `json:"name"`
	Symbol   string         `json:"symbol,omitempty"`
	Family   string         `json:"family,omitempty"`
	Shared   bool           `json:"shared,omitempty"`
	Nodes    []string       `json:"nodes"`
	Evidence []evidenceLine `json:"evidence"`
}

type evidenceLine struct {
	Path  string `json:"path"`
	Lines string `json:"lines"`
	Quote string `json:"quote"`
}

type partsOut struct {
	Parts []partOut `json:"parts"`
	Loose []string  `json:"loose"`
}

func partsJSON(r parts.Result) partsOut {
	out := partsOut{Parts: []partOut{}, Loose: r.Loose}
	if out.Loose == nil {
		out.Loose = []string{}
	}
	for _, p := range r.Parts {
		po := partOut{Name: p.Name, Symbol: p.Symbol, Family: p.Family, Shared: p.Shared, Nodes: p.Nodes, Evidence: []evidenceLine{}}
		for _, l := range p.Links {
			po.Evidence = append(po.Evidence, evidenceLine{l.From, fmt.Sprint(l.Line), l.Quote})
		}
		out.Parts = append(out.Parts, po)
	}
	return out
}
