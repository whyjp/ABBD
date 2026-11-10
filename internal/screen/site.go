package screen

import (
	"bytes"
	"regexp"
	"slices"
	"strings"

	"github.com/whyjp/abbd/internal/model"
)

// The blueprint is a site of pages cut from one model, as a reader moves
// from the whole to a part to one unit. index.html holds only the overview,
// at the level of modules. Each unit the overview draws has a page of its
// own when it holds units or has a focus view there: its focus view on the
// overview, then its inside and the inside's focus views. Each flow, stack
// and table has a page, and contracts.html and units.html index every
// contract and every unit. So a change rewrites the pages of the views that
// draw it, and a change inside one module rewrites that module's page. The
// template links by anchor as if it were one page; link turns each anchor
// into the page that holds it.
const (
	ContractsFile = "contracts.html"
	UnitsFile     = "units.html"
)

// page is one file and what its template draws. Current is the anchor of
// the page's own view, which the nav marks.
type page struct {
	*Model
	File, Title, Template, Current string
	Inside                         *Inside
	// Around is the focus view of the page's unit on the overview, and
	// Flow, Stack or Table the one figure a figure's page draws.
	Around *Focus
	Flow   *FlowView
	Stack  *StackView
	Table  *TableView
	// Trail is the chain of modules from the top down to this page's. Nav
	// is the tree of every module, which only the overview lists.
	Trail     []NavItem
	Nav       []NavItem
	Contracts []ContractRow
	// Paged are the units that have a page of their own.
	Paged map[string]bool
}

// NavItem is one module in the nav's tree, at its depth below the top.
// Feature says its top-level unit is a feature, which the nav lists apart.
type NavItem struct {
	ID, Name string
	Depth    int
	Feature  bool
}

// ContractRow is one contract in the contract index: who owns it, who
// provides it and who uses it, each a link to where that unit is read.
type ContractRow struct {
	ID, Name, Kind, Signature string
	// Sentence is what the contract is responsible for, else its purpose.
	Sentence         *Fact `json:",omitempty"`
	Owner            *Ref
	Providers, Users []Ref
}

// Ref names a unit and the anchor where a reader finds it: its own page
// when it holds units, its row in the unit index otherwise.
type Ref struct{ ID, Name, Href string }

// Pages renders every page of the blueprint. Its boxes also open their
// focus views, which graph.json does not hold, so only the pages link them.
func Pages(m Model) (map[string][]byte, error) {
	m.Overview = linkFocus(m.Overview, m.Focus)
	m.Insides = slices.Clone(m.Insides)
	for i := range m.Insides {
		m.Insides[i].Drawing = linkFocus(m.Insides[i].Drawing, m.Insides[i].Focus)
	}
	nav, parent := navTree(m)
	shared := page{Model: &m, Contracts: contractRows(m), Paged: map[string]bool{}}
	for _, ins := range m.Insides {
		shared.Paged[ins.ID] = true
	}
	trail := func(p *page, id string) {
		for ; id != ""; id = parent[id] {
			p.Trail = append([]NavItem{{ID: id, Name: nameOf(m, id)}}, p.Trail...)
		}
	}
	// Only the overview lists every module, so a module that comes or goes
	// rewrites the overview and its own page, and no other.
	index := withPage(shared, PageFile, "모듈과 계약", "index", "overview")
	index.Nav = nav
	pages := []page{index}
	around := map[string]*Focus{}
	for i := range m.Focus {
		around[m.Focus[i].Unit] = &m.Focus[i]
	}
	for i := range m.Insides {
		ins := &m.Insides[i]
		p := withPage(shared, ins.ID+".html", ins.Name, "module", "m-"+ins.ID)
		p.Inside, p.Around = ins, around[ins.ID]
		delete(around, ins.ID)
		trail(&p, ins.ID)
		pages = append(pages, p)
	}
	for i := range m.Focus {
		if f := &m.Focus[i]; around[f.Unit] != nil {
			p := withPage(shared, f.Unit+".html", f.Name, "around", f.ID)
			p.Around = f
			trail(&p, f.Unit)
			pages = append(pages, p)
		}
	}
	for i := range m.Flows {
		p := withPage(shared, m.Flows[i].ID+".html", m.Flows[i].Question, "flowpage", m.Flows[i].ID)
		p.Flow = &m.Flows[i]
		pages = append(pages, p)
	}
	for i := range m.Stacks {
		p := withPage(shared, m.Stacks[i].ID+".html", m.Stacks[i].Question, "stackpage", m.Stacks[i].ID)
		p.Stack = &m.Stacks[i]
		pages = append(pages, p)
	}
	for i := range m.Tables {
		p := withPage(shared, m.Tables[i].ID+".html", m.Tables[i].Question, "tablepage", m.Tables[i].ID)
		p.Table = &m.Tables[i]
		pages = append(pages, p)
	}
	pages = append(pages,
		withPage(shared, ContractsFile, "계약 목록", "contracts", "contracts"),
		withPage(shared, UnitsFile, "단위 목록", "units", "units"))
	out := map[string][]byte{}
	var order []string
	for _, p := range pages {
		var b bytes.Buffer
		if err := site.ExecuteTemplate(&b, p.Template, p); err != nil {
			return nil, err
		}
		out[p.File] = b.Bytes()
		order = append(order, p.File)
	}
	primary := map[string]string{}
	for _, p := range pages {
		primary[p.Current] = p.File
	}
	link(out, order, primary)
	return out, nil
}

func withPage(p page, file, title, tmpl, current string) page {
	p.File, p.Title, p.Template, p.Current = file, title, tmpl, current
	return p
}

var (
	idAttr   = regexp.MustCompile(` id="([^"]+)"`)
	hrefAttr = regexp.MustCompile(`href="#([^"]*)"`)
)

// link points each in-page anchor at the page that holds it: the same page
// when it does, else the first page in order that does. An anchor that is a
// page's own view opens that page at its top.
func link(pages map[string][]byte, order []string, primary map[string]string) {
	owner := map[string]string{}
	ids := map[string]map[string]bool{}
	for _, f := range order {
		ids[f] = map[string]bool{}
		for _, m := range idAttr.FindAllSubmatch(pages[f], -1) {
			id := string(m[1])
			ids[f][id] = true
			if _, ok := owner[id]; !ok {
				owner[id] = f
			}
		}
	}
	for id, f := range primary {
		owner[id] = f
	}
	for _, f := range order {
		pages[f] = hrefAttr.ReplaceAllFunc(pages[f], func(h []byte) []byte {
			id := string(hrefAttr.FindSubmatch(h)[1])
			switch to, ok := owner[id]; {
			case primary[id] != "" && primary[id] != f:
				return []byte(`href="` + primary[id] + `"`)
			case ids[f][id] || !ok:
				return h
			default:
				return []byte(`href="` + to + "#" + id + `"`)
			}
		})
	}
}

// navTree lists every module that has a page, each under the module that
// holds it, siblings by name.
func navTree(m Model) ([]NavItem, map[string]string) {
	parent := map[string]string{}
	for _, u := range m.Units {
		parent[u.ID] = u.Parent
	}
	has := map[string]bool{}
	for _, ins := range m.Insides {
		has[ins.ID] = true
	}
	children := map[string][]string{}
	for _, ins := range m.Insides {
		p := parent[ins.ID]
		if !has[p] {
			p = ""
		}
		children[p] = append(children[p], ins.ID)
	}
	var out []NavItem
	kind := map[string]string{}
	for _, u := range m.Units {
		kind[u.ID] = u.Kind
	}
	var walk func(id string, depth int, feature bool)
	walk = func(id string, depth int, feature bool) {
		kids := children[id]
		slices.SortFunc(kids, func(a, b string) int {
			return strings.Compare(nameOf(m, a)+a, nameOf(m, b)+b)
		})
		for _, k := range kids {
			f := feature || depth == 0 && kind[k] == string(model.Feature)
			out = append(out, NavItem{ID: k, Name: nameOf(m, k), Depth: depth, Feature: f})
			walk(k, depth+1, f)
		}
	}
	walk("", 0, false)
	for id, p := range parent {
		if !has[p] {
			parent[id] = ""
		}
	}
	return out, parent
}

func nameOf(m Model, id string) string {
	if i := slices.IndexFunc(m.Units, func(u UnitRow) bool { return u.ID == id }); i >= 0 {
		return m.Units[i].Name
	}
	return id
}

// contractRows lists every contract by kind and name, with the module
// that owns it and the live relations that provide and use it.
func contractRows(m Model) []ContractRow {
	pages := map[string]bool{}
	for _, ins := range m.Insides {
		pages[ins.ID] = true
	}
	ref := func(id string) Ref {
		if pages[id] {
			return Ref{ID: id, Name: nameOf(m, id), Href: "#m-" + id}
		}
		return Ref{ID: id, Name: nameOf(m, id), Href: "#u-" + id}
	}
	var out []ContractRow
	for _, u := range m.Units {
		if !model.Kind(u.Kind).IsContract() {
			continue
		}
		c := ContractRow{ID: u.ID, Name: u.Name, Kind: u.Kind, Signature: u.Signature, Sentence: u.Responsibility}
		if c.Sentence == nil || c.Sentence.Value == "" {
			c.Sentence = u.Purpose
		}
		if u.Parent != "" {
			r := ref(u.Parent)
			c.Owner = &r
		}
		add := func(rs []Ref, id string) []Ref {
			if slices.ContainsFunc(rs, func(r Ref) bool { return r.ID == id }) {
				return rs
			}
			return append(rs, ref(id))
		}
		for _, l := range m.Edges {
			switch {
			case l.To == u.ID && l.Kind == string(model.Provides):
				c.Providers = add(c.Providers, l.From)
			case l.To == u.ID && l.Kind == string(model.Consumes):
				c.Users = add(c.Users, l.From)
			}
		}
		out = append(out, c)
	}
	order := map[string]int{"IF": 0, "EVT": 1, "DATA": 2, "CFG": 3}
	slices.SortStableFunc(out, func(a, b ContractRow) int {
		if d := order[a.Kind] - order[b.Kind]; d != 0 {
			return d
		}
		return strings.Compare(a.Name+a.ID, b.Name+b.ID)
	})
	return out
}
