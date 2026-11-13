package relate

import (
	"fmt"
	"go/token"
	"go/types"
	"os"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/tools/go/callgraph/vta"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"

	"github.com/whyjp/abbd/internal/model"
)

// goEnv is the one build configuration the Go relater reads, the same on
// every host, so a Windows and a Linux run print the same relations. Without
// cgo the loader never runs a C compiler.
var goEnv = []string{"GOOS=linux", "GOARCH=amd64", "CGO_ENABLED=0"}

// golang reads the module's packages, tests left out, with go/types, builds
// their SSA form, and resolves every call, static or through an interface or
// a func value, with VTA. Dependencies outside the module come from export
// data, so only the module's own code is analysed.
func golang(t tree, o owners) (found, error) {
	cfg := &packages.Config{Mode: packages.LoadSyntax, Dir: t.dir, Env: append(os.Environ(), goEnv...), Fset: token.NewFileSet()}
	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		return found{}, err
	}
	g := goRead{tree: t, owners: o, fset: cfg.Fset, loaded: map[string]bool{}, f: found{covered: map[string]bool{}}}
	var broken []string
	for _, p := range pkgs {
		for _, f := range p.IgnoredFiles {
			if r, ok := t.rel(f); ok && strings.HasSuffix(r, ".go") && !strings.HasSuffix(r, "_test.go") {
				g.f.skipped++
			}
		}
		for _, e := range p.Errors {
			broken = append(broken, e.Error())
		}
		for _, f := range p.CompiledGoFiles {
			if r, ok := t.rel(f); ok {
				g.loaded[r] = true
				if u, ok := o.of(r); ok {
					g.f.covered[u] = true
				}
			}
		}
	}
	if len(broken) > 0 {
		return found{}, fmt.Errorf("the module does not type-check for %v, so its relations cannot be read: %s", goEnv, broken[0])
	}
	prog, _ := ssautil.Packages(pkgs, ssa.InstantiateGenerics)
	prog.Build()
	funcs := ssautil.AllFunctions(prog)
	g.calls(funcs)
	g.imports(pkgs)
	g.traits(pkgs, funcs)
	return g.f, nil
}

type goRead struct {
	tree
	owners
	fset *token.FileSet
	// loaded are the module's own files, so a vendored or replaced
	// dependency inside the root never counts as a unit's code.
	loaded map[string]bool
	f      found
}

// at is the root-relative site of pos, and the unit whose code holds it. It
// reads the file the compiler read, past any //line directive.
func (g *goRead) at(pos token.Pos) (site, string, bool) {
	if !pos.IsValid() {
		return site{}, "", false
	}
	p := g.fset.PositionFor(pos, false)
	r, ok := g.rel(p.Filename)
	if !ok || !g.loaded[r] {
		return site{}, "", false
	}
	u, ok := g.of(r)
	return site{path: r, line: p.Line}, u, ok
}

func (g *goRead) add(from site, fromUnit string, to token.Pos, kind model.RelationKind, via model.Via) {
	_, toUnit, ok := g.at(to)
	if ok && toUnit != fromUnit {
		g.f.facts = append(g.f.facts, fact{link{fromUnit, toUnit, kind}, via, from})
	}
}

// calls adds a call for every call site whose callee another unit declares.
// A call through an interface or a func value counts only into a package the
// caller imports: one into a package that does not know the caller's is a
// plug point or a callback calling back, and its dependency runs the other
// way, as the implementation or the call that passed the func.
func (g *goRead) calls(funcs map[*ssa.Function]bool) {
	cg := vta.CallGraph(funcs, nil)
	for fn, n := range cg.Nodes {
		caller := pkgOf(fn)
		if fn == nil || !funcs[fn] || caller == nil {
			continue
		}
		for _, e := range n.Out {
			if e.Site == nil {
				continue
			}
			callee := e.Callee.Func
			dynamic := e.Site.Common().StaticCallee() == nil
			if dynamic && !slices.Contains(caller.Imports(), pkgOf(callee)) {
				continue
			}
			if s, u, ok := g.at(e.Site.Pos()); ok {
				s.dynamic = dynamic
				g.add(s, u, declared(callee), model.DependsOn, "call")
			}
		}
	}
}

// pkgOf is the package that declares fn. A wrapper or a generic instance
// belongs to no SSA package, so it is the package of what it stands for.
func pkgOf(fn *ssa.Function) *types.Package {
	if fn == nil {
		return nil
	}
	if o := fn.Origin(); o != nil {
		fn = o
	}
	if fn.Pkg != nil {
		return fn.Pkg.Pkg
	}
	if obj := fn.Object(); obj != nil {
		return obj.Pkg()
	}
	return nil
}

// declared is where a function's source declares it. A wrapper or an
// instance the compiler makes has no syntax of its own, so it is placed at
// the method or generic function it stands for.
func declared(fn *ssa.Function) token.Pos {
	if o := fn.Origin(); o != nil {
		fn = o
	}
	if obj := fn.Object(); obj != nil {
		return obj.Pos()
	}
	return fn.Pos()
}

// imports adds an import for every use of a name another unit declares: a
// type, a constant, a variable, a field or a function. The via ranking keeps
// a call over it, so it shows only where the code uses and never calls. A
// blank import names nothing, and still links the package in and runs its
// init, which is how a plugin registers itself.
func (g *goRead) imports(pkgs []*packages.Package) {
	for _, p := range pkgs {
		for _, f := range p.Syntax {
			for _, spec := range f.Imports {
				if spec.Name == nil || spec.Name.Name != "_" {
					continue
				}
				path, _ := strconv.Unquote(spec.Path.Value)
				dep, ok := p.Imports[path]
				if !ok || len(dep.CompiledGoFiles) == 0 {
					continue
				}
				if s, u, ok := g.at(spec.Pos()); ok {
					if r, ok := g.rel(slices.Min(dep.CompiledGoFiles)); ok {
						if to, ok := g.of(r); ok && to != u {
							g.f.facts = append(g.f.facts, fact{link{u, to, model.DependsOn}, "import", s})
						}
					}
				}
			}
		}
		for id, obj := range p.TypesInfo.Uses {
			if _, ok := obj.(*types.PkgName); ok || obj.Pkg() == nil || obj.Pkg() == p.Types {
				continue
			}
			if s, u, ok := g.at(id.Pos()); ok {
				g.add(s, u, obj.Pos(), model.DependsOn, "import")
			}
		}
	}
}

// traits adds an implementation for every named type that implements an
// interface another unit declares, where the type's package imports the
// interface's or the code converts the type to it; a method set that only
// happens to match is no plug point. The implementer provides the
// interface's contract unit when one is anchored at it, and a unit that
// calls through the interface consumes it; otherwise the implementer depends
// on the declaring unit.
func (g *goRead) traits(pkgs []*packages.Package, funcs map[*ssa.Function]bool) {
	type pair struct{ t, i *types.TypeName }
	converted := map[pair]bool{}
	for fn := range funcs {
		for _, b := range fn.Blocks {
			for _, ins := range b.Instrs {
				switch x := ins.(type) {
				case *ssa.MakeInterface:
					if t, i := named(x.X.Type()), named(x.Type()); t != nil && i != nil {
						converted[pair{t, i}] = true
					}
				case ssa.CallInstruction:
					c := x.Common()
					if !c.IsInvoke() {
						continue
					}
					i := named(c.Value.Type())
					s, u, ok := g.at(x.Pos())
					if !ok || i == nil {
						continue
					}
					if decl, _, ok := g.at(i.Pos()); ok {
						if ifc, ok := g.contract(decl.path, i.Name()); ok && ifc != u {
							g.f.facts = append(g.f.facts, fact{link{u, ifc, model.Consumes}, "trait", s})
						}
					}
				}
			}
		}
	}
	var ifaces, concrete []*types.TypeName
	byPath := map[string]*packages.Package{}
	for _, p := range pkgs {
		byPath[p.PkgPath] = p
		for _, name := range p.Types.Scope().Names() {
			tn, ok := p.Types.Scope().Lookup(name).(*types.TypeName)
			if !ok || tn.IsAlias() {
				continue
			}
			nt, ok := tn.Type().(*types.Named)
			if !ok || nt.TypeParams().Len() > 0 {
				continue
			}
			if it, ok := nt.Underlying().(*types.Interface); ok {
				if it.NumMethods() > 0 && it.IsMethodSet() {
					ifaces = append(ifaces, tn)
				}
				continue
			}
			concrete = append(concrete, tn)
		}
	}
	for _, i := range ifaces {
		decl, declUnit, ok := g.at(i.Pos())
		if !ok {
			continue
		}
		it := i.Type().Underlying().(*types.Interface)
		ifc, isContract := g.contract(decl.path, i.Name())
		for _, t := range concrete {
			s, u, ok := g.at(t.Pos())
			if !ok || u == declUnit {
				continue
			}
			if !types.Implements(t.Type(), it) && !types.Implements(types.NewPointer(t.Type()), it) {
				continue
			}
			_, imports := byPath[t.Pkg().Path()].Imports[i.Pkg().Path()]
			if !imports && !converted[pair{t, i}] {
				continue
			}
			if isContract {
				g.f.facts = append(g.f.facts, fact{link{u, ifc, model.Provides}, "trait", s})
			} else {
				g.f.facts = append(g.f.facts, fact{link{u, declUnit, model.DependsOn}, "trait", s})
			}
		}
	}
}

// named is the declared type t names, through a pointer, or nil.
func named(t types.Type) *types.TypeName {
	if p, ok := types.Unalias(t).(*types.Pointer); ok {
		t = p.Elem()
	}
	if n, ok := types.Unalias(t).(*types.Named); ok {
		return n.Origin().Obj()
	}
	return nil
}
