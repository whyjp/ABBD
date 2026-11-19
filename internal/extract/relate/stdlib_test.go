package relate

import (
	"slices"
	"strings"
	"testing"

	"github.com/whyjp/abbd/internal/extract"
	"github.com/whyjp/abbd/internal/extract/frontend"
)

// The standard library, the runtime, system headers and third-party packages
// never become units, relations or contracts: each fixture under testdata/std-*
// is an app and a lib that use their language's standard library and a
// third-party package, and the one relation is the app's use of lib.

func stdUnits() []extract.DocUnit {
	return []extract.DocUnit{mod("mod-app", "app"), mod("mod-lib", "lib")}
}

func onlyTheRepoOwn(t *testing.T, in, out extract.Doc, want []string, outside ...string) {
	t.Helper()
	if got := lines(out); !slices.Equal(got, want) {
		t.Errorf("relations:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if len(out.Units) != len(in.Units) {
		t.Errorf("%d units, want the %d given: the relater made a unit", len(out.Units), len(in.Units))
	}
	own := map[string]bool{}
	for _, u := range in.Units {
		own[u.Ref] = true
	}
	var seen []string
	for _, u := range out.Units {
		if !own[u.Ref] {
			t.Errorf("unit %q is not one the repo's own", u.Ref)
		}
		seen = append(seen, u.Ref, u.Name)
		for _, a := range u.Anchors {
			seen = append(seen, a.Path, a.Symbol)
		}
	}
	for _, r := range out.Relations {
		if !own[string(r.From)] || !own[string(r.To)] {
			t.Errorf("relation %s -> %s has an end that is not the repo's own", r.From, r.To)
		}
		seen = append(seen, string(r.From), string(r.To), string(r.Via))
	}
	for _, s := range seen {
		for _, name := range outside {
			if strings.Contains(s, name) {
				t.Errorf("%q mentions %q, which is not the repo's own", s, name)
			}
		}
	}
}

func TestGoStandardLibraryImportsAreNoUnitOrRelation(t *testing.T) {
	s, err := ParseSpec("go")
	if err != nil {
		t.Fatal(err)
	}
	doc := extract.Doc{Units: stdUnits()}
	out, _, err := Relate("testdata/std-go", t.TempDir(), nil, doc, []frontend.Spec{s})
	if err != nil {
		t.Fatal(err)
	}
	onlyTheRepoOwn(t, doc, out, []string{"mod-app depends_on mod-lib via call [app/app.go:12:return lib.Name()]"}, "fmt", "os")
}

func TestCSystemHeadersAreNoUnitOrRelation(t *testing.T) {
	needC(t, true)
	doc := extract.Doc{Units: stdUnits()}
	out, _ := relateC(t, "testdata/std-c", "c", doc)
	onlyTheRepoOwn(t, doc, out, []string{"mod-app depends_on mod-lib via call [app/main.c:9:printf(\"%s %d\\n\", a, lib_size());]"}, "stdio", "string", "printf", "memcpy", "strlen")
}

func TestRustStandardLibraryUsesAreNoUnitOrRelation(t *testing.T) {
	doc := extract.Doc{Units: stdUnits()}
	needRust(t)
	s, err := ParseSpec("rust")
	if err != nil {
		t.Fatal(err)
	}
	out, _, err := Relate("testdata/std-rs", t.TempDir(), nil, doc, []frontend.Spec{s})
	if err != nil {
		t.Fatal(err)
	}
	onlyTheRepoOwn(t, doc, out, []string{"mod-app depends_on mod-lib via call [app/src/lib.rs:5:m.insert(lib::name(), 1);]"}, "std", "HashMap", "collections")
}

func TestTSNodeBuiltinsAndPackagesAreNoUnitOrRelation(t *testing.T) {
	needTS(t)
	doc := extract.Doc{Units: scriptUnits("app", "lib")}
	out, _ := relateScript(t, "testdata/std-ts", "ts", doc)
	onlyTheRepoOwn(t, doc, out, []string{"mod-app depends_on mod-lib via call [app/main.ts:6:export const out: string = pad(path.join(\"a\", fs.existsSync(\"b\") ? \"c\" : left(\"d\")));]"}, "node:fs", "fs", "path", "leftpad", "node_modules")
}

func TestPythonStandardLibraryAndSitePackagesAreNoUnitOrRelation(t *testing.T) {
	needPython(t)
	doc := extract.Doc{Units: scriptUnits("app", "lib")}
	out, _ := relateScript(t, "testdata/std-py", "python", doc)
	onlyTheRepoOwn(t, doc, out, []string{"mod-app depends_on mod-lib via call [app/main.py:7:out = json.dumps([os.sep, sys.platform, name()])]"}, "json", "os", "sys", "thirdparty", "site-packages")
}
