package main

import (
	"slices"
	"testing"

	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/screen"
)

// Lint names each overview line whose relations carry no live phrase, by
// the boxes at its ends: a contract on the box of the module that provides
// it, a unit inside a module, however deep, on that module's box, and a
// unit whose parent is gone on its own.
func TestUnphrasedNamesTheOverviewLinesWithNoLivePhrase(t *testing.T) {
	unit := func(id, name, parent string) model.Unit {
		return model.Unit{ID: model.ID(id), Name: model.Said(model.Script, name), Parent: model.ID(parent)}
	}
	note := func(to string, kind model.RelationKind, text string, s model.Status) model.PhraseNote {
		return model.PhraseNote{To: model.ID(to), Kind: kind, Text: text, Writer: model.Reader,
			Evidence: []model.Evidence{{Path: "a.go", Quote: "x", Status: s}}}
	}
	api, billing, ledger := unit("MOD-0001", "api", ""), unit("MOD-0002", "billing", ""), unit("MOD-0003", "ledger", "")
	billing.Relations = []model.Relation{{To: "IF-0001", Kind: model.Provides, Writer: model.Script}}
	api.Phrases = []model.PhraseNote{note("IF-0001", model.Consumes, "charge request", model.Live)}
	ledger.Phrases = []model.PhraseNote{note("MOD-0001", model.DependsOn, "audit hook", model.Drifted)}
	units := []model.Unit{api, billing, ledger, unit("IF-0001", "charge", ""), unit("MOD-0005", "post", "MOD-0003"),
		unit("MOD-0006", "entry", "MOD-0005"), unit("MOD-0007", "orphan", "MOD-0099")}
	d := screen.Drawing{Wires: []screen.Wire{
		{Rels: []string{"MOD-0001.consumes.IF-0001"}},
		{Rels: []string{"MOD-0002.depends_on.MOD-0006", "MOD-0002.depends_on.MOD-0003"}},
		{Rels: []string{"MOD-0002.provides.IF-0001"}},
		{Rels: []string{"MOD-0003.depends_on.MOD-0001", "MOD-0001.depends_on.MOD-0003"}, Both: true},
		{Rels: []string{"MOD-0007.depends_on.MOD-0001"}},
	}}
	got := unphrased(units, d)
	want := []string{
		"MOD-0001 api - MOD-0003 ledger (2 relations)",
		"MOD-0001 api - MOD-0007 orphan (1 relation)",
		"MOD-0002 billing - MOD-0003 ledger (2 relations)",
	}
	if !slices.Equal(got, want) {
		t.Errorf("unphrased =\n%q\nwant\n%q", got, want)
	}
}
