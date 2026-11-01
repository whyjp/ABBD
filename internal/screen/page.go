package screen

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"html/template"
	"io"
	"strings"
)

//go:embed page.html
var pageHTML string

var page = template.Must(template.New("page").Funcs(template.FuncMap{
	"join": strings.Join,
	"half": func(n int) int { return n / 2 },
	"plus": func(a, b int) int { return a + b },
	"short": func(s string, n int) string {
		if len(s) > n {
			return s[:n]
		}
		return s
	},
	"marks": func() []string { return []string{"contract", "bypass", "guess", "ink", "ok"} },
	"markColor": func(k string) string {
		return map[string]string{"contract": "accent", "bypass": "bypass", "guess": "guess", "ink": "ink", "ok": "ok"}[k]
	},
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
	"stated": "사람이 명시", "inferred": "추정", "extracted": "코드에서 추출",
	"MOD": "모듈", "IF": "인터페이스", "EVT": "이벤트", "DATA": "데이터", "CFG": "설정", "FEAT": "기능",
}

func Render(w io.Writer, m Model) error { return page.Execute(w, m) }

const (
	GraphFile = "graph.json"
	PageFile  = "index.html"
)

// Graph is graph.json: the blueprint's data, for tools that do not read the page.
type Graph struct {
	Watermark Watermark `json:"watermark"`
	Units     []UnitRow `json:"units"`
	Edges     []Line    `json:"edges"`
	// Overview and Modules are the two levels as the page draws them.
	Overview Drawing  `json:"overview"`
	Modules  []Inside `json:"modules"`
}

// Files renders every file. Each carries the watermark of the .abbd inputs it
// came from, so a reader can tell when it is STALE.
func Files(m Model) (map[string][]byte, error) {
	m.Blueprint = Fresh
	g := Graph{Watermark: m.Watermark, Units: m.Units, Edges: m.Edges, Overview: m.Overview, Modules: m.Insides}
	graph, err := json.MarshalIndent(g, "", "  ")
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	if err := Render(&b, m); err != nil {
		return nil, err
	}
	return map[string][]byte{GraphFile: append(graph, '\n'), PageFile: b.Bytes()}, nil
}
