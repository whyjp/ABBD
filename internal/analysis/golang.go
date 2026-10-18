package analysis

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/whyjp/abbd/internal/model"
)

type goResult struct {
	modules   []ModuleFacts
	tests     []TestFacts
	contracts []ContractFacts
	unknowns  []string
	// literals holds every string literal in non-test files, by module Path, for SQL table ownership.
	literals map[string][]stringLit
}

type stringLit struct{ value, location string }

type goFile struct {
	rel     string
	ast     *ast.File
	imports map[string]string // local name -> import path
}

type goPkg struct {
	dir     string
	name    string
	key     string // import path, or "./dir" when no go.mod owns the directory
	modRoot string // directory of the owning go.mod, "" when none
	files   []*goFile
	tests   []*goFile
}

// typeRef names a type by its package key. A pkg that is not an indexed package means "known, but not in this repo".
type typeRef struct{ pkg, name string }

type typeInfo struct {
	iface  bool
	fields map[string]typeRef
}

type constDecl struct {
	expr ast.Expr
	file *goFile
	pkg  *goPkg
}

// goIndex is the cross-package symbol table that stands in for go/types: just enough to resolve
// receiver types, string constants and in-repo callees without loading dependencies.
type goIndex struct {
	pkgs          map[string]*goPkg
	types         map[typeRef]*typeInfo
	consts        map[string]constDecl
	funcs         map[string]typeRef // "pkg.Func" or "pkg.Type.Method" -> first result type
	vars          map[string]typeRef
	methodsByName map[string][]string
}

type goExtractor struct {
	idx  *goIndex
	fset *token.FileSet
	out  *goResult
	rp   *repo
}

var goModLine = regexp.MustCompile(`(?m)^module\s+(\S+)`)

func analyzeGo(rp *repo) *goResult {
	out := &goResult{literals: map[string][]stringLit{}}
	modRoots := map[string]string{}
	byDir := map[string][]string{}
	for _, f := range rp.files {
		switch {
		case path.Base(f) == "go.mod":
			data, err := rp.read(f)
			if m := goModLine.FindSubmatch(data); err == nil && m != nil {
				modRoots[path.Dir(f)] = string(m[1])
			}
		case strings.HasSuffix(f, ".go"):
			byDir[path.Dir(f)] = append(byDir[path.Dir(f)], f)
		}
	}

	fset := token.NewFileSet()
	idx := &goIndex{pkgs: map[string]*goPkg{}, types: map[typeRef]*typeInfo{}, consts: map[string]constDecl{},
		funcs: map[string]typeRef{}, vars: map[string]typeRef{}, methodsByName: map[string][]string{}}
	var pkgs []*goPkg
	for _, dir := range model.SortedKeys(byDir) {
		p := &goPkg{dir: dir}
		for _, rel := range byDir[dir] {
			src, err := rp.read(rel)
			if err != nil {
				out.unknowns = append(out.unknowns, "cannot read "+rel+": "+err.Error())
				continue
			}
			af, err := parser.ParseFile(fset, rel, src, parser.SkipObjectResolution)
			if err != nil {
				out.unknowns = append(out.unknowns, "go parse error, file skipped: "+err.Error())
				continue
			}
			gf := &goFile{rel: rel, ast: af}
			if strings.HasSuffix(rel, "_test.go") {
				p.tests = append(p.tests, gf)
			} else {
				p.files = append(p.files, gf)
				if p.name == "" {
					p.name = af.Name.Name
				}
			}
		}
		p.modRoot, p.key = goImportPath(dir, modRoots)
		if p.modRoot == "" {
			out.unknowns = append(out.unknowns, "no go.mod owns "+dir+"; in-repo imports of it are unresolved")
		}
		pkgs = append(pkgs, p)
		if len(p.files) > 0 {
			idx.pkgs[p.key] = p
		}
	}

	x := &goExtractor{idx: idx, fset: fset, out: out, rp: rp}
	for _, p := range pkgs {
		for _, f := range slices.Concat(p.files, p.tests) {
			f.imports = x.importNames(f.ast)
		}
	}
	x.indexDecls()
	for _, p := range pkgs {
		x.extractPackage(p)
	}
	x.topicConstants()
	return out
}

// goImportPath finds the nearest go.mod at or above dir.
func goImportPath(dir string, modRoots map[string]string) (modRoot, key string) {
	for d := dir; ; d = path.Dir(d) {
		if mod, ok := modRoots[d]; ok {
			if d == dir {
				return d, mod
			}
			rel := strings.TrimPrefix(dir, d+"/")
			if d == "." {
				rel = dir
			}
			return d, mod + "/" + rel
		}
		if d == "." {
			return "", "./" + dir
		}
	}
}

func (x *goExtractor) importNames(f *ast.File) map[string]string {
	names := map[string]string{}
	for _, spec := range f.Imports {
		ip, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			continue
		}
		name := path.Base(ip)
		if p, ok := x.idx.pkgs[ip]; ok {
			name = p.name
		}
		if spec.Name != nil {
			name = spec.Name.Name
		}
		if name != "_" && name != "." {
			names[name] = ip
		}
	}
	return names
}

func (x *goExtractor) indexDecls() {
	idx := x.idx
	for _, key := range model.SortedKeys(idx.pkgs) {
		p := idx.pkgs[key]
		for _, f := range p.files {
			for _, d := range f.ast.Decls {
				gd, ok := d.(*ast.GenDecl)
				if !ok || gd.Tok != token.TYPE {
					continue
				}
				for _, s := range gd.Specs {
					ts := s.(*ast.TypeSpec)
					_, iface := ts.Type.(*ast.InterfaceType)
					idx.types[typeRef{p.key, ts.Name.Name}] = &typeInfo{iface: iface, fields: map[string]typeRef{}}
				}
			}
		}
	}
	for _, key := range model.SortedKeys(idx.pkgs) {
		p := idx.pkgs[key]
		for _, f := range p.files {
			for _, d := range f.ast.Decls {
				switch d := d.(type) {
				case *ast.FuncDecl:
					k := p.key + "." + d.Name.Name
					if recv := recvTypeName(d); recv != "" {
						k = p.key + "." + recv + "." + d.Name.Name
						idx.methodsByName[d.Name.Name] = append(idx.methodsByName[d.Name.Name], k)
					}
					idx.funcs[k] = x.firstResult(d.Type, f, p)
				case *ast.GenDecl:
					x.indexGenDecl(d, f, p)
				}
			}
		}
	}
	for _, key := range model.SortedKeys(idx.pkgs) {
		p := idx.pkgs[key]
		for _, f := range p.files {
			for _, d := range f.ast.Decls {
				gd, ok := d.(*ast.GenDecl)
				if !ok || gd.Tok != token.VAR {
					continue
				}
				for _, s := range gd.Specs {
					vs := s.(*ast.ValueSpec)
					w := x.walker(p, f, "")
					for i, n := range vs.Names {
						if t, ok := w.declType(vs, i); ok {
							idx.vars[p.key+"."+n.Name] = t
						}
					}
				}
			}
		}
	}
	for name := range idx.methodsByName {
		slices.Sort(idx.methodsByName[name])
	}
}

func (x *goExtractor) indexGenDecl(gd *ast.GenDecl, f *goFile, p *goPkg) {
	idx := x.idx
	for _, s := range gd.Specs {
		switch s := s.(type) {
		case *ast.TypeSpec:
			info := idx.types[typeRef{p.key, s.Name.Name}]
			switch t := s.Type.(type) {
			case *ast.StructType:
				for _, fld := range t.Fields.List {
					ref, ok := x.resolveType(fld.Type, f, p)
					for _, n := range fld.Names {
						if ok {
							info.fields[n.Name] = ref
						}
					}
				}
			case *ast.InterfaceType:
				for _, m := range t.Methods.List {
					ft, ok := m.Type.(*ast.FuncType)
					for _, n := range m.Names {
						if ok {
							k := p.key + "." + s.Name.Name + "." + n.Name
							idx.funcs[k] = x.firstResult(ft, f, p)
							idx.methodsByName[n.Name] = append(idx.methodsByName[n.Name], k)
						}
					}
				}
			}
		case *ast.ValueSpec:
			if gd.Tok != token.CONST {
				continue
			}
			for i, n := range s.Names {
				if i < len(s.Values) {
					idx.consts[p.key+"."+n.Name] = constDecl{s.Values[i], f, p}
				}
			}
		}
	}
}

func (x *goExtractor) firstResult(ft *ast.FuncType, f *goFile, p *goPkg) typeRef {
	if ft.Results == nil || len(ft.Results.List) == 0 {
		return typeRef{}
	}
	t, _ := x.resolveType(ft.Results.List[0].Type, f, p)
	return t
}

func recvTypeName(fd *ast.FuncDecl) string {
	if fd.Recv == nil || len(fd.Recv.List) == 0 {
		return ""
	}
	e := fd.Recv.List[0].Type
	for {
		switch t := e.(type) {
		case *ast.StarExpr:
			e = t.X
		case *ast.IndexExpr:
			e = t.X
		case *ast.IndexListExpr:
			e = t.X
		case *ast.Ident:
			return t.Name
		default:
			return ""
		}
	}
}

// resolveType maps a type expression to a typeRef. ok=false means it could not be named at all.
func (x *goExtractor) resolveType(e ast.Expr, f *goFile, p *goPkg) (typeRef, bool) {
	switch t := e.(type) {
	case *ast.StarExpr:
		return x.resolveType(t.X, f, p)
	case *ast.ParenExpr:
		return x.resolveType(t.X, f, p)
	case *ast.IndexExpr:
		return x.resolveType(t.X, f, p)
	case *ast.IndexListExpr:
		return x.resolveType(t.X, f, p)
	case *ast.Ident:
		if _, ok := x.idx.types[typeRef{p.key, t.Name}]; ok {
			return typeRef{p.key, t.Name}, true
		}
		return typeRef{"", t.Name}, true
	case *ast.SelectorExpr:
		if id, ok := t.X.(*ast.Ident); ok {
			if ip, ok := f.imports[id.Name]; ok {
				return typeRef{ip, t.Sel.Name}, true
			}
		}
	case *ast.ArrayType, *ast.MapType, *ast.FuncType, *ast.ChanType, *ast.InterfaceType, *ast.StructType:
		return typeRef{"", types.ExprString(e)}, true
	}
	return typeRef{}, false
}

// evalString folds a string constant expression: literals, in-repo constants and their concatenation.
func (x *goExtractor) evalString(e ast.Expr, f *goFile, p *goPkg, depth int) (string, bool) {
	if depth > 16 {
		return "", false
	}
	switch t := e.(type) {
	case *ast.BasicLit:
		if t.Kind == token.STRING {
			s, err := strconv.Unquote(t.Value)
			return s, err == nil
		}
	case *ast.ParenExpr:
		return x.evalString(t.X, f, p, depth+1)
	case *ast.Ident:
		if c, ok := x.idx.consts[p.key+"."+t.Name]; ok {
			return x.evalString(c.expr, c.file, c.pkg, depth+1)
		}
	case *ast.SelectorExpr:
		if id, ok := t.X.(*ast.Ident); ok {
			if c, ok := x.idx.consts[f.imports[id.Name]+"."+t.Sel.Name]; ok {
				return x.evalString(c.expr, c.file, c.pkg, depth+1)
			}
		}
	case *ast.BinaryExpr:
		if t.Op == token.ADD {
			l, ok1 := x.evalString(t.X, f, p, depth+1)
			r, ok2 := x.evalString(t.Y, f, p, depth+1)
			return l + r, ok1 && ok2
		}
	}
	return "", false
}

func (x *goExtractor) extractPackage(p *goPkg) {
	for _, f := range p.tests {
		for _, d := range f.ast.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if ok && fd.Recv == nil && isTestName(fd.Name.Name) {
				x.out.tests = append(x.out.tests, TestFacts{
					Name:       fd.Name.Name,
					ModulePath: p.dir,
					Location:   x.pos(f, fd.Pos()),
					Command:    goTestCommand(p, fd.Name.Name),
				})
			}
		}
	}
	if len(p.files) == 0 {
		return
	}

	m := ModuleFacts{Path: p.dir, Name: p.name, Language: "go", IsMain: p.name == "main", Analyzer: "go/ast"}
	m.TestCommand = strings.TrimSuffix(goTestCommand(p, ""), " -run '^$'")
	if p.modRoot != "" {
		m.ImportPath = p.key
	}
	for _, f := range slices.Concat(p.files, p.tests) {
		m.Files = append(m.Files, f.rel)
	}
	for _, f := range p.files {
		for _, ip := range f.imports {
			if dep, ok := x.idx.pkgs[ip]; ok && dep != p {
				m.Imports = append(m.Imports, dep.dir)
			}
		}
		x.collectLiterals(p, f)
		for _, d := range f.ast.Decls {
			switch d := d.(type) {
			case *ast.FuncDecl:
				m.Symbols = append(m.Symbols, x.funcSymbols(f, d)...)
				from := x.funcKey(p, d)
				w := x.walker(p, f, from)
				w.bindParams(d.Recv)
				w.bindParams(d.Type.Params)
				if d.Body != nil {
					w.walk(d.Body)
				}
				m.Calls = append(m.Calls, w.calls...)
				if m.IsMain && d.Recv == nil && d.Name.Name == "main" {
					x.out.contracts = append(x.out.contracts, ContractFacts{
						Kind: KindCLICommand, Direction: DirExposes, Selector: "cmd:" + x.rp.dirName(p.dir),
						OwnerPath: p.dir, Location: x.pos(f, d.Pos()), Symbol: from, Origin: model.Extracted,
					})
				}
			case *ast.GenDecl:
				m.Symbols = append(m.Symbols, x.genSymbols(f, d)...)
				x.walker(p, f, "").walk(d)
			}
		}
	}
	x.out.modules = append(x.out.modules, m)
}

func isTestName(name string) bool {
	if !strings.HasPrefix(name, "Test") {
		return false
	}
	rest := name[len("Test"):]
	return rest == "" || !unicode.IsLower(rune(rest[0]))
}

func goTestCommand(p *goPkg, name string) string {
	pkgArg := "./" + p.dir
	prefix := ""
	switch {
	case p.modRoot == p.dir:
		pkgArg = "."
	case p.modRoot != "" && p.modRoot != ".":
		pkgArg = "./" + strings.TrimPrefix(p.dir, p.modRoot+"/")
	}
	if p.modRoot != "" && p.modRoot != "." {
		prefix = "cd " + p.modRoot + " && "
	}
	return prefix + "go test " + pkgArg + " -run '^" + name + "$'"
}

func (x *goExtractor) pos(f *goFile, p token.Pos) string {
	return loc(f.rel, x.fset.Position(p).Line)
}

func (x *goExtractor) funcKey(p *goPkg, fd *ast.FuncDecl) string {
	if recv := recvTypeName(fd); recv != "" {
		return p.key + "." + recv + "." + fd.Name.Name
	}
	return p.key + "." + fd.Name.Name
}

func (x *goExtractor) funcSymbols(f *goFile, fd *ast.FuncDecl) []model.Symbol {
	if !fd.Name.IsExported() {
		return nil
	}
	if recv := recvTypeName(fd); recv != "" {
		if !ast.IsExported(recv) {
			return nil
		}
		return []model.Symbol{{Name: recv + "." + fd.Name.Name, Kind: "method", Location: x.pos(f, fd.Pos())}}
	}
	return []model.Symbol{{Name: fd.Name.Name, Kind: "func", Location: x.pos(f, fd.Pos())}}
}

func (x *goExtractor) genSymbols(f *goFile, gd *ast.GenDecl) []model.Symbol {
	kind := map[token.Token]string{token.TYPE: "type", token.CONST: "const", token.VAR: "var"}[gd.Tok]
	var out []model.Symbol
	for _, s := range gd.Specs {
		switch s := s.(type) {
		case *ast.TypeSpec:
			if s.Name.IsExported() {
				out = append(out, model.Symbol{Name: s.Name.Name, Kind: kind, Location: x.pos(f, s.Pos())})
			}
		case *ast.ValueSpec:
			for _, n := range s.Names {
				if n.IsExported() {
					out = append(out, model.Symbol{Name: n.Name, Kind: kind, Location: x.pos(f, n.Pos())})
				}
			}
		}
	}
	return out
}

func (x *goExtractor) collectLiterals(p *goPkg, f *goFile) {
	ast.Inspect(f.ast, func(n ast.Node) bool {
		if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING {
			if s, err := strconv.Unquote(lit.Value); err == nil {
				x.out.literals[p.dir] = append(x.out.literals[p.dir], stringLit{s, x.pos(f, lit.Pos())})
			}
		}
		return true
	})
}

// topicConstants records Topic-named string constants that no consume/produce site referenced,
// so a declared topic is never silently missing from the contract view.
func (x *goExtractor) topicConstants() {
	seen := map[string]bool{}
	for _, c := range x.out.contracts {
		if c.Kind == KindKafka {
			seen[c.Selector] = true
		}
	}
	for _, key := range model.SortedKeys(x.idx.consts) {
		c := x.idx.consts[key]
		name := key[strings.LastIndex(key, ".")+1:]
		if !strings.Contains(strings.ToLower(name), "topic") {
			continue
		}
		v, ok := x.evalString(c.expr, c.file, c.pkg, 0)
		if !ok || seen[v] {
			continue
		}
		seen[v] = true
		x.out.contracts = append(x.out.contracts, ContractFacts{
			Kind: KindKafka, Direction: topicDirection(name), Selector: v, OwnerPath: c.pkg.dir,
			Location: x.pos(c.file, c.expr.Pos()), Symbol: c.pkg.key + "." + name, Origin: model.Extracted,
			Note: "topic constant declared; no consume or produce site found",
		})
	}
}
