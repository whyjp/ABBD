package screen

import (
	"slices"
	"strings"
	"testing"

	"github.com/whyjp/abbd/internal/model"
)

func noted(u *model.Unit, kind model.RelationKind, to model.ID, text string, symbols ...string) {
	u.Phrases = append(u.Phrases, model.PhraseNote{To: to, Kind: kind, Writer: model.Reader, Text: text, Symbols: symbols})
}

// A phrase that names the symbols carrying it is listed with them beside the
// neighbor and the dependent it belongs to, a relation to a contract on the
// boxes that provide it; a phrase that names none adds nothing, so a
// repository that records no symbol has the pages it had.
func TestSymbolsAreListedBesideTheNeighborAndTheDependent(t *testing.T) {
	units := ripgrep()
	noted(&units[0], model.DependsOn, "MOD-0002", "검색 요청", "run_search", "SearchArgs")
	noted(&units[4], model.Consumes, "IF-0002", "결과 전달", "Sink::matched")
	noted(&units[2], model.DependsOn, "MOD-0004", "일치 질의")
	m := build(units)
	by := map[string]Neighbor{}
	for _, n := range m.Overview.Neighbors {
		by[n.Name] = n
	}
	carries := func(rs []Near, name string) []Carry {
		for _, r := range rs {
			if r.Name == name {
				return r.Carries
			}
		}
		return nil
	}
	want := []Carry{{"검색 요청", []string{"run_search", "SearchArgs"}}}
	if got := carries(by["ripgrep"].Uses, "grep"); !slices.EqualFunc(got, want, func(a, b Carry) bool { return a.Phrase == b.Phrase && slices.Equal(a.Symbols, b.Symbols) }) {
		t.Errorf("ripgrep uses grep with %v, want %v", got, want)
	}
	if got := carries(by["grep"].UsedBy, "ripgrep"); len(got) != 1 {
		t.Errorf("grep is used by ripgrep with %v", got)
	}
	if got := carries(by["grep-printer"].Uses, "grep-searcher"); len(got) != 1 || got[0].Symbols[0] != "Sink::matched" {
		t.Errorf("a use of a contract reaches the box that provides it: %v", got)
	}
	if got := carries(by["grep-searcher"].Uses, "grep-matcher"); got != nil {
		t.Errorf("a phrase with no symbol adds %v", got)
	}
	pages, err := Pages(m)
	if err != nil {
		t.Fatal(err)
	}
	if page := string(pages["index.html"]); !strings.Contains(page, `(검색 요청: <code>run_search</code>, <code>SearchArgs</code>)`) {
		t.Errorf("the overview page does not list the symbols")
	}
	with := ripgrep()
	noted(&with[0], model.DependsOn, "MOD-0002", "검색 요청")
	pw, _ := Pages(build(with))
	if page := string(pw["index.html"]); strings.Contains(page, `class="muted carries"`) {
		t.Errorf("a phrase with no symbol adds a symbol list to the page")
	}
}

// A dependent in a module's list of units carries the symbols of the phrase of
// its relation to the unit.
func TestADependentCarriesTheSymbolsOfItsRelation(t *testing.T) {
	units := tangled()
	for i := range units {
		if units[i].ID == "MOD-0105" {
			noted(&units[i], model.DependsOn, "MOD-0101", "전달", "hand_over")
		}
	}
	m := build(units)
	var got []Dependent
	for _, ins := range m.Insides {
		for _, u := range ins.Units {
			if u.ID == "MOD-0101" {
				got = u.Dependents
			}
		}
	}
	i := slices.IndexFunc(got, func(d Dependent) bool { return d.ID == "MOD-0105" })
	if i < 0 || len(got[i].Carries) != 1 || got[i].Carries[0].Symbols[0] != "hand_over" {
		t.Fatalf("dependents of MOD-0101: %+v", got)
	}
	pages, err := Pages(m)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(pages["MOD-0100.html"]), `(전달: <code>hand_over</code>)`) {
		t.Errorf("the module's page does not list the dependent's symbols")
	}
}

// A contract the module that holds it provides, with no provides line, takes
// its users' symbols to that module; a box that provides the contract is no
// user of it.
func TestSymbolsReachTheModuleThatHoldsAContractAndNotItsOwnProvider(t *testing.T) {
	units := []model.Unit{
		called("MOD-0001", "user", rel("IF-0001", model.Consumes), rel("MOD-0002", model.DependsOn)),
		called("MOD-0002", "owner"),
		inside("IF-0001", "MOD-0002"),
		called("MOD-0003", "other", rel("IF-0001", model.Provides), rel("IF-0001", model.Consumes), rel("MOD-0001", model.DependsOn)),
	}
	noted(&units[0], model.Consumes, "IF-0001", "질의 전달", "run_query")
	noted(&units[3], model.Consumes, "IF-0001", "자기 계약", "own_call")
	units = named(units)
	m := build(units)
	by := map[string]Neighbor{}
	for _, n := range m.Overview.Neighbors {
		by[n.Name] = n
	}
	var got []Carry
	for _, r := range by["user"].Uses {
		if r.Name == "owner" {
			got = r.Carries
		}
	}
	if len(got) != 1 || got[0].Symbols[0] != "run_query" {
		t.Errorf("user uses owner with %v, want run_query", got)
	}
	for _, r := range by["other"].Uses {
		for _, c := range r.Carries {
			if slices.Contains(c.Symbols, "own_call") {
				t.Errorf("a provider's use of its own contract is listed: %v", r)
			}
		}
	}
}
