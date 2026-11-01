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
	"join":  strings.Join,
	"lower": strings.ToLower,
	// badge is the CSS class of a trust badge: its first word.
	"badge": func(b string) string { return strings.TrimSuffix(strings.Fields(b + " x")[0], ",") },
	"half":  func(n int) int { return n / 2 },
	"short": func(s string, n int) string {
		if r := []rune(s); len(r) > n {
			return string(r[:n]) + "…"
		}
		return s
	},
	"plus":  func(a, b int) int { return a + b },
	"marks": func() []string { return []string{"contract", "bypass", "guess", "block", "warn", "ink", "ok"} },
	"markColor": func(k string) string {
		return map[string]string{"contract": "accent", "bypass": "bypass", "guess": "guess", "block": "block", "warn": "warn", "ink": "ink", "ok": "ok"}[k]
	},
	"label": func(k string) string {
		if l, ok := labels[k]; ok {
			return l
		}
		return k
	},
}).Parse(pageHTML))

// labels are the Korean words the screen shows for a phase or a trust badge.
// The English key stays the CSS class and the word the CLI prints.
var labels = map[string]string{
	"uninitialized": "미초기화", "bootstrapped": "정렬 전", "aligned": "정렬됨", "drifted": "어긋남",
	"stated": "사람이 명시", "stated, guess rejected": "사람이 명시, 추정 반려", "confirmed": "사람이 확인",
	"inferred": "추정", "adopted": "문서에서 채택", "extracted": "코드에서 추출",
	"MOD": "모듈", "IF": "인터페이스", "EVT": "이벤트", "DATA": "데이터", "CFG": "설정", "FEAT": "기능",
}

// Options says how a page is rendered. A static page has no actions and no
// script; a live page runs only the script that carries Nonce.
type Options struct {
	Static bool
	// Only renders one section: "queue" for the confirm queue file.
	Only  string
	Nonce string
}

func Render(w io.Writer, m Model, o Options) error {
	return page.Execute(w, struct {
		M Model
		O Options
	}{m, o})
}

const (
	GraphFile    = "graph.json"
	ExplorerFile = "explorer.html"
	QueueFile    = "confirm-queue.html"
)

// Graph is graph.json: the map's data, for tools that do not read the page.
type Graph struct {
	Watermark Watermark `json:"watermark"`
	Phase     string    `json:"phase"`
	Units     []UnitRow `json:"units"`
	Edges     []Line    `json:"edges"`
	// Overview and Modules are the two levels as the page draws them.
	Overview Drawing  `json:"overview"`
	Modules  []Inside `json:"modules"`
	Drift    []Row    `json:"drift"`
	Queue    []string `json:"confirm_queue"`
}

// Files renders every generated file. Each carries the watermark of the
// .abbd inputs it came from, so a reader can tell when it is STALE.
func Files(m Model) (map[string][]byte, error) {
	// The files describe the .abbd inputs only, so they are current by
	// construction and say nothing about the working tree.
	m.Generated, m.Actual, m.Tree = Fresh, "", ""
	g := Graph{Watermark: m.Watermark, Phase: string(m.Phase), Units: m.Units, Edges: m.Edges, Overview: m.Overview, Modules: m.Insides, Drift: m.Drift}
	for _, q := range m.Queue {
		g.Queue = append(g.Queue, q.Key)
	}
	graph, err := json.MarshalIndent(g, "", "  ")
	if err != nil {
		return nil, err
	}
	out := map[string][]byte{GraphFile: append(graph, '\n')}
	for name, only := range map[string]string{ExplorerFile: "", QueueFile: "queue"} {
		var b bytes.Buffer
		if err := Render(&b, m, Options{Static: true, Only: only}); err != nil {
			return nil, err
		}
		out[name] = b.Bytes()
	}
	return out, nil
}
