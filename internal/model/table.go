package model

import (
	"fmt"
	"maps"
	"slices"
	"strings"
)

// TableRows and TableColumns bound a table, as rule 1 bounds a view's boxes:
// a grid that needs more answers two questions. ColumnName bounds a column's
// name, which heads a narrow column and so is a short label.
const (
	TableRows    = 16
	TableColumns = 8
	ColumnName   = 32
)

// Column is one column of a table: a short label, and the stored unit it
// stands for, if any. A cell under a unit's column says what the row's unit
// does to that unit, so abbd reads it against the stored relations.
type Column struct {
	Name string `yaml:"name"`
	Unit ID     `yaml:"unit,omitempty"`
}

// Row is one row of a table as a reader found it in the code: the stored
// unit it is about, a short statement under each column it fills, and the
// code that shows them. A column it leaves out is an empty cell. Its place
// from 1 is its number, never stored.
type Row struct {
	Unit     ID                `yaml:"unit"`
	Cells    map[string]string `yaml:"cells"`
	Evidence []Evidence        `yaml:"evidence"`
}

// Kind makes a table's rows a kind of figure, with two to eight columns
// named in its head.
func (Row) Kind() FigureKind[Row] {
	return FigureKind[Row]{Noun: "table", Dir: "tables", Part: "row", Parts: "rows", Min: 2, Max: TableRows, MinColumns: 2, MaxColumns: TableColumns, Whole: fillGrid}
}

// Validate checks the row on its own: a unit ID, one cell at least, each a
// one-line statement under a named column, and the code it rests on.
func (r Row) Validate() error {
	if _, _, err := ParseID(string(r.Unit)); err != nil {
		return err
	}
	if len(r.Cells) == 0 {
		return fmt.Errorf("%s: a row fills one cell at least; leave out a column whose cell is empty", r.Unit)
	}
	for _, col := range slices.Sorted(maps.Keys(r.Cells)) {
		switch says := r.Cells[col]; {
		case strings.TrimSpace(says) == "":
			return fmt.Errorf("%s: the cell under %q is empty; leave the column out of the row instead", r.Unit, col)
		case strings.ContainsAny(says, "\r\n"):
			return fmt.Errorf("%s: the cell under %q is one line", r.Unit, col)
		}
	}
	if len(r.Evidence) == 0 {
		return fmt.Errorf("%s: a row cites the code that shows its cells", r.Unit)
	}
	return validateEvidence(r.Evidence)
}

func (r Row) Label() string { return string(r.Unit) }
func (r Row) Names() []ID   { return []ID{r.Unit} }

// Words are the row's cells by column name, so every cell is a word a store
// warns on past 80 characters.
func (r Row) Words() []Word {
	var out []Word
	for _, col := range slices.Sorted(maps.Keys(r.Cells)) {
		out = append(out, Word{col, r.Cells[col]})
	}
	return out
}

func (r Row) Quotes() []Evidence        { return r.Evidence }
func (r Row) Requote(es []Evidence) Row { r.Evidence = es; return r }
func (r Row) Retire() Row               { return r }

// Gaps reads the row against the state, as a layer is read: its unit is
// present, and one quote at least lies in that unit's code.
func (r Row) Gaps(units []Unit) []string {
	st := stateOf(units)
	if _, ok := st[r.Unit]; !ok {
		return []string{fmt.Sprintf("%s is no longer in the state", r.Unit)}
	}
	if !st.rests([]ID{r.Unit}, r.Evidence) {
		return []string{fmt.Sprintf("none of the row's quotes is in the code of %s or a unit inside it; cite the lines that show its cells", r.Unit)}
	}
	return nil
}

// ValidateColumns checks a table's head on its own: each column a short
// label on one line, named once, and a unit ID where it names a unit.
func ValidateColumns(cols []Column) error {
	for i, c := range cols {
		switch name := strings.TrimSpace(c.Name); {
		case name == "" || strings.ContainsAny(c.Name, "\r\n"):
			return fmt.Errorf("column %d has a name, on one line", i+1)
		case len([]rune(name)) > ColumnName:
			return fmt.Errorf("column %d's name %q is a short label of %d characters at most", i+1, c.Name, ColumnName)
		case slices.ContainsFunc(cols[:i], func(o Column) bool { return o.Name == c.Name }):
			return fmt.Errorf("column %q is named twice", c.Name)
		}
		if c.Unit != "" {
			if _, _, err := ParseID(string(c.Unit)); err != nil {
				return fmt.Errorf("column %q: %w", c.Name, err)
			}
		}
	}
	return nil
}

// fillGrid keeps a table a grid: each unit heads one row, each cell sits
// under a column the head names, and each unit a column names is present. A
// gap names the rows it involves: for a column whose unit is gone, the rows
// that fill it, or every row when none does.
func fillGrid(cols []Column, rows []Row, units []Unit) []Gap {
	st := stateOf(units)
	var gaps []Gap
	for i, r := range rows {
		if j := slices.IndexFunc(rows[:i], func(o Row) bool { return o.Unit == r.Unit }); j >= 0 {
			gaps = append(gaps, Gap{fmt.Sprintf("%s heads row %d and row %d; a unit heads one row", r.Unit, j+1, i+1), []int{j + 1, i + 1}})
		}
		for _, col := range slices.Sorted(maps.Keys(r.Cells)) {
			if !slices.ContainsFunc(cols, func(c Column) bool { return c.Name == col }) {
				gaps = append(gaps, Gap{fmt.Sprintf("row %d fills a cell under %q, which no column names", i+1, col), []int{i + 1}})
			}
		}
	}
	for _, c := range cols {
		if c.Unit == "" {
			continue
		}
		if _, ok := st[c.Unit]; ok {
			continue
		}
		var under, every []int
		for i, r := range rows {
			every = append(every, i+1)
			if _, ok := r.Cells[c.Name]; ok {
				under = append(under, i+1)
			}
		}
		if len(under) == 0 {
			under = every
		}
		gaps = append(gaps, Gap{fmt.Sprintf("%s, which column %q names, is no longer in the state", c.Unit, c.Name), under})
	}
	return gaps
}

// Reading is one cell under a unit's column read against the stored
// relations: the strongest live relation by which the row's unit, or a unit
// inside it, reaches the column's unit or a unit inside it. Kind is "" when
// none does. Row is the row's place from 1. A cell that says something with
// no relation under it is unbacked, and an empty cell over a relation is
// unsaid.
type Reading struct {
	Row          int
	From, To     ID
	Column, Says string
	Kind         RelationKind
	Via          Via
	Origin       Origin
}

func (r Reading) Unbacked() bool { return r.Says != "" && r.Kind == "" }
func (r Reading) Unsaid() bool   { return r.Says == "" && r.Kind != "" }

// Readings reads every cell under a unit's column, filled or empty, row by
// row and column by column. A row or column whose unit the state lost reads
// nothing, since the table is STALE for it already.
func Readings(cols []Column, rows []Row, units []Unit) []Reading {
	st := stateOf(units)
	var out []Reading
	for i, r := range rows {
		if _, ok := st[r.Unit]; !ok {
			continue
		}
		for _, c := range cols {
			if _, ok := st[c.Unit]; !ok || c.Unit == "" || c.Unit == r.Unit {
				continue
			}
			rd := Reading{Row: i + 1, From: r.Unit, To: c.Unit, Column: c.Name, Says: r.Cells[c.Name]}
			if rel, ok := st.strongest(r.Unit, c.Unit); ok {
				rd.Kind, rd.Via, rd.Origin = rel.Kind, rel.Via, rel.Origin
			}
			out = append(out, rd)
		}
	}
	return out
}
