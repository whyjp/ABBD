package screen

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/whyjp/abbd/internal/model"
)

// A common module is one most of the overview's boxes stand on: a line
// from each of them says only that they use it. Which ones are common, and
// whether one shapes the product's logic or only lends it tools, is read
// off the state alone, the same way on every repository, with nobody to
// confirm it.
//
// A box is common when at least commonFrom other boxes use it, they are at
// least half of the others, and the relations that run into it from other
// boxes are at least twice those that run out of it to other boxes: it is
// used much more than it uses. A box many use that uses as much back is
// coupled both ways, not common.
//
// A common box affects the logic when another box reaches into its state,
// configuration or execution (a DATA, CFG or EXEC unit inside it that a
// unit of another box has a relation to), when it provides a contract
// another box uses, or when it is a stage of the primary flow. Otherwise
// it is a utility: other boxes call it and nothing of theirs runs through
// what it holds. A unit nobody outside reaches says nothing either way, so
// a lone configuration read inside a helper layer leaves it a utility.

// commonFrom is the fewest boxes that must use a box for it to be common.
const commonFrom = 3

// Kinds of a common module.
const (
	CommonLogic   = "logic"
	CommonUtility = "utility"
)

// CommonView is one common module: its unit and name, whether it affects
// the logic or is a utility, why, the boxes that use it and those that do
// not, the relations into and out of it, and the words each user's
// relations into it carry.
type CommonView struct {
	Unit     string         `json:"unit"`
	Name     string         `json:"name"`
	Kind     string         `json:"kind"`
	Reasons  []string       `json:"reasons"`
	Users    []string       `json:"users"`
	NonUsers []string       `json:"non_users,omitempty"`
	In       int            `json:"in"`
	Out      int            `json:"out"`
	Phrases  []CommonPhrase `json:"phrases,omitempty"`
	// brief is the reasons in a few words each, without the names of the
	// units inside, as the overview's tooltip says them.
	brief []string
	// Band says the overview stands the module as a foundation band. A
	// utility stays a box where a reader reads through its lines: a stage
	// of the primary flow, a line of the record or the lead runs there, or
	// the band would take a word off the overview.
	Band bool `json:"band,omitempty"`
}

// CommonPhrase is the word one user's relations into a common module carry.
type CommonPhrase struct {
	From  string   `json:"from"`
	Words []string `json:"words"`
}

// SharedLibrary is a third-party library two boxes or more of the overview
// depend on, with those boxes. A library one box uses stays in that box's
// focus view.
type SharedLibrary struct {
	Unit  string   `json:"unit"`
	Name  string   `json:"name"`
	Users []string `json:"users"`
}

// Utility says the common module is a utility.
func (c CommonView) Utility() bool { return c.Kind == CommonUtility }

// commons classifies the common modules of the overview o, whose primary
// flow's stages are stage, and lists the libraries its boxes share.
func (o *overview) commons() ([]CommonView, []SharedLibrary) {
	_, users := o.overviewHubs()
	box := func(u model.ID) model.ID {
		if ch := o.tree.chain(u); len(ch) > 0 && isModuleOf(o, ch[0]) {
			return ch[0]
		}
		return ""
	}
	stage := o.flow.stagesOn(func(u model.ID) string { return string(box(u)) })
	in, out := map[model.ID]int{}, map[model.ID]int{}
	reached := map[model.ID]bool{}
	words := map[model.ID]map[model.ID][]string{}
	for _, u := range slices.Sorted(maps.Keys(o.tree.units)) {
		from := box(u)
		if from == "" {
			continue
		}
		for _, r := range o.tree.units[u].Relations {
			if r.Dead || o.tree.units[r.To].Kind == model.External {
				continue
			}
			if to := box(r.To); to != "" {
				if to != from {
					out[from]++
					in[to]++
					reached[r.To] = true
				}
				continue
			}
			// A use of a contract outside every box counts once out of the
			// user and once into each box that provides it, as the overview
			// pairs its users with them. Providing it is no use.
			if r.Kind == model.Provides {
				continue
			}
			used := false
			for _, to := range o.providers[r.To] {
				if to != from {
					in[to]++
					used = true
				}
			}
			if used {
				out[from]++
				reached[r.To] = true
			}
		}
		for _, p := range o.tree.units[u].Phrases {
			to := box(p.To)
			if to == "" || to == from || p.Status() == model.Dead || p.Text == "" {
				continue
			}
			if words[to] == nil {
				words[to] = map[model.ID][]string{}
			}
			if !slices.Contains(words[to][from], p.Text) {
				words[to][from] = append(words[to][from], p.Text)
			}
		}
	}
	var cs []CommonView
	n := len(o.modules)
	for _, m := range o.modules {
		us := users[string(m)]
		if len(us) < commonFrom || 2*len(us) < n-1 || in[m] < 2*out[m] {
			continue
		}
		c := CommonView{Unit: string(m), Name: o.name(m), In: in[m], Out: out[m]}
		for _, b := range o.modules {
			switch {
			case b == m:
			case us[string(b)]:
				c.Users = append(c.Users, string(b))
				if ws := words[m][b]; len(ws) > 0 {
					c.Phrases = append(c.Phrases, CommonPhrase{From: string(b), Words: ws})
				}
			default:
				c.NonUsers = append(c.NonUsers, string(b))
			}
		}
		c.Reasons, c.brief = o.logicOf(m, box, reached, stage)
		c.Kind = CommonLogic
		if len(c.Reasons) == 0 {
			c.Kind = CommonUtility
			c.Reasons = []string{"다른 상자가 닿는 상태·설정·실행 단위가 없고, 남이 쓰는 계약을 제공하지 않으며, 주 흐름의 단계도 아닙니다. 다른 상자는 이 모듈을 도구로 부를 뿐입니다."}
		}
		cs = append(cs, c)
	}
	return cs, o.sharedLibraries()
}

// logicOf says why the common module m affects the logic, nothing when it
// does not: the state, configuration and execution units inside it that
// another box reaches, the contracts it provides that another box uses,
// and its stage of the primary flow.
func (o *overview) logicOf(m model.ID, box func(model.ID) model.ID, reached map[model.ID]bool, stage map[string]int) (out, brief []string) {
	owned := map[model.Kind][]string{}
	for _, u := range slices.Sorted(maps.Keys(o.tree.units)) {
		k := o.tree.units[u].Kind
		if u != m && box(u) == m && reached[u] && (k == model.Data || k == model.Config || k == model.Exec) {
			owned[k] = append(owned[k], o.name(u))
		}
	}
	for _, k := range []model.Kind{model.Data, model.Config, model.Exec} {
		if len(owned[k]) > 0 {
			out = append(out, fmt.Sprintf("다른 상자가 닿는 %s %d개: %s.", commonKindWord[k], len(owned[k]), strings.Join(owned[k], ", ")))
			brief = append(brief, fmt.Sprintf("다른 상자가 닿는 %s %d개", commonKindWord[k], len(owned[k])))
		}
	}
	used := 0
	for _, c := range o.contracts {
		if !slices.Contains(o.providers[c], m) {
			continue
		}
		var us []string
		for _, u := range o.users[c] {
			if u != m {
				us = append(us, o.name(u))
			}
		}
		if len(us) > 0 {
			out = append(out, fmt.Sprintf("다른 상자가 쓰는 계약 %s을(를) 제공합니다: %s.", o.name(c), strings.Join(us, ", ")))
			used++
		}
	}
	if used > 0 {
		brief = append(brief, fmt.Sprintf("다른 상자가 쓰는 계약 %d개를 제공", used))
	}
	if s := stage[string(m)]; s > 0 {
		out = append(out, fmt.Sprintf("주 흐름의 %d단계입니다.", s))
		brief = append(brief, fmt.Sprintf("주 흐름 %d단계", s))
	}
	return out, brief
}

// commonKindWord says what a unit of each kind holds, as a common module's
// reason names it.
var commonKindWord = map[model.Kind]string{model.Data: "자료", model.Config: "설정", model.Exec: "실행 맥락"}

// sharedLibraries are the third-party libraries two boxes or more of the
// overview depend on, by name.
func (o *overview) sharedLibraries() []SharedLibrary {
	by := map[model.ID][]string{}
	for _, e := range o.relations {
		if e.Kind == model.DependsOn && isModuleOf(o, e.From) && o.units[e.To].Kind == model.External && !slices.Contains(by[e.To], string(e.From)) {
			by[e.To] = append(by[e.To], string(e.From))
		}
	}
	var out []SharedLibrary
	for _, x := range slices.Sorted(maps.Keys(by)) {
		if len(by[x]) >= 2 {
			us := by[x]
			slices.Sort(us)
			out = append(out, SharedLibrary{Unit: string(x), Name: o.name(x), Users: us})
		}
	}
	slices.SortStableFunc(out, func(a, b SharedLibrary) int { return strings.Compare(a.Name, b.Name) })
	return out
}

// utilities are the common modules that are utilities, by unit.
func utilities(cs []CommonView) map[model.ID]bool {
	var out map[model.ID]bool
	for _, c := range cs {
		if c.Utility() {
			if out == nil {
				out = map[model.ID]bool{}
			}
			out[model.ID(c.Unit)] = true
		}
	}
	return out
}

// commonNote is what a common module's tooltip says of it: its kind and
// why, the words each user's relations into it carry, and where its page is.
func (o *overview) commonNote(c CommonView) string {
	var b strings.Builder
	if c.Utility() {
		b.WriteString("공통 모듈 (유틸리티)")
	} else {
		b.WriteString("공통 모듈 (로직 영향): " + strings.Join(c.brief, ", "))
	}
	if len(c.Phrases) > 0 {
		var ws []string
		for _, p := range c.Phrases {
			ws = append(ws, o.name(model.ID(p.From))+": "+strings.Join(p.Words, ", "))
		}
		b.WriteString("\n오가는 것: " + strings.Join(ws, "; "))
	}
	b.WriteString("\n누가 어떻게 쓰는지는 공통 모듈 페이지에 있습니다")
	return b.String()
}

// commonByUnit indexes the common modules by unit.
func commonByUnit(cs []CommonView) map[model.ID]CommonView {
	out := map[model.ID]CommonView{}
	for _, c := range cs {
		out[model.ID(c.Unit)] = c
	}
	return out
}
