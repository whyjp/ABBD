package screen

import (
	"bytes"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/whyjp/abbd/internal/model"
)

func pages(t *testing.T, m Model) map[string]string {
	t.Helper()
	files, err := Pages(m)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for f, b := range files {
		out[f] = string(b)
	}
	return out
}

// renderAll is every page of the blueprint, one after another, for a test
// that asks whether the blueprint shows something anywhere.
func renderAll(t *testing.T, m Model) *bytes.Buffer {
	t.Helper()
	ps := pages(t, m)
	var names []string
	for f := range ps {
		names = append(names, f)
	}
	slices.Sort(names)
	var b bytes.Buffer
	for _, f := range names {
		b.WriteString(ps[f])
	}
	return &b
}

// The whole, each part and every unit have pages of their own: the
// overview, one page per module that holds units, and the two indexes.
func TestEveryModuleHasAPageOfItsOwn(t *testing.T) {
	m := fixture(t, "orca")
	ps := pages(t, m)
	for _, f := range []string{PageFile, ContractsFile, UnitsFile} {
		if ps[f] == "" {
			t.Errorf("the blueprint has no %s", f)
		}
	}
	for _, ins := range m.Insides {
		p := ps[ins.ID+".html"]
		if !strings.Contains(p, `<section class="inside page" id="m-`+ins.ID+`">`) {
			t.Errorf("%s has no page showing its inside", ins.ID)
		}
		if strings.Count(p, `<section class="inside page"`) != 1 {
			t.Errorf("%s's page shows another module's inside too", ins.ID)
		}
		if strings.Contains(ps[PageFile], `id="m-`+ins.ID+`"`) {
			t.Errorf("the overview page still holds %s's inside", ins.ID)
		}
	}
}

// Every link opens a page the blueprint has, at an anchor that page holds.
func TestEveryLinkOpensAPageAndASection(t *testing.T) {
	href, id := regexp.MustCompile(`href="([^"#]*)(?:#([^"]*))?"`), regexp.MustCompile(` id="([^"]+)"`)
	for _, name := range fixtures {
		ps := pages(t, fixture(t, name))
		ids := map[string]map[string]bool{}
		for f, p := range ps {
			ids[f] = map[string]bool{}
			for _, m := range id.FindAllStringSubmatch(p, -1) {
				ids[f][m[1]] = true
			}
		}
		broken := 0
		for f, p := range ps {
			for _, m := range href.FindAllStringSubmatch(p, -1) {
				file, anchor := m[1], m[2]
				if strings.HasPrefix(file, "data:") || strings.HasPrefix(file, "http") {
					continue
				}
				if file == "" {
					file = f
				}
				if _, ok := ps[file]; !ok || anchor != "" && !ids[file][anchor] {
					if broken++; broken <= 3 {
						t.Errorf("%s: %s links %s#%s, which the blueprint does not hold", name, f, m[1], anchor)
					}
				}
			}
		}
	}
}

// Clicking a module in the overview follows it inside, onto its page.
func TestAModuleBoxOpensItsPage(t *testing.T) {
	m := fixture(t, "orca")
	ps := pages(t, m)
	_, svg, _ := strings.Cut(ps[PageFile], `<section id="overview">`)
	svg, _, _ = strings.Cut(svg, "</svg>")
	opened := 0
	for _, ins := range m.Insides {
		if strings.Contains(svg, `<a href="`+ins.ID+`.html">`) {
			opened++
		}
	}
	if opened == 0 {
		t.Fatalf("no box of the overview opens a module's page:\n%.2000s", svg)
	}
	if strings.Contains(svg, `href="#m-`) {
		t.Errorf("a box of the overview still opens an anchor on the overview page")
	}
}

// Every page carries the same nav: the overview, both indexes and every
// module, under the module that holds it, with the page's own marked.
func TestEveryPageCarriesTheNav(t *testing.T) {
	m := fixture(t, "orca")
	ps := pages(t, m)
	for f, p := range ps {
		_, nav, _ := strings.Cut(p, "<nav ")
		nav, _, ok := strings.Cut(nav, "</nav>")
		if !ok {
			t.Fatalf("%s has no nav", f)
		}
		for _, to := range []string{PageFile, ContractsFile, UnitsFile} {
			if f != to && !strings.Contains(nav, `href="`+to+`"`) {
				t.Errorf("%s's nav has no link to %s", f, to)
			}
		}
		if n := strings.Count(nav, `aria-current="page"`); n != 1 {
			t.Errorf("%s's nav marks %d pages as the current one", f, n)
		}
		for _, ins := range m.Insides {
			if f != ins.ID+".html" && !strings.Contains(nav, `href="`+ins.ID+`.html"`) {
				t.Errorf("%s's nav does not list %s", f, ins.ID)
			}
		}
	}
	for _, ins := range m.Insides {
		p := ps[ins.ID+".html"]
		crumbs, _, _ := strings.Cut(p[strings.Index(p, `<p class="crumbs">`):], "</p>")
		if !strings.Contains(crumbs, `href="index.html"`) || !strings.Contains(crumbs, ins.Name) {
			t.Errorf("%s's page has no trail from the overview: %s", ins.ID, crumbs)
		}
	}
}

// The contract index lists every contract with the module that owns it and
// the units that provide and use it.
func TestTheContractIndexListsEveryContract(t *testing.T) {
	units := append(ripgrep(), inside("DATA-0001", "MOD-0004"))
	units[2].Relations = append(units[2].Relations, guess("DATA-0001", model.Consumes, "convention"))
	m := Build(Input{Units: units})
	page := pages(t, m)[ContractsFile]
	var contracts int
	for _, u := range m.Units {
		if model.Kind(u.Kind).IsContract() {
			contracts++
			if !strings.Contains(page, `<tr id="k-`+u.ID+`">`) {
				t.Errorf("the contract index has no row for %s", u.ID)
			}
		}
	}
	if contracts == 0 {
		t.Fatal("precondition: the fixture holds contracts")
	}
	row := page[strings.Index(page, `<tr id="k-DATA-0001">`):]
	row, _, _ = strings.Cut(row, "</tr>")
	if !strings.Contains(row, "grep-searcher") {
		t.Errorf("the row of DATA-0001 should name grep-searcher, which uses it:\n%s", row)
	}
}

// The unit index lists every unit with its files, so the browser's find
// looks a unit up by ID, name, path or symbol.
func TestTheUnitIndexListsEveryUnitWithItsFiles(t *testing.T) {
	m := fixture(t, "orca")
	page := pages(t, m)[UnitsFile]
	for _, u := range m.Units {
		if !strings.Contains(page, `<tr id="u-`+u.ID+`">`) {
			t.Errorf("the unit index has no row for %s", u.ID)
		}
		for _, a := range u.Anchors {
			if !strings.Contains(page, a) {
				t.Errorf("the unit index does not show %s's anchor %s", u.ID, a)
				break
			}
		}
	}
}
