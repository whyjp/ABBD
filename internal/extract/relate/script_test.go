package relate

import (
	"bytes"
	"cmp"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/whyjp/abbd/internal/extract"
	"github.com/whyjp/abbd/internal/extract/frontend"
	"github.com/whyjp/abbd/internal/extract/lsp"
	"github.com/whyjp/abbd/internal/model"
)

// tsFixture is testdata/ts: app calls util and lib's JavaScript, creates a
// Circle and annotates with shapes' interface; circle extends and
// implements shapes' types; circle's test calls util, and is left out.
// pyFixture is testdata/py, the same in Python, with a private name.
const (
	tsFixture = "testdata/ts"
	pyFixture = "testdata/py"
)

func scriptUnits(dirs ...string) []extract.DocUnit {
	var out []extract.DocUnit
	for _, d := range dirs {
		out = append(out, mod("mod-"+d, d))
	}
	return out
}

// needScript skips the test when the server is missing, as the CI step that
// installs the pinned one fails on any skip.
func needScript(t *testing.T, check func() error) {
	t.Helper()
	if err := check(); err != nil {
		t.Skip(err)
	}
}

func needTS(t *testing.T) {
	needScript(t, func() error {
		_, err := scriptServer("ABBD_LSP_TS", "typescript-language-server", tsInstall, true)
		return err
	})
}

func needPython(t *testing.T) {
	needScript(t, func() error {
		_, err := scriptServer("ABBD_LSP_PYTHON", "pyright-langserver", pythonInstall, false)
		return err
	})
}

func relateScript(t *testing.T, root, spec string, doc extract.Doc) (extract.Doc, []Unread) {
	t.Helper()
	return relateCIn(t, root, t.TempDir(), spec, doc)
}

func TestTSRelationsSayHowEachUnitUsesAnother(t *testing.T) {
	needTS(t)
	slot := extract.DocUnit{Ref: "if-shape", Kind: model.Interface, Name: "Shape", Anchors: []model.Anchor{{Path: "shapes/shape.ts", Symbol: "Shape"}}}
	doc := extract.Doc{Units: append(scriptUnits("app", "circle", "lib", "shapes", "util"), slot)}
	var outs [2]bytes.Buffer
	for i := range outs {
		out, unread := relateScript(t, tsFixture, "ts", doc)
		if err := out.Encode(&outs[i]); err != nil {
			t.Fatal(err)
		}
		if i > 0 {
			continue
		}
		want := []string{
			"mod-app consumes if-shape via import [app/main.ts:3:import type { Shape } from \"../shapes/shape\"; app/main.ts:6:const s: Shape = new Circle(2);]",
			"mod-app depends_on mod-circle via call [app/main.ts:6:const s: Shape = new Circle(2);]",
			"mod-app depends_on mod-lib via call [app/main.ts:7:export const total: number = add(LIMIT, s.area()) + twice(1);]",
			"mod-app depends_on mod-shapes via import [app/main.ts:3:import type { Shape } from \"../shapes/shape\"; app/main.ts:6:const s: Shape = new Circle(2);]",
			"mod-app depends_on mod-util via call [app/main.ts:7:export const total: number = add(LIMIT, s.area()) + twice(1);]",
			"mod-circle depends_on mod-shapes via trait [circle/circle.ts:3:export class Circle extends Base implements Shape {]",
			"mod-circle provides if-shape via trait [circle/circle.ts:3:export class Circle extends Base implements Shape {]",
		}
		if got := lines(out); !slices.Equal(got, want) {
			t.Errorf("relations:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
		}
		if len(unread) != 0 {
			t.Errorf("unread = %+v, want none", unread)
		}
	}
	if !bytes.Equal(outs[0].Bytes(), outs[1].Bytes()) {
		t.Errorf("two runs differ:\n%s\n%s", outs[0].String(), outs[1].String())
	}
}

func TestPythonRelationsSayHowEachUnitUsesAnother(t *testing.T) {
	needPython(t)
	slot := extract.DocUnit{Ref: "if-shape", Kind: model.Interface, Name: "Shape", Anchors: []model.Anchor{{Path: "shapes/shape.py", Symbol: "Shape"}}}
	doc := extract.Doc{Units: append(scriptUnits("app", "circle", "priv", "shapes", "util"), slot)}
	var outs [2]bytes.Buffer
	for i := range outs {
		out, unread := relateScript(t, pyFixture, "python", doc)
		if err := out.Encode(&outs[i]); err != nil {
			t.Fatal(err)
		}
		if i > 0 {
			continue
		}
		// priv is used only through a name that starts with an underscore.
		want := []string{
			"mod-app consumes if-shape via import [app/main.py:6:s: shape.Shape = Circle(2)]",
			"mod-app depends_on mod-circle via call [app/main.py:6:s: shape.Shape = Circle(2)]",
			"mod-app depends_on mod-shapes via import [app/main.py:6:s: shape.Shape = Circle(2)]",
			"mod-app depends_on mod-util via call [app/main.py:7:total = add(LIMIT, s.area()) + _secret()]",
			"mod-circle depends_on mod-shapes via trait [circle/circle.py:4:class Circle(Shape):]",
			"mod-circle provides if-shape via trait [circle/circle.py:4:class Circle(Shape):]",
		}
		if got := lines(out); !slices.Equal(got, want) {
			t.Errorf("relations:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
		}
		if len(unread) != 0 {
			t.Errorf("unread = %+v, want none", unread)
		}
	}
	if !bytes.Equal(outs[0].Bytes(), outs[1].Bytes()) {
		t.Errorf("two runs differ:\n%s\n%s", outs[0].String(), outs[1].String())
	}
}

func TestRefViaReadsWhatTheCodeDoesWithTheName(t *testing.T) {
	for _, c := range []struct {
		line, name string
		want       model.Via
		above      []string
		file       string
	}{
		{"const s = new Circle(2);", "Circle", "call", nil, ""},
		{"export const t = add(LIMIT, 1);", "add", "call", nil, ""},
		{"const v = pick<number>(1);", "pick", "call", nil, ""},
		{"import { add, LIMIT } from \"../util/util\";", "add", "import", nil, ""},
		{"const s: Shape = make();", "Shape", "import", nil, ""},
		{"export class Circle extends Base implements Shape {", "Base", "trait", nil, ""},
		{"export class Circle extends Base implements Shape {", "Shape", "trait", nil, ""},
		{"export interface Round extends Shape {", "Shape", "trait", nil, ""},
		{"class Circle(Shape):", "Shape", "trait", nil, "a.py"},
		{"class Circle(Base, Shape):", "Shape", "trait", nil, "a.py"},
		{"s = Circle(2)", "Circle", "call", nil, "a.py"},
		{"@app.get(\"/\")", "app", "import", nil, "a.py"},
		{"@register", "register", "call", nil, "a.py"},
		{"x = Shape  # class A(Shape):", "Shape", "import", nil, "a.py"},
		{"from util.maths import add", "add", "import", nil, ""},
		{"export class A extends Base<Foo> {", "Foo", "import", nil, ""},
		{"export class A extends Base<Foo> {", "Base", "trait", nil, ""},
		{"class A<T extends Foo> implements Base {", "Foo", "import", nil, ""},
		{"class A<T extends Foo> implements Base {", "Base", "trait", nil, ""},
		{"  Base,", "Base", "trait", []string{"export class A", "  extends Other,"}, ""},
		{"  implements Shape {", "Shape", "trait", []string{"export class A"}, ""},
		{"const x = Foo;", "Foo", "import", []string{"class A extends Base {", "}"}, ""},
		{"class A(Base, metaclass=Meta):", "Meta", "import", nil, "a.py"},
		{"class A(Base, metaclass=Meta):", "metaclass", "import", nil, "a.py"},
		{"class A(Base, metaclass=Meta):", "Base", "trait", nil, "a.py"},
		{"class A(Generic[T], Base):", "T", "import", nil, "a.py"},
		{"class A(Generic[T], Base):", "Generic", "trait", nil, "a.py"},
		{"    Base,", "Base", "trait", []string{"class A(", "    Other,"}, "a.py"},
		{"    x = helper(Foo)", "Foo", "import", []string{"class A(Base):", "    pass"}, "a.py"},
		{"const v = <Circle r={1} />;", "Circle", "call", nil, "a.tsx"},
		{"const v: Array<Circle> = [];", "Circle", "import", nil, "a.tsx"},
	} {
		i := strings.Index(c.line, c.name)
		if got := refVia(append(append([]string{}, c.above...), c.line), len(c.above), i, i+len(c.name), cmp.Or(c.file, "a.ts")); got != c.want {
			t.Errorf("refVia(%q, %s) = %q, want %q", c.line, c.name, got, c.want)
		}
	}
	if got := refVia([]string{"short"}, 0, 3, 99, "a.ts"); got != "import" {
		t.Errorf("a range past the line = %q, want import", got)
	}
}

func TestScriptTestPathsAreLeftOut(t *testing.T) {
	for p, want := range map[string]bool{
		"src/a.test.ts": true, "src/a.spec.js": true, "src/__tests__/a.ts": true, "src/__mocks__/a.ts": true, "test/a.ts": true,
		"e2e/a.ts": true, "src/e2e/b.ts": true, "src/latest.ts": false, "src/contest.ts": false, "src/a.ts": false,
	} {
		if tsTest(p) != want {
			t.Errorf("tsTest(%q) = %v, want %v", p, !want, want)
		}
	}
	for p, want := range map[string]bool{
		"tests/test_a.py": true, "pkg/test_a.py": true, "pkg/a_test.py": true, "pkg/conftest.py": true, "docs_src/a.py": false, "pkg/latest.py": false,
	} {
		if pyTest(p) != want {
			t.Errorf("pyTest(%q) = %v, want %v", p, !want, want)
		}
	}
}

// Files come back sorted and root-relative, with dependencies, build
// output, hidden directories, declaration files and tests left out, the
// same on every OS.
func TestScriptFilesSkipVendoredAndHiddenDirectories(t *testing.T) {
	root := t.TempDir()
	for _, f := range []string{
		"b/z.ts", "b/a.tsx", "b/types.d.ts", "b/a.test.ts", "b/node_modules/dep/index.ts", "b/dist/out.js", "b/.cache/x.ts", "b/Build/x.ts",
		"b/sub/m.mjs", "b/notes.md", "c/venv/lib/site-packages/x.py", "c/pkg/m.py", "c/pkg/__pycache__/m.py", "c/pkg.egg-info/x.py",
		"c/tests/test_m.py", "outside/o.ts",
	} {
		p := filepath.Join(root, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	o := index([]extract.DocUnit{mod("mod-b", "b"), mod("mod-c", "c")})
	got, err := scriptFiles(tree{root: root, dir: root}, o, tsLang)
	if want := []string{"b/a.tsx", "b/sub/m.mjs", "b/z.ts"}; err != nil || !slices.Equal(got, want) {
		t.Errorf("ts files = %v, %v, want %v", got, err, want)
	}
	got, err = scriptFiles(tree{root: root, dir: root}, o, pythonLang)
	if want := []string{"c/pkg/m.py"}; err != nil || !slices.Equal(got, want) {
		t.Errorf("python files = %v, %v, want %v", got, err, want)
	}
}

func TestScriptWithoutAServerSaysHowToGetIt(t *testing.T) {
	t.Setenv("ABBD_LSP_TS", filepath.Join(t.TempDir(), "no-tsls"))
	t.Setenv("ABBD_LSP_PYTHON", filepath.Join(t.TempDir(), "no-pyright"))
	for _, c := range []struct{ spec, want string }{{"ts", "npm i -g typescript-language-server typescript"}, {"python", "pip install pyright"}} {
		s, err := ParseSpec(c.spec)
		if err != nil {
			t.Fatal(err)
		}
		_, _, err = Relate("testdata/"+map[string]string{"ts": "ts", "python": "py"}[c.spec], t.TempDir(), nil, extract.Doc{}, []frontend.Spec{s})
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want install guidance %q", c.spec, err, c.want)
		}
	}
}

func TestScriptRelatersNeedTheirManifest(t *testing.T) {
	s, err := ParseSpec("python")
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = Relate(t.TempDir(), t.TempDir(), nil, extract.Doc{}, []frontend.Spec{s})
	if err == nil || !strings.Contains(err.Error(), "pyproject.toml") {
		t.Errorf("err = %v, want the missing manifest named", err)
	}
}

func TestDestructuredAndLiteralDeclarationsAreNotAsked(t *testing.T) {
	lines := []string{"const { isFunction } = utils;", "export const [a, b] = pair;", "const f = utils.isFunction;", "export default null;", "export default {", "export default 'x';"}
	for i, want := range []bool{true, true, false, true, false, true} {
		kind := lsp.KindVariable
		if i >= 3 {
			kind = lsp.KindObject
		}
		s := lsp.DocumentSymbol{Kind: kind, Range: lsp.Range{Start: lsp.Position{Line: i}}}
		if got := destructured(lines, s); got != want {
			t.Errorf("destructured(%q) = %v, want %v", lines[i], got, want)
		}
	}
}
