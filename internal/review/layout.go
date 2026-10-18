package review

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"

	"github.com/whyjp/abbd/internal/ssot"
)

const LayoutFile = "generated/layout.json"

// Layout keeps node cells stable across revisions (PRD v1.4 08B section 2, FR-31). A node keeps
// its cell forever; a new node takes the first free cell in its preferred row; a removed
// node's cell stays reserved for one revision so its red strike-through has a place.
type Layout struct {
	Version int                    `json:"version"`
	Views   map[string]*ViewLayout `json:"views"`
}

type ViewLayout struct {
	Cells    map[string]Cell     `json:"cells"`
	Reserved map[string]Reserved `json:"reserved,omitempty"`
}

type Cell struct {
	Col int `json:"col"`
	Row int `json:"row"`
}

type Reserved struct {
	Cell
	Since string `json:"since"` // workspace fingerprint at which the node disappeared
}

// Want is a node the view must show, with the row and column it prefers when it is new.
type Want struct {
	ID       string
	Row, Col int
}

func LoadLayout(root string) *Layout {
	l := &Layout{Version: 1, Views: map[string]*ViewLayout{}}
	if b, err := os.ReadFile(ssot.Path(root, LayoutFile)); err == nil {
		_ = json.Unmarshal(b, l)
	}
	if l.Views == nil {
		l.Views = map[string]*ViewLayout{}
	}
	return l
}

func (l *Layout) Save(root string) error {
	b, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return err
	}
	p := ssot.Path(root, LayoutFile)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, b, 0o644)
}

// Place updates one view for the nodes wanted at this revision and returns their cells plus
// the reserved cells of nodes removed at this revision. It is idempotent for a fixed revision.
func (l *Layout) Place(view string, wants []Want, revision string) (map[string]Cell, map[string]Cell) {
	v := l.Views[view]
	if v == nil {
		v = &ViewLayout{Cells: map[string]Cell{}, Reserved: map[string]Reserved{}}
		l.Views[view] = v
	}
	if v.Reserved == nil {
		v.Reserved = map[string]Reserved{}
	}
	want := map[string]bool{}
	for _, w := range wants {
		want[w.ID] = true
	}
	for id, c := range v.Cells {
		if !want[id] {
			v.Reserved[id] = Reserved{Cell: c, Since: revision}
			delete(v.Cells, id)
		}
	}
	gone := map[string]Cell{}
	for id, r := range v.Reserved {
		switch {
		case want[id]: // came back: same cell again
			v.Cells[id] = r.Cell
			delete(v.Reserved, id)
		case r.Since != revision: // held for one revision already
			delete(v.Reserved, id)
		default:
			gone[id] = r.Cell
		}
	}
	taken := map[Cell]bool{}
	for _, c := range v.Cells {
		taken[c] = true
	}
	for _, r := range v.Reserved {
		taken[r.Cell] = true
	}
	sort.SliceStable(wants, func(i, j int) bool { return wants[i].ID < wants[j].ID })
	for _, w := range wants {
		if _, ok := v.Cells[w.ID]; ok {
			continue
		}
		c := Cell{Col: w.Col, Row: w.Row}
		for taken[c] {
			c.Col++
		}
		v.Cells[w.ID] = c
		taken[c] = true
	}
	out := make(map[string]Cell, len(v.Cells))
	for id, c := range v.Cells {
		out[id] = c
	}
	return out, gone
}

// Relayout drops every stored cell so the next render places all nodes fresh. Only an explicit
// user request calls it (`abbd review --relayout`).
func Relayout(root string) error {
	return (&Layout{Version: 1, Views: map[string]*ViewLayout{}}).Save(root)
}
