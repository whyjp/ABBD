package screen

import (
	"github.com/whyjp/abbd/internal/model"
)

// A table answers one question with a grid, such as which part of the CLI
// reads and writes which file: a row per stored unit, a column per short
// label, and a one-line statement in each cell the row fills. The page draws
// it as an HTML table, so every cell wraps, and at 560px and below each row
// folds to a card. A table stores no unit and no relation. A row's head
// names a stored unit and opens the inside that shows it. A column may
// stand for a stored unit too, and each cell under it is read against the
// stored relations: a cell that says a use no relation carries, and an
// empty cell over a relation, are marked.

// TableView is one table as the page draws it. Badge is the weakest origin
// of its rows and Source the one every row shares, if any. Stale counts its
// STALE rows, and Reasons say why its rows taken together with its head are
// not true of the state. Unbacked and Unsaid count the cells the stored
// relations do not bear out.
type TableView struct {
	ID, Key, Question, Badge, Source string
	Caption                          []Block  `json:",omitempty"`
	Reasons                          []string `json:",omitempty"`
	Rows, Stale, Unbacked, Unsaid    int
	Columns                          []ColumnHead
	Body                             []RowView
	held                             []model.ID
}

// ColumnHead is one column: its label, and the unit it stands for, if any.
type ColumnHead struct {
	Name string
	Unit *UnitChip `json:",omitempty"`
}

// RowView is one row. N is its place from 1 and ID its anchor. Badge and
// Source are set only where they differ from the table's. Cells hold one
// cell per column, in the head's order.
type RowView struct {
	N                 int
	ID, Badge, Source string
	Unit              UnitChip
	Cells             []CellView
	Quotes            []string `json:",omitempty"`
	Reasons           []string `json:",omitempty"`
	Stale             bool     `json:",omitempty"`
}

// CellView is one cell. Verb names the stored relation under a cell of a
// unit's column. Unbacked marks a cell that says a use no relation carries,
// and Unsaid an empty cell over a relation.
type CellView struct {
	Column, Says, Verb string
	Unbacked, Unsaid   bool `json:",omitempty"`
}

// TableRef is a table as a module's inside lists it: the rows whose units
// the module holds.
type TableRef struct {
	ID, Question string
	Rows         []int
}

// tables derives each stored table's view, in key order.
func (o *overview) tables(in Input) []TableView {
	var out []TableView
	for _, f := range in.Figures.Tables {
		if !f.Known {
			continue
		}
		rows := f.Claims()
		read := map[[2]string]model.Reading{}
		for _, r := range model.Readings(f.Columns, rows, in.Units) {
			read[[2]string{string(r.From), r.Column}] = r
		}
		v := TableView{ID: figureID[model.Row](f.Key), Key: string(f.Key), Question: f.Question, Badge: Badge(f.Origin()), Source: sharedSource(f), Caption: blocks(f.Caption), Rows: len(f.Parts)}
		for _, g := range f.Gaps(in.Units) {
			v.Reasons = append(v.Reasons, g.Text)
		}
		for _, c := range f.Columns {
			h := ColumnHead{Name: c.Name}
			if c.Unit != "" {
				h.Unit = &UnitChip{ID: string(c.Unit), Name: o.unitName(c.Unit), Kind: kindWord(o.tree.units[c.Unit].Kind), Href: o.href(c.Unit)}
			}
			v.Columns = append(v.Columns, h)
		}
		for i, p := range f.Parts {
			r := p.Claim
			rv := RowView{N: i + 1, ID: partID[model.Row](f.Key, i+1), Reasons: p.Why(in.Units),
				Unit: UnitChip{ID: string(r.Unit), Name: o.unitName(r.Unit), Kind: kindWord(o.tree.units[r.Unit].Kind), Href: o.href(r.Unit)}}
			rv.Badge, rv.Source = partMarks(p.Stamp, v.Badge, v.Source)
			rv.Stale = len(rv.Reasons) > 0
			for _, c := range f.Columns {
				cell := CellView{Column: c.Name, Says: r.Cells[c.Name]}
				if rd, ok := read[[2]string{string(r.Unit), c.Name}]; ok {
					cell.Verb, cell.Unbacked, cell.Unsaid = verbs[rd.Kind], rd.Unbacked(), rd.Unsaid()
				}
				if cell.Unbacked {
					v.Unbacked++
				}
				if cell.Unsaid {
					v.Unsaid++
				}
				rv.Cells = append(rv.Cells, cell)
			}
			for _, e := range r.Evidence {
				rv.Quotes = append(rv.Quotes, e.String())
			}
			if rv.Stale {
				v.Stale++
			}
			v.held = append(v.held, r.Unit)
			v.Body = append(v.Body, rv)
		}
		out = append(out, v)
	}
	return out
}
