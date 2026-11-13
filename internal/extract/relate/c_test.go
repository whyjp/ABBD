package relate

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/whyjp/abbd/internal/extract"
	"github.com/whyjp/abbd/internal/extract/frontend"
	"github.com/whyjp/abbd/internal/model"
)

// cFixture is testdata/c, a CMake project: app calls core, plugin and
// util, and uses api's function-like macro; util_add is declared in api's
// header and defined in util; plugin fills core's source_info table with
// its own functions and core calls through it; extra defines its own
// struct state, which shares its name with core's; app calls extra only
// under an #ifdef the build leaves unset, and plugin names api's macro only
// in an #if; plugin/legacy.c is in no target. In C++, circle's class
// derives from shapes' and viewer calls through shapes' class.
const cFixture = "testdata/c"

func cUnits() []extract.DocUnit {
	return []extract.DocUnit{
		mod("mod-api", "api"), mod("mod-app", "app"), mod("mod-core", "core"), mod("mod-extra", "extra"),
		mod("mod-plugin", "plugin"), mod("mod-util", "util"), mod("mod-shapes", "shapes"), mod("mod-circle", "circle"),
		mod("mod-viewer", "viewer"),
	}
}

func needC(t *testing.T, cmake bool) {
	t.Helper()
	if _, err := clangdPath(); err != nil {
		t.Skip(err)
	}
	if _, err := exec.LookPath("cmake"); cmake && err != nil {
		t.Skip(err)
	}
}

func relateC(t *testing.T, root, spec string, doc extract.Doc) (extract.Doc, []Unread) {
	t.Helper()
	s, err := ParseSpec(spec)
	if err != nil {
		t.Fatal(err)
	}
	out, unread, err := Relate(root, t.TempDir(), doc, []frontend.Spec{s})
	if err != nil {
		t.Fatal(err)
	}
	return out, unread
}

// One run checks the relations, the contracts and what was not read, since
// each run configures CMake and starts clangd.
func TestCRelationsSayHowEachUnitUsesAnother(t *testing.T) {
	needC(t, true)
	slot := extract.DocUnit{Ref: "if-source", Kind: model.Interface, Name: "source_info", Anchors: []model.Anchor{{Path: "core/core.h", Symbol: "source_info"}}}
	add := extract.DocUnit{Ref: "if-add", Kind: model.Interface, Name: "util_add", Anchors: []model.Anchor{{Path: "api/api.h", Symbol: "util_add"}}}
	shape := extract.DocUnit{Ref: "if-shape", Kind: model.Interface, Name: "Shape", Anchors: []model.Anchor{{Path: "shapes/shape.h", Symbol: "Shape"}}}
	doc := extract.Doc{Units: append(cUnits(), slot, add, shape), Relations: []extract.DocRelation{
		{From: "mod-app", To: "mod-extra", Kind: model.DependsOn, Via: "call"},
		{From: "mod-plugin", To: "mod-api", Kind: model.DependsOn, Via: "import"},
		{From: "mod-app", To: "mod-core", Kind: model.DependsOn, Via: "link"},
	}}
	out, unread := relateC(t, cFixture, "c", doc)
	// app calls extra only under an #ifdef the build leaves unset, so its
	// link goes; plugin names api only in an #if, but plugin/legacy.c went
	// unread and may be where it uses api, so its link stays, as the link
	// the build makes does.
	got := lines(out)
	want := []string{
		"mod-app consumes if-add via call [app/main.c:16:return core_run(&s) + util_add(1, 2) + API_TWICE(orders[0](1, 2));]",
		"mod-app depends_on mod-api via macro [app/main.c:16:return core_run(&s) + util_add(1, 2) + API_TWICE(orders[0](1, 2));]",
		"mod-app depends_on mod-core via link []",
		"mod-app depends_on mod-plugin via call [app/main.c:12:plugin_init();]",
		"mod-app depends_on mod-util via call [app/main.c:16:return core_run(&s) + util_add(1, 2) + API_TWICE(orders[0](1, 2));]",
		"mod-circle depends_on mod-shapes via import [circle/circle.cpp:5:Shape *make_circle(int r) { return new Circle(r); } circle/circle.h:15:Shape *make_circle(int r);]",
		"mod-circle provides if-shape via trait [circle/circle.h:6:class Circle : public Shape {]",
		"mod-core consumes if-source via callback [core/registry.c:12:total += sources[i]->create(n);]",
		"mod-plugin consumes if-add via call [plugin/plugin.c:10:return util_add(l.n, 1);]",
		"mod-plugin depends_on mod-api via import []",
		"mod-plugin depends_on mod-core via call [plugin/plugin.c:23:register_source(&plugin_source);]",
		"mod-plugin depends_on mod-util via call [plugin/plugin.c:10:return util_add(l.n, 1);]",
		"mod-plugin provides if-source via callback [plugin/plugin.c:15:static const struct source_info plugin_source = { plugin/plugin.c:17:plug_create, plugin/plugin.c:18:plug_destroy,]",
		"mod-viewer consumes if-shape via trait [viewer/viewer.cpp:5:int a = s->area();]",
		"mod-viewer depends_on mod-circle via call [viewer/viewer.cpp:4:Shape *s = make_circle(r);]",
		"mod-viewer depends_on mod-shapes via call [viewer/viewer.cpp:5:int a = s->area();]",
	}
	if !slices.Equal(got, want) {
		t.Errorf("relations:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if len(unread) != 2 || !slices.Equal(unread[0].Paths, []string{"plugin/legacy.c"}) || unread[0].Gated ||
		!slices.Equal(unread[1].Paths, []string{"app/main.c (1 lines)"}) || !unread[1].Gated {
		t.Errorf("unread = %+v, want plugin/legacy.c unread and app/main.c gated", unread)
	}
}

// Without a contract unit at the slot type, the host that calls through
// the table depends on the unit whose functions fill it, via callback, and
// two runs print the same bytes.
func TestCTableWithoutAContractIsACallbackFromHostToFiller(t *testing.T) {
	needC(t, true)
	var outs [2]bytes.Buffer
	for i := range outs {
		out, _ := relateC(t, cFixture, "c", extract.Doc{Units: cUnits()})
		if err := out.Encode(&outs[i]); err != nil {
			t.Fatal(err)
		}
		if i > 0 {
			continue
		}
		got := lines(out)
		for _, want := range []string{
			"mod-core depends_on mod-plugin via callback [core/registry.c:12:total += sources[i]->create(n); plugin/plugin.c:17:plug_create, plugin/plugin.c:18:plug_destroy,]",
			"mod-circle depends_on mod-shapes via trait [circle/circle.h:6:class Circle : public Shape {]",
		} {
			if !slices.Contains(got, want) {
				t.Errorf("relations:\n%s\nwant among them:\n%s", strings.Join(got, "\n"), want)
			}
		}
		for _, l := range got {
			if strings.HasPrefix(l, "mod-extra ") || strings.HasPrefix(l, "mod-plugin depends_on mod-api ") || strings.HasPrefix(l, "mod-app depends_on mod-extra ") {
				t.Errorf("%s: a same-named type, a name tested by #if or a call the build leaves out is no relation", l)
			}
		}
	}
	if !bytes.Equal(outs[0].Bytes(), outs[1].Bytes()) {
		t.Errorf("two runs differ:\n%s\n%s", outs[0].String(), outs[1].String())
	}
}

// A directory holding compile_commands.json is read as it is: a source it
// has no entry for is unread, and clangd's index stays out of the
// repository.
func TestCReadsAGivenCompileDatabase(t *testing.T) {
	needC(t, false)
	root := t.TempDir()
	if err := os.CopyFS(root, os.DirFS(cFixture)); err != nil {
		t.Fatal(err)
	}
	var db []map[string]string
	for _, f := range []string{"app/main.c", "core/core.c", "core/registry.c", "plugin/plugin.c", "util/util.c"} {
		db = append(db, map[string]string{"directory": root, "file": f, "command": "cc -Iapi -Icore -Iutil -c " + f})
	}
	b, err := json.Marshal(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "build"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "build", "compile_commands.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	out, unread := relateC(t, root, "c=build", extract.Doc{Units: cUnits()})
	if got := lines(out); !slices.Contains(got, "mod-plugin depends_on mod-util via call [plugin/plugin.c:10:return util_add(l.n, 1);]") {
		t.Errorf("relations:\n%s", strings.Join(got, "\n"))
	}
	if len(unread) == 0 || !slices.Equal(unread[0].Paths, []string{"circle/circle.cpp", "extra/extra.c", "plugin/legacy.c", "viewer/viewer.cpp"}) {
		t.Errorf("unread = %+v, want the four sources it has no entry for", unread)
	}
	if _, err := os.Stat(filepath.Join(root, "build", ".cache")); err == nil {
		t.Error("clangd wrote its index into the repository")
	}
}

func TestCWithoutClangdSaysHowToGetIt(t *testing.T) {
	t.Setenv("ABBD_LSP_C", filepath.Join(t.TempDir(), "no-clangd"))
	_, err := clangdPath()
	if err == nil || !strings.Contains(err.Error(), "apt install clangd") || !strings.Contains(err.Error(), "winget install LLVM.LLVM") {
		t.Errorf("err = %v, want install guidance", err)
	}
}

func TestCTestPathsAreLeftOut(t *testing.T) {
	for p, want := range map[string]bool{
		"db/db_test.cc": true, "test/format-test.cc": true, "util/testutil.cc": false, "lib/compress/zstd_fast.c": false,
		"benchmarks/db_bench.cc": true, "tests/fuzz/x.c": true, "src/contest.c": false,
	} {
		if cTest(p) != want {
			t.Errorf("cTest(%q) = %v, want %v", p, !want, want)
		}
	}
}
