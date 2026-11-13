// Package relate holds the relaters abbd ships, one per language. A relater
// runs after the extraction that names the units: it reads the code with
// the language's own compiler front-end and adds the code-level relations
// between those units, with the call, import or implementation that each
// one rests on.
package relate

import (
	"cmp"
	"fmt"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/whyjp/abbd/internal/extract"
	"github.com/whyjp/abbd/internal/extract/frontend"
	"github.com/whyjp/abbd/internal/model"
)

// relaters is the one list of bundled relaters. manifest is the file the
// relater's directory must hold, and unread says why the code a relater
// reports unread went unread.
var relaters = map[string]struct {
	manifest, unread string
	run              func(t tree, o owners) (found, error)
}{
	"go":   {"go.mod", "left out by build constraints (read as " + strings.Join(goEnv, ", ") + ")", golang},
	"rust": {"Cargo.toml", "reached by no crate's module tree under " + rustReads + ", so cfg-inactive or never declared", rust},
}

// Names lists the bundled relaters.
func Names() []string {
	var out []string
	for n := range relaters {
		out = append(out, n)
	}
	slices.Sort(out)
	return out
}

// ParseSpec reads a relater named on the command line as RELATER[=DIR], with
// DIR relative to the repository root.
func ParseSpec(s string) (frontend.Spec, error) { return frontend.SpecOf("relater", s, Names()) }

// maxQuotes bounds the evidence a relation carries: enough to show the
// coupling, few enough that the blueprint stays readable.
const maxQuotes = 3

// vias ranks what a code-level depends_on rests on, best first: a call is
// what runs, an implementation is a plug point, and an import that is never
// called only shares a type or a constant.
var vias = []model.Via{"call", "trait", "import"}

// link is one relation between two units, by ref.
type link struct {
	from, to string
	kind     model.RelationKind
}

// fact is one place in the code a relation rests on.
type fact struct {
	link
	via model.Via
	at  site
}

// site is a line of a root-relative, forward-slash path. A dynamic site is
// a call the analysis resolved through an interface or a func value; it is
// quoted only after every direct one, since a reader checks a direct call at
// a glance.
type site struct {
	path    string
	line    int
	dynamic bool
}

// found is what one relater read: the facts, and the units whose code it
// read, over which it owns the depends_on relations.
type found struct {
	facts   []fact
	covered map[string]bool
	// when is the condition a link holds under, for a relater that reads
	// several build configurations and finds the link in only some.
	when map[link]string
	// unread are the root-relative source files under the units that no
	// configuration the relater reads compiles.
	unread []string
}

// Unread is the code one relater could not read, and why.
type Unread struct {
	Relater, Why string
	Paths        []string
}

// Relate runs each relater over its directory under root and adds what it
// finds to doc. A relater owns the depends_on relations between the units
// whose code it read, so it replaces the extraction's there. It never makes
// a unit, and adds a provides or consumes only where the extraction has none,
// since the extraction owns its contracts.
func Relate(root string, doc extract.Doc, specs []frontend.Spec) (extract.Doc, []Unread, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return doc, nil, err
	}
	o := index(doc.Units)
	var unread []Unread
	all := found{covered: map[string]bool{}, when: map[link]string{}}
	for i, s := range specs {
		if slices.Contains(specs[:i], s) {
			continue
		}
		r := relaters[s.Name]
		dir := filepath.Join(root, filepath.FromSlash(s.Dir))
		if _, err := os.Stat(filepath.Join(dir, r.manifest)); err != nil {
			return doc, nil, fmt.Errorf("%s: %s has no %s", s.Name, s.Dir, r.manifest)
		}
		f, err := r.run(tree{root: root, dir: dir}, o)
		if err != nil {
			return doc, nil, fmt.Errorf("%s: %w", s.Name, err)
		}
		all.facts = append(all.facts, f.facts...)
		maps.Copy(all.covered, f.covered)
		maps.Copy(all.when, f.when)
		if len(f.unread) > 0 {
			slices.Sort(f.unread)
			unread = append(unread, Unread{s.Name, r.unread, slices.Compact(f.unread)})
		}
	}
	doc, err = merge(root, doc, all)
	return doc, unread, err
}

// codeVia is whether a depends_on's via is one the code alone shows, so a
// relater that read both ends owns it. A config, spawn, data or callback
// relation lives outside the Go code and is never replaced.
func codeVia(v model.Via) bool { return v == "" || slices.Contains(vias, v) }

// merge folds the facts into the extraction under the ownership rule Relate
// states, one relation per link with the best via's quotes.
func merge(root string, doc extract.Doc, f found) (extract.Doc, error) {
	had := map[link]bool{}
	var kept []extract.DocRelation
	for _, r := range doc.Relations {
		if r.Kind == model.DependsOn && codeVia(r.Via) && f.covered[r.From] && f.covered[r.To] {
			continue
		}
		had[link{r.From, r.To, r.Kind}] = true
		kept = append(kept, r)
	}
	sites := map[link]map[model.Via]map[site]bool{}
	for _, x := range f.facts {
		if x.from == x.to || had[x.link] {
			continue
		}
		if sites[x.link] == nil {
			sites[x.link] = map[model.Via]map[site]bool{}
		}
		if sites[x.link][x.via] == nil {
			sites[x.link][x.via] = map[site]bool{}
		}
		sites[x.link][x.via][x.at] = true
	}
	q := quoter{root: root, lines: map[string][]string{}}
	for l, byVia := range sites {
		v := vias[slices.IndexFunc(vias, func(v model.Via) bool { return byVia[v] != nil })]
		at := slices.SortedFunc(maps.Keys(byVia[v]), func(a, b site) int {
			return cmp.Or(compareBool(a.dynamic, b.dynamic), cmp.Compare(a.path, b.path), cmp.Compare(a.line, b.line))
		})
		r := extract.DocRelation{From: l.from, To: l.to, Kind: l.kind, Via: v, When: f.when[l]}
		for _, s := range at {
			if len(r.Evidence) == maxQuotes {
				break
			}
			if slices.ContainsFunc(r.Evidence, func(e model.Evidence) bool { return e.Path == s.path && e.Lines == strconv.Itoa(s.line) }) {
				continue
			}
			text, err := q.line(s)
			if err != nil {
				return doc, err
			}
			if text != "" {
				r.Evidence = append(r.Evidence, model.Evidence{Path: s.path, Lines: strconv.Itoa(s.line), Quote: text})
			}
		}
		kept = append(kept, r)
	}
	doc.Relations = kept
	return doc, nil
}

func compareBool(a, b bool) int {
	switch {
	case a == b:
		return 0
	case a:
		return 1
	}
	return -1
}

// owners finds the unit that holds a file and the contract unit that
// declares a symbol.
type owners struct {
	// code are the anchors of the modules and features, innermost first.
	code []held
	// contracts maps path#symbol to the contract unit anchored there.
	contracts map[string]string
}

type held struct {
	path, ref string
	depth     int
}

func index(units []extract.DocUnit) owners {
	parent := map[string]string{}
	for _, u := range units {
		parent[cmp.Or(u.Ref, u.Key)] = u.Parent
	}
	depth := func(ref string) int {
		n := 0
		for seen := map[string]bool{}; parent[ref] != "" && !seen[ref]; ref = parent[ref] {
			seen[ref] = true
			n++
		}
		return n
	}
	o := owners{contracts: map[string]string{}}
	for _, u := range units {
		ref := cmp.Or(u.Ref, u.Key)
		for _, a := range u.Anchors {
			p := path.Clean(strings.Trim(strings.ReplaceAll(a.Path, `\`, "/"), "/"))
			switch {
			case u.Kind.IsContract() && a.Symbol != "":
				o.contracts[p+"#"+a.Symbol] = cmp.Or(o.contracts[p+"#"+a.Symbol], ref)
			case (u.Kind == model.Module || u.Kind == model.Feature) && a.Symbol == "":
				o.code = append(o.code, held{p, ref, depth(ref)})
			}
		}
	}
	slices.SortFunc(o.code, func(a, b held) int {
		return cmp.Or(cmp.Compare(len(b.path), len(a.path)), cmp.Compare(b.depth, a.depth), cmp.Compare(a.ref, b.ref))
	})
	return o
}

// of is the innermost module or feature whose anchor holds path.
func (o owners) of(path string) (string, bool) {
	for _, h := range o.code {
		if h.path == path || h.path == "." || strings.HasPrefix(path, h.path+"/") {
			return h.ref, true
		}
	}
	return "", false
}

// contract is the contract unit anchored at the symbol declared in path.
func (o owners) contract(path, symbol string) (string, bool) {
	r, ok := o.contracts[path+"#"+symbol]
	return r, ok
}

// tree is where a relater runs: the repository root and its own directory,
// both absolute.
type tree struct{ root, dir string }

// rel gives an absolute path as the root-relative, forward-slash path an
// anchor holds, and false for a path outside the root.
func (t tree) rel(abs string) (string, bool) {
	r, err := filepath.Rel(t.root, filepath.Clean(abs))
	if err != nil || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) || filepath.IsAbs(r) {
		return "", false
	}
	return filepath.ToSlash(r), true
}

// quoter reads the lines that evidence quotes, each file once.
type quoter struct {
	root  string
	lines map[string][]string
}

// line is the site's line with its indentation and line ending trimmed, so
// a checkout with CRLF endings quotes the same text.
func (q quoter) line(s site) (string, error) {
	ls, ok := q.lines[s.path]
	if !ok {
		b, err := os.ReadFile(filepath.Join(q.root, filepath.FromSlash(s.path)))
		if err != nil {
			return "", err
		}
		ls = strings.Split(string(b), "\n")
		q.lines[s.path] = ls
	}
	if s.line < 1 || s.line > len(ls) {
		return "", nil
	}
	return strings.TrimSpace(ls[s.line-1]), nil
}
