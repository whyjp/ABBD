package analysis

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"github.com/whyjp/for-my-harness/internal/model"
)

// walker visits one declaration. scope is flat per function: shadowing is ignored, which can only
// widen a receiver guess, never hide a call.
type walker struct {
	x     *goExtractor
	p     *goPkg
	f     *goFile
	from  string
	scope map[string]typeRef
	calls []CallEdge
}

func (x *goExtractor) walker(p *goPkg, f *goFile, from string) *walker {
	return &walker{x: x, p: p, f: f, from: from, scope: map[string]typeRef{}}
}

func (w *walker) bindParams(fl *ast.FieldList) {
	if fl == nil {
		return
	}
	for _, fld := range fl.List {
		t, ok := w.x.resolveType(fld.Type, w.f, w.p)
		for _, n := range fld.Names {
			if ok {
				w.scope[n.Name] = t
			}
		}
	}
}

func (w *walker) walk(n ast.Node) {
	ast.Inspect(n, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.AssignStmt:
			w.assign(n)
		case *ast.ValueSpec:
			for i, name := range n.Names {
				if t, ok := w.declType(n, i); ok {
					w.scope[name.Name] = t
				}
			}
		case *ast.FuncLit:
			w.bindParams(n.Type.Params)
		case *ast.CallExpr:
			w.call(n)
		case *ast.CompositeLit:
			w.topicFields(n)
		}
		return true
	})
}

func (w *walker) assign(a *ast.AssignStmt) {
	switch {
	case len(a.Lhs) == len(a.Rhs):
		for i, l := range a.Lhs {
			if id, ok := l.(*ast.Ident); ok {
				if t, ok := w.typeOf(a.Rhs[i]); ok {
					w.scope[id.Name] = t
				}
			}
		}
	case len(a.Rhs) == 1:
		if id, ok := a.Lhs[0].(*ast.Ident); ok {
			if t, ok := w.typeOf(a.Rhs[0]); ok {
				w.scope[id.Name] = t
			}
		}
	}
}

func (w *walker) declType(vs *ast.ValueSpec, i int) (typeRef, bool) {
	if vs.Type != nil {
		return w.x.resolveType(vs.Type, w.f, w.p)
	}
	if i < len(vs.Values) {
		return w.typeOf(vs.Values[i])
	}
	return typeRef{}, false
}

func (w *walker) importOf(e ast.Expr) (string, bool) {
	id, ok := e.(*ast.Ident)
	if !ok {
		return "", false
	}
	if _, local := w.scope[id.Name]; local {
		return "", false
	}
	ip, ok := w.f.imports[id.Name]
	return ip, ok
}

func (w *walker) typeOf(e ast.Expr) (typeRef, bool) {
	idx := w.x.idx
	switch t := e.(type) {
	case *ast.Ident:
		if r, ok := w.scope[t.Name]; ok {
			return r, true
		}
		r, ok := idx.vars[w.p.key+"."+t.Name]
		return r, ok
	case *ast.ParenExpr:
		return w.typeOf(t.X)
	case *ast.StarExpr:
		return w.typeOf(t.X)
	case *ast.UnaryExpr:
		if t.Op == token.AND {
			return w.typeOf(t.X)
		}
	case *ast.CompositeLit:
		if t.Type != nil {
			return w.x.resolveType(t.Type, w.f, w.p)
		}
	case *ast.CallExpr:
		targets, conf := w.callees(t)
		if len(targets) == 1 && (conf == ConfidenceStatic || idx.isIface(targets[0])) {
			r := idx.funcs[targets[0]]
			return r, r != typeRef{}
		}
	case *ast.SelectorExpr:
		if ip, ok := w.importOf(t.X); ok {
			r, ok := idx.vars[ip+"."+t.Sel.Name]
			return r, ok
		}
		if r, ok := w.typeOf(t.X); ok {
			if info := idx.types[r]; info != nil {
				f, ok := info.fields[t.Sel.Name]
				return f, ok
			}
		}
	}
	return typeRef{}, false
}

// isIface reports whether a "pkg.Type.Method" key is a method of an in-repo interface.
func (idx *goIndex) isIface(key string) bool {
	i := strings.LastIndex(key, ".")
	j := strings.LastIndex(key[:max(i, 0)], ".")
	if i < 0 || j < 0 {
		return false
	}
	info := idx.types[typeRef{key[:j], key[j+1 : i]}]
	return info != nil && info.iface
}

// callees resolves the in-repo targets of a call. Unresolved receivers fan out to every in-repo
// method of that name as "possible" edges rather than being dropped.
func (w *walker) callees(c *ast.CallExpr) ([]string, string) {
	idx := w.x.idx
	switch fun := c.Fun.(type) {
	case *ast.Ident:
		if _, local := w.scope[fun.Name]; !local {
			if k := w.p.key + "." + fun.Name; hasKey(idx.funcs, k) {
				return []string{k}, ConfidenceStatic
			}
		}
	case *ast.SelectorExpr:
		if ip, ok := w.importOf(fun.X); ok {
			if k := ip + "." + fun.Sel.Name; hasKey(idx.funcs, k) {
				return []string{k}, ConfidenceStatic
			}
			return nil, ""
		}
		r, ok := w.typeOf(fun.X)
		if !ok {
			return idx.methodsByName[fun.Sel.Name], ConfidencePossible
		}
		k := r.pkg + "." + r.name + "." + fun.Sel.Name
		if !hasKey(idx.funcs, k) {
			return nil, ""
		}
		if idx.isIface(k) {
			return []string{k}, ConfidencePossible
		}
		return []string{k}, ConfidenceStatic
	}
	return nil, ""
}

func hasKey[V any](m map[string]V, k string) bool {
	_, ok := m[k]
	return ok
}

func (w *walker) call(c *ast.CallExpr) {
	if w.from != "" {
		targets, conf := w.callees(c)
		for _, t := range targets {
			w.calls = append(w.calls, CallEdge{From: w.from, To: t, Confidence: conf, Location: w.x.pos(w.f, c.Pos())})
		}
	}
	name := ""
	switch fun := c.Fun.(type) {
	case *ast.Ident:
		name = fun.Name
	case *ast.SelectorExpr:
		name = fun.Sel.Name
		if ip, ok := w.importOf(fun.X); ok && ip == "os" && (name == "Getenv" || name == "LookupEnv") && len(c.Args) > 0 {
			w.contract(KindEnvVar, DirReads, c.Args[0], c.Pos(), w.from, "env var name assembled at runtime")
			return
		}
	}
	w.route(c, name)
	w.topicCall(c, name)
}

var routeVerbs = map[string]bool{"GET": true, "POST": true, "PUT": true, "DELETE": true, "PATCH": true}

func (w *walker) route(c *ast.CallExpr, name string) {
	if (name != "HandleFunc" && name != "Handle" && !routeVerbs[name]) || len(c.Args) < 2 {
		return
	}
	if _, ok := c.Fun.(*ast.SelectorExpr); !ok {
		return
	}
	handler := w.handlerSymbol(c.Args[len(c.Args)-1])
	v, ok := w.x.evalString(c.Args[0], w.f, w.p, 0)
	switch {
	case ok && routeVerbs[name] && strings.HasPrefix(v, "/"):
		w.add(KindHTTPRoute, DirExposes, name+" "+v, c.Pos(), handler, model.Extracted, "")
	case ok && !routeVerbs[name] && looksLikePattern(v):
		w.add(KindHTTPRoute, DirExposes, v, c.Pos(), handler, model.Extracted, "")
	case !ok && name == "HandleFunc":
		w.add(KindHTTPRoute, DirExposes, types.ExprString(c.Args[0]), c.Pos(), handler, model.UnknownOrigin, "route pattern assembled at runtime")
	}
}

// looksLikePattern accepts "/path" and Go 1.22 "METHOD /path" mux patterns, so Handle(ctx, msg) is not a route.
func looksLikePattern(v string) bool {
	if strings.HasPrefix(v, "/") {
		return true
	}
	method, rest, ok := strings.Cut(v, " ")
	return ok && method == strings.ToUpper(method) && strings.HasPrefix(strings.TrimSpace(rest), "/")
}

func (w *walker) handlerSymbol(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.Ident:
		if k := w.p.key + "." + t.Name; hasKey(w.x.idx.funcs, k) {
			return k
		}
	case *ast.SelectorExpr:
		if ip, ok := w.importOf(t.X); ok {
			return ip + "." + t.Sel.Name
		}
		if r, ok := w.typeOf(t.X); ok && w.x.idx.types[r] != nil {
			return r.pkg + "." + r.name + "." + t.Sel.Name
		}
	}
	return w.from
}

var topicFieldNames = map[string]bool{"Topic": true, "Topics": true, "GroupTopics": true}

func (w *walker) topicFields(lit *ast.CompositeLit) {
	typeName := ""
	if lit.Type != nil {
		typeName = types.ExprString(lit.Type)
	}
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok || !topicFieldNames[key.Name] {
			continue
		}
		values := []ast.Expr{kv.Value}
		if list, ok := kv.Value.(*ast.CompositeLit); ok {
			values = list.Elts
		}
		dir := topicDirection(typeName, shortName(w.from))
		for _, v := range values {
			w.contract(KindKafka, dir, v, v.Pos(), w.from, "topic assembled at runtime")
		}
	}
}

func (w *walker) topicCall(c *ast.CallExpr, name string) {
	isTopic := strings.Contains(name, "Topic")
	isClient := strings.HasPrefix(name, "New") && containsAny(strings.ToLower(name), clientWords...)
	isPubSub := hasAnyPrefix(name, "Publish", "Produce", "Subscribe", "Consume")
	if !isTopic && !isClient && !isPubSub {
		return
	}
	dir := topicDirection(name, shortName(w.from))
	for _, a := range c.Args {
		if v, ok := w.x.evalString(a, w.f, w.p, 0); ok {
			w.add(KindKafka, dir, v, a.Pos(), w.from, model.Extracted, "")
			return
		}
	}
	if !isTopic {
		return
	}
	for _, a := range c.Args {
		if id, ok := a.(*ast.Ident); ok && id.Name == "ctx" {
			continue
		}
		switch a.(type) {
		case *ast.Ident, *ast.BinaryExpr, *ast.CallExpr, *ast.SelectorExpr:
			w.add(KindKafka, dir, types.ExprString(a), a.Pos(), w.from, model.UnknownOrigin, "topic assembled at runtime")
		}
		return
	}
}

var clientWords = []string{"reader", "consumer", "subscriber", "writer", "producer", "publisher"}

func topicDirection(names ...string) string {
	for _, n := range names {
		l := strings.ToLower(n)
		switch {
		case containsAny(l, "reader", "consumer", "subscribe", "consume"):
			return DirConsumes
		case containsAny(l, "writer", "producer", "publish", "produce", "send"):
			return DirProduces
		}
	}
	return DirUnknown
}

// shortName drops the import path so words in a module path do not steer direction heuristics.
func shortName(sym string) string { return sym[strings.LastIndex(sym, "/")+1:] }

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

func hasAnyPrefix(s string, prefixes ...string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

// contract records a selector that is either a resolvable constant (EXTRACTED) or not (UNKNOWN).
func (w *walker) contract(kind, dir string, e ast.Expr, pos token.Pos, symbol, runtimeNote string) {
	if v, ok := w.x.evalString(e, w.f, w.p, 0); ok {
		w.add(kind, dir, v, pos, symbol, model.Extracted, "")
		return
	}
	w.add(kind, dir, types.ExprString(e), pos, symbol, model.UnknownOrigin, runtimeNote)
}

func (w *walker) add(kind, dir, selector string, pos token.Pos, symbol string, origin model.Origin, note string) {
	if dir == DirUnknown && note == "" {
		note = "direction could not be determined from names"
	}
	w.x.out.contracts = append(w.x.out.contracts, ContractFacts{
		Kind: kind, Direction: dir, Selector: selector, OwnerPath: w.p.dir,
		Location: w.x.pos(w.f, pos), Symbol: symbol, Origin: origin, Note: note,
	})
}
