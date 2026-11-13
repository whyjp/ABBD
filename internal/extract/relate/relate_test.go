package relate

import (
	"bytes"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/whyjp/abbd/internal/extract"
	"github.com/whyjp/abbd/internal/extract/frontend"
	"github.com/whyjp/abbd/internal/model"
)

// fixture is testdata/fx: app calls core, calls plugin only through an
// interface value, uses shapes only for a type and a constant, and imports
// util blank; plugin implements core's interface, which core calls through;
// app's test calls extra, which nothing else uses.
const fixture = "testdata/fx"

func mod(ref, dir string) extract.DocUnit {
	return extract.DocUnit{Ref: ref, Kind: model.Module, Name: ref, Anchors: []model.Anchor{{Path: dir}}}
}

func modules() []extract.DocUnit {
	return []extract.DocUnit{
		mod("mod-app", "./app"), mod("mod-core", "core/"), mod("mod-plugin", "plugin"),
		mod("mod-shapes", "shapes"), mod("mod-util", "util"), mod("mod-extra", "extra"),
		mod("mod-docs", "docs"),
	}
}

func run(t *testing.T, doc extract.Doc) []string {
	t.Helper()
	s, err := ParseSpec("go")
	if err != nil {
		t.Fatal(err)
	}
	out, _, err := Relate(fixture, doc, []frontend.Spec{s})
	if err != nil {
		t.Fatal(err)
	}
	var lines []string
	for _, r := range out.Relations {
		var ev []string
		for _, e := range r.Evidence {
			ev = append(ev, e.Path+":"+e.Lines+":"+e.Quote)
		}
		lines = append(lines, fmt.Sprintf("%s %s %s via %s %v", r.From, r.Kind, r.To, r.Via, ev))
	}
	slices.Sort(lines)
	return lines
}

func TestGoRelationsSayHowEachUnitUsesAnother(t *testing.T) {
	got := run(t, extract.Doc{Units: modules()})
	want := []string{
		"mod-app depends_on mod-core via call [app/app.go:13:return s.Speak() + core.Run(s)]",
		"mod-app depends_on mod-plugin via call [app/app.go:13:return s.Speak() + core.Run(s)]",
		"mod-app depends_on mod-shapes via import [app/app.go:10:var b shapes.Box app/app.go:11:b.N = shapes.Max]",
		"mod-app depends_on mod-util via import [app/plugins.go:3:import _ \"example.com/fx/util\"]",
		"mod-plugin depends_on mod-core via trait [plugin/plugin.go:5:type Dog struct{}]",
	}
	if !slices.Equal(got, want) {
		t.Errorf("relations:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestGoInterfaceWithAContractUnitIsProvidedAndConsumed(t *testing.T) {
	ifc := extract.DocUnit{Ref: "if-speaker", Kind: model.Interface, Name: "Speaker", Anchors: []model.Anchor{{Path: "core/core.go", Symbol: "Speaker"}}}
	got := run(t, extract.Doc{Units: append(modules(), ifc)})
	want := []string{
		"mod-app consumes if-speaker via trait [app/app.go:13:return s.Speak() + core.Run(s)]",
		"mod-app depends_on mod-core via call [app/app.go:13:return s.Speak() + core.Run(s)]",
		"mod-app depends_on mod-plugin via call [app/app.go:13:return s.Speak() + core.Run(s)]",
		"mod-app depends_on mod-shapes via import [app/app.go:10:var b shapes.Box app/app.go:11:b.N = shapes.Max]",
		"mod-app depends_on mod-util via import [app/plugins.go:3:import _ \"example.com/fx/util\"]",
		"mod-core consumes if-speaker via trait [core/core.go:8:return s.Speak()]",
		"mod-plugin depends_on mod-core via import [plugin/plugin.go:9:var _ core.Speaker = Dog{}]",
		"mod-plugin provides if-speaker via trait [plugin/plugin.go:5:type Dog struct{}]",
	}
	if !slices.Equal(got, want) {
		t.Errorf("relations:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestGoRelaterReplacesOnlyTheDependsOnBetweenCodeItRead(t *testing.T) {
	ifc := extract.DocUnit{Ref: "if-speaker", Kind: model.Interface, Name: "Speaker", Anchors: []model.Anchor{{Path: "core/core.go", Symbol: "Speaker"}}}
	doc := extract.Doc{Units: append(modules(), ifc), Relations: []extract.DocRelation{
		{From: "mod-app", To: "mod-extra", Kind: model.DependsOn, Via: "call"},
		{From: "mod-app", To: "mod-docs", Kind: model.DependsOn, Via: "data"},
		{From: "mod-plugin", To: "if-speaker", Kind: model.Provides, Via: "call"},
	}}
	got := run(t, doc)
	if slices.ContainsFunc(got, func(l string) bool { return strings.HasPrefix(l, "mod-app depends_on mod-extra ") }) {
		t.Error("kept the extraction's depends_on between two units whose non-test Go code shows none")
	}
	for _, kept := range []string{"mod-app depends_on mod-docs via data []", "mod-plugin provides if-speaker via call []"} {
		if !slices.Contains(got, kept) {
			t.Errorf("lost or rewrote %q, which the extraction owns:\n%s", kept, strings.Join(got, "\n"))
		}
	}
	if !slices.Contains(got, "mod-app depends_on mod-core via call [app/app.go:13:return s.Speak() + core.Run(s)]") {
		t.Errorf("the relater's own depends_on is missing:\n%s", strings.Join(got, "\n"))
	}
}

func TestGoRelaterKeepsNonCodeRelationsBetweenUnitsItRead(t *testing.T) {
	doc := extract.Doc{Units: modules(), Relations: []extract.DocRelation{
		{From: "mod-app", To: "mod-core", Kind: model.DependsOn, Via: "config"},
		{From: "mod-core", To: "mod-shapes", Kind: model.DependsOn, Via: "spawn"},
		{From: "mod-app", To: "mod-plugin", Kind: model.DependsOn, Via: "data"},
		{From: "mod-app", To: "mod-extra", Kind: model.DependsOn, Via: "call"},
	}}
	got := run(t, doc)
	for _, kept := range []string{"mod-app depends_on mod-core via config []", "mod-core depends_on mod-shapes via spawn []", "mod-app depends_on mod-plugin via data []"} {
		if !slices.Contains(got, kept) {
			t.Errorf("lost %q, a relation the code cannot show:\n%s", kept, strings.Join(got, "\n"))
		}
	}
	if slices.ContainsFunc(got, func(l string) bool { return strings.HasPrefix(l, "mod-app depends_on mod-extra ") }) {
		t.Error("kept a code-level call the relater read no sign of")
	}
}

func TestGoRelaterCountsFilesSkippedByBuildConstraints(t *testing.T) {
	s, _ := ParseSpec("go")
	_, skipped, err := Relate(fixture, extract.Doc{Units: modules()}, []frontend.Spec{s})
	if err != nil {
		t.Fatal(err)
	}
	if skipped != 1 {
		t.Errorf("skipped = %d, want 1 (util/util_windows.go)", skipped)
	}
}

func TestGoRelationsPrintTheSameBytesEveryRun(t *testing.T) {
	s, _ := ParseSpec("go")
	var outs [2]bytes.Buffer
	for i := range outs {
		doc, _, err := Relate(fixture, extract.Doc{Units: modules()}, []frontend.Spec{s})
		if err != nil {
			t.Fatal(err)
		}
		if err := doc.Encode(&outs[i]); err != nil {
			t.Fatal(err)
		}
	}
	if !bytes.Equal(outs[0].Bytes(), outs[1].Bytes()) {
		t.Errorf("two runs differ:\n%s\n%s", outs[0].String(), outs[1].String())
	}
}

func TestUnknownRelaterIsRefused(t *testing.T) {
	if _, err := ParseSpec("lsp:rust"); err == nil || !strings.Contains(err.Error(), "unknown relater") {
		t.Errorf("err = %v, want an unknown relater", err)
	}
}
