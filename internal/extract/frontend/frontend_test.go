package frontend

import (
	"bytes"
	"encoding/json"
	"maps"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/whyjp/abbd/internal/extract"
	"github.com/whyjp/abbd/internal/model"
)

// run extracts a fixture with the named front-end, skipping when the build
// tool is not installed, and checks setup would accept the output.
func run(t *testing.T, tool, fixture string) extract.Doc {
	t.Helper()
	if _, err := exec.LookPath(tool); err != nil {
		t.Skipf("%s is not installed", tool)
	}
	root, err := filepath.Abs(filepath.Join("testdata", fixture))
	if err != nil {
		t.Fatal(err)
	}
	spec, err := ParseSpec(tool)
	if err != nil {
		t.Fatal(err)
	}
	cache := t.TempDir()
	var outs [][]byte
	for range 2 {
		doc, err := Extract(root, cache, []Spec{spec})
		if err != nil {
			t.Fatal(err)
		}
		var b bytes.Buffer
		if err := doc.Encode(&b); err != nil {
			t.Fatal(err)
		}
		outs = append(outs, b.Bytes())
	}
	if !bytes.Equal(outs[0], outs[1]) {
		t.Fatalf("two runs printed different output:\n%s\n---\n%s", outs[0], outs[1])
	}
	if _, err := extract.Parse(bytes.NewReader(outs[0])); err != nil {
		t.Fatalf("setup would refuse the output: %v\n%s", err, outs[0])
	}
	var doc extract.Doc
	if err := json.Unmarshal(outs[0], &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}

func unit(t *testing.T, d extract.Doc, ref string) extract.DocUnit {
	t.Helper()
	i := slices.IndexFunc(d.Units, func(u extract.DocUnit) bool { return u.Ref == ref })
	if i < 0 {
		t.Fatalf("no unit %s in %+v", ref, d.Units)
	}
	return d.Units[i]
}

func relations(d extract.Doc) map[string]model.Via {
	out := map[string]model.Via{}
	for _, r := range d.Relations {
		out[r.From+" -> "+r.To] = r.Via
	}
	return out
}

func TestCargoWorkspaceMembersAreModulesAndTheirDependenciesRelations(t *testing.T) {
	d := run(t, "cargo", "cargo")
	for ref, want := range map[string][]model.Artifact{
		"cargo:app":          {"exe"},
		"cargo:fixture-core": {"cdylib"},
		"cargo:macros":       nil,
	} {
		if got := unit(t, d, ref).Artifacts; !slices.Equal(got, want) {
			t.Errorf("%s builds %v, want %v", ref, got, want)
		}
	}
	if got := unit(t, d, "cargo:fixture-core").Anchors; !slices.Equal(got, []model.Anchor{{Path: "core/Cargo.toml"}, {Path: "core/src"}}) {
		t.Errorf("fixture-core anchors %v, want its manifest and its source directory", got)
	}
	want := map[string]model.Via{
		"cargo:app -> cargo:fixture-core":    "import",
		"cargo:app -> cargo:macros":          "build-feature",
		"cargo:fixture-core -> cargo:macros": "macro",
	}
	if got := relations(d); !maps.Equal(got, want) {
		t.Errorf("relations %v, want %v; a dev-dependency is no relation", got, want)
	}
}

func TestCMakeTargetsAreModulesCarryingTheirArtifactAndLinks(t *testing.T) {
	d := run(t, "cmake", "cmake")
	for ref, want := range map[string]model.Artifact{
		"cmake:store": "static", "cmake:net": "shared", "cmake:plugin": "module", "cmake:tool": "exe",
	} {
		if got := unit(t, d, ref).Artifacts; !slices.Equal(got, []model.Artifact{want}) {
			t.Errorf("%s builds %v, want %s", ref, got, want)
		}
	}
	if len(d.Units) != 4 {
		t.Errorf("want 4 units, the test target left out, got %+v", d.Units)
	}
	if got := unit(t, d, "cmake:tool").Anchors; !slices.Equal(got, []model.Anchor{{Path: "CMakeLists.txt", Symbol: "tool"}, {Path: "tool"}}) {
		t.Errorf("tool anchors %v, want where it is defined and its source directory", got)
	}
	want := map[string]model.Via{
		"cmake:tool -> cmake:store": "link",
		"cmake:tool -> cmake:net":   "link",
		"cmake:plugin -> cmake:net": "link",
	}
	if got := relations(d); !maps.Equal(got, want) {
		t.Errorf("relations %v, want %v", got, want)
	}
}

func TestAStaticAndASharedBuildOfOneLibraryAreOneModule(t *testing.T) {
	d := run(t, "cmake", "cmake-variants")
	lib := unit(t, d, "cmake:my-lib")
	if !slices.Equal(lib.Artifacts, []model.Artifact{"shared", "static"}) {
		t.Errorf("my-lib builds %v, want shared and static", lib.Artifacts)
	}
	want := []model.Anchor{{Path: "CMakeLists.txt", Symbol: "my-lib-shared"}, {Path: "CMakeLists.txt", Symbol: "my-lib-static"}, {Path: "mylib"}}
	if !slices.Equal(lib.Anchors, want) {
		t.Errorf("my-lib anchors %v, want %v", lib.Anchors, want)
	}
	var refs []string
	for _, u := range d.Units {
		refs = append(refs, u.Ref)
	}
	if want := []string{"cmake:app", "cmake:my-lib", "cmake:my_lib"}; !slices.Equal(refs, want) {
		t.Errorf("units %v, want %v", refs, want)
	}
	if got := relations(d); got["cmake:app -> cmake:my-lib"] != "link" || got["cmake:app -> cmake:my_lib"] != "link" {
		t.Errorf("relations %v, want app linking both libraries", got)
	}
}

func TestAModuleNameNeverTakesAnotherTargetsName(t *testing.T) {
	merged := []string{"my_lib_static", "my_lib_shared"}
	for _, names := range [][][]string{
		{slices.Clone(merged), {"my_lib"}},
		{{"my_lib"}, slices.Clone(merged)},
	} {
		got := moduleRefs(names)
		byFirst := map[string]string{}
		for i, ns := range names {
			byFirst[ns[0]] = got[i]
		}
		want := map[string]string{"my_lib": "cmake:my_lib", "my_lib_shared": "cmake:my_lib_shared"}
		if !maps.Equal(byFirst, want) {
			t.Errorf("refs %v, want %v whatever order CMake lists the targets in", byFirst, want)
		}
	}
	if got := moduleRefs([][]string{{"zstd_a", "zstd_b"}, {"zstd_c", "zstd_d"}}); got[0] == got[1] {
		t.Errorf("two modules share the ref %s", got[0])
	}
	if got := commonName([]string{"libzstd_shared", "libzstd_static"}); got != "libzstd" {
		t.Errorf("got %q, want libzstd", got)
	}
	if got := commonName([]string{"a_x", "a_y"}); got != "a_x" {
		t.Errorf("a name shorter than 3 characters falls back to the first, got %q", got)
	}
}

func TestASpecNamesABundledFrontEndAndADirectoryInsideTheRoot(t *testing.T) {
	if s, err := ParseSpec("cmake=build/cmake"); err != nil || s != (Spec{"cmake", "build/cmake"}) {
		t.Errorf("got %+v, %v", s, err)
	}
	for _, bad := range []string{"gradle", "cargo=../other"} {
		if _, err := ParseSpec(bad); err == nil {
			t.Errorf("%q: want an error", bad)
		}
	}
}

func TestABuildOrderFromAddDependenciesIsNoLink(t *testing.T) {
	d := run(t, "cmake", "cmake-order")
	want := map[string]model.Via{"cmake:cli -> cmake:core": "link"}
	if got := relations(d); !maps.Equal(got, want) {
		t.Errorf("relations %v, want %v; add_dependencies(other cli) only orders the build", got, want)
	}
}

func TestALinkStaysALinkWhenAddDependenciesNamesTheSameTarget(t *testing.T) {
	d := run(t, "cmake", "cmake-order-link")
	want := map[string]model.Via{"cmake:cli -> cmake:core": "link", "cmake:cli2 -> cmake:core": "link"}
	if got := relations(d); !maps.Equal(got, want) {
		t.Errorf("relations %v, want %v; cli adds the order before its link, cli2 after", got, want)
	}
}

func TestATargetLinkingAnInterfaceLibraryLinksWhatItLinks(t *testing.T) {
	d := run(t, "cmake", "cmake-interface")
	want := map[string]model.Via{"cmake:cli -> cmake:p1_core": "link", "cmake:tool2 -> cmake:p1_core": "link", "cmake:tool3 -> cmake:p1_core": "link"}
	if got := relations(d); !maps.Equal(got, want) {
		t.Errorf("relations %v, want %v; tool2 links p1_core through iface, tool3 through outer and iface", got, want)
	}
}


func TestACodeModelOlderThanCMake331LinksEveryDependency(t *testing.T) {
	tg := cmakeTarget{Dependencies: []cmakeTargetRef{{"core"}, {"gen"}}, LinkLibraries: []cmakeTargetRef{{"core"}}}
	var cm cmakeCodemodel
	cm.Version.Major, cm.Version.Minor = 2, 8
	if got := links(cm, tg); len(got) != 2 {
		t.Errorf("code model 2.8 links %v, want every dependency", got)
	}
	cm.Version.Minor = 9
	if got := links(cm, tg); len(got) != 1 || got[0].ID != "core" {
		t.Errorf("code model 2.9 links %v, want only core", got)
	}
}

func TestARegistryCrateNamedLikeAMemberIsNoRelation(t *testing.T) {
	d := run(t, "cargo", "cargo-registry")
	unit(t, d, "cargo:serde")
	if got := relations(d); len(got) != 0 {
		t.Errorf("relations %v, want none; app depends on serde from the registry, not on the member", got)
	}
}

func TestASpecNamedTwiceRunsOnceAndTwoSpecsMakingOneRefFail(t *testing.T) {
	if _, err := exec.LookPath("cargo"); err != nil {
		t.Skip("cargo is not installed")
	}
	cache := t.TempDir()
	d, err := Extract(filepath.Join("testdata", "cargo"), cache, []Spec{{"cargo", "."}, {"cargo", "."}})
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Units) != 3 {
		t.Errorf("want the 3 members once, got %+v", d.Units)
	}
	_, err = Extract(filepath.Join("testdata", "cargo-twice"), cache, []Spec{{"cargo", "one"}, {"cargo", "two"}})
	if err == nil {
		t.Fatal("want an error for crate a in two workspaces")
	}
	for _, s := range []string{"cargo:a", "cargo=one", "cargo=two"} {
		if !strings.Contains(err.Error(), s) {
			t.Errorf("error %q does not name %s", err, s)
		}
	}
}
