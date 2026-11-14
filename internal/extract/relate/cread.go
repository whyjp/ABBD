package relate

import (
	"context"
	"fmt"
	"os"
	"path"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/whyjp/abbd/internal/extract/lsp"
	"github.com/whyjp/abbd/internal/model"
)

// cUse is what naming a declaration does, which picks the via of each
// place that names it.
type cUse byte

const (
	// cCalled is a function or method: a call where the name is called,
	// a callback where the code takes it as a value.
	cCalled cUse = iota
	// cExpanded is a function-like macro.
	cExpanded
	// cNamed is a type, a variable, a constant, an enumerator or an
	// object-like macro, which the code only names.
	cNamed
)

// cDecl is a declaration other files can name.
type cDecl struct {
	path, name string
	at         lsp.Position
	use        cUse
	// verify is whether each reference is checked to resolve to this
	// declaration in the referencing file. C types and macros have no
	// linkage, so clangd's index merges two of one name from different
	// files.
	verify bool
	// linked is whether the name has linkage, so the unit that defines it
	// owns it wherever a header declares it.
	linked bool
	// usr is clang's id for a macro, which names the file and the byte
	// offset of its name in the #define, so a reference resolves to it when
	// its id is the same. clangd's symbolInfo gives a macro no declaration,
	// and gives none at all at a #define the header's own parse leaves
	// out, though the sources that include it set the macro.
	usr string
	// class is the class or namespace that holds a C++ declaration, as the
	// index names it, last segment only.
	class string
	owner string
	slot  *cSlot
}

// cPlace is a line of code, with the unit that holds it.
type cPlace struct {
	unit string
	at   site
}

// cSlot is a function-pointer type: a struct whose fields include function
// pointers, or an alias of a function pointer. A brace initializer of it
// fills a table a host calls through.
type cSlot struct {
	fields []lsp.Position
	alias  bool
}

func (d cDecl) via(next byte) model.Via {
	switch {
	case d.use == cCalled && next == '(':
		return "call"
	case d.use == cCalled:
		return "callback"
	case d.use == cExpanded:
		return "macro"
	}
	return "import"
}

var (
	cDefine = regexp.MustCompile(`^\s*#\s*define\s+([A-Za-z_]\w*)(\()?`)
	// cDeclarator follows a type's name in a declaration with a brace
	// initializer: struct ops name = {, or a table, name[] = {.
	cDeclarator = regexp.MustCompile(`^\s*(?:const\s+|volatile\s+|\*\s*)*([A-Za-z_]\w*)\s*(?:\[[^\]]*\]\s*)*=\s*\{`)
	// cNamedVar follows a type's name in a declaration without one: the
	// declared name and what comes after it.
	cNamedVar = regexp.MustCompile(`^\s*(?:const\s+|volatile\s+|\*\s*)*([A-Za-z_]\w*)\s*(\S?)`)
)

// relate reads the files: the sources' declarations; once the background
// index is ready, the names the index holds; then the headers'
// declarations, the references to each name, and last the
// function-pointer tables. Headers open only after the index is ready, so
// clangd parses each with the command of a source that includes it rather
// than one it guesses from a nearby file, which can be another language;
// and the names are read before, so the index's view of a header, read in
// each including source's context, is not replaced by the header's own
// parse.
func (r *cReader) relate(ctx context.Context, read []string) ([]fact, error) {
	var sources, headers []string
	for _, p := range read {
		b, err := os.ReadFile(r.abs(p))
		if err != nil {
			return nil, err
		}
		ls := strings.Split(string(b), "\n")
		starts := make([]int, len(ls))
		for i := range ls {
			if i > 0 {
				starts[i] = starts[i-1] + len(ls[i-1]) + 1
			}
		}
		for i := range ls {
			ls[i] = strings.TrimSuffix(ls[i], "\r")
		}
		r.src[p], r.starts[p] = ls, starts
		if cExt(p, cHeaders) {
			headers = append(headers, p)
		} else {
			sources = append(sources, p)
		}
	}
	if err := r.open(ctx, sources); err != nil {
		return nil, err
	}
	if err := r.srv.Wait(ctx, 3*time.Second); err != nil {
		return nil, err
	}
	decls, err := r.decls(ctx, read)
	if err != nil {
		return nil, err
	}
	if err := r.open(ctx, headers); err != nil {
		return nil, err
	}
	if err := r.srv.Wait(ctx, 3*time.Second); err != nil {
		return nil, err
	}
	for _, d := range decls {
		if d.use == cNamed && d.usr == "" && cExt(d.path, cHeaders) {
			d.slot = r.slot(d.path, d.at)
		}
	}
	refs := make([][]lsp.Location, len(decls))
	err = parallel(len(decls), func(i int) error {
		var err error
		refs[i], err = r.uses(ctx, decls[i])
		return err
	})
	if err != nil {
		return nil, err
	}
	for i, d := range decls {
		if d.slot != nil {
			if err := r.table(ctx, d, refs[i]); err != nil {
				return nil, err
			}
		}
	}
	// A unit that provides a contract names its types in doing so, which
	// is no consuming.
	provides := map[link]bool{}
	for _, x := range r.facts {
		if x.kind == model.Provides {
			provides[link{x.from, x.to, model.Consumes}] = true
		}
	}
	return slices.DeleteFunc(r.facts, func(x fact) bool { return provides[x.link] }), nil
}

func cLanguage(p string) string {
	switch {
	case strings.HasSuffix(p, ".c"):
		return "c"
	case strings.HasSuffix(p, ".m"):
		return "objective-c"
	case strings.HasSuffix(p, ".mm"):
		return "objective-cpp"
	case strings.HasSuffix(p, ".h"):
		return "c"
	}
	return "cpp"
}

// text is the source a range spans.
func (r *cReader) text(p string, rg lsp.Range) string {
	ls := r.lines(p)
	if rg.Start.Line >= len(ls) {
		return ""
	}
	var b strings.Builder
	for l := rg.Start.Line; l <= rg.End.Line && l < len(ls); l++ {
		s := ls[l]
		if l == rg.End.Line {
			s = s[:min(rg.End.Character, len(s))]
		}
		if l == rg.Start.Line {
			s = s[min(rg.Start.Character, len(s)):]
		}
		b.WriteString(s)
		b.WriteByte('\n')
	}
	return b.String()
}

var cStatic = regexp.MustCompile(`\bstatic\b`)

// decls are the declarations other files can name, as clangd's index
// holds them: a header read in the context of each source that includes
// it, so a declaration under an #if the source sets counts, and each name
// located where it is defined. A source file offers only its functions,
// methods and variables with linkage: a type or a macro it defines is its
// own. A header offers every named declaration but a field, since naming a
// field names its type too, and every macro, which the index does not
// list.
func (r *cReader) decls(ctx context.Context, read []string) ([]*cDecl, error) {
	syms, err := r.srv.WorkspaceSymbols(ctx, "")
	if err != nil {
		return nil, err
	}
	var out []*cDecl
	seen := map[string]bool{}
	for _, s := range syms {
		abs, err := lsp.Path(s.Location.URI)
		if err != nil {
			continue
		}
		p, ok := r.rel(abs)
		if !ok || !r.read[p] || strings.Contains(s.ContainerName, "anonymous") {
			continue
		}
		key := site3(p, s.Location.Range.Start)
		if seen[key] {
			continue
		}
		seen[key] = true
		d := &cDecl{path: p, name: s.Name, at: s.Location.Range.Start}
		d.owner, _ = r.of(p)
		if i := strings.LastIndex(s.ContainerName, "::"); i >= 0 {
			d.class = s.ContainerName[i+2:]
		} else {
			d.class = s.ContainerName
		}
		header := cExt(p, cHeaders)
		switch s.Kind {
		case lsp.KindFunction, lsp.KindMethod, lsp.KindConstructor:
			if strings.HasPrefix(s.Name, "operator") || strings.HasPrefix(s.Name, "~") || !header && r.static(p, d.at) {
				continue
			}
			d.use, d.linked, d.verify = cCalled, true, s.Kind != lsp.KindFunction
		case lsp.KindVariable, lsp.KindConstant:
			if !header && r.static(p, d.at) {
				continue
			}
			d.use, d.linked = cNamed, true
		case lsp.KindField, lsp.KindNamespace, lsp.KindModule, lsp.KindTypeParameter:
			continue
		default:
			if !header {
				continue
			}
			d.use, d.verify = cNamed, true
			r.classes[s.Name] = append(r.classes[s.Name], p)
		}
		out = append(out, d)
	}
	for _, p := range read {
		if cExt(p, cHeaders) {
			out = append(out, r.macros(p)...)
		}
	}
	slices.SortFunc(out, func(a, b *cDecl) int { return strings.Compare(site3(a.path, a.at), site3(b.path, b.at)) })
	return out, nil
}

// open opens the files and reads their document symbols.
func (r *cReader) open(ctx context.Context, files []string) error {
	for _, p := range files {
		if err := r.srv.Open(r.abs(p), cLanguage(p)); err != nil {
			return err
		}
	}
	return parallel(len(files), func(i int) error {
		syms, err := r.srv.DocumentSymbols(ctx, r.abs(files[i]))
		if err != nil {
			return err
		}
		r.mu.Lock()
		r.syms[files[i]] = syms
		r.mu.Unlock()
		return nil
	})
}

// symbolAt is the document symbol whose name starts at pos in p.
func (r *cReader) symbolAt(p string, pos lsp.Position) (lsp.DocumentSymbol, bool) {
	r.mu.Lock()
	syms := r.syms[p]
	r.mu.Unlock()
	var find func(list []lsp.DocumentSymbol) (lsp.DocumentSymbol, bool)
	find = func(list []lsp.DocumentSymbol) (lsp.DocumentSymbol, bool) {
		for _, s := range list {
			if s.SelectionRange.Start == pos {
				return s, true
			}
			if c, ok := find(s.Children); ok {
				return c, true
			}
		}
		return lsp.DocumentSymbol{}, false
	}
	return find(syms)
}

// static is whether the declaration at pos in a source file is static or
// extern, by the text before its name: internal linkage, or a declaration
// another file defines.
func (r *cReader) static(p string, pos lsp.Position) bool {
	head := ""
	if s, ok := r.symbolAt(p, pos); ok {
		head = r.text(p, lsp.Range{Start: s.Range.Start, End: pos})
	} else if ls := r.lines(p); pos.Line < len(ls) {
		head = ls[pos.Line][:min(pos.Character, len(ls[pos.Line]))]
	}
	return cStatic.MatchString(head) || strings.Contains(head, "extern")
}

// slot is the function-pointer slot a header's type declares at pos: a
// struct with a function-pointer field, as clangd's document symbols show
// the fields' types, or an alias of a function pointer.
func (r *cReader) slot(p string, pos lsp.Position) *cSlot {
	s, ok := r.symbolAt(p, pos)
	if !ok {
		return nil
	}
	if s.Detail == "type alias" && strings.Contains(r.text(p, s.Range), "(*") {
		return &cSlot{alias: true}
	}
	var out *cSlot
	for _, f := range s.Children {
		if f.Kind == lsp.KindField && strings.Contains(f.Detail, "(*)") {
			if out == nil {
				out = &cSlot{}
			}
			out.fields = append(out.fields, f.SelectionRange.Start)
		}
	}
	return out
}

// macros are a header's #define lines, which clangd lists no symbol for.
// An include guard, a #define of no value right after the #ifndef of its
// own name, is left out.
func (r *cReader) macros(p string) []*cDecl {
	ls := r.lines(p)
	r.mu.Lock()
	starts := r.starts[p]
	r.mu.Unlock()
	owner, _ := r.of(p)
	var out []*cDecl
	prev := ""
	for i, l := range ls {
		m := cDefine.FindStringSubmatchIndex(l)
		if m == nil {
			if t := strings.TrimSpace(l); t != "" {
				prev = t
			}
			continue
		}
		name := l[m[2]:m[3]]
		guard := m[4] < 0 && strings.TrimSpace(l[m[1]:]) == "" && strings.Join(strings.Fields(strings.TrimPrefix(prev, "#")), " ") == "ifndef "+name
		prev = strings.TrimSpace(l)
		if guard {
			continue
		}
		use := cNamed
		if m[4] >= 0 {
			use = cExpanded
		}
		usr := fmt.Sprintf("c:%s@%d@macro@%s", path.Base(p), starts[i]+m[2], name)
		out = append(out, &cDecl{path: p, name: name, at: lsp.Position{Line: i, Character: m[2]}, use: use, verify: true, usr: usr, owner: owner})
	}
	return out
}

// place is the read file a location sits in, its unit and its 1-based
// line, and false for a place outside the code the relater read.
func (r *cReader) place(l lsp.Location) (string, string, int, bool) {
	abs, err := lsp.Path(l.URI)
	if err != nil {
		return "", "", 0, false
	}
	p, ok := r.rel(abs)
	if !ok || !r.read[p] {
		return "", "", 0, false
	}
	u, ok := r.of(p)
	return p, u, l.Range.Start.Line + 1, ok
}

// uses adds a fact for each place another unit names d, and a consumes for
// each place any unit but d's names it where a contract unit anchors d. It
// returns every reference, for the table pass.
func (r *cReader) uses(ctx context.Context, d *cDecl) ([]lsp.Location, error) {
	p, at := r.askAt(d)
	refs, err := r.srv.References(ctx, r.abs(p), at)
	if err != nil {
		return nil, err
	}
	// The table pass reads a slot's contract: naming the type to fill a
	// table provides it.
	ifc, contract, err := r.contract(ctx, d)
	if err != nil {
		return nil, err
	}
	contract = contract && d.slot == nil
	for _, l := range refs {
		p, u, line, ok := r.place(l)
		if !ok || u == d.owner || r.directive(p, l.Range.Start.Line) || r.declared(p, l.Range.Start) {
			continue
		}
		if d.verify {
			ok, err := r.resolves(ctx, p, l.Range.Start, d)
			if err != nil {
				return nil, err
			}
			if !ok {
				continue
			}
		}
		at := site{path: p, line: line}
		if d.use == cNamed && cBase.MatchString(r.before(p, l.Range.Start)) {
			if contract {
				r.add(fact{link{u, ifc, model.Provides}, "trait", at})
			} else {
				r.add(fact{link{u, d.owner, model.DependsOn}, "trait", at})
			}
			continue
		}
		via := d.via(r.after(p, l.Range.End))
		r.add(fact{link{u, d.owner, model.DependsOn}, via, at})
		if contract {
			r.add(fact{link{u, ifc, model.Consumes}, via, at})
		} else if c, ok := r.method(d); ok && via == "call" {
			r.add(fact{link{u, c, model.Consumes}, "trait", at})
		}
	}
	return refs, nil
}

// cBase is the text before a class named as a base in a C++ class head,
// as in class PosixEnv : public leveldb::Env.
var cBase = regexp.MustCompile(`\b(?:class|struct)\s+[\w:<>, ]*?[:,]\s*(?:(?:public|protected|private|virtual)\s+)*(?:\w+::)*$`)

// before is the text of pos's line before it.
func (r *cReader) before(p string, pos lsp.Position) string {
	ls := r.lines(p)
	if pos.Line >= len(ls) {
		return ""
	}
	return ls[pos.Line][:min(pos.Character, len(ls[pos.Line]))]
}

// method is the contract unit anchored at the class that declares the
// method d, so a call through it consumes the class, as a call through a
// trait does.
func (r *cReader) method(d *cDecl) (string, bool) {
	if d.use != cCalled || d.class == "" {
		return "", false
	}
	for _, p := range r.classes[d.class] {
		if c, ok := r.owners.contract(p, d.class); ok {
			return c, true
		}
	}
	return "", false
}

// after is the first byte after pos on its line that is no blank, or 0.
func (r *cReader) after(p string, pos lsp.Position) byte {
	ls := r.lines(p)
	if pos.Line >= len(ls) {
		return 0
	}
	rest := strings.TrimLeft(ls[pos.Line][min(pos.Character, len(ls[pos.Line])):], " \t")
	if rest == "" {
		return 0
	}
	return rest[0]
}

// directive is whether a line belongs to a preprocessor directive that
// tests or forgets a name, as #if, #ifdef or #undef do, which uses nothing.
// A #define's body does use what it names.
func (r *cReader) directive(p string, line int) bool {
	ls := r.lines(p)
	for line > 0 && line < len(ls) && strings.HasSuffix(strings.TrimRight(ls[line-1], " \t"), `\`) {
		line--
	}
	if line >= len(ls) {
		return false
	}
	t := strings.TrimSpace(ls[line])
	if !strings.HasPrefix(t, "#") {
		return false
	}
	return !strings.HasPrefix(strings.TrimSpace(t[1:]), "define")
}

// cSymbolInfo is what clangd's symbolInfo says of the name at a position,
// read from the file's own AST rather than the index.
type cSymbolInfo struct {
	USR  string        `json:"usr"`
	Decl *lsp.Location `json:"declarationRange"`
	Def  *lsp.Location `json:"definitionRange"`
}

func (r *cReader) symbolInfo(ctx context.Context, p string, pos lsp.Position) ([]cSymbolInfo, error) {
	var infos []cSymbolInfo
	params := map[string]any{"textDocument": map[string]any{"uri": lsp.URI(r.abs(p))}, "position": pos}
	return infos, r.srv.Call(ctx, "textDocument/symbolInfo", params, &infos)
}

// resolves is whether the name at pos in p resolves, in p's own
// translation unit, to d: a macro by its id, anything else by a
// declaration in d's file.
func (r *cReader) resolves(ctx context.Context, p string, pos lsp.Position, d *cDecl) (bool, error) {
	infos, err := r.symbolInfo(ctx, p, pos)
	if err != nil {
		return false, err
	}
	for _, in := range infos {
		if d.usr != "" && in.USR == d.usr {
			return true, nil
		}
		for _, l := range []*lsp.Location{in.Decl, in.Def} {
			if l == nil {
				continue
			}
			if abs, err := lsp.Path(l.URI); err == nil {
				if q, ok := r.rel(abs); ok && q == d.path {
					return true, nil
				}
			}
		}
	}
	return false, nil
}

// contract is the contract unit anchored at d: at d's own file, or, for a
// name with linkage that the index locates where it is defined, at a
// header that declares it.
func (r *cReader) contract(ctx context.Context, d *cDecl) (string, bool, error) {
	if c, ok := r.owners.contract(d.path, d.name); ok || !d.linked {
		return c, ok, nil
	}
	anchored := false
	for k := range r.contracts {
		anchored = anchored || strings.HasSuffix(k, "#"+d.name)
	}
	if !anchored {
		return "", false, nil
	}
	decls, err := r.srv.Declarations(ctx, r.abs(d.path), d.at)
	if err != nil {
		return "", false, err
	}
	for _, l := range decls {
		if abs, err := lsp.Path(l.URI); err == nil {
			if q, ok := r.rel(abs); ok {
				if c, ok := r.owners.contract(q, d.name); ok {
					return c, true, nil
				}
			}
		}
	}
	return "", false, nil
}

// table reads the function-pointer tables of one slot type: each brace
// initializer of it, whose functions' units fill it, and each call through
// it, whose unit is the host. Under the plug-point rule the units that fill
// a slot a contract unit anchors provide it and the hosts consume it;
// without one each host depends on each filling unit via callback, since
// the host's code runs the filler's through the table.
func (r *cReader) table(ctx context.Context, d *cDecl, refs []lsp.Location) error {
	var fills, members, hosts []cPlace
	for _, l := range refs {
		p, u, line, ok := r.place(l)
		if !ok {
			continue
		}
		ms, isTable, err := r.members(ctx, p, l.Range.End)
		if err != nil {
			return err
		}
		if isTable {
			fills = append(fills, cPlace{u, site{path: p, line: line}})
			members = append(members, ms...)
			continue
		}
		if d.slot.alias {
			hs, err := r.callsThroughVar(ctx, p, l.Range.End)
			if err != nil {
				return err
			}
			hosts = append(hosts, hs...)
		}
	}
	if len(fills) == 0 {
		return nil
	}
	for _, f := range d.slot.fields {
		hs, err := r.callsThrough(ctx, d.path, f)
		if err != nil {
			return err
		}
		hosts = append(hosts, hs...)
	}
	ifc, contract, err := r.contract(ctx, d)
	if err != nil {
		return err
	}
	if contract {
		for _, f := range slices.Concat(fills, members) {
			r.add(fact{link{f.unit, ifc, model.Provides}, "callback", f.at})
		}
		for _, h := range hosts {
			r.add(fact{link{h.unit, ifc, model.Consumes}, "callback", h.at})
		}
		return nil
	}
	for _, h := range hosts {
		for _, m := range members {
			if h.unit != m.unit {
				r.add(fact{link{h.unit, m.unit, model.DependsOn}, "callback", h.at})
				r.add(fact{link{h.unit, m.unit, model.DependsOn}, "callback", m.at})
			}
		}
	}
	return nil
}

// members reads the declaration after a slot type's name at pos: when it
// has a brace initializer, the functions the initializer names, each with
// its unit and the line that names it.
func (r *cReader) members(ctx context.Context, p string, pos lsp.Position) ([]cPlace, bool, error) {
	ls := r.lines(p)
	rest := ls[pos.Line][min(pos.Character, len(ls[pos.Line])):]
	for l := pos.Line + 1; l < len(ls) && l <= pos.Line+2 && !strings.Contains(rest, "{") && !strings.Contains(rest, ";"); l++ {
		rest += "\n" + ls[l]
	}
	m := cDeclarator.FindStringIndex(rest)
	if m == nil {
		return nil, false, nil
	}
	// Walk the initializer from its brace to the matching one.
	line, col := pos.Line, pos.Character
	for i := 0; i < m[1]; i++ {
		if rest[i] == '\n' {
			line, col = line+1, 0
		} else {
			col++
		}
	}
	var out []cPlace
	seen := map[string]bool{}
	depth := 1
	for line < len(ls) && depth > 0 {
		s := ls[line]
		for col < len(s) && depth > 0 {
			c := s[col]
			switch {
			case c == '{':
				depth++
			case c == '}':
				depth--
			case c == '"' || c == '\'':
				col++
				for col < len(s) && s[col] != c {
					if s[col] == '\\' {
						col++
					}
					col++
				}
			case c == '/' && col+1 < len(s) && s[col+1] == '/':
				col = len(s)
				continue
			case identStart(c):
				end := col
				for end < len(s) && ident(s[end]) {
					end++
				}
				name := s[col:end]
				prev := strings.TrimRight(s[:col], " \t")
				if !strings.HasSuffix(prev, ".") && !seen[name] && name != "NULL" && name != "nullptr" {
					seen[name] = true
					defs, err := r.srv.Definitions(ctx, r.abs(p), lsp.Position{Line: line, Character: col})
					if err != nil {
						return nil, true, err
					}
					for _, d := range defs {
						if q, u, _, ok := r.place(d); ok && r.function(q, d.Range.Start) {
							out = append(out, cPlace{u, site{path: p, line: line + 1}})
						}
					}
				}
				col = end
				continue
			}
			col++
		}
		line, col = line+1, 0
	}
	return out, true, nil
}

// function is whether a function or method is declared at pos in p.
func (r *cReader) function(p string, pos lsp.Position) bool {
	r.mu.Lock()
	syms := r.syms[p]
	r.mu.Unlock()
	var find func(list []lsp.DocumentSymbol) bool
	find = func(list []lsp.DocumentSymbol) bool {
		for _, s := range list {
			if s.SelectionRange.Start == pos {
				return s.Kind == lsp.KindFunction || s.Kind == lsp.KindMethod
			}
			if find(s.Children) {
				return true
			}
		}
		return false
	}
	return find(syms)
}

// callsThrough are the places that call through the field or variable at
// pos, with their units.
func (r *cReader) callsThrough(ctx context.Context, p string, pos lsp.Position) ([]cPlace, error) {
	refs, err := r.srv.References(ctx, r.abs(p), pos)
	if err != nil {
		return nil, err
	}
	var out []cPlace
	for _, l := range refs {
		if q, u, line, ok := r.place(l); ok && r.after(q, l.Range.End) == '(' {
			out = append(out, cPlace{u, site{path: q, line: line}})
		}
	}
	return out, nil
}

// callsThroughVar finds the name an alias slot's declaration at pos
// declares, a variable, parameter or field, and the calls through it. A
// function that returns the alias declares no pointer.
func (r *cReader) callsThroughVar(ctx context.Context, p string, pos lsp.Position) ([]cPlace, error) {
	ls := r.lines(p)
	rest := ls[pos.Line][min(pos.Character, len(ls[pos.Line])):]
	m := cNamedVar.FindStringSubmatchIndex(rest)
	if m == nil || m[4] < m[5] && rest[m[4]] == '(' {
		return nil, nil
	}
	return r.callsThrough(ctx, p, lsp.Position{Line: pos.Line, Character: pos.Character + m[2]})
}

// gatedAt is whether clangd's parse of p leaves line out.
func (r *cReader) gatedAt(p string, line int) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.ContainsFunc(r.gated[p], func(g lsp.Range) bool { return g.Start.Line <= line && line <= g.End.Line })
}

// askAt is where to ask for d's references. clangd resolves the name at a
// position from the file's own parse, and a header's own parse leaves out
// a declaration under an #if that the sources including it set, such as
// zstd's FSE_STATIC_LINKING_ONLY section of fse.h, so it would find no
// symbol there. Such a declaration is asked at the first place another
// read file spells its name outside code its parse leaves out; the
// references come from the index either way.
func (r *cReader) askAt(d *cDecl) (string, lsp.Position) {
	if !r.gatedAt(d.path, d.at.Line) {
		return d.path, d.at
	}
	word := regexp.MustCompile(`\b` + regexp.QuoteMeta(d.name) + `\b`)
	for _, p := range r.order {
		if p == d.path {
			continue
		}
		for i, l := range r.lines(p) {
			if m := word.FindStringIndex(l); m != nil && !r.gatedAt(p, i) && !strings.HasPrefix(strings.TrimSpace(l), "#") {
				return p, lsp.Position{Line: i, Character: m[0]}
			}
		}
	}
	return d.path, d.at
}

// declared is whether a document symbol's name starts at pos: a
// declaration, such as the base method clangd lists among an override's
// references, and no use.
func (r *cReader) declared(p string, pos lsp.Position) bool {
	_, ok := r.symbolAt(p, pos)
	return ok
}
