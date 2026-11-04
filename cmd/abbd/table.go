package main

import (
	"fmt"

	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/store"
)

// table stores a table from a draft, or prints the stored one, as figure
// does for every kind, then says how many cells under a unit's column
// disagree with the stored relations: on stdout after a store, on stderr
// after a print.
func (e env) table(key string, args []string) error {
	if err := figure[model.Row](e, key, args); err != nil {
		return err
	}
	t, _, err := store.LoadFigure[model.Row](e.st, model.FigureKey(key))
	if err != nil {
		return err
	}
	units, err := e.st.LoadUnits()
	if err != nil {
		return err
	}
	out := e.stdout
	if len(args) == 0 {
		out = e.stderr
	}
	unbacked, unsaid := cellCounts(model.Readings(t.Columns, t.Claims(), units))
	fmt.Fprintf(out, "cells: %d cell(s) say a use no stored relation carries, %d empty cell(s) sit over a stored relation; abbd status lists them\n", unbacked, unsaid)
	return nil
}

func cellCounts(rs []model.Reading) (unbacked, unsaid int) {
	for _, r := range rs {
		switch {
		case r.Unbacked():
			unbacked++
		case r.Unsaid():
			unsaid++
		}
	}
	return unbacked, unsaid
}

// statusCells reports each cell under a unit's column that the stored
// relations do not bear out: one that says a use no relation carries, and an
// empty one over a relation. Either is a relation not yet recorded, a cell
// read wrong, or a use the table missed, and only a reader of the code can
// say which, so it never fails the check.
func (e env) statusCells(units []model.Unit, tables []store.Table) {
	name := map[model.ID]string{}
	for _, u := range units {
		name[u.ID] = u.Name.Value
	}
	var lines []string
	var unbacked, unsaid, known int
	for _, t := range tables {
		if !t.Known {
			continue
		}
		known++
		for _, r := range model.Readings(t.Columns, t.Claims(), units) {
			switch {
			case r.Unbacked():
				unbacked++
				lines = append(lines, fmt.Sprintf("unbacked: table %s row %d: %s %s says %q under %s %s, and no stored relation joins them; record the relation, or read the cell again", t.Key, r.Row, r.From, name[r.From], r.Says, r.To, name[r.To]))
			case r.Unsaid():
				unsaid++
				lines = append(lines, fmt.Sprintf("unsaid: table %s row %d: %s %s %s %s %s (%s), and the cell under %q is empty; fill it when the row's code makes that use", t.Key, r.Row, r.From, name[r.From], r.Kind, r.To, name[r.To], r.Origin, r.Column))
			}
		}
	}
	if known == 0 {
		return
	}
	fmt.Fprintf(e.stdout, "cells: %d cell(s) say a use no stored relation carries, %d empty cell(s) sit over a stored relation; reported, not checked\n", unbacked, unsaid)
	for _, l := range lines {
		fmt.Fprintln(e.stdout, l)
	}
}
