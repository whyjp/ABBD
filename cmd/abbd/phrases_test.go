package main

import (
	"slices"
	"testing"

	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/screen"
)

// Lint names each pair of overview boxes whose lines carry no phrase,
// counting a contract on the box of the module that provides it and a
// unit inside a module on that module's box.
func TestUnphrasedNamesTheOverviewLinesWithNoPhrase(t *testing.T) {
	unit := func(id, name, parent string) model.Unit {
		return model.Unit{ID: model.ID(id), Name: model.Said(model.Script, name), Parent: model.ID(parent)}
	}
	api, billing := unit("MOD-0001", "api", ""), unit("MOD-0002", "billing", "")
	billing.Relations = []model.Relation{{To: "IF-0001", Kind: model.Provides, Writer: model.Script}}
	api.Phrases = []model.PhraseNote{{To: "IF-0001", Kind: model.Consumes, Text: "charge request", Writer: model.Reader,
		Evidence: []model.Evidence{{Path: "api/a.go", Quote: "Charge("}}}}
	units := []model.Unit{api, billing, unit("MOD-0003", "ledger", ""), unit("IF-0001", "charge", ""), unit("MOD-0005", "post", "MOD-0003")}
	d := screen.Drawing{Wires: []screen.Wire{
		{Rels: []string{"MOD-0001.consumes.IF-0001"}},
		{Rels: []string{"MOD-0002.depends_on.MOD-0005", "MOD-0002.depends_on.MOD-0003"}},
		{Rels: []string{"MOD-0002.provides.IF-0001"}},
	}}
	got := unphrased(units, d)
	want := []string{"MOD-0002 billing -> MOD-0003 ledger (2 relations)"}
	if !slices.Equal(got, want) {
		t.Errorf("unphrased = %q, want %q", got, want)
	}
}
