package topology

import (
	"cmp"
	"fmt"
	"path"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"github.com/whyjp/abbd/internal/model"
)

// Tier says how a query named a unit: by the unit's own ID or name, by code
// its anchors or evidence hold, or by words of what it is.
type Tier int

const (
	ByID Tier = iota
	ByName
	ByCode
	ByPhrase
)

// Match is one unit a query names, and why.
type Match struct {
	Unit model.Unit
	Tier Tier
	Why  string
}

// Exact reports whether the query named the unit itself.
func (m Match) Exact() bool { return m.Tier <= ByName }

// Find resolves what an agent has in hand to the units it names, best first:
// a unit's ID or name; a file, directory or symbol, held by the units whose
// anchors or evidence cover it, innermost first; or a phrase, ranked by how
// many of its words the unit's name, description, quotes and files hold.
// files lists the project's files. It is read only when the query names no
// unit, so a file no anchor names is still found through the directory
// anchor that holds it. The order is deterministic.
func Find(units []model.Unit, files func() []string, q string) []Match {
	present := model.Present(units)
	q = strings.TrimSpace(q)
	var named []Match
	for _, u := range present {
		switch {
		case strings.EqualFold(string(u.ID), q):
			return []Match{{Unit: u, Tier: ByID, Why: "its ID"}}
		case strings.EqualFold(u.Name.Value, q):
			named = append(named, Match{Unit: u, Tier: ByName, Why: "its name"})
		}
	}
	if len(named) > 0 {
		return named
	}
	fs := files()
	code := newRanking(present).byCode(fs, q)
	// A bare word such as "theme" may be a file name and a topic at once, so
	// the units its words describe follow the ones whose code holds it. A
	// path is only a path.
	if len(code) > 0 && strings.ContainsAny(q, `/\.`) {
		return code
	}
	for _, m := range newRanking(present).byPhrase(fs, q) {
		if !slices.ContainsFunc(code, func(c Match) bool { return c.Unit.ID == m.Unit.ID }) {
			code = append(code, m)
		}
	}
	return code
}

// Nearest lists the n units whose names are fewest edits from q.
func Nearest(units []model.Unit, q string, n int) []model.Unit {
	present := model.Present(units)
	q = strings.ToLower(q)
	dist := map[model.ID]int{}
	for _, u := range present {
		dist[u.ID] = edits(q, strings.ToLower(u.Name.Value))
	}
	slices.SortStableFunc(present, func(a, b model.Unit) int { return cmp.Compare(dist[a.ID], dist[b.ID]) })
	return present[:min(n, len(present))]
}

// place is code a unit rests on: one of its anchors or its evidence.
type place struct {
	what, path, symbol string
}

func places(u model.Unit) []place {
	var out []place
	for _, a := range u.Anchors {
		out = append(out, place{"anchor", a.Path, a.Symbol})
	}
	for _, e := range u.Evidence {
		out = append(out, place{"evidence", e.Path, e.Symbol})
	}
	return out
}

// ranking keeps each unit's best match. A key compares larger first; ties
// go to the unit deeper inside its modules, then to the smaller ID.
type ranking struct {
	units []model.Unit
	depth map[model.ID]int
	best  map[model.ID]ranked
}

type ranked struct {
	Match
	key []int
}

func newRanking(present []model.Unit) ranking {
	parent := map[model.ID]model.ID{}
	for _, u := range present {
		parent[u.ID] = u.Parent
	}
	depth := map[model.ID]int{}
	for _, u := range present {
		for x := u.Parent; x != "" && depth[u.ID] < len(present); x = parent[x] {
			depth[u.ID]++
		}
	}
	return ranking{units: present, depth: depth, best: map[model.ID]ranked{}}
}

func (r ranking) add(u model.Unit, tier Tier, why string, key ...int) {
	key = append(key, r.depth[u.ID])
	if old, ok := r.best[u.ID]; !ok || slices.Compare(key, old.key) > 0 {
		r.best[u.ID] = ranked{Match{Unit: u, Tier: tier, Why: why}, key}
	}
}

func (r ranking) sorted() []Match {
	var rs []ranked
	for _, x := range r.best {
		rs = append(rs, x)
	}
	slices.SortFunc(rs, func(a, b ranked) int {
		if c := slices.Compare(b.key, a.key); c != 0 {
			return c
		}
		return cmp.Compare(a.Unit.ID, b.Unit.ID)
	})
	out := make([]Match, len(rs))
	for i, x := range rs {
		out[i] = x.Match
	}
	return out
}

var lineSuffix = regexp.MustCompile(`:\d+(-\d+)?$`)

// byCode reads q as a symbol, or as a path or file name such as an agent
// copies from an editor or a stack trace, and ranks the units whose code
// holds it. A unit whose anchor or evidence holds the path ranks by how
// specific that anchor is, so a module inside another comes first. A unit
// with code under a directory the query names ranks after them.
func (r ranking) byCode(files []string, q string) []Match {
	for _, u := range r.units {
		for _, pl := range places(u) {
			if pl.symbol != "" && strings.EqualFold(pl.symbol, q) {
				r.add(u, ByCode, fmt.Sprintf("symbol %s at %s", pl.symbol, pl.path), 2, 0, -len(pl.path))
			}
		}
	}
	p := path.Clean(lineSuffix.ReplaceAllString(strings.ReplaceAll(q, `\`, "/"), ""))
	p = strings.TrimPrefix(p, "./")
	if p == "." || p == "" || strings.ContainsAny(p, " \t") {
		return r.sorted()
	}
	for _, t := range r.targets(files, p) {
		for _, u := range r.units {
			for _, pl := range places(u) {
				switch {
				case pl.path == t:
					r.add(u, ByCode, fmt.Sprintf("%s %s", pl.what, pl.path), 1, 0, -len(t))
				case strings.HasPrefix(t, pl.path+"/"):
					r.add(u, ByCode, fmt.Sprintf("%s %s holds %s", pl.what, pl.path, t), 1, len(pl.path)-len(t), -len(t))
				case strings.HasPrefix(pl.path, t+"/"):
					r.add(u, ByCode, fmt.Sprintf("%s %s is under %s", pl.what, pl.path, t), 0, -len(pl.path), -len(t))
				}
			}
		}
	}
	return r.sorted()
}

// targets are the paths p may mean: p itself, the files whose path ends
// with it or that it ends with, and, for a bare name, the files of that name
// with or without their extension.
func (r ranking) targets(files []string, p string) []string {
	known := slices.Clone(files)
	for _, u := range r.units {
		for _, pl := range places(u) {
			known = append(known, pl.path)
		}
	}
	out := []string{p}
	bare := !strings.Contains(p, "/")
	for _, f := range known {
		base := path.Base(f)
		stem, _, _ := strings.Cut(base, ".")
		switch {
		case bare && (strings.EqualFold(base, p) || strings.EqualFold(stem, p)),
			!bare && (strings.HasSuffix(f, "/"+p) || strings.HasSuffix(p, "/"+f)):
			out = append(out, f)
		}
	}
	slices.Sort(out[1:])
	return slices.Compact(out)
}

var stopWords = map[string]bool{
	"a": true, "an": true, "and": true, "by": true, "does": true, "for": true, "from": true, "how": true,
	"in": true, "is": true, "of": true, "on": true, "or": true, "the": true, "to": true, "what": true,
	"where": true, "which": true, "who": true, "with": true,
}

// words splits text into lower-case words at punctuation and camelCase.
func words(s string) []string {
	var out []string
	var w []rune
	flush := func() {
		if len(w) > 0 {
			out = append(out, strings.ToLower(string(w)))
			w = w[:0]
		}
	}
	rs := []rune(s)
	for i, c := range rs {
		switch {
		case !unicode.IsLetter(c) && !unicode.IsDigit(c):
			flush()
			continue
		case unicode.IsUpper(c) && i > 0 && unicode.IsLower(rs[i-1]):
			flush()
		}
		w = append(w, c)
	}
	flush()
	return out
}

func tokens(q string) []string {
	var out []string
	for _, w := range words(q) {
		if len(w) >= 2 && !stopWords[w] && !slices.Contains(out, w) {
			out = append(out, w)
		}
	}
	return out
}

func holds(text, tok string) bool {
	return slices.ContainsFunc(words(text), func(w string) bool {
		return w == tok || len(tok) >= 3 && strings.HasPrefix(w, tok)
	})
}

// field is text a phrase is matched against, weighted by how much it says
// about what the unit is.
type field struct {
	label  string
	weight int
	text   string
}

func fields(u model.Unit) []field {
	fs := []field{{"name", 4, u.Name.Value}}
	for _, f := range []struct {
		label string
		fact  *model.Fact
	}{{"responsibility", u.Responsibility}, {"purpose", u.Purpose}, {"signature", u.Signature}} {
		if f.fact != nil {
			fs = append(fs, field{f.label, 2, f.fact.Value})
		}
	}
	for _, e := range u.Evidence {
		fs = append(fs, field{"quote", 1, e.Quote})
	}
	for _, rel := range u.Relations {
		for _, e := range rel.Evidence {
			fs = append(fs, field{"quote", 1, e.Quote})
		}
	}
	for _, pl := range places(u) {
		fs = append(fs, field{pl.what + " " + pl.path, 1, pl.path})
	}
	return fs
}

// byPhrase ranks units by how many of the phrase's words they hold, then by
// where: a word in the name says most, one in a file the unit's anchors hold
// least.
func (r ranking) byPhrase(files []string, q string) []Match {
	toks := tokens(q)
	if len(toks) == 0 {
		return nil
	}
	holder := map[string][]int{}
	for i, u := range r.units {
		for _, pl := range places(u) {
			holder[pl.path] = append(holder[pl.path], i)
		}
	}
	// hit[i][tok] is the best field of unit i that holds tok.
	hit := make([]map[string]field, len(r.units))
	note := func(i int, tok string, f field) {
		if hit[i] == nil {
			hit[i] = map[string]field{}
		}
		if old, ok := hit[i][tok]; !ok || f.weight > old.weight {
			hit[i][tok] = f
		}
	}
	for i, u := range r.units {
		for _, f := range fields(u) {
			for _, tok := range toks {
				if holds(f.text, tok) {
					note(i, tok, f)
				}
			}
		}
	}
	for _, file := range files {
		var held []string
		for _, tok := range toks {
			if holds(file, tok) {
				held = append(held, tok)
			}
		}
		if len(held) == 0 {
			continue
		}
		for d := file; d != "." && d != "/"; d = path.Dir(d) {
			for _, i := range holder[d] {
				for _, tok := range held {
					note(i, tok, field{"file " + file, 0, file})
				}
			}
		}
	}
	for i, u := range r.units {
		if len(hit[i]) == 0 {
			continue
		}
		score := 0
		var labels []string
		said := map[string][]string{}
		for _, tok := range toks {
			if f, ok := hit[i][tok]; ok {
				score += f.weight
				if _, seen := said[f.label]; !seen {
					labels = append(labels, f.label)
				}
				said[f.label] = append(said[f.label], fmt.Sprintf("%q", tok))
			}
		}
		var why []string
		for _, l := range labels {
			why = append(why, l+" has "+strings.Join(said[l], ", "))
		}
		r.add(u, ByPhrase, strings.Join(why, "; "), len(hit[i]), score)
	}
	return r.sorted()
}

// edits is the Levenshtein distance between a and b.
func edits(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur := make([]int, len(rb)+1)
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(rb)]
}
