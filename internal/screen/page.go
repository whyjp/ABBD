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
}).Parse(pageHTML))

// Options says how a page is rendered. A static page has no actions.
type Options struct {
	Token  string
	Static bool
	// Only renders one section: "queue" for the confirm queue file.
	Only    string
	Flash   string
	Failure bool
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
	Nodes     []Node    `json:"nodes"`
	Edges     []Line    `json:"edges"`
	Drift     []Row     `json:"drift"`
	Queue     []string  `json:"confirm_queue"`
}

// Files renders every generated file. Each carries the watermark of the
// Markdown it came from, so a reader can tell when it is STALE.
func Files(m Model) (map[string][]byte, error) {
	// The files describe the Markdown only, so they are current by
	// construction and say nothing about the working tree.
	m.Generated, m.Actual, m.Tree = Fresh, "", ""
	g := Graph{Watermark: m.Watermark, Phase: string(m.Phase), Units: m.Units, Nodes: m.Map.Nodes, Edges: m.Map.Edges, Drift: m.Drift}
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
