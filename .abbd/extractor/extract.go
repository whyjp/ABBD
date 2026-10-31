// Command extract prints the ABBD extraction result for the ABBD repository.
// Run it from the repository root: go run .abbd/extractor/extract.go
//
// Modules are Go packages, except that cmd/abbd splits by file into the CLI,
// the PreToolUse guard and the screen server. Contracts are read from the
// code that defines their shape. Relations come from the symbols each unit
// references and from the `abbd ...` commands the skills quote.
package main

import (
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
	imports map[string]string // local name -> repo-relative package dir
}

// module groups: which files of cmd/abbd belong to which unit. The CLI takes
// every file no other group names, so a new file lands there.
var mainGroups = []struct{ key, name, file, symbol string }{
	{"mod-guard", "PreToolUse guard", "cmd/abbd/guard.go", "guard"},
	{"mod-serve", "screen server", "cmd/abbd/serve.go", "screenServer"},
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
	if hooksCallAbbdHook() {
		rels[relation{"cfg-hooks", "if-hook", "consumes"}] = true
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
		f, err := parser.ParseFile(fset, p, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		sf := &srcFile{path: rel, dir: path.Dir(rel), ast: f, imports: map[string]string{}}
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
			for _, g := range mainGroups {
				if g.file == p {
					owners[p] = g.key
				}
			}
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
		owned := false
		for _, g := range mainGroups {
			owned = owned || g.file == p
		}
		if !owned {
			cli.Anchors = append(cli.Anchors, anchor{Path: p})
		}
	}
	out = append(out, cli)
	for _, g := range mainGroups {
		out = append(out, unit{Key: g.key, Kind: "MOD", Name: g.name, Anchors: []anchor{{Path: g.file}, {Path: g.file, Symbol: g.symbol}}})
	}
	return out
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
}

func contractTable() []contract {
	fields := func(prefix, typ, tag string) func() (string, error) {
		return func() (string, error) {
			s, err := structSig(typ, tag, map[string]bool{})
			return prefix + ": " + s, err
		}
	}
	return []contract{
		{"if-cli", "IF", "abbd command line", []anchor{{"cmd/abbd/main.go", "commands"}, {"cmd/abbd/main.go", "flagDefs"}, {"cmd/abbd/main.go", "exitNeedHuman"}}, "mod-cmd-abbd", cliSig},
		{"if-hook", "IF", "Claude Code hook protocol", []anchor{{"cmd/abbd/hook.go", "hookInput"}, {"cmd/abbd/hook.go", "hookOutput"}, {"cmd/abbd/hook.go", "hooks"}}, "mod-cmd-abbd", hookSig},
		{"if-serve-http", "IF", "screen HTTP", []anchor{{"cmd/abbd/serve.go", "ServeHTTP"}, {"cmd/abbd/serve.go", "actions"}}, "mod-serve", serveSig},
		{"if-change-key", "IF", "change item key", []anchor{{"internal/topology/topology.go", "Item"}, {"internal/topology/topology.go", "ParseKey"}, {"internal/topology/topology.go", "Edge"}}, "mod-topology", changeKeySig},
		{"if-confirm-key", "IF", "confirm item key", []anchor{{"internal/confirm/confirm.go", "Item"}, {"internal/confirm/confirm.go", "Queue"}, {"internal/confirm/confirm.go", "Apply"}}, "mod-confirm", confirmKeySig},
		{"data-extraction", "DATA", "extraction JSON", []anchor{{"internal/extract/extract.go", "rawResult"}, {"internal/extract/extract.go", "rawUnit"}, {"internal/extract/extract.go", "Parse"}}, "mod-extract", fields("extraction json", "internal/extract.rawResult", "json")},
		{"cfg-project", "CFG", "project.yaml", []anchor{{"internal/store/store.go", "Project"}, {"internal/store/store.go", "LoadProject"}, {"internal/store/store.go", "SaveProject"}}, "mod-store", fields(".abbd/project.yaml", "internal/store.Project", "yaml")},
		{"data-intended", "DATA", "intended topology", []anchor{{"internal/store/store.go", "SaveIntended"}, {"internal/store/store.go", "LoadIntended"}, {"internal/topology/topology.go", "Intended"}}, "mod-store", fields(".abbd/intended.yaml", "internal/topology.Intended", "yaml")},
		{"data-units", "DATA", "unit files", []anchor{{"internal/store/store.go", "SaveUnits"}, {"internal/store/store.go", "LoadUnits"}, {"internal/model/model.go", "Unit"}}, "mod-store", fields(".abbd/{architecture,features}/ID.md frontmatter", "internal/model.Unit", "yaml")},
		{"data-changes", "DATA", "change records", []anchor{{"internal/store/turn.go", "SaveTurn"}, {"internal/store/turn.go", "LoadTurns"}, {"internal/turn/turn.go", "Turn"}}, "mod-store", fields(".abbd/changes/CHG-*.md frontmatter", "internal/turn.Turn", "yaml")},
		{"data-decisions", "DATA", "decision records", []anchor{{"internal/store/store.go", "WriteADR"}, {"internal/store/turn.go", "LoadADRs"}, {"internal/store/store.go", "ADR"}}, "mod-store", fields(".abbd/decisions/ADR-*.md frontmatter", "internal/store.ADR", "yaml")},
		{"data-requirements", "DATA", "requirements", []anchor{{"internal/store/turn.go", "WriteRequirement"}, {"internal/store/turn.go", "Requirement"}}, "mod-store", fields(".abbd/requirements/ID.md frontmatter", "internal/store.Requirement", "yaml")},
		{"data-runs", "DATA", "verification runs", []anchor{{"internal/store/turn.go", "SaveRun"}, {"internal/store/turn.go", "LoadRun"}, {"internal/turn/turn.go", "Run"}}, "mod-store", fields(".abbd/verification/runs/run-*.yaml", "internal/turn.Run", "yaml")},
		{"data-generated", "DATA", "generated files", []anchor{{"internal/screen/page.go", "Graph"}, {"internal/screen/page.go", "Files"}, {"internal/screen/screen.go", "Watermark"}}, "mod-screen", generatedSig},
		{"cfg-hooks", "CFG", "hook registration", []anchor{{"plugins/abbd/hooks/hooks.json", ""}, {"plugins/abbd/hooks/hooks.json", "hooks"}}, "", hooksJSONSig},
		{"cfg-plugin", "CFG", "plugin and marketplace manifest", []anchor{{".claude-plugin/marketplace.json", ""}, {"plugins/abbd/.claude-plugin/plugin.json", ""}}, "", pluginSig},
	}
}

func contracts() ([]unit, error) {
	var out []unit
	for _, c := range contractTable() {
		sig, err := c.signature()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", c.key, err)
		}
		out = append(out, unit{Key: c.key, Kind: c.kind, Name: c.name, Signature: sig, Anchors: c.anchors})
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
	return sortedKeys(found)
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

func cliSig() (string, error) {
	cl, err := varLit("cmd/abbd/main.go", "commands")
	if err != nil {
		return "", err
	}
	var cmds []string
	for _, e := range cl.Elts {
		kv := e.(*ast.KeyValueExpr)
		var flags, subs, pos []string
		var extra []string
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
		s := lit(kv.Key) + "[" + strings.Join(flags, ",") + "]"
		if subs != nil {
			s += " sub{" + strings.Join(subs, ",") + "}"
		}
		s += " pos{" + strings.Join(pos, ",") + "}"
		for _, x := range extra {
			s += " " + x
		}
		cmds = append(cmds, s)
	}
	sort.Strings(cmds)
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
	sort.Strings(exits)
	return strings.Join(cmds, "; ") + "; exit " + strings.Join(exits, ","), nil
}

func hookSig() (string, error) {
	cl, err := varLit("cmd/abbd/hook.go", "hooks")
	if err != nil {
		return "", err
	}
	in, err := structSig("cmd/abbd.hookInput", "json", map[string]bool{})
	if err != nil {
		return "", err
	}
	out, err := structSig("cmd/abbd.hookOutput", "json", map[string]bool{})
	if err != nil {
		return "", err
	}
	return "abbd hook {" + strings.Join(mapKeys(cl), ",") + "}; stdin {" + in + "}; stdout {" + out + "}", nil
}

func serveSig() (string, error) {
	f := files["cmd/abbd/serve.go"]
	var routes []string
	for _, d := range f.ast.Decls {
		fd, ok := d.(*ast.FuncDecl)
		if !ok || fd.Name.Name != "ServeHTTP" {
			continue
		}
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			cc, ok := n.(*ast.CaseClause)
			if !ok {
				return true
			}
			var p, m string
			for _, e := range cc.List {
				ast.Inspect(e, func(n ast.Node) bool {
					be, ok := n.(*ast.BinaryExpr)
					if !ok || be.Op != token.EQL {
						return true
					}
					switch exprText(be.X) {
					case "r.URL.Path":
						p = lit(be.Y)
					case "r.Method":
						m = strings.ToUpper(strings.TrimPrefix(exprText(be.Y), "http.Method"))
					}
					return true
				})
			}
			if p != "" {
				routes = append(routes, m+" "+p)
			}
			return true
		})
	}
	sort.Strings(routes)
	cl, err := varLit("cmd/abbd/serve.go", "actions")
	if err != nil {
		return "", err
	}
	// The form is f inside the actions and their helpers, and r.PostForm in the handler.
	form := map[string]bool{"f": true, "r.PostForm": true}
	formFields := map[string]bool{}
	ast.Inspect(f.ast, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.CallExpr:
			if s, ok := n.Fun.(*ast.SelectorExpr); ok && s.Sel.Name == "Get" && form[exprText(s.X)] && len(n.Args) == 1 && lit(n.Args[0]) != "" {
				formFields[lit(n.Args[0])] = true
			}
		case *ast.IndexExpr:
			if v := lit(n.Index); v != "" && form[exprText(n.X)] {
				formFields[v] = true
			}
		}
		return true
	})
	return strings.Join(routes, "; ") + "; act do={" + strings.Join(mapKeys(cl), ",") + "} form{" + strings.Join(sortedKeys(formFields), ",") + "}", nil
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

func changeKeySig() (string, error) {
	changes := constValues("internal/topology/topology.go", "Change")
	kinds := constValues("internal/model/model.go", "RelationKind")
	if len(changes) == 0 || len(kinds) == 0 {
		return "", fmt.Errorf("no Change or RelationKind constants")
	}
	return "{" + strings.Join(changes, ",") + "}:ID or :ID.{" + strings.Join(kinds, ",") + "}.ID", nil
}

// confirmKeySig reads the ".field" suffixes Queue names its fact items with.
func confirmKeySig() (string, error) {
	var suffixes []string
	for _, d := range files["internal/confirm/confirm.go"].ast.Decls {
		fd, ok := d.(*ast.FuncDecl)
		if !ok || fd.Name.Name != "Queue" {
			continue
		}
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			if v := lit(asExpr(n)); strings.HasPrefix(v, ".") {
				suffixes = append(suffixes, strings.TrimPrefix(v, "."))
			}
			return true
		})
	}
	sort.Strings(suffixes)
	kinds := constValues("internal/model/model.go", "RelationKind")
	if len(suffixes) == 0 {
		return "", fmt.Errorf("Queue names no fact items")
	}
	return "ID.{" + strings.Join(suffixes, ",") + "} or ID.{" + strings.Join(kinds, ",") + "}.ID", nil
}

func asExpr(n ast.Node) ast.Expr {
	e, _ := n.(ast.Expr)
	return e
}

func generatedSig() (string, error) {
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
	return ".abbd/generated/{" + strings.Join(names, ",") + "}; graph.json {" + graph + "}", nil
}

type hookFile struct {
	Hooks map[string][]struct {
		Matcher string `json:"matcher"`
		Hooks   []struct {
			Command string   `json:"command"`
			Args    []string `json:"args"`
		} `json:"hooks"`
	} `json:"hooks"`
}

func readHooks() (hookFile, error) {
	var h hookFile
	b, err := os.ReadFile("plugins/abbd/hooks/hooks.json")
	if err != nil {
		return h, err
	}
	return h, json.Unmarshal(b, &h)
}

func hooksJSONSig() (string, error) {
	h, err := readHooks()
	if err != nil {
		return "", err
	}
	var parts []string
	for _, ev := range sortedKeys(h.Hooks) {
		for _, g := range h.Hooks[ev] {
			for _, c := range g.Hooks {
				s := ev
				if g.Matcher != "" {
					s += "[" + g.Matcher + "]"
				}
				parts = append(parts, s+" -> "+strings.Join(append([]string{c.Command}, c.Args...), " "))
			}
		}
	}
	sort.Strings(parts)
	return strings.Join(parts, "; "), nil
}

func hooksCallAbbdHook() bool {
	h, err := readHooks()
	if err != nil {
		return false
	}
	for _, gs := range h.Hooks {
		for _, g := range gs {
			for _, c := range g.Hooks {
				if c.Command == "abbd" && len(c.Args) > 0 && c.Args[0] == "hook" {
					return true
				}
			}
		}
	}
	return false
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
	abbdCmd   = regexp.MustCompile(`\babbd\s+([a-z]+)`)
)

// features are the plugin skills. A skill owns an agent file when it names
// the agent as abbd:<name>. A skill consumes the CLI when its code spans run
// an abbd command, and the change key when they quote one.
func features() ([]unit, []relation, error) {
	skills, err := filepath.Glob("plugins/abbd/skills/*/SKILL.md")
	if err != nil {
		return nil, nil, err
	}
	agents, err := filepath.Glob("plugins/abbd/agents/*.md")
	if err != nil {
		return nil, nil, err
	}
	sort.Strings(skills)
	sort.Strings(agents)
	cmds, err := varLit("cmd/abbd/main.go", "commands")
	if err != nil {
		return nil, nil, err
	}
	known := map[string]bool{}
	for _, k := range mapKeys(cmds) {
		first, _, _ := strings.Cut(k, " ")
		known[first] = true
	}
	changes := constValues("internal/topology/topology.go", "Change")
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
		for _, a := range agents {
			a = filepath.ToSlash(a)
			am := frontName.FindStringSubmatch(readOrEmpty(a))
			if am != nil && strings.Contains(text, "abbd:"+am[1]) {
				u.Anchors = append(u.Anchors, anchor{Path: a, Symbol: am[1]})
			}
		}
		units = append(units, u)
		usesCLI, usesKey := false, false
		for _, span := range codeSpan.FindAllString(text, -1) {
			for _, cm := range abbdCmd.FindAllStringSubmatch(span, -1) {
				usesCLI = usesCLI || known[cm[1]]
			}
			for _, c := range changes {
				usesKey = usesKey || strings.Contains(span, c+":")
			}
		}
		if usesCLI {
			rels = append(rels, relation{key, "if-cli", "consumes"})
		}
		if usesKey {
			rels = append(rels, relation{key, "if-change-key", "consumes"})
		}
	}
	return units, rels, nil
}

func readOrEmpty(p string) string {
	b, _ := os.ReadFile(p)
	return string(b)
}
