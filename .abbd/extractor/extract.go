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
	Key       string   `json:"key"`
	Kind      string   `json:"kind"`
	Name      string   `json:"name"`
	Signature string   `json:"signature,omitempty"`
	Anchors   []anchor `json:"anchors"`
	Digest    string   `json:"digest,omitempty"`
}

type relation struct {
	From string `json:"from"`
	To   string `json:"to"`
	Kind string `json:"kind"`
}

type result struct {
	Units     []unit     `json:"units"`
	Relations []relation `json:"relations"`
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
	feats, featRels, err := features()
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

	out := result{Units: units}
	for r := range rels {
		out.Relations = append(out.Relations, r)
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
}

func contractTable() []contract {
	fields := func(prefix, typ, tag string) func() (string, error) {
		return func() (string, error) {
			s, err := structSig(typ, tag, map[string]bool{})
			return prefix + ": " + s, err
		}
	}
	return append(commandContracts(), []contract{
		{"if-cli", "IF", "abbd exit codes", []anchor{{"cmd/abbd/main.go", "flagDefs"}, {"cmd/abbd/main.go", "exitUsage"}, {"cmd/abbd/main.go", "parse"}}, "mod-cmd-abbd", exitSig, nil},
		{"if-edge-key", "IF", "relation key", []anchor{{"internal/model/model.go", "Edge"}, {"internal/model/model.go", "ParseEdge"}}, "mod-model", edgeKeySig, nil},
		{"data-extraction", "DATA", "extraction JSON", []anchor{{"internal/extract/extract.go", "rawResult"}, {"internal/extract/extract.go", "rawUnit"}, {"internal/extract/extract.go", "Parse"}}, "mod-extract", fields("extraction json", "internal/extract.rawResult", "json"), nil},
		{"cfg-project", "CFG", "project.yaml", []anchor{{"internal/store/store.go", "Project"}, {"internal/store/store.go", "LoadProject"}, {"internal/store/store.go", "SaveProject"}}, "mod-store", fields(".abbd/project.yaml", "internal/store.Project", "yaml"), nil},
		{"data-units", "DATA", "unit files", []anchor{{"internal/store/store.go", "SaveUnits"}, {"internal/store/store.go", "LoadUnits"}, {"internal/model/model.go", "Unit"}}, "mod-store", fields(".abbd/{architecture,features}/ID.md frontmatter", "internal/model.Unit", "yaml"), nil},
		{"data-understanding", "DATA", "module understanding", []anchor{{"internal/store/understanding.go", "SaveUnderstanding"}, {"internal/store/understanding.go", "LoadUnderstandings"}, {"internal/store/understanding.go", "Understanding"}}, "mod-store", fields(".abbd/understanding/MOD.md frontmatter", "internal/store.Understanding", "yaml"), nil},
		{"data-state", "DATA", "sync state", []anchor{{"internal/store/store.go", "SaveState"}, {"internal/store/store.go", "LoadState"}, {"internal/store/store.go", "State"}}, "mod-store", fields(".abbd/state.yaml", "internal/store.State", "yaml"), nil},
		{"data-blueprint", "DATA", "blueprint files", []anchor{{"internal/screen/page.go", "Graph"}, {"internal/screen/page.go", "Files"}, {"internal/screen/screen.go", "Watermark"}}, "mod-screen", blueprintSig, nil},
		{"cfg-plugin", "CFG", "plugin and marketplace manifest", []anchor{{".claude-plugin/marketplace.json", ""}, {"plugins/abbd/.claude-plugin/plugin.json", ""}}, "", pluginSig, nil},
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
		out = append(out, unit{Key: c.key, Kind: c.kind, Name: c.name, Signature: sig, Anchors: c.anchors, Digest: d})
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

func commandSig(kv *ast.KeyValueExpr) string {
	var flags, subs, pos, extra []string
	for _, fe := range kv.Value.(*ast.CompositeLit).Elts {
		fkv, ok := fe.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		switch fkv.Key.(*ast.Ident).Name {
		case "flags":
			flags = strList(fkv.Value)
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
	sort.Strings(flags)
	sort.Strings(extra)
	s := "abbd " + lit(kv.Key) + "[" + strings.Join(flags, ",") + "]"
	if subs != nil {
		s += " sub{" + strings.Join(subs, ",") + "}"
	}
	s += " pos{" + strings.Join(pos, ",") + "}"
	for _, x := range extra {
		s += " " + x
	}
	return s
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
	var names []string
	for _, d := range files["internal/screen/page.go"].ast.Decls {
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
	abbdCmd   = regexp.MustCompile(`\babbd[ \t]+([a-z]+)(?:[ \t]+([a-z]+))?`)
)

// features are the plugin skills. A skill consumes the CLI when its code
// spans run an abbd command.
func features() ([]unit, []relation, error) {
	skills, err := filepath.Glob("plugins/abbd/skills/*/SKILL.md")
	if err != nil {
		return nil, nil, err
	}
	sort.Strings(skills)
	cmds, err := varLit("cmd/abbd/main.go", "commands")
	if err != nil {
		return nil, nil, err
	}
	known := map[string]bool{}
	for _, k := range mapKeys(cmds) {
		known[k] = true
	}
	var units []unit
	var rels []relation
	for _, s := range skills {
		s = filepath.ToSlash(s)
		b, err := os.ReadFile(s)
		if err != nil {
			return nil, nil, err
		}
		text := string(b)
		m := frontName.FindStringSubmatch(text)
		if m == nil {
			return nil, nil, fmt.Errorf("%s has no frontmatter name", s)
		}
		key := "feat-" + m[1]
		u := unit{Key: key, Kind: "FEAT", Name: "/" + m[1], Anchors: []anchor{{Path: s, Symbol: m[1]}, {Path: path.Dir(s)}}}
		units = append(units, u)
		usesCLI := false
		for _, span := range codeSpan.FindAllString(text, -1) {
			for _, cm := range abbdCmd.FindAllStringSubmatch(span, -1) {
				cmd := cm[1] + " " + cm[2]
				if !known[cmd] {
					cmd = cm[1]
				}
				if known[cmd] {
					usesCLI = true
					rels = append(rels, relation{key, "if-cli-" + strings.ReplaceAll(cmd, " ", "-"), "consumes"})
				}
			}
		}
		if usesCLI {
			rels = append(rels, relation{key, "if-cli", "consumes"})
		}
	}
	return units, rels, nil
}
