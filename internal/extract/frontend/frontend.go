// Package frontend holds the extractors abbd ships, one per build system.
// Each reads the build system's own model of the code, by running cargo or
// cmake, and returns the extraction JSON a per-repository script would print.
package frontend

import (
	"bytes"
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/whyjp/abbd/internal/extract"
)

// frontends is the one list of bundled front-ends. manifest is the file the
// front-end's directory must hold.
var frontends = map[string]struct {
	manifest string
	run      func(t tree) (extract.Doc, error)
}{
	"cargo": {"Cargo.toml", cargo},
	"cmake": {"CMakeLists.txt", cmake},
}

// Names lists the bundled front-ends.
func Names() []string {
	var out []string
	for n := range frontends {
		out = append(out, n)
	}
	slices.Sort(out)
	return out
}

// Spec is one front-end named on the command line as FRONTEND[=DIR], with
// DIR relative to the repository root.
type Spec struct{ Name, Dir string }

func ParseSpec(s string) (Spec, error) { return SpecOf("front-end", s, Names()) }

// SpecOf reads NAME[=DIR] for one of names, which are what kind of tool
// the command line names, such as a front-end or a relater.
func SpecOf(what, s string, names []string) (Spec, error) {
	name, dir, _ := strings.Cut(s, "=")
	if !slices.Contains(names, name) {
		w := strings.ToUpper(strings.ReplaceAll(what, "-", ""))
		return Spec{}, fmt.Errorf("unknown %s %q: use %s, as %s or %s=DIR", what, name, strings.Join(names, " or "), w, w)
	}
	dir = filepath.ToSlash(filepath.Clean(cmp.Or(dir, ".")))
	if filepath.IsAbs(dir) || dir == ".." || strings.HasPrefix(dir, "../") {
		return Spec{}, fmt.Errorf("%s: %q must be a directory inside the repository, relative to its root", name, dir)
	}
	return Spec{Name: name, Dir: dir}, nil
}

// Extract runs each front-end over its directory under root and returns
// one document. A spec named twice runs once, and two specs that make a unit
// of the same ref are an error, since setup could not tell the units apart.
// cache holds the build trees a front-end configures, and must lie outside
// root so a run never changes the code it reads.
func Extract(root, cache string, specs []Spec) (extract.Doc, error) {
	return ExtractWith(root, cache, specs, nil)
}

// ExtractWith is Extract with the cache options, each spelled -DNAME=VALUE,
// that the cmake front-end passes to the configure of its build tree. A
// repository that needs one to configure, such as an option that turns off
// the part that wants git submodules, names it there.
func ExtractWith(root, cache string, specs []Spec, defs []string) (extract.Doc, error) {
	for _, d := range defs {
		if !strings.HasPrefix(d, "-D") || len(d) < 3 {
			return extract.Doc{}, fmt.Errorf("%q is no cache option: spell it -DNAME=VALUE", d)
		}
	}
	if len(defs) > 0 && !slices.ContainsFunc(specs, func(s Spec) bool { return s.Name == "cmake" }) {
		return extract.Doc{}, fmt.Errorf("%s is a CMake cache option, and no cmake front-end is named", defs[0])
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return extract.Doc{}, err
	}
	if cache, err = filepath.Abs(cache); err != nil {
		return extract.Doc{}, err
	}
	if inside(root, cache) {
		return extract.Doc{}, fmt.Errorf("the build cache %s lies inside the repository; set ABBD_CACHE_DIR to a directory outside it", cache)
	}
	var doc extract.Doc
	from := map[string]Spec{}
	for i, s := range specs {
		if slices.Contains(specs[:i], s) {
			continue
		}
		f := frontends[s.Name]
		dir := filepath.Join(root, filepath.FromSlash(s.Dir))
		if _, err := os.Stat(filepath.Join(dir, f.manifest)); err != nil {
			return extract.Doc{}, fmt.Errorf("%s: %s has no %s", s.Name, s.Dir, f.manifest)
		}
		d, err := f.run(tree{root: root, dir: dir, cache: cache, defs: defs})
		if err != nil {
			return extract.Doc{}, fmt.Errorf("%s: %w", s.Name, err)
		}
		for _, u := range d.Units {
			if o, ok := from[u.Ref]; ok {
				return extract.Doc{}, fmt.Errorf("%s=%s and %s=%s both make a unit %s; name only one of them", o.Name, o.Dir, s.Name, s.Dir, u.Ref)
			}
			from[u.Ref] = s
		}
		doc.Units = append(doc.Units, d.Units...)
		doc.Relations = append(doc.Relations, d.Relations...)
	}
	return doc, nil
}

// tree is where a front-end runs: the repository root, its own directory
// and the cache for build trees, all absolute, and the cache options the
// cmake front-end configures with.
type tree struct {
	root, dir, cache string
	defs             []string
}

// relDir is the front-end's directory relative to the root.
func relDir(t tree) string {
	r, ok := t.rel(t.dir)
	if !ok || r == "" {
		return "."
	}
	return r
}

// rel gives an absolute path as the root-relative, forward-slash path an
// anchor holds, and false for a path outside the root.
func (t tree) rel(abs string) (string, bool) {
	r, err := filepath.Rel(t.root, filepath.Clean(abs))
	if err != nil || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) || filepath.IsAbs(r) {
		return "", false
	}
	return filepath.ToSlash(r), true
}

// buildDir is a build tree for the front-end's directory, the same on every
// run so a reconfigure is quick.
func (t tree) buildDir(name string) string {
	h := sha256.Sum256([]byte(strings.ToLower(filepath.ToSlash(t.dir)) + "\n" + strings.Join(t.defs, "\n")))
	return filepath.Join(t.cache, name, hex.EncodeToString(h[:6]))
}

// run runs a build tool in the front-end's directory and returns its stdout.
func (t tree) run(argv ...string) ([]byte, error) {
	var out, errb bytes.Buffer
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir, cmd.Stdout, cmd.Stderr = t.dir, &out, &errb
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%s failed: %w\n%s", strings.Join(argv, " "), err, tail(errb.String(), 20))
	}
	return out.Bytes(), nil
}

func tail(s string, n int) string {
	ls := strings.Split(strings.TrimRight(s, "\n"), "\n")
	return strings.Join(ls[max(0, len(ls)-n):], "\n")
}

func inside(root, p string) bool {
	r, err := filepath.Rel(root, p)
	return err == nil && r != ".." && !strings.HasPrefix(r, ".."+string(filepath.Separator)) && !filepath.IsAbs(r)
}
