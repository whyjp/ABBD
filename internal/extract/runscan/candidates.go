package runscan

import (
	"cmp"
	"slices"
	"strconv"
	"strings"

	"github.com/whyjp/abbd/internal/model"
)

// Fact is what a reader stores once a candidate holds.
type Fact string

const (
	ExecFact     Fact = "EXEC"
	PostsToFact  Fact = Fact(model.PostsTo)
	RunsOnFact   Fact = Fact(model.RunsOn)
	ProvidesFact Fact = Fact(model.Provides)
)

// Candidate is a fact the records point at and only a reader can store: a
// spawn site is an EXEC, a send and a recv on one channel a posts_to, a
// registration a runs_on, an impl a provides. Stored says the state already
// cites the candidate's sites.
type Candidate struct {
	Fact Fact
	// Owner is the module or feature whose code holds the first site, and
	// Other the one that holds the recv of a posts_to, or the contract an
	// impl provides. Either is "" when no unit anchors the code.
	Owner, Other model.ID
	API          string
	// Entry is the function a spawn or a registration runs, when the call
	// names it.
	Entry   string
	Channel string
	Sites   []Site
	// sends and recvs are every end of a posts_to pair, any one of each of
	// which a stored posts_to may quote.
	sends, recvs []Site
	Stored       bool
}

// Candidates maps records onto the units: the owner of each site is the
// module or feature that anchors its code most closely. A send and a recv
// pair only on one channel name, and a syntactic pair only inside one
// owner, or one file where no unit anchors the code, since a name such as
// tx says nothing about which channel it is across modules.
func Candidates(rs []Record, units []model.Unit) []Candidate {
	present := model.Present(units)
	owner := func(p string) model.ID { return ownerOf(present, p) }
	var out []Candidate
	type end struct {
		r     Record
		owner model.ID
	}
	sends, recvs := map[string][]end{}, map[string][]end{}
	for _, r := range rs {
		o := owner(r.Evidence.Path)
		switch r.Kind {
		case Spawn:
			out = append(out, Candidate{Fact: ExecFact, Owner: o, API: r.API, Entry: r.To, Sites: []Site{r.Evidence}})
		case RegisterCallback:
			out = append(out, Candidate{Fact: RunsOnFact, Owner: o, API: r.API, Entry: r.To, Sites: []Site{r.Evidence}})
		case Impl:
			out = append(out, Candidate{Fact: ProvidesFact, Owner: o, Other: contract(present, r.To), API: r.API, Entry: r.To, Sites: []Site{r.Evidence}})
		case Send, Recv:
			if r.Channel == "" {
				continue
			}
			key := r.Channel
			if r.Confidence != "semantic" {
				key += "\x00" + cmp.Or(string(o), r.Evidence.Path)
			}
			if r.Kind == Send {
				sends[key] = append(sends[key], end{r, o})
			} else {
				recvs[key] = append(recvs[key], end{r, o})
			}
		}
	}
	for key, ss := range sends {
		rs := recvs[key]
		if len(rs) == 0 {
			continue
		}
		pairs := map[[2]model.ID]int{}
		for _, s := range ss {
			for _, r := range rs {
				pair := [2]model.ID{s.owner, r.owner}
				i, ok := pairs[pair]
				if !ok {
					i, pairs[pair] = len(out), len(out)
					out = append(out, Candidate{Fact: PostsToFact, Owner: s.owner, Other: r.owner, API: s.r.API, Channel: s.r.Channel, Sites: []Site{s.r.Evidence, r.r.Evidence}})
				}
				c := &out[i]
				c.sends = appendSite(c.sends, s.r.Evidence)
				c.recvs = appendSite(c.recvs, r.r.Evidence)
			}
		}
	}
	for i := range out {
		out[i].Stored = stored(present, out[i])
	}
	slices.SortFunc(out, func(a, b Candidate) int {
		return cmp.Or(
			cmp.Compare(a.Sites[0].Path, b.Sites[0].Path),
			cmp.Compare(a.Sites[0].Line, b.Sites[0].Line),
			cmp.Compare(a.Fact, b.Fact),
			cmp.Compare(a.API, b.API),
			cmp.Compare(a.Channel, b.Channel),
			cmp.Compare(siteKey(a.Sites), siteKey(b.Sites)),
		)
	})
	return out
}

func siteKey(ss []Site) string {
	var b strings.Builder
	for _, s := range ss {
		b.WriteString(s.Path + ":" + strconv.Itoa(s.Line) + " ")
	}
	return b.String()
}

// ownerOf is the module or feature whose anchor names the path most
// closely, or "" when none does. On a tie the unit nested deepest wins, as
// a module and the module inside it both anchor the inner directory, then
// the lowest ID.
func ownerOf(units []model.Unit, p string) model.ID {
	parent := map[model.ID]model.ID{}
	for _, u := range units {
		parent[u.ID] = u.Parent
	}
	depth := func(id model.ID) int {
		n := 0
		for ; parent[id] != "" && n < len(units); n++ {
			id = parent[id]
		}
		return n
	}
	best, id := -1, model.ID("")
	e := model.Evidence{Path: p}
	for _, u := range units {
		if !u.Kind.Holds() {
			continue
		}
		for _, a := range u.Anchors {
			a.Symbol = ""
			if c := a.Closeness(e); c > best || c == best && c >= 0 && cmp.Or(cmp.Compare(depth(id), depth(u.ID)), cmp.Compare(u.ID, id)) < 0 {
				best, id = c, u.ID
			}
		}
	}
	return id
}

// contract is the contract whose name or anchored symbol is the name an
// impl gives, or "".
func contract(units []model.Unit, name string) model.ID {
	if name == "" {
		return ""
	}
	for _, u := range units {
		if !u.Kind.IsContract() {
			continue
		}
		if u.Name.Value == name || slices.ContainsFunc(u.Anchors, func(a model.Anchor) bool { return a.Symbol == name }) {
			return u.ID
		}
	}
	return ""
}

// stored reports whether the state already holds the candidate: an EXEC, or
// a relation of the candidate's kind, whose evidence cites its site, or for
// a posts_to one send and one recv of its pair.
func stored(units []model.Unit, c Candidate) bool {
	quotes := func(es []model.Evidence, ss []Site) bool {
		return slices.ContainsFunc(ss, func(s Site) bool { return slices.ContainsFunc(es, func(e model.Evidence) bool { return cited(e, s) }) })
	}
	ends := [][]Site{c.Sites}
	if c.Fact == PostsToFact {
		ends = [][]Site{c.sends, c.recvs}
	}
	cites := func(es []model.Evidence) bool {
		return !slices.ContainsFunc(ends, func(ss []Site) bool { return !quotes(es, ss) })
	}
	for _, u := range units {
		if c.Fact == ExecFact {
			if u.Kind == model.Exec && cites(u.Evidence) {
				return true
			}
			continue
		}
		for _, r := range u.Relations {
			if !r.Dead && Fact(r.Kind) == c.Fact && cites(r.Evidence) {
				return true
			}
		}
	}
	return false
}

// cited reports whether an evidence item quotes the site: same file, and
// its lines hold the site's line, or, with no lines, its quote lies in the
// site's.
func cited(e model.Evidence, s Site) bool {
	if e.Path != s.Path {
		return false
	}
	if first, last, ok := e.Span(); ok {
		return first <= s.Line && s.Line <= last
	}
	q := strings.TrimSpace(e.Quote)
	return q != "" && strings.Contains(s.Quote, q)
}

func appendSite(ss []Site, s Site) []Site {
	if slices.Contains(ss, s) {
		return ss
	}
	return append(ss, s)
}
