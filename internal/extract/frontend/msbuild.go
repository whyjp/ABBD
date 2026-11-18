package frontend

import (
	"bytes"
	"cmp"
	"encoding/xml"
	"fmt"
	"io"
	"maps"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"unicode/utf16"

	"github.com/whyjp/abbd/internal/extract"
	"github.com/whyjp/abbd/internal/model"
)

// IsMSBuildPath is whether a path names a Visual Studio solution or C++
// project, which the msbuild front-end and the C relater read.
func IsMSBuildPath(p string) bool {
	switch strings.ToLower(path.Ext(filepath.ToSlash(p))) {
	case ".sln", ".slnx", ".vcxproj":
		return true
	}
	return false
}

// msbuildArtifacts maps a ConfigurationType to what the build makes. Utility
// and Makefile projects make nothing a host or a user runs here.
var msbuildArtifacts = map[string]model.Artifact{"application": "exe", "staticlibrary": "static", "dynamiclibrary": "shared"}

// toolchainPrefixes begin the names of properties that Visual Studio's own
// targets and the installed toolset define. They are not in any project
// file, so an unresolved one is no gap in the project and goes unreported;
// the C relater says which headers clangd then misses.
var toolchainPrefixes = []string{
	"vctargetspath", "userrootdir", "msbuild", "vcinstalldir", "vsinstalldir", "windowssdk", "vc_", "vctools", "vcidepath",
	"includepath", "externalincludepath", "librarypath", "executablepath", "sourcepath", "excludepath", "referencepath", "universalcrt", "netfxsdk",
	"frameworksdk", "devenvdir", "visualstudioversion", "wdk", "platformtoolset", "targetplatformversion",
}

func toolchainName(n string) bool {
	n = strings.ToLower(n)
	return slices.ContainsFunc(toolchainPrefixes, func(p string) bool { return strings.HasPrefix(n, p) })
}

// xnode is one element of an MSBuild file, names and attributes lower-cased
// since MSBuild reads them without regard to case.
type xnode struct {
	name string
	attr map[string]string
	text string
	kids []*xnode
}

func parseXML(b []byte) (*xnode, error) {
	switch {
	case bytes.HasPrefix(b, []byte{0xEF, 0xBB, 0xBF}):
		b = b[3:]
	case bytes.HasPrefix(b, []byte{0xFF, 0xFE}), bytes.HasPrefix(b, []byte{0xFE, 0xFF}):
		le := b[0] == 0xFF
		var u []uint16
		for i := 2; i+1 < len(b); i += 2 {
			if le {
				u = append(u, uint16(b[i])|uint16(b[i+1])<<8)
			} else {
				u = append(u, uint16(b[i])<<8|uint16(b[i+1]))
			}
		}
		b = []byte(string(utf16.Decode(u)))
	}
	d := xml.NewDecoder(bytes.NewReader(b))
	d.CharsetReader = func(_ string, in io.Reader) (io.Reader, error) { return in, nil }
	var root *xnode
	var stack []*xnode
	for {
		tok, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			n := &xnode{name: strings.ToLower(t.Name.Local), attr: map[string]string{}}
			for _, a := range t.Attr {
				n.attr[strings.ToLower(a.Name.Local)] = a.Value
			}
			if len(stack) > 0 {
				p := stack[len(stack)-1]
				p.kids = append(p.kids, n)
			} else if root == nil {
				root = n
			}
			stack = append(stack, n)
		case xml.EndElement:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		case xml.CharData:
			if len(stack) > 0 {
				stack[len(stack)-1].text += string(t)
			}
		}
	}
	if root == nil {
		return nil, fmt.Errorf("no XML element")
	}
	return root, nil
}

// msConfig is what one project builds under one Configuration|Platform.
type msConfig struct {
	ctype             string
	includes, defines []string
	libs              []string
	std, stdC         string
	compileAs         string
	forced            []string
	sources, headers  []string
	refs              []string
}

type msProject struct {
	file    string
	dir     string
	name    string
	target  string
	doc     *xnode
	configs []string
	conf    map[string]*msConfig
}

// msbuilder reads a solution or project and everything it names. Paths are
// kept in forward slashes, resolved without regard to case so the result
// is the same on every OS, and held under root; a path outside it is
// reported, never followed.
type msbuilder struct {
	root     string
	dirs     map[string][]string
	isDir    map[string]bool
	xmls     map[string]*xnode
	notes    map[string]bool
	projects map[string]*msProject
}

func newMSBuilder(root string) *msbuilder {
	return &msbuilder{root: strings.TrimSuffix(filepath.ToSlash(root), "/"), dirs: map[string][]string{}, isDir: map[string]bool{}, xmls: map[string]*xnode{}, notes: map[string]bool{}, projects: map[string]*msProject{}}
}

func (m *msbuilder) note(format string, a ...any) { m.notes[fmt.Sprintf(format, a...)] = true }

func (m *msbuilder) sortedNotes() []string {
	return slices.Sorted(maps.Keys(m.notes))
}

// rel is a path relative to the root, or the path itself when outside it.
func (m *msbuilder) rel(p string) string {
	if p == m.root {
		return "."
	}
	if r, ok := strings.CutPrefix(p, m.root+"/"); ok {
		return r
	}
	return p
}

func (m *msbuilder) inside(p string) bool { return p == m.root || strings.HasPrefix(p, m.root+"/") }

// join resolves a path a project file writes against a directory, reading
// both slash spellings. A drive letter or a rooted path is absolute.
func join(base, s string) string {
	s = strings.ReplaceAll(strings.TrimSpace(s), `\`, "/")
	if len(s) >= 2 && s[1] == ':' || strings.HasPrefix(s, "/") {
		return path.Clean(s)
	}
	return path.Clean(base + "/" + s)
}

func (m *msbuilder) list(dir string) []string {
	if l, ok := m.dirs[dir]; ok {
		return l
	}
	es, _ := os.ReadDir(filepath.FromSlash(dir))
	var l []string
	for _, e := range es {
		l = append(l, e.Name())
		m.isDir[dir+"/"+e.Name()] = e.IsDir()
	}
	slices.Sort(l)
	m.dirs[dir] = l
	return l
}

// resolve finds a path under the root, matching each name without regard to
// case, as Windows does, and says whether it exists. A path outside the root
// exists only if it is there as spelled.
func (m *msbuilder) resolve(p string) (string, bool) {
	p = path.Clean(p)
	if !m.inside(p) {
		_, err := os.Stat(filepath.FromSlash(p))
		return p, err == nil
	}
	cur := m.root
	for _, seg := range strings.Split(strings.TrimPrefix(strings.TrimPrefix(p, m.root), "/"), "/") {
		if seg == "" {
			continue
		}
		names := m.list(cur)
		next := ""
		if slices.Contains(names, seg) {
			next = seg
		} else if i := slices.IndexFunc(names, func(n string) bool { return strings.EqualFold(n, seg) }); i >= 0 {
			next = names[i]
		}
		if next == "" {
			return p, false
		}
		cur += "/" + next
	}
	return cur, true
}

func (m *msbuilder) xml(p string) (*xnode, error) {
	if n, ok := m.xmls[p]; ok {
		return n, nil
	}
	b, err := os.ReadFile(filepath.FromSlash(p))
	if err != nil {
		return nil, err
	}
	n, err := parseXML(b)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", m.rel(p), err)
	}
	m.xmls[p] = n
	return n, nil
}

var slnProject = regexp.MustCompile(`^Project\("\{[0-9A-Fa-f-]+\}"\)\s*=\s*"[^"]*"\s*,\s*"([^"]*)"`)

// projectPaths lists the C++ projects a solution names, and notes the
// others.
func (m *msbuilder) projectPaths(sln string) ([]string, error) {
	var raw []string
	if strings.EqualFold(path.Ext(sln), ".slnx") {
		n, err := m.xml(sln)
		if err != nil {
			return nil, err
		}
		var walk func(n *xnode)
		walk = func(n *xnode) {
			if p := n.attr["path"]; n.name == "project" && p != "" {
				raw = append(raw, p)
			}
			for _, k := range n.kids {
				walk(k)
			}
		}
		walk(n)
	} else {
		b, err := os.ReadFile(filepath.FromSlash(sln))
		if err != nil {
			return nil, err
		}
		for _, l := range strings.Split(string(b), "\n") {
			if g := slnProject.FindStringSubmatch(strings.TrimSpace(strings.TrimPrefix(l, "FEFF"))); g != nil {
				raw = append(raw, g[1])
			}
		}
	}
	var out []string
	for _, r := range raw {
		p, ok := m.resolve(join(path.Dir(sln), r))
		switch {
		case !strings.EqualFold(path.Ext(r), ".vcxproj") && strings.Contains(path.Base(strings.ReplaceAll(r, `\`, "/")), "."):
			m.note("msbuild: %s names %s, which is no C++ project and is not read", m.rel(sln), strings.ReplaceAll(r, `\`, "/"))
		case !strings.EqualFold(path.Ext(r), ".vcxproj"):
			// A solution folder, which has no file.
		case !ok:
			m.note("msbuild: %s names %s, which does not exist here", m.rel(sln), strings.ReplaceAll(r, `\`, "/"))
		default:
			out = append(out, p)
		}
	}
	slices.Sort(out)
	return slices.Compact(out), nil
}

// load reads a project and, for every configuration it declares, evaluates
// it. A project that cannot be read is noted and left out.
func (m *msbuilder) load(file, solutionDir string) *msProject {
	if p, ok := m.projects[file]; ok {
		return p
	}
	doc, err := m.xml(file)
	if err != nil {
		m.note("msbuild: %s is unreadable and left out: %v", m.rel(file), err)
		m.projects[file] = nil
		return nil
	}
	p := &msProject{file: file, dir: path.Dir(file), name: strings.TrimSuffix(path.Base(file), path.Ext(file)), doc: doc, conf: map[string]*msConfig{}}
	m.projects[file] = p
	var find func(n *xnode)
	find = func(n *xnode) {
		if n.name == "projectconfiguration" && n.attr["include"] != "" {
			p.configs = append(p.configs, n.attr["include"])
		}
		for _, k := range n.kids {
			find(k)
		}
	}
	find(doc)
	slices.Sort(p.configs)
	p.configs = slices.Compact(p.configs)
	if len(p.configs) == 0 {
		m.note("msbuild: %s declares no ProjectConfiguration, so it is read once with no Configuration or Platform", m.rel(file))
		p.configs = []string{"|"}
	}
	for i, c := range p.configs {
		e := &evalCtx{m: m, p: p, props: map[string]string{}, defs: map[string]map[string]string{}, out: &msConfig{}}
		conf, plat, _ := strings.Cut(c, "|")
		for k, v := range map[string]string{
			"configuration": conf, "platform": plat, "projectname": p.name, "projectdir": p.dir + "/", "projectpath": file,
			"projectfilename": path.Base(file), "projectext": path.Ext(file), "msbuildprojectdirectory": p.dir,
			"msbuildprojectname": p.name, "msbuildprojectfile": path.Base(file), "msbuildthisfiledirectory": p.dir + "/",
		} {
			e.props[k] = v
		}
		if solutionDir != "" {
			e.props["solutiondir"] = solutionDir + "/"
		}
		for e.pass = 1; e.pass <= 2; e.pass++ {
			e.walk(doc, file, 0)
		}
		out := e.out
		out.ctype = strings.ToLower(strings.TrimSpace(e.props["configurationtype"]))
		cl := e.defs["clcompile"]
		out.includes = e.dirs(cl["additionalincludedirectories"])
		// IncludePath and ExternalIncludePath are what cl searches after the
		// directories above; what Visual Studio adds to them stays unresolved.
		for _, k := range []string{"externalincludepath", "includepath"} {
			out.includes = append(out.includes, e.dirs(e.props[k])...)
		}
		out.includes = dedupe(out.includes)
		out.defines = e.list(cl["preprocessordefinitions"])
		out.libs = slices.Concat(e.list(e.defs["link"]["additionaldependencies"]), e.list(e.defs["lib"]["additionaldependencies"]))
		out.std, out.stdC = strings.ToLower(strings.TrimSpace(cl["languagestandard"])), strings.ToLower(strings.TrimSpace(cl["languagestandard_c"]))
		out.compileAs = strings.ToLower(strings.TrimSpace(cl["compileas"]))
		for _, f := range e.list(cl["forcedincludefiles"]) {
			if r, ok := m.resolve(join(p.dir, f)); ok {
				out.forced = append(out.forced, r)
			}
		}
		slices.Sort(out.sources)
		out.sources = slices.Compact(out.sources)
		slices.Sort(out.headers)
		out.headers = slices.Compact(out.headers)
		slices.Sort(out.refs)
		out.refs = slices.Compact(out.refs)
		p.conf[c] = out
		if i == 0 {
			p.target = cmp.Or(strings.TrimSpace(e.props["targetname"]), p.name)
			if strings.Contains(p.target, "$(") {
				p.target = p.name
			}
		}
	}
	return p
}

// evalCtx evaluates one project under one configuration. Pass 1 reads
// properties and pass 2 the item definitions and items, as MSBuild orders
// them, each following every import in place.
type evalCtx struct {
	m     *msbuilder
	p     *msProject
	props map[string]string
	defs  map[string]map[string]string
	out   *msConfig
	pass  int
}

func (e *evalCtx) unresolved(names []string, where string) {
	for _, n := range names {
		if !toolchainName(n) {
			e.m.note("msbuild: %s: $(%s) is unresolved in %s and what it names is left out", e.m.rel(e.p.file), n, where)
		}
	}
}

// expand replaces each $(Name) the project defines. A name it does not
// define stays as written and is returned.
func (e *evalCtx) expand(s string) (string, []string) {
	var b strings.Builder
	var miss []string
	for i := 0; i < len(s); {
		if s[i] != '$' || i+1 >= len(s) || s[i+1] != '(' {
			b.WriteByte(s[i])
			i++
			continue
		}
		depth, j := 1, i+2
		for j < len(s) && depth > 0 {
			switch s[j] {
			case '(':
				depth++
			case ')':
				depth--
			}
			j++
		}
		if depth != 0 {
			b.WriteString(s[i:])
			break
		}
		name := s[i+2 : j-1]
		if v, ok := e.props[strings.ToLower(name)]; ok && !strings.ContainsAny(name, "[]:") {
			b.WriteString(v)
		} else {
			b.WriteString(s[i:j])
			miss = append(miss, name)
		}
		i = j
	}
	return b.String(), miss
}

// test evaluates a Condition. One it cannot decide is false and reported.
func (e *evalCtx) test(c string, where string) bool {
	s, miss := e.expand(c)
	if len(miss) > 0 {
		e.unresolved(miss, "a Condition in "+where)
		return false
	}
	v, err := e.condition(s)
	if err != nil {
		e.m.note("msbuild: %s: the Condition %q is not one this front-end reads (%v), so what it guards is left out", e.m.rel(e.p.file), strings.TrimSpace(c), err)
		return false
	}
	return v
}

var condToken = regexp.MustCompile(`^\s*('[^']*'|==|!=|<=|>=|<|>|\(|\)|!|,|[A-Za-z0-9_.\-]+)`)

// condition evaluates an expanded Condition: == and != over quoted strings,
// numbers or words, And, Or, !, parentheses, Exists and HasTrailingSlash.
func (e *evalCtx) condition(s string) (bool, error) {
	var toks []string
	for strings.TrimSpace(s) != "" {
		g := condToken.FindStringSubmatch(s)
		if g == nil {
			return false, fmt.Errorf("cannot read %q", strings.TrimSpace(s))
		}
		toks = append(toks, g[1])
		s = s[len(g[0]):]
	}
	pos := 0
	peek := func() string {
		if pos < len(toks) {
			return strings.ToLower(toks[pos])
		}
		return ""
	}
	str := func(t string) string { return strings.Trim(t, "'") }
	var or func() (bool, error)
	operand := func() (string, error) {
		if pos >= len(toks) {
			return "", fmt.Errorf("an operand is missing")
		}
		t := toks[pos]
		pos++
		return str(t), nil
	}
	primary := func() (bool, error) {
		switch peek() {
		case "(":
			pos++
			v, err := or()
			if err != nil {
				return false, err
			}
			if peek() != ")" {
				return false, fmt.Errorf("a ) is missing")
			}
			pos++
			return v, nil
		case "!":
			pos++
			v, err := or()
			return !v, err
		case "exists", "hastrailingslash":
			fn := peek()
			pos++
			if peek() != "(" {
				return false, fmt.Errorf("%s needs (", fn)
			}
			pos++
			arg, err := operand()
			if err != nil {
				return false, err
			}
			if peek() != ")" {
				return false, fmt.Errorf("a ) is missing")
			}
			pos++
			if fn == "hastrailingslash" {
				return strings.HasSuffix(arg, `\`) || strings.HasSuffix(arg, "/"), nil
			}
			_, ok := e.m.resolve(join(e.p.dir, arg))
			return ok, nil
		}
		l, err := operand()
		if err != nil {
			return false, err
		}
		op := peek()
		if !slices.Contains([]string{"==", "!=", "<", ">", "<=", ">="}, op) {
			switch strings.ToLower(l) {
			case "true", "on", "yes", "1":
				return true, nil
			case "false", "off", "no", "0", "":
				return false, nil
			}
			return false, fmt.Errorf("%q is no boolean", l)
		}
		pos++
		r, err := operand()
		if err != nil {
			return false, err
		}
		c := strings.Compare(strings.ToLower(l), strings.ToLower(r))
		switch op {
		case "==":
			return c == 0, nil
		case "!=":
			return c != 0, nil
		case "<":
			return c < 0, nil
		case ">":
			return c > 0, nil
		case "<=":
			return c <= 0, nil
		}
		return c >= 0, nil
	}
	and := func() (bool, error) {
		v, err := primary()
		for err == nil && peek() == "and" {
			pos++
			var w bool
			w, err = primary()
			v = v && w
		}
		return v, err
	}
	or = func() (bool, error) {
		v, err := and()
		for err == nil && peek() == "or" {
			pos++
			var w bool
			w, err = and()
			v = v || w
		}
		return v, err
	}
	v, err := or()
	if err == nil && pos != len(toks) {
		err = fmt.Errorf("%q is unexpected", toks[pos])
	}
	return v, err
}

// walk follows a project or property sheet in order.
func (e *evalCtx) walk(n *xnode, file string, depth int) {
	here := e.props["msbuildthisfiledirectory"]
	e.props["msbuildthisfiledirectory"] = path.Dir(file) + "/"
	defer func() { e.props["msbuildthisfiledirectory"] = here }()
	for _, c := range n.kids {
		if cond, ok := c.attr["condition"]; ok && !e.test(cond, "this file") {
			continue
		}
		switch c.name {
		case "propertygroup":
			if e.pass != 1 {
				continue
			}
			for _, pr := range c.kids {
				if cond, ok := pr.attr["condition"]; ok && !e.test(cond, "this file") {
					continue
				}
				v, miss := e.expand(strings.TrimSpace(pr.text))
				e.unresolved(miss, "the property "+pr.name)
				e.props[pr.name] = v
			}
		case "import":
			e.imp(c, file, depth)
		case "importgroup":
			for _, i := range c.kids {
				if cond, ok := i.attr["condition"]; ok && !e.test(cond, "this file") {
					continue
				}
				if i.name == "import" {
					e.imp(i, file, depth)
				}
			}
		case "itemdefinitiongroup":
			if e.pass == 2 {
				e.definitions(c)
			}
		case "itemgroup":
			if e.pass == 2 {
				e.items(c)
			}
		case "target":
			if e.pass == 2 && adds(c) {
				e.m.note("msbuild: %s: the Target %s in %s adds sources or headers when it runs, which is not evaluated, so they are left out", e.m.rel(e.p.file), slash(c.attr["name"]), e.m.rel(file))
			}
		}
	}
}

func (e *evalCtx) imp(c *xnode, file string, depth int) {
	target, miss := e.expand(c.attr["project"])
	if len(miss) > 0 {
		e.unresolved(miss, "an Import")
		return
	}
	if strings.ContainsAny(target, "*?") || depth >= 12 {
		e.m.note("msbuild: %s: the Import %s is not followed", e.m.rel(e.p.file), slash(c.attr["project"]))
		return
	}
	p, ok := e.m.resolve(join(path.Dir(file), target))
	if !ok {
		e.m.note("msbuild: %s: the Import %s does not exist here and is left out", e.m.rel(e.p.file), strings.ReplaceAll(target, `\`, "/"))
		return
	}
	n, err := e.m.xml(p)
	if err != nil {
		e.m.note("msbuild: %s: the Import %s is unreadable (%v) and left out", e.m.rel(e.p.file), e.m.rel(p), err)
		return
	}
	e.walk(n, p, depth+1)
}

// definitions reads an ItemDefinitionGroup, each metadata name appended to
// what an earlier definition gave by %(Name).
func (e *evalCtx) definitions(g *xnode) {
	for _, tool := range g.kids {
		if cond, ok := tool.attr["condition"]; ok && !e.test(cond, "this file") {
			continue
		}
		if e.defs[tool.name] == nil {
			e.defs[tool.name] = map[string]string{}
		}
		for _, meta := range tool.kids {
			if cond, ok := meta.attr["condition"]; ok && !e.test(cond, "this file") {
				continue
			}
			v, miss := e.expand(strings.TrimSpace(meta.text))
			e.unresolved(miss, "the "+tool.name+" setting "+meta.name)
			e.defs[tool.name][meta.name] = inherit(meta.name).ReplaceAllLiteralString(v, e.defs[tool.name][meta.name])
		}
	}
}

// list reads a ;-separated value, leaving out what stays unresolved, which
// the expansion already reported.
func (e *evalCtx) list(v string) []string {
	var out []string
	for _, x := range strings.Split(v, ";") {
		x = strings.TrimSpace(x)
		if x != "" && !strings.HasPrefix(x, "%(") && !strings.Contains(x, "$(") {
			out = append(out, x)
		}
	}
	return out
}

var envVar = regexp.MustCompile(`%[A-Za-z_][A-Za-z0-9_]*%`)

func slash(s string) string { return `"` + strings.ReplaceAll(s, `\`, "/") + `"` }

// dirs reads include directories: each must exist inside the repository.
// One that names an environment variable depends on the machine and is left
// out.
func (e *evalCtx) dirs(v string) []string {
	var out []string
	for _, d := range e.list(v) {
		if envVar.MatchString(d) {
			e.m.note("msbuild: %s: the include directory %s names the environment variable %s, which depends on the machine, and is left out", e.m.rel(e.p.file), slash(d), envVar.FindString(d))
			continue
		}
		p, ok := e.m.resolve(join(e.p.dir, d))
		switch {
		case !e.m.inside(p):
			e.m.note("msbuild: %s: the include directory %s is outside the repository", e.m.rel(e.p.file), slash(d))
		case !ok:
			e.m.note("msbuild: %s: the include directory %s does not exist here", e.m.rel(e.p.file), slash(d))
		default:
			out = append(out, p)
		}
	}
	return out
}

func (e *evalCtx) items(g *xnode) {
	for _, it := range g.kids {
		if cond, ok := it.attr["condition"]; ok && !e.test(cond, "this file") {
			continue
		}
		if it.name != "clcompile" && it.name != "clinclude" && it.name != "projectreference" {
			continue
		}
		inc, miss := e.expand(it.attr["include"])
		e.unresolved(miss, "an "+it.name+" item")
		excluded := false
		for _, meta := range it.kids {
			if meta.name == "excludedfrombuild" {
				if cond, ok := meta.attr["condition"]; ok && !e.test(cond, "this file") {
					continue
				}
				v, _ := e.expand(strings.TrimSpace(meta.text))
				excluded = strings.EqualFold(v, "true")
			}
		}
		for _, f := range e.list(inc) {
			var found []string
			if strings.ContainsAny(f, "*?") {
				found = e.m.glob(join(e.p.dir, f))
			} else if p, ok := e.m.resolve(join(e.p.dir, f)); ok {
				found = []string{p}
			}
			if len(found) == 0 {
				e.m.note("msbuild: %s: the %s %s matches no file here", e.m.rel(e.p.file), it.name, strings.ReplaceAll(f, `\`, "/"))
			}
			for _, p := range found {
				switch {
				case it.name == "projectreference":
					e.out.refs = append(e.out.refs, p)
				case it.name == "clinclude":
					e.out.headers = append(e.out.headers, p)
				case !excluded:
					e.out.sources = append(e.out.sources, p)
				}
			}
		}
	}
}

// glob expands the * and ? of an Include, which stay inside one name, and a
// ** segment that spans directories, under the root.
func (m *msbuilder) glob(p string) []string {
	if !m.inside(p) {
		return nil
	}
	segs := strings.Split(strings.TrimPrefix(p, m.root+"/"), "/")
	var out []string
	var walk func(dir string, segs []string)
	walk = func(dir string, segs []string) {
		if len(segs) == 0 {
			return
		}
		seg, rest := segs[0], segs[1:]
		switch {
		case seg == "**":
			walk(dir, rest)
			for _, n := range m.list(dir) {
				if m.isDir[dir+"/"+n] {
					walk(dir+"/"+n, segs)
				}
			}
		case strings.ContainsAny(seg, "*?"):
			for _, n := range m.list(dir) {
				if ok, _ := path.Match(strings.ToLower(seg), strings.ToLower(n)); !ok {
					continue
				}
				switch {
				case len(rest) == 0 && !m.isDir[dir+"/"+n]:
					out = append(out, dir+"/"+n)
				case len(rest) > 0 && m.isDir[dir+"/"+n]:
					walk(dir+"/"+n, rest)
				}
			}
		default:
			if r, ok := m.resolve(dir + "/" + seg); ok {
				if len(rest) == 0 {
					if !m.isDir[r] {
						out = append(out, r)
					}
				} else {
					walk(r, rest)
				}
			}
		}
	}
	walk(m.root, segs)
	slices.Sort(out)
	return slices.Compact(out)
}

// readTargets loads the projects the spec names: a solution's C++
// projects, or a project and the projects it references at any depth.
func (m *msbuilder) readTargets(file string) ([]*msProject, string, error) {
	file, ok := m.resolve(file)
	if !ok {
		return nil, "", fmt.Errorf("%s does not exist", m.rel(file))
	}
	if !IsMSBuildPath(file) {
		return nil, "", fmt.Errorf("%s is no .sln, .slnx or .vcxproj file", m.rel(file))
	}
	var out []*msProject
	if !strings.EqualFold(path.Ext(file), ".vcxproj") {
		ps, err := m.projectPaths(file)
		if err != nil {
			return nil, "", err
		}
		for _, p := range ps {
			if l := m.load(p, path.Dir(file)); l != nil {
				out = append(out, l)
			}
		}
		return out, path.Dir(file), nil
	}
	queue := []string{file}
	for len(queue) > 0 {
		f := queue[0]
		queue = queue[1:]
		if _, seen := m.projects[f]; seen {
			continue
		}
		p := m.load(f, "")
		if p == nil {
			if f == file {
				return nil, "", fmt.Errorf("%s cannot be read", m.rel(f))
			}
			continue
		}
		out = append(out, p)
		for _, c := range p.conf {
			for _, r := range c.refs {
				if strings.EqualFold(path.Ext(r), ".vcxproj") {
					queue = append(queue, r)
				}
			}
		}
	}
	slices.SortFunc(out, func(a, b *msProject) int { return cmp.Compare(a.file, b.file) })
	return out, path.Dir(file), nil
}

// msModule is a project the front-end keeps.
type msModule struct {
	*msProject
	ref       string
	artifacts []model.Artifact
}

// msNotBuilt are the words of a project's directory below the solution, or
// of its name, that say it is a test, a bench or someone else's code. A
// solution that lists examples lists them to build them, so they stay.
var (
	msNotBuiltDirs  = map[string]bool{"test": true, "tests": true, "bench": true, "benches": true, "benchmark": true, "benchmarks": true, "fuzz": true, "third_party": true, "thirdparty": true, "3rdparty": true, "vendor": true, "external": true}
	msNotBuiltWords = map[string]bool{"test": true, "tests": true, "unittest": true, "unittests": true, "bench": true, "benches": true, "benchmark": true, "benchmarks": true, "fuzz": true, "fuzzer": true}
)

func msNotBuilt(rel, name string) bool {
	for _, s := range strings.Split(strings.ToLower(rel), "/") {
		if msNotBuiltDirs[s] {
			return true
		}
	}
	return slices.ContainsFunc(strings.FieldsFunc(strings.ToLower(name), func(r rune) bool { return r == '_' || r == '-' || r == '.' || r == ' ' }), func(w string) bool { return msNotBuiltWords[w] })
}

// modules keeps the projects that build something a host or a user runs
// from sources of their own, and gives each a unique ref. The project a
// spec names directly is always kept.
func (m *msbuilder) modules(ps []*msProject, base, named string) []*msModule {
	var out []*msModule
	var skipped []string
	for _, p := range ps {
		var arts []model.Artifact
		sources := false
		for _, c := range p.configs {
			if a, ok := msbuildArtifacts[p.conf[c].ctype]; ok {
				arts = append(arts, a)
				sources = sources || len(p.conf[c].sources) > 0
			}
		}
		rel := strings.TrimPrefix(strings.TrimPrefix(p.dir, base), "/")
		switch {
		case len(arts) == 0 || !sources:
			skipped = append(skipped, p.name+" (builds nothing from sources)")
			continue
		case p.file != named && msNotBuilt(rel, p.name):
			skipped = append(skipped, p.name+" (test, bench or vendored)")
			continue
		}
		slices.Sort(arts)
		out = append(out, &msModule{msProject: p, artifacts: slices.Compact(arts)})
	}
	if len(skipped) > 0 {
		slices.Sort(skipped)
		m.note("msbuild: left out: %s", strings.Join(skipped, ", "))
	}
	names := map[string]int{}
	for _, mod := range out {
		names[strings.ToLower(mod.name)]++
	}
	for _, mod := range out {
		mod.ref = "msbuild:" + mod.name
		if names[strings.ToLower(mod.name)] > 1 {
			mod.ref = "msbuild:" + m.rel(mod.dir) + "/" + mod.name
		}
	}
	return out
}

// msbuild reads a solution or project with no MSBuild: each C++ project is a
// module that lists what it builds, and a ProjectReference or a .lib
// that names a sibling project's output is a link, under the configurations
// of the project that hold it.
func msbuild(t tree) (extract.Doc, error) {
	m := newMSBuilder(t.root)
	ps, base, err := m.readTargets(filepath.ToSlash(t.dir))
	if err != nil {
		return extract.Doc{}, err
	}
	mods := m.modules(ps, base, namedProject(m, t.dir))
	var doc extract.Doc
	byFile := map[string]*msModule{}
	users, dirUsers := m.users(mods)
	m.noteShared(users)
	for _, mod := range mods {
		byFile[mod.file] = mod
		anchors := m.anchors(mod, users, dirUsers)
		doc.Units = append(doc.Units, extract.DocUnit{Ref: mod.ref, Kind: model.Module, Name: strings.TrimPrefix(mod.ref, "msbuild:"), Anchors: model.SortAnchors(anchors), Artifacts: mod.artifacts})
	}
	libs := map[string][]*msModule{}
	for _, mod := range mods {
		if a := msbuildArtifacts[mod.conf[mod.configs[0]].ctype]; a == "static" || a == "shared" {
			k := strings.ToLower(mod.target)
			libs[k] = append(libs[k], mod)
		}
	}
	for _, mod := range mods {
		held := map[*msModule]map[string]bool{}
		for _, c := range mod.configs {
			add := func(to *msModule) {
				if to != mod {
					if held[to] == nil {
						held[to] = map[string]bool{}
					}
					held[to][c] = true
				}
			}
			for _, r := range mod.conf[c].refs {
				if to, ok := byFile[r]; ok {
					add(to)
				}
			}
			for _, l := range mod.conf[c].libs {
				l = strings.ReplaceAll(l, `\`, "/")
				if b := path.Base(strings.Trim(l, `"`)); strings.EqualFold(path.Ext(b), ".lib") && !strings.Contains(b, "$(") {
					switch to := libs[strings.ToLower(strings.TrimSuffix(b, b[len(b)-4:]))]; len(to) {
					case 0:
					case 1:
						add(to[0])
					default:
						m.note("msbuild: %s: %s names several projects' output (%s) and links none of them", m.rel(mod.file), b, strings.Join(refsOf(to), ", "))
					}
				}
			}
		}
		for to, cs := range held {
			doc.Relations = append(doc.Relations, extract.DocRelation{From: mod.ref, To: to.ref, Kind: model.DependsOn, Via: "link", When: m.when(mod, cs)})
		}
	}
	doc.Notes = m.sortedNotes()
	return doc, nil
}

func refsOf(ms []*msModule) []string {
	var out []string
	for _, x := range ms {
		out = append(out, x.ref)
	}
	slices.Sort(out)
	return out
}

// when names the configurations an edge holds under, none when it holds
// under all of the project's. A list over the bound of a condition keeps
// its first entries and says how many more.
func (m *msbuilder) when(mod *msModule, held map[string]bool) string {
	if len(held) == len(mod.configs) {
		return ""
	}
	cs := slices.Sorted(maps.Keys(held))
	for n := len(cs); ; n-- {
		w := strings.Join(cs[:n], " | ")
		if n < len(cs) {
			w += fmt.Sprintf(" | +%d", len(cs)-n)
		}
		if model.ValidateWhen(w) == nil || n == 1 {
			return w
		}
	}
}

// MSBuildCompiles reads a solution or project as the msbuild front-end does
// and returns how each C or C++ source of a kept project is compiled, as
// cl.exe would, under one configuration of each project: Debug|x64 when it
// has it, else x64, else Debug, else the first by name. A source two
// projects compile takes the command of the project whose ref sorts first.
// The second result are the notes the front-end reports.
func MSBuildCompiles(root, file string) ([]Compile, []string, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, nil, err
	}
	m := newMSBuilder(abs)
	ps, base, err := m.readTargets(filepath.ToSlash(file))
	if err != nil {
		return nil, nil, err
	}
	mods := m.modules(ps, base, namedProject(m, file))
	slices.SortFunc(mods, func(a, b *msModule) int { return cmp.Compare(a.ref, b.ref) })
	seen := map[string]bool{}
	used := map[string]int{}
	var out []Compile
	for _, mod := range mods {
		conf := mod.configs[0]
		for _, c := range mod.configs {
			if confRank(c) < confRank(conf) {
				conf = c
			}
		}
		used[conf]++
		cc := mod.conf[conf]
		flags := []string{"/nologo"}
		for _, d := range cc.includes {
			flags = append(flags, "-I"+filepath.FromSlash(d))
		}
		for _, d := range cc.defines {
			flags = append(flags, "-D"+d)
		}
		for _, f := range cc.forced {
			flags = append(flags, "/FI"+filepath.FromSlash(f))
		}
		switch cc.compileAs {
		case "compileasc":
			flags = append(flags, "/TC")
		case "compileascpp":
			flags = append(flags, "/TP")
		}
		for _, s := range cc.sources {
			if seen[s] || !cExtOf(s) {
				continue
			}
			seen[s] = true
			args := append([]string{"cl.exe"}, flags...)
			std := map[string]string{"stdcpp14": "/std:c++14", "stdcpp17": "/std:c++17", "stdcpp20": "/std:c++20", "stdcpplatest": "/std:c++latest"}[cc.std]
			if strings.EqualFold(path.Ext(s), ".c") {
				std = map[string]string{"stdc11": "/std:c11", "stdc17": "/std:c17"}[cc.stdC]
			}
			if std != "" {
				args = append(args, std)
			}
			out = append(out, Compile{File: filepath.FromSlash(s), Dir: filepath.FromSlash(mod.dir), Args: append(args, "/c", filepath.FromSlash(s))})
		}
	}
	slices.SortFunc(out, func(a, b Compile) int { return cmp.Compare(a.File, b.File) })
	notes := m.sortedNotes()
	var by []string
	for _, c := range slices.Sorted(maps.Keys(used)) {
		by = append(by, fmt.Sprintf("%s for %d", c, used[c]))
	}
	return out, append(notes, "msbuild: compile commands are cl.exe's, read under one configuration of each project: "+strings.Join(by, ", ")), nil
}

// confRank orders configurations, lowest first: Debug|x64, then x64, then Debug.
func confRank(c string) string {
	conf, plat, _ := strings.Cut(strings.ToLower(c), "|")
	a, b := "1", "1"
	if plat == "x64" {
		a = "0"
	}
	if conf == "debug" {
		b = "0"
	}
	return a + b + c
}

func cExtOf(p string) bool {
	switch strings.ToLower(path.Ext(p)) {
	case ".c", ".cc", ".cpp", ".cxx", ".c++":
		return true
	}
	return false
}

// namedProject is the project file a spec names directly, empty for a
// solution.
func namedProject(m *msbuilder, file string) string {
	if !strings.EqualFold(path.Ext(file), ".vcxproj") {
		return ""
	}
	p, _ := m.resolve(filepath.ToSlash(file))
	return p
}

// inherit matches the %(Name) that puts what an earlier definition gave
// into a metadata value.
func inherit(name string) *regexp.Regexp {
	return regexp.MustCompile(`(?i)%\(` + regexp.QuoteMeta(name) + `\)`)
}

// adds is whether a Target puts sources or headers into the build.
func adds(target *xnode) bool {
	for _, g := range target.kids {
		if g.name != "itemgroup" {
			continue
		}
		for _, it := range g.kids {
			if it.name == "clcompile" || it.name == "clinclude" {
				return true
			}
		}
	}
	return false
}

func dedupe(l []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, x := range l {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}

// files lists the files of a module's configurations inside the root.
func (m *msbuilder) files(mod *msModule) []string {
	var out []string
	for _, c := range mod.configs {
		for _, f := range slices.Concat(mod.conf[c].sources, mod.conf[c].headers) {
			if m.inside(f) {
				out = append(out, f)
			}
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// users maps each file, and each directory, to the modules that list a file
// in it.
func (m *msbuilder) users(mods []*msModule) (files, dirs map[string]map[string]bool) {
	files, dirs = map[string]map[string]bool{}, map[string]map[string]bool{}
	for _, mod := range mods {
		for _, f := range m.files(mod) {
			for k, set := range map[string]map[string]map[string]bool{f: files, path.Dir(f): dirs} {
				if set[k] == nil {
					set[k] = map[string]bool{}
				}
				set[k][mod.ref] = true
			}
		}
	}
	return files, dirs
}

// anchors are where a module's code lies: a directory only it lists files in,
// else each of its own files there. A file that several projects compile is
// compiled into each of them, so it is no one module's, and anchors none.
func (m *msbuilder) anchors(mod *msModule, files, dirs map[string]map[string]bool) []model.Anchor {
	var out []model.Anchor
	for _, f := range m.files(mod) {
		switch d := path.Dir(f); {
		case len(files[f]) > 1:
		case len(dirs[d]) == 1 && d != m.root:
			out = append(out, model.Anchor{Path: m.rel(d)})
		default:
			out = append(out, model.Anchor{Path: m.rel(f)})
		}
	}
	if m.inside(mod.file) {
		out = append(out, model.Anchor{Path: m.rel(mod.file)})
	}
	return model.SortAnchors(out)
}

// noteShared reports the files several projects compile, which belong to no
// module, with the busiest first.
func (m *msbuilder) noteShared(files map[string]map[string]bool) {
	var shared []string
	for f, set := range files {
		if len(set) > 1 {
			shared = append(shared, f)
		}
	}
	if len(shared) == 0 {
		return
	}
	slices.SortFunc(shared, func(a, b string) int {
		return cmp.Or(cmp.Compare(len(files[b]), len(files[a])), cmp.Compare(a, b))
	})
	var eg []string
	for _, f := range shared[:min(3, len(shared))] {
		eg = append(eg, fmt.Sprintf("%s (%d projects)", m.rel(f), len(files[f])))
	}
	m.note("msbuild: files listed by several projects, which each compile their own copy, anchor no module, so no relation to them is drawn (%d, such as %s)", len(shared), strings.Join(eg, ", "))
}
