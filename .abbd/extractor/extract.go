// Command extract prints the ABBD extraction result for the ABBD repository.
// Run it from the repository root: go run .abbd/extractor/extract.go
//
// Modules are Go packages, and cmd/abbd is the CLI. Contracts are read from
// the code that defines their shape; each CLI command is a contract of its own.
// Relations come from the symbols each unit references, the command names
// other cmd/abbd modules spell out, and the `abbd ...` commands the skills quote.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
)

const module = "github.com/whyjp/abbd/"

type anchor struct {
	Path   string `json:"path"`
	Symbol string `json:"symbol,omitempty"`
}

type unit struct {
	Key       string   `json:"ref"`
	Kind      string   `json:"kind"`
	Name      string   `json:"name"`
	Signature string   `json:"signature,omitempty"`
	Anchors   []anchor `json:"anchors"`
	Digest    string   `json:"digest,omitempty"`
	Entries   []entry  `json:"entries,omitempty"`
	Parent    string   `json:"parent,omitempty"`
}

// entry is one part of a contract the code spells: a command's flag.
type entry struct {
	Name     string `json:"name"`
	Shape    string `json:"shape,omitempty"`
	Required *bool  `json:"required,omitempty"`
}

type relation struct {
	From string `json:"from"`
	To   string `json:"to"`
	Kind string `json:"kind"`
}

// cited is a relation as printed, with the lines of text that state it.
type cited struct {
	relation
	Via      string     `json:"via,omitempty"`
	Evidence []evidence `json:"evidence,omitempty"`
	Entries  []string   `json:"entries,omitempty"`
}

// via says how a relation the script reads arises, so the drawing tells a
// design-only line from a run-time one: a package another names is called,
// a command is called, a file contract carries data, a config is
// read as config and an event calls back.
func via(r relation, kind map[string]string) string {
	if r.Kind == "depends_on" {
		// A dependency that names only types or values arises at build
		// time, but drawing those dotted moves this repository's
		// layering past its lint ceilings, so the render track decides it.
		return "call"
	}
	switch kind[r.To] {
	case "DATA":
		return "data"
	case "CFG":
		return "config"
	case "EVT":
		return "callback"
	}
	return "call"
}

type evidence struct {
	Path  string `json:"path"`
	Lines string `json:"lines"`
	Quote string `json:"quote"`
}

type result struct {
	Units     []unit  `json:"units"`
	Relations []cited `json:"relations"`
}

// srcFile is one parsed non-test Go file.
type srcFile struct {
	path    string // repo-relative, forward slashes
	dir     string
	ast     *ast.File
	src     []byte
	imports map[string]string // local name -> repo-relative package dir
}

var (
	fset  = token.NewFileSet()
	files = map[string]*srcFile{}
	// decls maps a package dir to the file that declares each top-level name.
	decls = map[string]map[string]string{}
	// methods holds "dir.Name" for every method a package declares.
	methods = map[string]bool{}

	// types maps "dir.Name" to its type spec and declaring file.
	types = map[string]typeDecl{}
)

type typeDecl struct {
	spec *ast.TypeSpec
	file *srcFile
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "extract:", err)
		os.Exit(1)
	}
}

func run() error {
	if err := parseTree("cmd"); err != nil {
		return err
	}
	if err := parseTree("internal"); err != nil {
		return err
	}
	owners := moduleOwners()
	var units []unit
	for _, m := range modules() {
		units = append(units, m)
	}
	cs, err := contracts()
	if err != nil {
		return err
	}
	units = append(units, cs...)
	feats, featRels, featCites, featEntries, err := features()
	if err != nil {
		return err
	}
	units = append(units, feats...)
	for _, u := range units {
		if err := checkAnchors(u); err != nil {
			return err
		}
	}

	rels := map[relation]bool{}
	refs := references(owners)
	for from, tos := range refs {
		for to := range tos {
			if from != to {
				rels[relation{from, to, "depends_on"}] = true
			}
		}
	}
	for _, c := range cs {
		prov := provider(c, owners)
		if prov != "" {
			rels[relation{prov, c.Key, "provides"}] = true
		}
		for _, consumer := range consumers(c, owners) {
			if consumer != prov {
				rels[relation{consumer, c.Key, "consumes"}] = true
			}
		}
	}
	for _, r := range featRels {
		rels[r] = true
	}
	// A module that writes a field of a file contract provides it, and one
	// that reads a field consumes it, narrowed to those fields.
	for _, c := range contractTable() {
		if c.typ == "" {
			continue
		}
		writes, reads := fieldUses(c, owners)
		for kind, by := range map[string]map[string][]string{"provides": writes, "consumes": reads} {
			for owner, fields := range by {
				if kind == "consumes" && owner == c.provider {
					continue
				}
				r := relation{owner, c.key, kind}
				rels[r] = true
				featEntries[r] = append(featEntries[r], fields...)
			}
		}
	}

	out := result{Units: units}
	kind := map[string]string{}
	for _, u := range units {
		kind[u.Key] = u.Kind
	}
	for r := range rels {
		out.Relations = append(out.Relations, cited{r, via(r, kind), featCites[r], featEntries[r]})
	}
	sort.Slice(out.Units, func(i, j int) bool { return out.Units[i].Key < out.Units[j].Key })
	sort.Slice(out.Relations, func(i, j int) bool {
		a, b := out.Relations[i], out.Relations[j]
		return a.From+"\x00"+a.To+"\x00"+a.Kind < b.From+"\x00"+b.To+"\x00"+b.Kind
	})
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func parseTree(root string) error {
	return filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		rel := filepath.ToSlash(p)
		src, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		f, err := parser.ParseFile(fset, p, src, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		sf := &srcFile{path: rel, dir: path.Dir(rel), ast: f, src: src, imports: map[string]string{}}
		for _, im := range f.Imports {
			ip, _ := strconv.Unquote(im.Path.Value)
			if !strings.HasPrefix(ip, module) {
				continue
			}
			dir := strings.TrimPrefix(ip, module)
			name := path.Base(dir)
			if im.Name != nil {
				name = im.Name.Name
			}
			sf.imports[name] = dir
		}
		files[rel] = sf
		if decls[sf.dir] == nil {
			decls[sf.dir] = map[string]string{}
		}
		for _, name := range topLevel(f) {
			decls[sf.dir][name] = rel
		}
		for _, d := range f.Decls {
			if fd, ok := d.(*ast.FuncDecl); ok && fd.Recv != nil {
				methods[sf.dir+"."+fd.Name.Name] = true
			}
			if g, ok := d.(*ast.GenDecl); ok {
				for _, s := range g.Specs {
					if ts, ok := s.(*ast.TypeSpec); ok {
						types[sf.dir+"."+ts.Name.Name] = typeDecl{ts, sf}
					}
				}
			}
		}
		return nil
	})
}

// topLevel lists the names a file declares, methods included.
func topLevel(f *ast.File) []string {
	var out []string
	for _, d := range f.Decls {
		switch d := d.(type) {
		case *ast.FuncDecl:
			out = append(out, d.Name.Name)
		case *ast.GenDecl:
			for _, s := range d.Specs {
				switch s := s.(type) {
				case *ast.TypeSpec:
					out = append(out, s.Name.Name)
				case *ast.ValueSpec:
					for _, n := range s.Names {
						out = append(out, n.Name)
					}
				}
			}
		}
	}
	return out
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// moduleOwners maps each Go file to the module unit that owns it.
func moduleOwners() map[string]string {
	owners := map[string]string{}
	for _, p := range sortedKeys(files) {
		f := files[p]
		switch {
		case f.dir == "cmd/abbd":
			owners[p] = "mod-cmd-abbd"
		case strings.HasPrefix(f.dir, "internal/"):
			owners[p] = "mod-" + strings.TrimPrefix(f.dir, "internal/")
		}
	}
	return owners
}

func modules() []unit {
	byDir := map[string][]string{}
	for _, p := range sortedKeys(files) {
		byDir[files[p].dir] = append(byDir[files[p].dir], p)
	}
	var out []unit
	for _, dir := range sortedKeys(byDir) {
		if !strings.HasPrefix(dir, "internal/") {
			continue
		}
		u := unit{Key: "mod-" + strings.TrimPrefix(dir, "internal/"), Kind: "MOD", Name: dir, Anchors: []anchor{{Path: dir}}}
		// A package inside another package's directory sits inside its module.
		if up := path.Dir(dir); up != "internal" && len(byDir[up]) > 0 {
			u.Parent = "mod-" + strings.TrimPrefix(up, "internal/")
		}
		for _, p := range byDir[dir] {
			u.Anchors = append(u.Anchors, anchor{Path: p})
		}
		out = append(out, u)
	}
	cli := unit{Key: "mod-cmd-abbd", Kind: "MOD", Name: "abbd CLI", Anchors: []anchor{{Path: "cmd/abbd"}}}
	for _, p := range byDir["cmd/abbd"] {
		cli.Anchors = append(cli.Anchors, anchor{Path: p})
	}
	return append(out, cli)
}

// references maps each module to the modules whose symbols it uses.
func references(owners map[string]string) map[string]map[string]bool {
	out := map[string]map[string]bool{}
	for _, p := range sortedKeys(files) {
		f := files[p]
		from := owners[p]
		if from == "" {
			continue
		}
		if out[from] == nil {
			out[from] = map[string]bool{}
		}
		for _, ref := range symbolRefs(f) {
			dir, name, _ := strings.Cut(ref, ".")
			if decl, ok := decls[dir][name]; ok && owners[decl] != "" {
				out[from][owners[decl]] = true
			}
		}
	}
	return out
}

// symbolRefs lists "dir.Name" for every module symbol a file names: imported
// package members through selectors, methods of imported packages by name,
// and same-package names declared in another file. Matching by name can
// overcount a local name that shares a declared one, never undercount.
func symbolRefs(f *srcFile) []string {
	seen := map[string]bool{}
	own := map[string]bool{}
	for _, n := range topLevel(f.ast) {
		own[n] = true
	}
	ast.Inspect(f.ast, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.SelectorExpr:
			if x, ok := n.X.(*ast.Ident); ok {
				if dir, ok := f.imports[x.Name]; ok {
					seen[dir+"."+n.Sel.Name] = true
					return false
				}
			}
			for _, dir := range f.imports {
				if methods[dir+"."+n.Sel.Name] {
					seen[dir+"."+n.Sel.Name] = true
				}
			}
		case *ast.Ident:
			if decl, ok := decls[f.dir][n.Name]; ok && decl != f.path && !own[n.Name] {
				seen[f.dir+"."+n.Name] = true
			}
		}
		return true
	})
	return sortedKeys(seen)
}

// contract is how one contract is anchored and how its signature is read.
type contract struct {
	key, kind, name string
	anchors         []anchor
	// provider is the module whose code produces the contract; "" when no
	// module owns it, as for a file the host reads.
	provider  string
	signature func() (string, error)
	// digest fingerprints the code the contract's meaning lives in; nil
	// fingerprints the anchors.
	digest func() (string, error)
	// entries are the contract's parts one by one; nil when it lists none.
	entries func() []entry
	// typ is the struct a file contract serializes, by "dir.Name", and tag
	// the struct tag that names its fields; its fields are the entries.
	typ, tag string
}

func contractTable() []contract {
	literal := func(s string) func() (string, error) { return func() (string, error) { return s, nil } }
	return append(commandContracts(), []contract{
		{"if-cli", "IF", "abbd exit codes", []anchor{{"cmd/abbd/main.go", "flagDefs"}, {"cmd/abbd/main.go", "exitUsage"}, {"cmd/abbd/main.go", "parse"}}, "mod-cmd-abbd", exitSig, nil, nil, "", ""},
		{"if-edge-key", "IF", "relation key", []anchor{{"internal/model/model.go", "Edge"}, {"internal/model/model.go", "ParseEdge"}}, "mod-model", edgeKeySig, nil, nil, "", ""},
		{"data-extraction", "DATA", "extraction JSON", []anchor{{"internal/extract/extract.go", "Doc"}, {"internal/extract/extract.go", "DocUnit"}, {"internal/extract/extract.go", "Parse"}}, "mod-extract", literal("extraction json"), nil, nil, "internal/extract.Doc", "json"},
		{"cfg-project", "CFG", "project.yaml", []anchor{{"internal/store/store.go", "Project"}, {"internal/store/store.go", "LoadProject"}, {"internal/store/store.go", "SaveProject"}}, "mod-store", literal(".abbd/project.yaml"), nil, nil, "internal/store.Project", "yaml"},
		{"data-units", "DATA", "unit files", []anchor{{"internal/store/store.go", "SaveUnits"}, {"internal/store/store.go", "LoadUnits"}, {"internal/model/model.go", "Unit"}}, "mod-store", literal(".abbd/{architecture,features}/ID.md frontmatter"), nil, nil, "internal/model.Unit", "yaml"},
		{"data-understanding", "DATA", "module understanding", []anchor{{"internal/store/understanding.go", "SaveUnderstanding"}, {"internal/store/understanding.go", "LoadUnderstandings"}, {"internal/store/understanding.go", "Understanding"}}, "mod-store", literal(".abbd/understanding/MOD.md frontmatter"), nil, nil, "internal/store.Understanding", "yaml"},
		{"data-notes", "DATA", "slot notes", []anchor{{"internal/store/note.go", "SaveNote"}, {"internal/store/note.go", "LoadNotes"}, {"internal/store/note.go", "Note"}}, "mod-store", literal(".abbd/understanding/ID/SLOT[.KEY].md frontmatter"), nil, nil, "internal/store.Note", "yaml"},
		{"data-state", "DATA", "sync state", []anchor{{"internal/store/store.go", "SaveState"}, {"internal/store/store.go", "LoadState"}, {"internal/store/store.go", "State"}}, "mod-store", literal(".abbd/state.yaml"), nil, nil, "internal/store.State", "yaml"},
		{"data-blueprint", "DATA", "blueprint files", []anchor{{"internal/screen/page.go", "Graph"}, {"internal/screen/page.go", "Files"}, {"internal/screen/site.go", "Pages"}}, "mod-screen", blueprintSig, nil, nil, "", ""},
		{"cfg-plugin", "CFG", "plugin and marketplace manifest", []anchor{{".claude-plugin/marketplace.json", ""}, {"plugins/abbd/.claude-plugin/plugin.json", ""}}, "", pluginSig, nil, nil, "", ""},
	}...)
}

func contracts() ([]unit, error) {
	var out []unit
	for _, c := range contractTable() {
		sig, err := c.signature()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", c.key, err)
		}
		digest := c.digest
		if digest == nil {
			digest = func() (string, error) { return anchorsDigest(c.anchors) }
		}
		d, err := digest()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", c.key, err)
		}
		u := unit{Key: c.key, Kind: c.kind, Name: c.name, Signature: sig, Anchors: c.anchors, Digest: d}
		if c.entries != nil {
			u.Entries = c.entries()
		}
		if c.typ != "" {
			for _, f := range structFields(c.typ, c.tag, "", map[string]bool{}) {
				u.Entries = append(u.Entries, f.entry)
			}
		}
		out = append(out, u)
	}
	return out, nil
}

func provider(u unit, _ map[string]string) string {
	for _, c := range contractTable() {
		if c.key == u.Key {
			return c.provider
		}
	}
	return ""
}

// consumers are the modules that name one of the contract's anchors in its
// provider's code. A shape type defined elsewhere, such as model.Unit, is
// shared vocabulary: using it does not read or write the contract.
func consumers(u unit, owners map[string]string) []string {
	prov := provider(u, owners)
	targets := map[string]bool{}
	for _, a := range u.Anchors {
		if a.Symbol != "" && strings.HasSuffix(a.Path, ".go") && owners[a.Path] == prov {
			targets[path.Dir(a.Path)+"."+a.Symbol] = true
		}
	}
	found := map[string]bool{}
	for _, p := range sortedKeys(files) {
		o := owners[p]
		if o == "" || o == prov {
			continue
		}
		for _, ref := range symbolRefs(files[p]) {
			if targets[ref] {
				found[o] = true
			}
		}
	}
	if name, ok := strings.CutPrefix(u.Name, "abbd "); ok && strings.HasPrefix(u.Key, "if-cli-") {
		for _, p := range sortedKeys(files) {
			if o := owners[p]; o != "" && o != prov && files[p].dir == "cmd/abbd" && spellsCommand(files[p], name) {
				found[o] = true
			}
		}
	}
	return sortedKeys(found)
}

// spellsCommand reports whether a file of package main names a command as a
// string, as a message that tells the reader what to run next does.
func spellsCommand(f *srcFile, name string) bool {
	found := false
	ast.Inspect(f.ast, func(n ast.Node) bool {
		if b, ok := n.(*ast.BasicLit); ok && b.Kind == token.STRING {
			if v := lit(b); v == name || strings.HasPrefix(v, name+" ") {
				found = true
			}
		}
		return !found
	})
	return found
}

func checkAnchors(u unit) error {
	if len(u.Anchors) < 2 {
		return fmt.Errorf("%s has %d anchor(s); ABBD keeps an ID only through two", u.Key, len(u.Anchors))
	}
	for _, a := range u.Anchors {
		st, err := os.Stat(filepath.FromSlash(a.Path))
		if err != nil {
			return fmt.Errorf("%s: anchor %s: %w", u.Key, a.Path, err)
		}
		if a.Symbol == "" || st.IsDir() || !strings.HasSuffix(a.Path, ".go") {
			continue
		}
		if decls[path.Dir(a.Path)][a.Symbol] != a.Path {
			return fmt.Errorf("%s: %s declares no %s", u.Key, a.Path, a.Symbol)
		}
	}
	return nil
}

// structSig renders the serialized field names of a struct, sorted, recursing
// into module struct types as name{...}.
func structSig(qualified, tagKey string, visiting map[string]bool) (string, error) {
	td, ok := types[qualified]
	if !ok {
		return "", fmt.Errorf("no type %s", qualified)
	}
	st, ok := td.spec.Type.(*ast.StructType)
	if !ok {
		return "", fmt.Errorf("%s is not a struct", qualified)
	}
	if visiting[qualified] {
		return "", nil
	}
	visiting[qualified] = true
	defer delete(visiting, qualified)
	return fieldsSig(st, td.file, tagKey, visiting)
}

func fieldsSig(st *ast.StructType, f *srcFile, tagKey string, visiting map[string]bool) (string, error) {
	var parts []string
	for _, fl := range st.Fields.List {
		tag := ""
		if fl.Tag != nil {
			raw, _ := strconv.Unquote(fl.Tag.Value)
			tag = reflect.StructTag(raw).Get(tagKey)
		}
		name, opts, _ := strings.Cut(tag, ",")
		if name == "-" {
			continue
		}
		inner, err := nested(fl.Type, f, tagKey, visiting)
		if err != nil {
			return "", err
		}
		if len(fl.Names) == 0 || strings.Contains(opts, "inline") {
			if inner != "" {
				parts = append(parts, strings.Split(inner, ",")...)
			}
			continue
		}
		for _, n := range fl.Names {
			if !n.IsExported() {
				continue
			}
			fname := name
			if fname == "" {
				fname = n.Name
				if tagKey == "yaml" {
					fname = strings.ToLower(n.Name)
				}
			}
			if inner != "" {
				fname += "{" + inner + "}"
			}
			parts = append(parts, fname)
		}
	}
	sort.Strings(parts)
	return strings.Join(parts, ","), nil
}

// nested returns the field signature of a struct-typed field, or "" for a leaf.
func nested(t ast.Expr, f *srcFile, tagKey string, visiting map[string]bool) (string, error) {
	switch t := t.(type) {
	case *ast.StarExpr:
		return nested(t.X, f, tagKey, visiting)
	case *ast.ArrayType:
		return nested(t.Elt, f, tagKey, visiting)
	case *ast.MapType:
		return nested(t.Value, f, tagKey, visiting)
	case *ast.StructType:
		return fieldsSig(t, f, tagKey, visiting)
	case *ast.Ident:
		return structOrLeaf(f.dir+"."+t.Name, tagKey, visiting)
	case *ast.SelectorExpr:
		if x, ok := t.X.(*ast.Ident); ok {
			if dir, ok := f.imports[x.Name]; ok {
				return structOrLeaf(dir+"."+t.Sel.Name, tagKey, visiting)
			}
		}
	}
	return "", nil
}

func structOrLeaf(q, tagKey string, visiting map[string]bool) (string, error) {
	td, ok := types[q]
	if !ok {
		return "", nil
	}
	if _, ok := td.spec.Type.(*ast.StructType); !ok {
		return "", nil
	}
	return structSig(q, tagKey, visiting)
}

func lit(e ast.Expr) string {
	if b, ok := e.(*ast.BasicLit); ok {
		s, err := strconv.Unquote(b.Value)
		if err != nil {
			return b.Value
		}
		return s
	}
	return ""
}

// varLit finds the composite literal a package-level var is set to.
func varLit(file, name string) (*ast.CompositeLit, error) {
	f := files[file]
	if f == nil {
		return nil, fmt.Errorf("no file %s", file)
	}
	for _, d := range f.ast.Decls {
		g, ok := d.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, s := range g.Specs {
			vs, ok := s.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, n := range vs.Names {
				if n.Name == name && i < len(vs.Values) {
					if cl, ok := vs.Values[i].(*ast.CompositeLit); ok {
						return cl, nil
					}
				}
			}
		}
	}
	return nil, fmt.Errorf("%s has no literal var %s", file, name)
}

func mapKeys(cl *ast.CompositeLit) []string {
	var out []string
	for _, e := range cl.Elts {
		if kv, ok := e.(*ast.KeyValueExpr); ok {
			out = append(out, lit(kv.Key))
		}
	}
	sort.Strings(out)
	return out
}

func strList(e ast.Expr) []string {
	cl, ok := e.(*ast.CompositeLit)
	if !ok {
		return nil
	}
	var out []string
	for _, el := range cl.Elts {
		if b, ok := el.(*ast.BasicLit); ok {
			out = append(out, strings.Trim(b.Value, `"`))
		}
	}
	return out
}

// commandContracts makes each entry of the commands table a contract of its
// own, anchored at the table and at the method the entry runs, so a change
// to one command asks only that command's consumers.
func commandContracts() []contract {
	cl, err := varLit("cmd/abbd/main.go", "commands")
	if err != nil {
		return nil
	}
	f := files["cmd/abbd/main.go"]
	var out []contract
	for _, e := range cl.Elts {
		kv := e.(*ast.KeyValueExpr)
		name := lit(kv.Key)
		handler := commandHandler(name, kv.Value.(*ast.CompositeLit))
		h := anchor{decls["cmd/abbd"][handler], handler}
		out = append(out, contract{
			key: "if-cli-" + strings.ReplaceAll(name, " ", "-"), kind: "IF", name: "abbd " + name,
			anchors:   []anchor{{"cmd/abbd/main.go", "commands"}, h},
			provider:  "mod-cmd-abbd",
			signature: func() (string, error) { return commandSig(kv), nil },
			entries:   func() []entry { return commandFlags(kv) },
			digest: func() (string, error) {
				body, err := anchorsDigest([]anchor{h})
				return digestOf(string(nodeText(f, kv)), body), err
			},
		})
	}
	return out
}

// commandHandler is the env method an entry runs: the method value it names,
// or the first e.method call its function literal makes. An entry with no run
// function, such as hook, is run by the method of its own name.
func commandHandler(name string, entry *ast.CompositeLit) string {
	for _, fe := range entry.Elts {
		fkv, ok := fe.(*ast.KeyValueExpr)
		if !ok || fkv.Key.(*ast.Ident).Name != "run" {
			continue
		}
		if sel, ok := fkv.Value.(*ast.SelectorExpr); ok {
			return sel.Sel.Name
		}
		found := ""
		ast.Inspect(fkv.Value, func(n ast.Node) bool {
			if c, ok := n.(*ast.CallExpr); ok && found == "" {
				if sel, ok := c.Fun.(*ast.SelectorExpr); ok {
					if x, ok := sel.X.(*ast.Ident); ok && x.Name == "e" {
						found = sel.Sel.Name
					}
				}
			}
			return found == ""
		})
		return found
	}
	return name
}

// commandSig is a command's shape less its flags, which are its entries.
func commandSig(kv *ast.KeyValueExpr) string {
	var subs, pos, extra []string
	for _, fe := range kv.Value.(*ast.CompositeLit).Elts {
		fkv, ok := fe.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		switch fkv.Key.(*ast.Ident).Name {
		case "subs":
			subs = strList(fkv.Value)
		case "pos":
			pos = strList(fkv.Value)
		case "dashArgv":
			extra = append(extra, "--argv")
		case "session":
			extra = append(extra, "needs-session")
		}
	}
	sort.Strings(extra)
	s := "abbd " + lit(kv.Key)
	if subs != nil {
		s += " sub{" + strings.Join(subs, ",") + "}"
	}
	s += " pos{" + strings.Join(pos, ",") + "}"
	for _, x := range extra {
		s += " " + x
	}
	return s
}

// commandFlags are the flags an entry of the commands table names, in its
// order, each with the kind of value flagDefs binds it to.
func commandFlags(kv *ast.KeyValueExpr) []entry {
	var out []entry
	for _, fe := range kv.Value.(*ast.CompositeLit).Elts {
		if fkv, ok := fe.(*ast.KeyValueExpr); ok && fkv.Key.(*ast.Ident).Name == "flags" {
			for _, f := range strList(fkv.Value) {
				out = append(out, entry{Name: "--" + f, Shape: flagKinds()[f]})
			}
		}
	}
	return out
}

// flagKinds reads what each flagDefs entry binds: a string, a bool, or a
// value repeated on the command line.
func flagKinds() map[string]string {
	out := map[string]string{}
	cl, err := varLit("cmd/abbd/main.go", "flagDefs")
	if err != nil {
		return out
	}
	for _, e := range cl.Elts {
		kv := e.(*ast.KeyValueExpr)
		ast.Inspect(kv.Value, func(n ast.Node) bool {
			if sel, ok := n.(*ast.SelectorExpr); ok {
				switch sel.Sel.Name {
				case "StringVar":
					out[lit(kv.Key)] = "string"
				case "BoolVar":
					out[lit(kv.Key)] = "bool"
				case "Var":
					out[lit(kv.Key)] = "repeated"
				}
			}
			return true
		})
	}
	return out
}

func exitSig() (string, error) {
	var exits []string
	for _, d := range files["cmd/abbd/main.go"].ast.Decls {
		g, ok := d.(*ast.GenDecl)
		if !ok || g.Tok != token.CONST {
			continue
		}
		for _, s := range g.Specs {
			vs := s.(*ast.ValueSpec)
			for i, n := range vs.Names {
				if strings.HasPrefix(n.Name, "exit") && i < len(vs.Values) {
					if b, ok := vs.Values[i].(*ast.BasicLit); ok {
						exits = append(exits, strings.ToLower(strings.TrimPrefix(n.Name, "exit"))+"="+b.Value)
					}
				}
			}
		}
	}
	if len(exits) == 0 {
		return "", fmt.Errorf("no exit constants")
	}
	sort.Strings(exits)
	return "exit " + strings.Join(exits, ","), nil
}

func nodeText(f *srcFile, n ast.Node) []byte {
	return f.src[fset.Position(n.Pos()).Offset:fset.Position(n.End()).Offset]
}

// declText is the source of the top-level declaration that names symbol.
func declText(file, symbol string) []byte {
	f := files[file]
	if f == nil {
		return nil
	}
	for _, d := range f.ast.Decls {
		switch d := d.(type) {
		case *ast.FuncDecl:
			if d.Name.Name == symbol {
				return nodeText(f, d)
			}
		case *ast.GenDecl:
			for _, sp := range d.Specs {
				switch sp := sp.(type) {
				case *ast.TypeSpec:
					if sp.Name.Name == symbol {
						return nodeText(f, sp)
					}
				case *ast.ValueSpec:
					for _, n := range sp.Names {
						if n.Name == symbol {
							return nodeText(f, sp)
						}
					}
				}
			}
		}
	}
	return nil
}

// anchorsDigest fingerprints what each anchor names: a Go symbol's
// declaration, or a whole file. A directory anchor adds nothing.
func anchorsDigest(anchors []anchor) (string, error) {
	var parts []string
	for _, a := range anchors {
		var b []byte
		if a.Symbol != "" && strings.HasSuffix(a.Path, ".go") {
			b = declText(a.Path, a.Symbol)
		} else if st, err := os.Stat(filepath.FromSlash(a.Path)); err == nil && st.Mode().IsRegular() {
			if b, err = os.ReadFile(filepath.FromSlash(a.Path)); err != nil {
				return "", err
			}
		}
		parts = append(parts, a.Path+" "+a.Symbol+"\n"+string(b))
	}
	return digestOf(parts...), nil
}

func digestOf(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		p = strings.ReplaceAll(p, "\r\n", "\n")
		fmt.Fprintf(h, "%d:%s", len(p), p)
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

func exprText(e ast.Expr) string {
	switch e := e.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		return exprText(e.X) + "." + e.Sel.Name
	}
	return ""
}

// constValues lists the string constants of a named type in a file.
func constValues(file, typ string) []string {
	var out []string
	for _, d := range files[file].ast.Decls {
		g, ok := d.(*ast.GenDecl)
		if !ok || g.Tok != token.CONST {
			continue
		}
		for _, s := range g.Specs {
			vs := s.(*ast.ValueSpec)
			if id, ok := vs.Type.(*ast.Ident); !ok || id.Name != typ {
				continue
			}
			for _, v := range vs.Values {
				if s := lit(v); s != "" {
					out = append(out, s)
				}
			}
		}
	}
	sort.Strings(out)
	return out
}

func edgeKeySig() (string, error) {
	kinds := constValues("internal/model/model.go", "RelationKind")
	if len(kinds) == 0 {
		return "", fmt.Errorf("no RelationKind constants")
	}
	return "ID.{" + strings.Join(kinds, ",") + "}.ID", nil
}

func blueprintSig() (string, error) {
	names := []string{"<ID>.html", "flow-<KEY>.html", "stack-<KEY>.html", "table-<KEY>.html"}
	for _, path := range []string{"internal/screen/page.go", "internal/screen/site.go"} {
		for _, d := range files[path].ast.Decls {
			g, ok := d.(*ast.GenDecl)
			if !ok || g.Tok != token.CONST {
				continue
			}
			for _, s := range g.Specs {
				vs := s.(*ast.ValueSpec)
				for i, n := range vs.Names {
					if strings.HasSuffix(n.Name, "File") && i < len(vs.Values) {
						names = append(names, lit(vs.Values[i]))
					}
				}
			}
		}
	}
	sort.Strings(names)
	graph, err := structSig("internal/screen.Graph", "json", map[string]bool{})
	if err != nil {
		return "", err
	}
	return ".abbd/blueprint/{" + strings.Join(names, ",") + "}; graph.json {" + graph + "}", nil
}

func pluginSig() (string, error) {
	var mk struct {
		Name    string `json:"name"`
		Plugins []struct {
			Name   string `json:"name"`
			Source string `json:"source"`
		} `json:"plugins"`
	}
	var pl struct {
		Name string `json:"name"`
	}
	for p, v := range map[string]any{".claude-plugin/marketplace.json": &mk, "plugins/abbd/.claude-plugin/plugin.json": &pl} {
		b, err := os.ReadFile(p)
		if err != nil {
			return "", err
		}
		if err := json.Unmarshal(b, v); err != nil {
			return "", fmt.Errorf("%s: %w", p, err)
		}
	}
	var ps []string
	for _, p := range mk.Plugins {
		ps = append(ps, p.Name+"="+p.Source)
	}
	sort.Strings(ps)
	return "marketplace " + mk.Name + " {" + strings.Join(ps, ",") + "}; plugin " + pl.Name, nil
}

var (
	frontName = regexp.MustCompile(`(?m)^name:\s*(\S+)\s*$`)
	codeSpan  = regexp.MustCompile("(?s)```.*?```|`[^`\n]+`")
	flagWord  = regexp.MustCompile(`--[a-z]+`)
	abbdCmd   = regexp.MustCompile(`\babbd[ \t]+([a-z]+)(?:[ \t]+([a-z]+))?`)
)

// features are the plugin skills. A skill consumes the CLI when its code
// spans run an abbd command, cited at the first line of each file that does,
// and uses each of the command's flags the same line of a span passes it.
func features() ([]unit, []relation, map[relation][]evidence, map[relation][]string, error) {
	skills, err := filepath.Glob("plugins/abbd/skills/*/SKILL.md")
	if err != nil {
		return nil, nil, nil, nil, err
	}
	sort.Strings(skills)
	cmds, err := varLit("cmd/abbd/main.go", "commands")
	if err != nil {
		return nil, nil, nil, nil, err
	}
	known := map[string]bool{}
	flags := map[string]map[string]bool{}
	for _, e := range cmds.Elts {
		kv := e.(*ast.KeyValueExpr)
		known[lit(kv.Key)] = true
		flags[lit(kv.Key)] = map[string]bool{}
		for _, en := range commandFlags(kv) {
			flags[lit(kv.Key)][en.Name] = true
		}
	}
	uses := map[relation][]string{}
	var units []unit
	var rels []relation
	cites := map[relation][]evidence{}
	for _, s := range skills {
		s = filepath.ToSlash(s)
		b, err := os.ReadFile(s)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		text := string(b)
		m := frontName.FindStringSubmatch(text)
		if m == nil {
			return nil, nil, nil, nil, fmt.Errorf("%s has no frontmatter name", s)
		}
		key := "feat-" + m[1]
		u := unit{Key: key, Kind: "FEAT", Name: "/" + m[1], Anchors: []anchor{{Path: s, Symbol: m[1]}, {Path: path.Dir(s)}}}
		units = append(units, u)
		usesCLI := false
		for _, loc := range codeSpan.FindAllStringIndex(text, -1) {
			span := text[loc[0]:loc[1]]
			for _, cl := range abbdCmd.FindAllStringSubmatchIndex(span, -1) {
				cm := []string{span[cl[0]:cl[1]], span[cl[2]:cl[3]], ""}
				if cl[4] >= 0 {
					cm[2] = span[cl[4]:cl[5]]
				}
				cmd := cm[1] + " " + cm[2]
				quote := cm[0]
				if !known[cmd] {
					cmd = cm[1]
					quote = "abbd " + cm[1]
				}
				if !known[cmd] {
					continue
				}
				usesCLI = true
				r := relation{key, "if-cli-" + strings.ReplaceAll(cmd, " ", "-"), "consumes"}
				rest, _, _ := strings.Cut(span[cl[1]:], "\n")
				for _, fl := range flagWord.FindAllString(rest, -1) {
					if flags[cmd][fl] && !slices.Contains(uses[r], fl) {
						uses[r] = append(uses[r], fl)
					}
				}
				if _, ok := cites[r]; !ok {
					rels = append(rels, r)
					line := strings.Count(text[:loc[0]+cl[0]], "\n") + 1
					cites[r] = []evidence{{Path: s, Lines: strconv.Itoa(line), Quote: quote}}
				}
			}
		}
		if usesCLI {
			rels = append(rels, relation{key, "if-cli", "consumes"})
		}
	}
	return units, rels, cites, uses, nil
}

// structField is one serialized field of a file contract: its entry, and the
// struct and Go name that declare it, by which code that writes or reads it
// is found.
type structField struct {
	entry
	typ, goName string
}

// structFields lists a struct's serialized fields in declaration order, each
// with its Go type and whether the file always holds it: a field without
// omitempty is written every time. A field of a module struct type is
// followed by that struct's fields, named under it as parent.child.
func structFields(qualified, tagKey, prefix string, visiting map[string]bool) []structField {
	td, ok := types[qualified]
	if !ok || visiting[qualified] {
		return nil
	}
	st, ok := td.spec.Type.(*ast.StructType)
	if !ok {
		return nil
	}
	visiting[qualified] = true
	defer delete(visiting, qualified)
	var out []structField
	for _, fl := range st.Fields.List {
		tag := ""
		if fl.Tag != nil {
			raw, _ := strconv.Unquote(fl.Tag.Value)
			tag = reflect.StructTag(raw).Get(tagKey)
		}
		name, opts, _ := strings.Cut(tag, ",")
		if name == "-" {
			continue
		}
		inner := innerStruct(fl.Type, td.file)
		if len(fl.Names) == 0 || strings.Contains(opts, "inline") {
			out = append(out, structFields(inner, tagKey, prefix, visiting)...)
			continue
		}
		for _, n := range fl.Names {
			if !n.IsExported() {
				continue
			}
			fname := name
			if fname == "" {
				fname = n.Name
				if tagKey == "yaml" {
					fname = strings.ToLower(n.Name)
				}
			}
			required := !strings.Contains(opts, "omitempty")
			out = append(out, structField{entry{Name: prefix + fname, Shape: string(nodeText(td.file, fl.Type)), Required: &required}, qualified, n.Name})
			out = append(out, structFields(inner, tagKey, prefix+fname+".", visiting)...)
		}
	}
	return out
}

// innerStruct is the module struct type a field holds, through pointers,
// slices and maps, as "dir.Name", or "".
func innerStruct(t ast.Expr, f *srcFile) string {
	switch t := t.(type) {
	case *ast.StarExpr:
		return innerStruct(t.X, f)
	case *ast.ArrayType:
		return innerStruct(t.Elt, f)
	case *ast.MapType:
		return innerStruct(t.Value, f)
	case *ast.Ident:
		if td, ok := types[f.dir+"."+t.Name]; ok {
			if _, ok := td.spec.Type.(*ast.StructType); ok {
				return f.dir + "." + t.Name
			}
		}
	case *ast.SelectorExpr:
		if x, ok := t.X.(*ast.Ident); ok {
			if dir, ok := f.imports[x.Name]; ok {
				if td, ok := types[dir+"."+t.Sel.Name]; ok {
					if _, ok := td.spec.Type.(*ast.StructType); ok {
						return dir + "." + t.Sel.Name
					}
				}
			}
		}
	}
	return ""
}

// fieldNames counts the struct types that declare each exported field name.
func fieldNames() map[string]int {
	out := map[string]int{}
	for _, q := range sortedKeys(types) {
		if st, ok := types[q].spec.Type.(*ast.StructType); ok {
			for _, fl := range st.Fields.List {
				for _, n := range fl.Names {
					out[n.Name]++
				}
			}
		}
	}
	return out
}

// fieldUses finds the modules that write and read each top-level field of
// a file contract, as the code spells it: a keyed composite literal of the
// struct writes its fields, an assignment to x.Field writes and any other
// x.Field reads. A name that two struct types declare is skipped, since
// without types the code does not say whose field it is. Only a struct the
// provider declares counts: one declared elsewhere, such as model.Unit, is
// shared vocabulary, and building one writes no file.
func fieldUses(c contract, owners map[string]string) (writes, reads map[string][]string) {
	writes, reads = map[string][]string{}, map[string][]string{}
	if owners[types[c.typ].file.path] != c.provider {
		return writes, reads
	}
	byGo := map[string]string{}
	for _, f := range structFields(c.typ, c.tag, "", map[string]bool{}) {
		if f.typ == c.typ {
			byGo[f.goName] = f.Name
		}
	}
	tdir, tname := path.Dir(strings.ReplaceAll(c.typ, ".", "/")), path.Base(strings.ReplaceAll(c.typ, ".", "/"))
	count := fieldNames()
	add := func(m map[string][]string, owner, field string) {
		if owner != "" && !slices.Contains(m[owner], field) {
			m[owner] = append(m[owner], field)
		}
	}
	for _, p := range sortedKeys(files) {
		f, owner := files[p], owners[p]
		isType := func(e ast.Expr) bool {
			switch e := e.(type) {
			case *ast.Ident:
				return f.dir == tdir && e.Name == tname
			case *ast.SelectorExpr:
				x, ok := e.X.(*ast.Ident)
				return ok && f.imports[x.Name] == tdir && e.Sel.Name == tname
			}
			return false
		}
		written := map[ast.Node]bool{}
		ast.Inspect(f.ast, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.CompositeLit:
				if isType(n.Type) {
					for _, el := range n.Elts {
						if kv, ok := el.(*ast.KeyValueExpr); ok {
							if k, ok := kv.Key.(*ast.Ident); ok && byGo[k.Name] != "" {
								add(writes, owner, byGo[k.Name])
							}
						}
					}
				}
			case *ast.AssignStmt:
				for _, l := range n.Lhs {
					written[fieldOf(l)] = true
				}
			case *ast.IncDecStmt:
				written[fieldOf(n.X)] = true
			case *ast.SelectorExpr:
				x, isIdent := n.X.(*ast.Ident)
				if field := byGo[n.Sel.Name]; field != "" && count[n.Sel.Name] == 1 && !(isIdent && f.imports[x.Name] != "") {
					if written[n] {
						add(writes, owner, field)
					} else {
						add(reads, owner, field)
					}
				}
			}
			return true
		})
	}
	return writes, reads
}

// fieldOf is the x.Field an assignment's left side writes, through an index
// or a dereference, or nil.
func fieldOf(e ast.Expr) ast.Node {
	for {
		switch t := e.(type) {
		case *ast.IndexExpr:
			e = t.X
		case *ast.StarExpr:
			e = t.X
		case *ast.ParenExpr:
			e = t.X
		case *ast.SelectorExpr:
			return t
		default:
			return nil
		}
	}
}
