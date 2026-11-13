package screen

import (
	_ "embed"
	"encoding/json"
	"html/template"
	"strings"
)

//go:embed page.html
var pageHTML string

//go:embed page.css
var pageCSS string

var site = template.Must(template.New("site").Funcs(template.FuncMap{
	"join": strings.Join,
	"half": func(n int) int { return n / 2 },
	"plus": func(a, b int) int { return a + b },
	"short": func(s string, n int) string {
		if len(s) > n {
			return s[:n]
		}
		return s
	},
	"head":       head,
	"routeHead":  routeHead,
	"routeHeads": func() []string { return routeHeads },
	"role":       role,
	"legendFor": func(kind string, quiet bool) any {
		return struct {
			Kind  string
			Quiet bool
		}{kind, quiet}
	},
	"tip": func(k string) string { return tips[k] },
	"css": func(p page) template.CSS {
		return template.CSS(pageCSS + hueCSS + hoverCSS(p.hoverSize()))
	},
	"nameOf": func(m *Model, id string) string { return nameOf(*m, id) },
	"label": func(k string) string {
		if l, ok := labels[k]; ok {
			return l
		}
		return k
	},
}).Parse(pageHTML))

// labels are the Korean words the screen shows for an origin badge or a kind.
// The English key stays the CSS class and the word the CLI prints.
var labels = map[string]string{
	"stated": "사람", "inferred": "판독", "extracted": "추출",
	"MOD": "모듈", "IF": "인터페이스", "EVT": "이벤트", "DATA": "데이터", "CFG": "설정", "FEAT": "기능",
}

// tips say what each origin word means; the page shows them on hover.
var tips = map[string]string{
	"inferred":  "에이전트(리더)가 읽음. 인용한 코드가 아직 그대로인지 sync마다 검사함",
	"stated":    "사람이 말한 것",
	"extracted": "프로젝트의 추출 스크립트가 코드에서 읽음",
}

const (
	GraphFile = "graph.json"
	PageFile  = "index.html"
)

// Graph is graph.json: the blueprint's data, for tools that do not read the
// page. It holds no value of the sync that rendered it, which the state
// records, and of the drawing, which the pages hold, only each box's Rank:
// the order a reader reads it in. A change that moves boxes or lines
// rewrites the pages that draw them, and this file only when a box changes
// rows.
type Graph struct {
	Units []UnitRow `json:"units"`
	Edges []Line    `json:"edges"`
	// Modules are the modules' insides as their pages list them, less the
	// drawings.
	Modules []Inside `json:"modules"`
	// Flows, Stacks and Tables are the stored figures as the page draws them.
	Flows  []FlowView  `json:"flows,omitempty"`
	Stacks []StackView `json:"stacks,omitempty"`
	Tables []TableView `json:"tables,omitempty"`
}

// Files renders every file.
func Files(m Model) (map[string][]byte, error) {
	g := Graph{Units: m.Units, Edges: m.Edges, Modules: m.Insides, Flows: m.Flows, Stacks: m.Stacks, Tables: m.Tables}
	graph, err := json.MarshalIndent(g, "", "  ")
	if err != nil {
		return nil, err
	}
	files, err := Pages(m)
	if err != nil {
		return nil, err
	}
	files[GraphFile] = append(graph, '\n')
	return files, nil
}
