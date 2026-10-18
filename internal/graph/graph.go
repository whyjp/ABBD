// Package graph is the derived, regenerable knowledge graph (PRD ch.08).
// Path priority: Feature -> Module -> Interface (external contracts) -> CodeSymbol -> Test.
package graph

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"

	"github.com/whyjp/for-my-harness/internal/analysis"
	"github.com/whyjp/for-my-harness/internal/model"
	"github.com/whyjp/for-my-harness/internal/ssot"
)

const File = "generated/graph.json"

// Edge types. The first nine are PRD ch.06; DEPENDS_ON, USES and DEFINES are the structural
// edges the map needs to walk Feature -> Module -> Symbol.
const (
	Intends    = "INTENDS"
	Delivers   = "DELIVERS"
	OwnedBy    = "OWNED_BY"
	Exposes    = "EXPOSES"
	Implements = "IMPLEMENTS"
	Calls      = "CALLS"
	VerifiedBy = "VERIFIED_BY"
	Affects    = "AFFECTS"
	Supersedes = "SUPERSEDES"
	DependsOn  = "DEPENDS_ON"
	Uses       = "USES"
	Defines    = "DEFINES"
)

type Node struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Label    string `json:"label"`
	Location string `json:"location,omitempty"`
}

type Edge struct {
	From           string       `json:"from"`
	To             string       `json:"to"`
	Type           string       `json:"type"`
	Origin         model.Origin `json:"origin"`
	Confidence     string       `json:"confidence"`
	Analyzer       string       `json:"analyzer,omitempty"`
	Revision       string       `json:"revision"`
	SourceLocation string       `json:"source_location,omitempty"`
}

type Graph struct {
	Revision    string `json:"revision"`
	Fingerprint string `json:"fingerprint"`
	SSOTHash    string `json:"ssot_hash"`
	BuiltAt     string `json:"built_at"`
	Nodes       []Node `json:"nodes"`
	Edges       []Edge `json:"edges"`

	index map[string]Node
	adj   map[string][]int
}

func SymbolID(name string) string { return "SYM:" + name }

// Build derives the graph from the SSOT and the latest analysis. Full rebuild is cheap at
// Phase 1 scale; incremental rebuild is an optimization left for Phase 3.
func Build(s *ssot.Snapshot, res *analysis.Result, rev, fingerprint string) *Graph {
	g := &Graph{Revision: rev, Fingerprint: fingerprint}
	nodes := map[string]Node{}
	add := func(n Node) { nodes[n.ID] = n }
	edge := func(from, to, typ string, o model.Origin, conf, analyzer, loc string) {
		g.Edges = append(g.Edges, Edge{From: from, To: to, Type: typ, Origin: o, Confidence: conf, Analyzer: analyzer, Revision: rev, SourceLocation: loc})
	}

	for _, m := range s.Modules {
		if m.Status == model.ItemRemoved {
			continue
		}
		add(Node{ID: m.ID, Type: "module", Label: m.Name, Location: m.Path})
		for _, d := range m.DependsOn {
			edge(m.ID, d, DependsOn, model.Extracted, "static", m.Analyzer, m.Path)
		}
		for _, c := range m.ExternalContracts {
			if c.Status == model.ItemRemoved {
				continue
			}
			add(Node{ID: c.ID, Type: "contract:" + c.Kind, Label: c.Direction + " " + c.Selector, Location: c.Location})
			conf := "static"
			if c.Origin != model.Extracted {
				conf = "unknown"
			}
			edge(m.ID, c.ID, Exposes, c.Origin, conf, m.Analyzer, c.Location)
			if c.Symbol != "" {
				add(Node{ID: SymbolID(c.Symbol), Type: "symbol", Label: c.Symbol, Location: c.Location})
				edge(c.ID, SymbolID(c.Symbol), Implements, model.Extracted, "static", m.Analyzer, c.Location)
			}
		}
		for _, sym := range m.Symbols {
			id := SymbolID(qualify(m, sym.Name))
			add(Node{ID: id, Type: "symbol", Label: sym.Name, Location: sym.Location})
			edge(m.ID, id, Defines, model.Extracted, "static", m.Analyzer, sym.Location)
		}
		for _, t := range m.Tests {
			edge(m.ID, t, VerifiedBy, model.Extracted, "static", m.Analyzer, "")
		}
	}
	for _, t := range s.Tests {
		if t.Status != model.ItemRemoved {
			add(Node{ID: t.ID, Type: "test", Label: t.Name, Location: t.Location})
		}
	}
	for _, f := range s.Features {
		if f.Status == model.ItemRemoved {
			continue
		}
		add(Node{ID: f.ID, Type: "feature", Label: f.Name})
		for _, m := range f.OwnedBy {
			edge(f.ID, m, OwnedBy, f.Intent.Purpose.Origin, "declared", "", "")
		}
		for _, c := range f.ExternalContracts {
			edge(f.ID, c, Uses, model.Extracted, "static", "", "")
		}
		for _, t := range f.Verification {
			edge(f.ID, t.TestID, VerifiedBy, model.Extracted, string(t.Verification), "", "")
		}
	}
	for _, r := range s.Requirements {
		add(Node{ID: r.ID, Type: "requirement", Label: r.Statement.Value})
	}
	for _, c := range s.Changes {
		add(Node{ID: c.ID, Type: "change", Label: c.RequestSummary})
		for _, r := range c.Intends {
			edge(c.ID, r, Intends, model.UserStated, "declared", "", "")
		}
		for _, f := range c.Delivers {
			edge(c.ID, f, Delivers, model.Observed, string(c.Verification), "", "")
		}
		for _, x := range c.Impacted {
			edge(c.ID, x, Affects, model.Inferred, "impact", "", "")
		}
	}
	for _, d := range s.Decisions {
		add(Node{ID: d.ID, Type: "decision", Label: d.Title})
		if d.Supersedes != "" {
			edge(d.ID, d.Supersedes, Supersedes, d.Rationale.Origin, "declared", "", "")
		}
	}
	if res != nil {
		for _, m := range res.Modules {
			for _, c := range m.Calls {
				add(Node{ID: SymbolID(c.From), Type: "symbol", Label: c.From})
				add(Node{ID: SymbolID(c.To), Type: "symbol", Label: c.To})
				edge(SymbolID(c.From), SymbolID(c.To), Calls, model.Extracted, c.Confidence, m.Analyzer, c.Location)
			}
		}
	}

	// Drop edges whose endpoints do not exist instead of inventing nodes for them.
	kept := g.Edges[:0]
	for _, e := range g.Edges {
		if _, ok := nodes[e.From]; !ok {
			continue
		}
		if _, ok := nodes[e.To]; !ok {
			continue
		}
		kept = append(kept, e)
	}
	g.Edges = kept
	for _, id := range model.SortedKeys(nodes) {
		g.Nodes = append(g.Nodes, nodes[id])
	}
	sort.SliceStable(g.Edges, func(i, j int) bool {
		a, b := g.Edges[i], g.Edges[j]
		if a.From != b.From {
			return a.From < b.From
		}
		if a.To != b.To {
			return a.To < b.To
		}
		return a.Type < b.Type
	})
	g.reindex()
	return g
}

func qualify(m *model.Module, name string) string {
	for _, a := range m.Anchors {
		if a.Kind == model.AnchorSymbol {
			return trimLang(a.Ref) + "." + name
		}
	}
	return m.Path + "." + name
}

// trimLang strips the "go:" style language prefix of a symbol anchor.
func trimLang(ref string) string {
	for i := 0; i < len(ref) && i < 12; i++ {
		if ref[i] == ':' {
			return ref[i+1:]
		}
	}
	return ref
}

func (g *Graph) reindex() {
	g.index = map[string]Node{}
	g.adj = map[string][]int{}
	for _, n := range g.Nodes {
		g.index[n.ID] = n
	}
	for i, e := range g.Edges {
		g.adj[e.From] = append(g.adj[e.From], i)
		g.adj[e.To] = append(g.adj[e.To], i)
	}
}

func (g *Graph) Node(id string) (Node, bool) { n, ok := g.index[id]; return n, ok }

func Save(root string, g *Graph) error {
	b, err := json.MarshalIndent(g, "", "  ")
	if err != nil {
		return err
	}
	p := ssot.Path(root, File)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, b, 0o644)
}

func Load(root string) (*Graph, error) {
	b, err := os.ReadFile(ssot.Path(root, File))
	if err != nil {
		return nil, err
	}
	g := &Graph{}
	if err := json.Unmarshal(b, g); err != nil {
		return nil, err
	}
	g.reindex()
	return g, nil
}

// Subgraph is a bounded query result: never the whole graph (PRD ch.08 Context Selector).
type Subgraph struct {
	Root      string `json:"root"`
	Nodes     []Node `json:"nodes"`
	Edges     []Edge `json:"edges"`
	Truncated bool   `json:"truncated"`
}

func (g *Graph) Neighbors(id string, depth, limit int) Subgraph {
	sub := Subgraph{Root: id}
	seen := map[string]bool{id: true}
	edgeSeen := map[int]bool{}
	frontier := []string{id}
	for d := 0; d < depth && len(frontier) > 0; d++ {
		var next []string
		for _, cur := range frontier {
			for _, ei := range g.adj[cur] {
				if edgeSeen[ei] {
					continue
				}
				if len(sub.Edges) >= limit {
					sub.Truncated = true
					break
				}
				edgeSeen[ei] = true
				e := g.Edges[ei]
				sub.Edges = append(sub.Edges, e)
				other := e.To
				if other == cur {
					other = e.From
				}
				if !seen[other] {
					seen[other] = true
					next = append(next, other)
				}
			}
		}
		frontier = next
	}
	for _, nid := range model.SortedKeys(seen) {
		if n, ok := g.index[nid]; ok {
			sub.Nodes = append(sub.Nodes, n)
		}
	}
	return sub
}

// Trace returns the shortest undirected path between two nodes with the evidence of each edge.
func (g *Graph) Trace(from, to string) ([]Edge, bool) {
	if _, ok := g.index[from]; !ok {
		return nil, false
	}
	prev := map[string]int{from: -1}
	queue := []string{from}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if cur == to {
			break
		}
		for _, ei := range g.adj[cur] {
			e := g.Edges[ei]
			other := e.To
			if other == cur {
				other = e.From
			}
			if _, ok := prev[other]; !ok {
				prev[other] = ei
				queue = append(queue, other)
			}
		}
	}
	if _, ok := prev[to]; !ok {
		return nil, false
	}
	var path []Edge
	for cur := to; prev[cur] != -1; {
		e := g.Edges[prev[cur]]
		path = append([]Edge{e}, path...)
		if e.To == cur {
			cur = e.From
		} else {
			cur = e.To
		}
	}
	return path, true
}
