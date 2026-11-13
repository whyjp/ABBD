package relate

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"

	"github.com/whyjp/abbd/internal/extract"
	"github.com/whyjp/abbd/internal/extract/frontend"
	"github.com/whyjp/abbd/internal/model"
)

// rustFixture is testdata/rs, a Cargo workspace: app calls engine and,
// with the fancy feature, fancy; it uses shapes only for a type and a
// constant and plugin only for a unit struct; app's net module calls a
// private fn of app and calls util; plugin implements engine's trait and
// invokes its macro; app's cfg(windows) module calls util, and its test
// module and integration test use extra, and none of those is read.
const rustFixture = "testdata/rs"

func rustUnits() []extract.DocUnit {
	return []extract.DocUnit{
		mod("mod-app", "app"), mod("mod-net", "app/src/net"), mod("mod-engine", "engine"), mod("mod-plugin", "plugin"),
		mod("mod-shapes", "shapes"), mod("mod-util", "util"), mod("mod-extra", "extra"), mod("mod-fancy", "fancy"),
	}
}

func needRust(t *testing.T) {
	t.Helper()
	if _, err := rustAnalyzer(); err != nil {
		t.Skip(err)
	}
	if _, err := exec.LookPath("cargo"); err != nil {
		t.Skip(err)
	}
}

func relateRust(t *testing.T, doc extract.Doc) (extract.Doc, []Unread) {
	t.Helper()
	needRust(t)
	s, err := ParseSpec("rust")
	if err != nil {
		t.Fatal(err)
	}
	out, unread, err := Relate(rustFixture, doc, []frontend.Spec{s})
	if err != nil {
		t.Fatal(err)
	}
	return out, unread
}

func lines(doc extract.Doc) []string {
	var out []string
	for _, r := range doc.Relations {
		var ev []string
		for _, e := range r.Evidence {
			ev = append(ev, e.Path+":"+e.Lines+":"+e.Quote)
		}
		w := ""
		if r.When != "" {
			w = " when " + r.When
		}
		out = append(out, fmt.Sprintf("%s %s %s via %s%s %v", r.From, r.Kind, r.To, r.Via, w, ev))
	}
	slices.Sort(out)
	return out
}

// One run checks the relations, the unread report and the ownership rule,
// since each run starts rust-analyzer twice.
func TestRustRelationsSayHowEachUnitUsesAnother(t *testing.T) {
	ifc := extract.DocUnit{Ref: "if-speaker", Kind: model.Interface, Name: "Speaker", Anchors: []model.Anchor{{Path: "engine/src/lib.rs", Symbol: "Speaker"}}}
	doc := extract.Doc{Units: append(rustUnits(), ifc), Relations: []extract.DocRelation{
		{From: "mod-app", To: "mod-extra", Kind: model.DependsOn, Via: "import"},
		{From: "mod-app", To: "mod-util", Kind: model.DependsOn, Via: "build-feature"},
	}}
	out, unread := relateRust(t, doc)
	got := lines(out)
	want := []string{
		"mod-app depends_on mod-engine via call [app/src/lib.rs:12:engine::run(&dog)]",
		"mod-app depends_on mod-fancy via call when all features [app/src/lib.rs:21:fancy::glow()]",
		"mod-app depends_on mod-plugin via import [app/src/lib.rs:9:let dog = plugin::Dog;]",
		"mod-app depends_on mod-shapes via import [app/src/lib.rs:1:use shapes::{Square, MAX}; app/src/lib.rs:10:let sq = Square { n: MAX };]",
		"mod-app depends_on mod-util via build-feature []",
		"mod-engine consumes if-speaker via trait [engine/src/lib.rs:6:s.speak()]",
		"mod-net depends_on mod-app via call [app/src/net/mod.rs:2:super::helper() + util::wide()]",
		"mod-net depends_on mod-util via call [app/src/net/mod.rs:2:super::helper() + util::wide()]",
		"mod-plugin depends_on mod-engine via macro [plugin/src/lib.rs:7:engine::loud!(\"woof\")]",
		"mod-plugin provides if-speaker via trait [plugin/src/lib.rs:5:impl Speaker for Dog {]",
	}
	if !slices.Equal(got, want) {
		t.Errorf("relations:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if len(unread) != 1 || unread[0].Relater != "rust" || !slices.Equal(unread[0].Paths, []string{"app/src/win.rs"}) {
		t.Errorf("unread = %+v, want app/src/win.rs from rust", unread)
	}
	if _, err := os.Stat(rustFixture + "/target"); err == nil {
		t.Error("the relater built the workspace into target/")
	}
}

func TestRustTraitImplementationWithoutAContractIsADependency(t *testing.T) {
	out, _ := relateRust(t, extract.Doc{Units: rustUnits()})
	got := lines(out)
	if !slices.Contains(got, "mod-plugin depends_on mod-engine via trait [plugin/src/lib.rs:5:impl Speaker for Dog {]") {
		t.Errorf("the trait implementation is missing:\n%s", strings.Join(got, "\n"))
	}
	var again bytes.Buffer
	out2, _ := relateRust(t, extract.Doc{Units: rustUnits()})
	var first bytes.Buffer
	if err := out.Encode(&first); err != nil {
		t.Fatal(err)
	}
	if err := out2.Encode(&again); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.Bytes(), again.Bytes()) {
		t.Errorf("two runs differ:\n%s\n%s", first.String(), again.String())
	}
}

func TestRustSourceTellsCodeFromUsesAttributesAndComments(t *testing.T) {
	src := "use a::{b,\n  c};\n// use d;\nfn f() { let s = \"use x;\"; g(r#\"use\"#, 'u', b'\\''); use e::h; h::<'a>() }\nfn k() -> impl T + use<'a> { m() }\npub(crate) use n::o;\npub use p::q;\n#[derive(Ser)]\n#![a(b = \"]\")] x();\n"
	s := readRust([]byte(src))
	cases := []struct {
		line, col int
		want      span
	}{
		{0, 0, inUse}, {0, 8, inUse}, {1, 2, inUse}, {2, 8, inText},
		{3, 18, inText}, {3, 13, inCode}, {3, 31, inText}, {3, 52, inUse}, {3, 62, inCode}, {4, 30, inCode},
		{5, 15, inExport}, {6, 8, inExport}, {7, 9, inAttr}, {8, 9, inText}, {8, 13, inAttr}, {8, 15, inCode},
	}
	for _, c := range cases {
		if got := s.at(c.line, c.col); got != c.want {
			t.Errorf("%d:%d %q = %d, want %d", c.line, c.col, s.lines[c.line][c.col:], got, c.want)
		}
	}
}
