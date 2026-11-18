package frontend

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/whyjp/abbd/internal/extract"
)

var updateMSBuild = flag.Bool("update-msbuild", false, "rewrite the msbuild golden files")

// msbuildFixture is testdata/msbuild, a solution of Windows-style paths:
// app links core by ProjectReference and plugin through plug.lib under
// x64 only, plugin links core.lib, tests and tools are left out, and the
// sources and projects are spelled in another case than they lie on disk.
func msbuildFixture(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("testdata", "msbuild"))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func extractMSBuild(t *testing.T, target string) (string, string) {
	t.Helper()
	doc, err := Extract(msbuildFixture(t), t.TempDir(), []Spec{{"msbuild", target}})
	if err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	if err := doc.Encode(&b); err != nil {
		t.Fatal(err)
	}
	if _, err := extract.Parse(bytes.NewReader(b.Bytes())); err != nil {
		t.Fatalf("setup would refuse the output: %v\n%s", err, b.String())
	}
	return b.String(), strings.Join(doc.Notes, "\n") + "\n"
}

func golden(t *testing.T, name, got string) {
	t.Helper()
	p := filepath.Join("testdata", name)
	if *updateMSBuild {
		if err := os.WriteFile(p, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if w := strings.ReplaceAll(string(want), "\r\n", "\n"); w != got {
		t.Fatalf("%s differs; run go test -run MSBuild -update-msbuild after checking the change:\n--- want\n%s\n--- got\n%s", name, w, got)
	}
}

func TestAMSBuildSolutionIsOneModulePerProjectWithItsArtifactsAndLinks(t *testing.T) {
	out, notes := extractMSBuild(t, "app.sln")
	golden(t, "msbuild.golden.json", out)
	golden(t, "msbuild.golden.notes", notes)
}

func TestAMSBuildRunPrintsTheSameBytesTwice(t *testing.T) {
	a, an := extractMSBuild(t, "app.sln")
	b, bn := extractMSBuild(t, "app.sln")
	if a != b || an != bn {
		t.Fatalf("two runs differ:\n%s\n---\n%s", a, b)
	}
}

func TestAMSBuildProjectFollowsItsReferencesAndKeepsItself(t *testing.T) {
	out, _ := extractMSBuild(t, "tests/tests.vcxproj")
	for _, want := range []string{`"ref": "msbuild:tests"`, `"ref": "msbuild:core"`, `"to": "msbuild:core"`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %s in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "msbuild:app") {
		t.Errorf("a project nothing references was read:\n%s", out)
	}
}

func TestAMSBuildSpecNamingNoProjectFileFails(t *testing.T) {
	for _, target := range []string{"app", "props/common.props", "missing.sln"} {
		_, err := Extract(msbuildFixture(t), t.TempDir(), []Spec{{"msbuild", target}})
		if err == nil || !strings.Contains(err.Error(), "no .sln, .slnx or .vcxproj file") {
			t.Errorf("%s: err = %v", target, err)
		}
	}
}

func TestAMSBuildConditionsReadTheCommonCases(t *testing.T) {
	m := newMSBuilder(msbuildFixture(t))
	e := &evalCtx{m: m, p: &msProject{dir: m.root}, props: map[string]string{}}
	for cond, want := range map[string]bool{
		`'Debug|x64'=='debug|X64'`:                      true,
		`'a' != 'a'`:                                    false,
		`'1'=='1' And !('x'=='y')`:                      true,
		`'a'=='b' Or 'c'=='c'`:                          true,
		`Exists('app\app.vcxproj')`:                     true,
		`exists('APP/APP.VCXPROJ')`:                     true,
		`Exists('app\none.vcxproj')`:                    false,
		`HasTrailingSlash('a\')`:                        true,
		`'a'=='b' Or 'c'=='d' Or 'e'=='e' And 'f'=='f'`: true,
	} {
		got, err := e.condition(cond)
		if err != nil || got != want {
			t.Errorf("%s = %v, %v; want %v", cond, got, err, want)
		}
	}
	if _, err := e.condition(`$([MSBuild]::IsOSPlatform('Windows'))`); err == nil {
		t.Error("an MSBuild function was read")
	}
}

func TestAMSBuildFileInUTF16WithABOMIsRead(t *testing.T) {
	var b []byte
	b = append(b, 0xFF, 0xFE)
	for _, r := range "<?xml version=\"1.0\" encoding=\"utf-16\"?><Project><PropertyGroup><A>1</A></PropertyGroup></Project>" {
		b = append(b, byte(r), byte(r>>8))
	}
	n, err := parseXML(b)
	if err != nil || n.name != "project" || len(n.kids) != 1 || n.kids[0].kids[0].text != "1" {
		t.Fatalf("%+v, %v", n, err)
	}
}
