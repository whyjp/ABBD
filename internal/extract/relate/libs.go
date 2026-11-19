package relate

import (
	"bufio"
	"bytes"
	"cmp"
	"encoding/json"
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/whyjp/abbd/internal/extract"
	"github.com/whyjp/abbd/internal/model"
)

// A third-party library is one lump: the manifests declare it, and each
// module that uses it gets one depends_on to it, never a line per call. The
// standard library and the system's headers are no library a manifest
// declares, so they never become one.

// ecosystem is one kind of manifest: the file, how to read the libraries it
// declares, the source files it governs and how to read what a source file
// imports.
type ecosystem struct {
	name     string
	manifest string
	declares func(text []byte, local localNames) []declaration
	source   func(p string) bool
	imports  func(text []byte) []imported
}

// declaration is one library a manifest declares: its name, the name the code
// spells it by, and the manifest's line that declares it.
type declaration struct {
	name, code string
	line       int
}

// imported is one library name a source line imports.
type imported struct {
	spec string
	line int
}

// localNames are the names of the repository's own packages, which a
// manifest may require and which are no third party.
type localNames struct {
	modules map[string]bool
	// cargoLocal says whether a workspace dependency is a path dependency.
	cargoLocal map[string]bool
}

var ecosystems = []ecosystem{
	{name: "go", manifest: "go.mod", declares: goDeclares, source: goSource, imports: goImports},
	{name: "npm", manifest: "package.json", declares: npmDeclares, source: npmSource, imports: npmImports},
	{name: "cargo", manifest: "Cargo.toml", declares: cargoDeclares, source: rustLibSource, imports: rustImports},
}

// libsSkipped are directories that hold dependencies, vendored copies or
// test data, whose manifests and code are not the project's own. A target
// directory is skipped only beside a Cargo.toml, where cargo builds into it.
var libsSkipped = map[string]bool{
	"node_modules": true, "vendor": true, "vendored": true, "third_party": true, "thirdparty": true,
	"testdata": true, "venv": true, "site-packages": true, "__pycache__": true,
}

// Libraries adds to doc each third-party library a go.mod, package.json or
// Cargo.toml under root declares, as one EXT unit anchored at each line that
// declares it, and one depends_on, via import, from each module whose code
// imports it, quoting the import. A library no code imports is used by the
// module that holds its manifest, quoting the declaration. A library no
// module uses is left out, and the notes say so. A dev or test dependency,
// an indirect one and one the repository holds itself are not declared uses.
func Libraries(root string, doc extract.Doc) (extract.Doc, []string, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return doc, nil, err
	}
	var mods []extract.DocUnit
	for _, u := range doc.Units {
		if u.Kind == model.External {
			return doc, nil, fmt.Errorf("the extraction already reports the external library %s; run abbd libs once", cmp.Or(u.Ref, u.Key))
		}
		if u.Kind == model.Module {
			mods = append(mods, u)
		}
	}
	o := index(mods)
	manifests := map[string][]string{}
	sources := map[string][]string{}
	var notes []string
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		r, _ := filepath.Rel(root, p)
		r = filepath.ToSlash(r)
		if err != nil {
			// What it may not read is left out, and said.
			notes = append(notes, fmt.Sprintf("libs: %s was not read: %v", r, err))
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			name := strings.ToLower(d.Name())
			if p != root && (strings.HasPrefix(d.Name(), ".") || libsSkipped[name] || name == "target" && isFile(filepath.Join(filepath.Dir(p), "Cargo.toml"))) {
				return filepath.SkipDir
			}
			return nil
		}
		for _, e := range ecosystems {
			if d.Name() == e.manifest {
				manifests[e.name] = append(manifests[e.name], r)
			}
			if e.source(r) {
				sources[e.name] = append(sources[e.name], r)
			}
		}
		return nil
	})
	if err != nil {
		return doc, nil, err
	}
	read := func(p string) []byte {
		b, _ := os.ReadFile(filepath.Join(root, filepath.FromSlash(p)))
		return b
	}
	type lib struct{ eco, name string }
	sites := map[lib]map[string][]site{}
	anchors := map[lib][]string{}
	for _, e := range ecosystems {
		local := localsOf(e.name, manifests[e.name], read)
		var unused []string
		for _, m := range manifests[e.name] {
			ds := e.declares(read(m), local)
			if len(ds) == 0 {
				continue
			}
			byCode := map[string][]declaration{}
			for _, d := range ds {
				byCode[d.code] = append(byCode[d.code], d)
			}
			used := map[string]bool{}
			for _, s := range governed(m, sources[e.name], manifests[e.name]) {
				from, ok := o.of(s)
				if !ok {
					continue
				}
				for _, im := range e.imports(read(s)) {
					d, ok := match(e.name, im.spec, byCode)
					if !ok {
						continue
					}
					l := lib{e.name, d.name}
					if sites[l] == nil {
						sites[l] = map[string][]site{}
					}
					sites[l][from] = append(sites[l][from], site{path: s, line: im.line})
					used[d.name] = true
					if !slices.Contains(anchors[l], m) {
						anchors[l] = append(anchors[l], m)
					}
				}
			}
			holder, held := o.of(m)
			for _, d := range ds {
				if used[d.name] {
					continue
				}
				l := lib{e.name, d.name}
				if !held {
					unused = append(unused, d.name)
					continue
				}
				if sites[l] == nil {
					sites[l] = map[string][]site{}
				}
				sites[l][holder] = append(sites[l][holder], site{path: m, line: d.line})
				if !slices.Contains(anchors[l], m) {
					anchors[l] = append(anchors[l], m)
				}
			}
		}
		if unused = slices.Compact(slices.Sorted(slices.Values(unused))); len(unused) > 0 {
			notes = append(notes, fmt.Sprintf("libs: %s: %d declared libraries are imported by no module the extraction names, and no module holds their manifest, so they are left out: %s", e.name, len(unused), strings.Join(unused, ", ")))
		}
	}
	q := quoter{root: root, lines: map[string][]string{}}
	for _, l := range slices.SortedFunc(func(yield func(lib) bool) {
		for l := range sites {
			if !yield(l) {
				return
			}
		}
	}, func(a, b lib) int { return cmp.Or(cmp.Compare(a.eco, b.eco), cmp.Compare(a.name, b.name)) }) {
		if len(sites[l]) == 0 {
			continue
		}
		ref := "lib:" + l.eco + ":" + l.name
		u := extract.DocUnit{Ref: ref, Kind: model.External, Name: l.name}
		for _, m := range anchors[l] {
			u.Anchors = append(u.Anchors, model.Anchor{Path: m, Symbol: l.name})
		}
		doc.Units = append(doc.Units, u)
		for _, from := range slices.Sorted(func(yield func(string) bool) {
			for f := range sites[l] {
				if !yield(f) {
					return
				}
			}
		}) {
			at := sites[l][from]
			slices.SortFunc(at, func(a, b site) int { return cmp.Or(cmp.Compare(a.path, b.path), cmp.Compare(a.line, b.line)) })
			at = slices.Compact(at)
			r := extract.DocRelation{From: from, To: ref, Kind: model.DependsOn, Via: "import"}
			for _, s := range at {
				if len(r.Evidence) == maxQuotes {
					break
				}
				text, err := q.line(s)
				if err != nil {
					return doc, nil, err
				}
				if text != "" {
					r.Evidence = append(r.Evidence, model.Evidence{Path: s.path, Lines: strconv.Itoa(s.line), Quote: text})
				}
			}
			doc.Relations = append(doc.Relations, r)
		}
	}
	return doc, notes, nil
}

// governed are the source files a manifest governs: those under its
// directory that no deeper manifest of its kind governs.
func governed(m string, sources, manifests []string) []string {
	dir := path.Dir(m)
	var out []string
	for _, s := range sources {
		if under(s, dir) && nearest(s, manifests) == m {
			out = append(out, s)
		}
	}
	return out
}

func under(p, dir string) bool { return dir == "." || strings.HasPrefix(p, dir+"/") }

// nearest is the manifest of the deepest directory that holds p.
func nearest(p string, manifests []string) string {
	best := ""
	for _, m := range manifests {
		if d := path.Dir(m); under(p, d) && (best == "" || len(d) > len(path.Dir(best))) {
			best = m
		}
	}
	return best
}

// match is the declared library an import names: the longest declared name
// that is the import or a path under it, for Go; the package an npm
// specifier names; the crate a Rust path starts with.
func match(eco, spec string, byCode map[string][]declaration) (declaration, bool) {
	switch eco {
	case "go":
		best := ""
		for c := range byCode {
			if (spec == c || strings.HasPrefix(spec, c+"/")) && len(c) > len(best) {
				best = c
			}
		}
		if best == "" {
			return declaration{}, false
		}
		return byCode[best][0], true
	case "npm":
		parts := strings.SplitN(spec, "/", 3)
		name := parts[0]
		if strings.HasPrefix(spec, "@") && len(parts) > 1 {
			name += "/" + parts[1]
		}
		ds, ok := byCode[name]
		if !ok {
			return declaration{}, false
		}
		return ds[0], true
	default:
		ds, ok := byCode[spec]
		if !ok {
			return declaration{}, false
		}
		return ds[0], true
	}
}

// localsOf reads the names the repository's own manifests give its packages.
func localsOf(eco string, manifests []string, read func(string) []byte) localNames {
	l := localNames{modules: map[string]bool{}, cargoLocal: map[string]bool{}}
	for _, m := range manifests {
		text := read(m)
		switch eco {
		case "go":
			for _, line := range strings.Split(string(text), "\n") {
				if f := strings.Fields(stripComment(line, "//")); len(f) == 2 && f[0] == "module" {
					l.modules[strings.Trim(f[1], `"`)] = true
				}
			}
		case "npm":
			var p struct {
				Name string `json:"name"`
			}
			if json.Unmarshal(text, &p) == nil && p.Name != "" {
				l.modules[p.Name] = true
			}
		case "cargo":
			for _, t := range tomlLines(text) {
				if t.section == "package" && t.key == "name" {
					l.modules[t.str()] = true
				}
				if t.section == "workspace.dependencies" && t.key != "" {
					name, _, _ := strings.Cut(t.key, ".")
					if strings.HasSuffix(t.key, ".path") || t.inline("path") != "" {
						l.cargoLocal[name] = true
					}
				}
				if strings.HasPrefix(t.section, "workspace.dependencies.") && t.key == "path" {
					l.cargoLocal[strings.TrimPrefix(t.section, "workspace.dependencies.")] = true
				}
			}
		}
	}
	return l
}

func stripComment(line, mark string) string {
	if i := strings.Index(line, mark); i >= 0 {
		return line[:i]
	}
	return line
}

// goDeclares reads a go.mod's direct requires, leaving out the indirect
// ones, the repository's own modules and those a replace points at a
// directory of the repository.
func goDeclares(text []byte, local localNames) []declaration {
	lines := strings.Split(string(text), "\n")
	replaced := map[string]bool{}
	block := ""
	var reqs []declaration
	for i, raw := range lines {
		line := strings.TrimSpace(raw)
		body := strings.TrimSpace(stripComment(line, "//"))
		f := strings.Fields(body)
		switch {
		case len(f) == 0:
			continue
		case block != "" && f[0] == ")":
			block = ""
			continue
		case len(f) == 2 && f[1] == "(":
			block = f[0]
			continue
		}
		verb := block
		if verb == "" {
			verb, f = f[0], f[1:]
		}
		switch verb {
		case "require":
			if len(f) >= 1 && !strings.Contains(line, "// indirect") {
				name := strings.Trim(f[0], `"`)
				reqs = append(reqs, declaration{name: name, code: name, line: i + 1})
			}
		case "replace":
			if j := slices.Index(f, "=>"); j > 0 && j+1 < len(f) && (strings.HasPrefix(f[j+1], ".") || strings.HasPrefix(f[j+1], "/")) {
				replaced[strings.Trim(f[0], `"`)] = true
			}
		}
	}
	return slices.DeleteFunc(reqs, func(d declaration) bool { return local.modules[d.name] || replaced[d.name] })
}

func goSource(p string) bool {
	return strings.HasSuffix(p, ".go") && !strings.HasSuffix(p, "_test.go")
}

func goImports(text []byte) []imported {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "", text, parser.ImportsOnly)
	if err != nil || f == nil {
		return nil
	}
	var out []imported
	for _, im := range f.Imports {
		if s, err := strconv.Unquote(im.Path.Value); err == nil {
			out = append(out, imported{s, fset.Position(im.Path.Pos()).Line})
		}
	}
	return out
}

// npmSections are the package.json sections whose libraries the package
// uses when it runs; devDependencies only build or test it.
var npmSections = []string{"dependencies", "peerDependencies", "optionalDependencies"}

// npmDeclares reads a package.json's runtime dependencies, leaving out a
// workspace, file, link or portal one and the repository's own packages.
func npmDeclares(text []byte, local localNames) []declaration {
	var p map[string]json.RawMessage
	if json.Unmarshal(text, &p) != nil {
		return nil
	}
	lines := strings.Split(string(text), "\n")
	var out []declaration
	for _, sec := range npmSections {
		var deps map[string]string
		if json.Unmarshal(p[sec], &deps) != nil {
			continue
		}
		at := slices.IndexFunc(lines, func(l string) bool { return jsonKey(l) == sec })
		for _, name := range slices.Sorted(func(yield func(string) bool) {
			for n := range deps {
				if !yield(n) {
					return
				}
			}
		}) {
			v := deps[name]
			if local.modules[name] || strings.HasPrefix(v, "workspace:") || strings.HasPrefix(v, "file:") || strings.HasPrefix(v, "link:") || strings.HasPrefix(v, "portal:") {
				continue
			}
			// The line that declares it, else the first that names it, as a
			// section written on one line has it.
			line := 0
			for i := max(at, 0); i < len(lines) && line == 0; i++ {
				if jsonKey(lines[i]) == name {
					line = i + 1
				}
			}
			for i := max(at, 0); i < len(lines) && line == 0; i++ {
				if strings.Contains(lines[i], strconv.Quote(name)) {
					line = i + 1
				}
			}
			if line > 0 && !slices.ContainsFunc(out, func(d declaration) bool { return d.name == name }) {
				out = append(out, declaration{name: name, code: name, line: line})
			}
		}
	}
	return out
}

// jsonKey is the key a line of JSON opens with, or "".
func jsonKey(line string) string {
	t := strings.TrimSpace(line)
	if !strings.HasPrefix(t, `"`) {
		return ""
	}
	end := strings.Index(t[1:], `"`)
	if end < 0 || !strings.HasPrefix(strings.TrimSpace(t[end+2:]), ":") {
		return ""
	}
	return t[1 : end+1]
}

func npmSource(p string) bool {
	_, ok := tsLang.exts[strings.ToLower(path.Ext(p))]
	return ok && !tsTest(p) && !declarationFile(p)
}

var npmImport = regexp.MustCompile(`(?:^\s*(?:import|export)\b.*\bfrom\s*|^\s*\}\s*from\s*|\brequire\s*\(\s*|\bimport\s*\(\s*|^\s*import\s+)['"]([^'"]+)['"]`)

func npmImports(text []byte) []imported {
	var out []imported
	sc := bufio.NewScanner(bytes.NewReader(text))
	sc.Buffer(nil, 1<<20)
	for n := 1; sc.Scan(); n++ {
		line := sc.Text()
		if t := strings.TrimSpace(line); strings.HasPrefix(t, "//") || strings.HasPrefix(t, "*") {
			continue
		}
		for _, m := range npmImport.FindAllStringSubmatch(stripBlock(line), -1) {
			s := m[1]
			if strings.HasPrefix(s, ".") || strings.HasPrefix(s, "/") || strings.HasPrefix(s, "node:") {
				continue
			}
			out = append(out, imported{s, n})
		}
	}
	return out
}

// tomlLine is one key line of a TOML file, with the table it is under.
type tomlLine struct {
	section, key, value string
	line                int
}

// str is the value as a plain string.
func (t tomlLine) str() string { return strings.Trim(strings.TrimSpace(t.value), `"'`) }

// inline is a key's string value in an inline table value, or "".
func (t tomlLine) inline(key string) string {
	m := tomlInline[key].FindStringSubmatch(t.value)
	if m == nil {
		return ""
	}
	return strings.Trim(m[1], `"'`)
}

// tomlInline reads the keys of a dependency's inline table that say where
// it comes from.
var tomlInline = map[string]*regexp.Regexp{}

func init() {
	for _, k := range []string{"path", "workspace", "package"} {
		tomlInline[k] = regexp.MustCompile(`(?:^|[{,\s])` + k + `\s*=\s*("[^"]*"|'[^']*'|true|false)`)
	}
}

// tomlLines reads the key lines of a TOML file, each under its table. It
// reads what a Cargo.toml says of its dependencies, not all of TOML: one key
// per line, and inline tables on one line.
func tomlLines(text []byte) []tomlLine {
	var out []tomlLine
	section := ""
	// open counts the brackets a value left open, so the lines that go on
	// with an inline table or an array join its value and are no keys.
	open := 0
	for i, raw := range strings.Split(string(text), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if open > 0 {
			out[len(out)-1].value += " " + line
			open += brackets(line)
			continue
		}
		if strings.HasPrefix(line, "[") {
			s := strings.Trim(strings.TrimSpace(stripComment(line, "#")), "[]")
			section = strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(s), `"`, ""), "'", "")
			out = append(out, tomlLine{section: section, line: i + 1})
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		out = append(out, tomlLine{section: section, key: strings.Trim(strings.TrimSpace(k), `"'`), value: strings.TrimSpace(v), line: i + 1})
		open = max(brackets(v), 0)
	}
	return out
}

// brackets is how many more brackets and braces a line of TOML opens than
// it closes, outside its strings and its comment.
func brackets(line string) int {
	n := 0
	var quote rune
	for _, c := range line {
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '#':
			return n
		case c == '[' || c == '{':
			n++
		case c == ']' || c == '}':
			n--
		}
	}
	return n
}

// cargoSection is whether a table lists dependencies the crate builds with:
// [dependencies], [build-dependencies] and their per-target forms, not
// [dev-dependencies].
func cargoSection(s string) bool {
	if strings.HasPrefix(s, "target.") {
		i := strings.LastIndex(s, ".")
		s = s[i+1:]
	}
	return s == "dependencies" || s == "build-dependencies"
}

// cargoDeclares reads a Cargo.toml's dependencies, leaving out the dev ones,
// a path dependency, and a workspace one the workspace gives a path. A
// dependency the crate renames is the package it names, spelled in the code
// by its key.
func cargoDeclares(text []byte, local localNames) []declaration {
	type dep struct {
		declaration
		path, workspace bool
		pkg             string
	}
	var deps []*dep
	byTable := map[string]*dep{}
	for _, t := range tomlLines(text) {
		if t.key == "" {
			// A table of one dependency, as [dependencies.serde].
			if i := strings.LastIndex(t.section, "."); i > 0 && cargoSection(t.section[:i]) {
				d := &dep{declaration: declaration{name: t.section[i+1:], code: t.section[i+1:], line: t.line}}
				deps = append(deps, d)
				byTable[t.section] = d
			}
			continue
		}
		if d, ok := byTable[t.section]; ok {
			switch t.key {
			case "path":
				d.path = true
			case "workspace":
				d.workspace = t.str() == "true"
			case "package":
				d.pkg = t.str()
			}
			continue
		}
		if !cargoSection(t.section) {
			continue
		}
		key := t.key
		d := &dep{declaration: declaration{line: t.line}}
		switch {
		case strings.HasSuffix(key, ".workspace"):
			key, d.workspace = strings.TrimSuffix(key, ".workspace"), true
		case strings.HasSuffix(key, ".path"):
			key, d.path = strings.TrimSuffix(key, ".path"), true
		case strings.Contains(key, "."):
			key, _, _ = strings.Cut(key, ".")
		}
		d.path = d.path || t.inline("path") != ""
		d.workspace = d.workspace || t.inline("workspace") == "true"
		d.pkg = t.inline("package")
		d.name, d.code = key, key
		if j := slices.IndexFunc(deps, func(o *dep) bool { return o.code == key }); j >= 0 {
			continue
		}
		deps = append(deps, d)
	}
	var out []declaration
	for _, d := range deps {
		if d.pkg != "" {
			d.name = d.pkg
		}
		d.code = strings.ReplaceAll(d.code, "-", "_")
		if d.path || d.workspace && local.cargoLocal[d.name] || local.modules[d.name] {
			continue
		}
		if !slices.ContainsFunc(out, func(o declaration) bool { return o.code == d.code }) {
			out = append(out, d.declaration)
		}
	}
	return out
}

func rustLibSource(p string) bool { return strings.HasSuffix(p, ".rs") && !rustTest(p) }

// rustPath reads the crate a path starts with: a name followed by :: that
// no :: or name runs into, as serde::Serialize or #[tokio::main], and the
// crate a use or extern crate names whole.
var rustPath = regexp.MustCompile(`(?:^|[^:\w])([A-Za-z_][A-Za-z0-9_]*)\s*::|\b(?:use|extern\s+crate)\s+([A-Za-z_][A-Za-z0-9_]*)\b`)

// rustLocal are the path roots that name the crate's own code.
var rustLocal = map[string]bool{"crate": true, "self": true, "super": true, "Self": true}

// rustImports reads each crate name a line of Rust starts a path with, its
// strings and its comment left out. Names the code does not declare as a
// dependency, such as std, match nothing.
func rustImports(text []byte) []imported {
	var out []imported
	for i, line := range strings.Split(string(text), "\n") {
		t := strings.TrimSpace(codeOnly(line))
		if strings.HasPrefix(t, "*") {
			continue
		}
		for _, m := range rustPath.FindAllStringSubmatch(t, -1) {
			if name := cmp.Or(m[1], m[2]); !rustLocal[name] {
				out = append(out, imported{name, i + 1})
			}
		}
	}
	return out
}

// codeOnly is a line of C-like code without its string literals and its
// trailing // comment.
func codeOnly(line string) string {
	var b strings.Builder
	var quote rune
	escaped := false
	rs := []rune(line)
	for i := 0; i < len(rs); i++ {
		c := rs[i]
		switch {
		case quote != 0:
			if escaped {
				escaped = false
			} else if c == '\\' {
				escaped = true
			} else if c == quote {
				quote = 0
			}
		case c == '"':
			quote = c
		case c == '/' && i+1 < len(rs) && rs[i+1] == '/':
			return b.String()
		default:
			b.WriteRune(c)
		}
	}
	return b.String()
}

// stripBlock is a line without the /* */ comments it opens and closes.
func stripBlock(line string) string {
	for {
		i := strings.Index(line, "/*")
		if i < 0 {
			return line
		}
		j := strings.Index(line[i+2:], "*/")
		if j < 0 {
			return line[:i]
		}
		line = line[:i] + line[i+2+j+2:]
	}
}
