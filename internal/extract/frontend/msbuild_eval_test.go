package frontend

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// readTree writes files under a temporary root and reads the target.
func readTree(t *testing.T, files map[string]string, target string) (*msbuilder, []*msProject) {
	t.Helper()
	root := t.TempDir()
	for p, c := range files {
		full := filepath.Join(root, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	m := newMSBuilder(root)
	ps, _, err := m.readTargets(filepath.ToSlash(filepath.Join(root, target)))
	if err != nil {
		t.Fatal(err)
	}
	return m, ps
}

func vcx(body string) string {
	return `<Project xmlns="http://schemas.microsoft.com/developer/msbuild/2003">
<ItemGroup><ProjectConfiguration Include="Debug|x64"><Configuration>Debug</Configuration><Platform>x64</Platform></ProjectConfiguration></ItemGroup>
<PropertyGroup><ConfigurationType>StaticLibrary</ConfigurationType></PropertyGroup>` + body + `</Project>`
}

const slnLine = "Project(\"{8BC9CEB8-8B4A-11D0-8D11-00A0C91BC942}\") = \"p\", \"p.vcxproj\", \"{1}\"\n"

func hasNote(m *msbuilder, part string) bool {
	for _, n := range m.sortedNotes() {
		if strings.Contains(n, part) {
			return true
		}
	}
	return false
}

func TestAMSBuildNotBindsToTheNextOperandOnly(t *testing.T) {
	m := newMSBuilder(msbuildFixture(t))
	e := &evalCtx{m: m, p: &msProject{dir: m.root}, props: map[string]string{}}
	for cond, want := range map[string]bool{
		`!Exists('none.txt') And 'a'=='b'`: false,
		`!Exists('none.txt') And 'a'=='a'`: true,
		`!('a'=='b') Or 'c'=='d'`:          true,
		`'10' > '9'`:                       true,
		`'1.10' < '1.9'`:                   true,
		`'abc' < 'abd'`:                    true,
	} {
		got, err := e.condition(cond)
		if err != nil || got != want {
			t.Errorf("%s = %v, %v; want %v", cond, got, err, want)
		}
	}
}

func TestAMSBuildPathsOutsideTheRootAreNeverFollowed(t *testing.T) {
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "o.props"), []byte(`<Project><ItemDefinitionGroup><ClCompile><PreprocessorDefinitions>FROM_OUTSIDE</PreprocessorDefinitions></ClCompile></ItemDefinitionGroup></Project>`), 0o644); err != nil {
		t.Fatal(err)
	}
	o := filepath.ToSlash(outside)
	m, ps := readTree(t, map[string]string{
		"a/a.vcxproj": vcx(`<Import Project="` + o + `/o.props" /><PropertyGroup Condition="Exists('` + o + `/o.props')"><Hit>1</Hit></PropertyGroup><ItemGroup><ClCompile Include="a.cpp"/></ItemGroup>`),
		"a/a.cpp":     "",
		"s.sln": "Project(\"{8BC9CEB8-8B4A-11D0-8D11-00A0C91BC942}\") = \"a\", \"a/a.vcxproj\", \"{1}\"\n" +
			"Project(\"{8BC9CEB8-8B4A-11D0-8D11-00A0C91BC942}\") = \"far\", \"../far/far.vcxproj\", \"{2}\"\n",
	}, "s.sln")
	if len(ps) != 1 || slices.Contains(ps[0].conf["Debug|x64"].defines, "FROM_OUTSIDE") {
		t.Fatalf("projects %d", len(ps))
	}
	for _, want := range []string{"the Import", "Exists(", "far.vcxproj, which lies outside the repository"} {
		if !hasNote(m, want) {
			t.Errorf("no note holds %q: %q", want, m.sortedNotes())
		}
	}
}

func TestAMSBuildImportLoopsEndAndDiamondsReadOnce(t *testing.T) {
	m, ps := readTree(t, map[string]string{
		"p.vcxproj": vcx(`<Import Project="a.props"/><Import Project="b.props"/><ItemGroup><ClCompile Include="p.cpp"/></ItemGroup>`),
		"p.cpp":     "",
		"a.props":   `<Project><Import Project="c.props"/><Import Project="p.vcxproj"/></Project>`,
		"b.props":   `<Project><Import Project="c.props"/></Project>`,
		"c.props":   `<Project><Import Project="a.props"/><ItemDefinitionGroup><ClCompile><PreprocessorDefinitions>C;%(PreprocessorDefinitions)</PreprocessorDefinitions></ClCompile></ItemDefinitionGroup></Project>`,
	}, "p.vcxproj")
	if d := ps[0].conf["Debug|x64"].defines; !slices.Equal(d, []string{"C"}) {
		t.Fatalf("defines = %v, notes %q", d, m.sortedNotes())
	}
}

func TestAMSBuildImportConditionsAreDecidedOnceAndItemsWaitForFinalProperties(t *testing.T) {
	m, ps := readTree(t, map[string]string{
		"p.vcxproj": vcx(`<PropertyGroup><Flag>1</Flag></PropertyGroup>
<Import Project="x.props" Condition="'$(Flag)'=='1'"/>
<ItemGroup Condition="'$(Late)'=='yes'"><ClCompile Include="late.cpp"/></ItemGroup>
<PropertyGroup><Late>yes</Late></PropertyGroup>`),
		"late.cpp": "",
		"x.props":  `<Project><PropertyGroup><Flag>2</Flag></PropertyGroup><ItemDefinitionGroup><ClCompile><PreprocessorDefinitions>X</PreprocessorDefinitions></ClCompile></ItemDefinitionGroup></Project>`,
	}, "p.vcxproj")
	c := ps[0].conf["Debug|x64"]
	if !slices.Equal(c.defines, []string{"X"}) || len(c.sources) != 1 {
		t.Fatalf("defines %v sources %v", c.defines, c.sources)
	}
	if hasNote(m, "unresolved") {
		t.Errorf("a false note: %q", m.sortedNotes())
	}
}

func TestAMSBuildExcludeAndRemoveTrimClCompile(t *testing.T) {
	_, ps := readTree(t, map[string]string{
		"p.vcxproj": vcx(`<ItemGroup><ClCompile Include="src/*.cpp" Exclude="src/skip*.cpp"/><ClCompile Include="gone.cpp"/><ClCompile Remove="gone.cpp"/></ItemGroup>`),
		"src/a.cpp": "", "src/skip1.cpp": "", "gone.cpp": "",
	}, "p.vcxproj")
	got := ps[0].conf["Debug|x64"].sources
	if len(got) != 1 || !strings.HasSuffix(got[0], "src/a.cpp") {
		t.Fatalf("sources = %v", got)
	}
}

func TestAMSBuildSolutionPropertiesAreSeeded(t *testing.T) {
	_, ps := readTree(t, map[string]string{
		"s.sln":           slnLine,
		"p.vcxproj":       vcx(`<ItemGroup><ClCompile Include="$(SolutionName)-$(PlatformName)-$(ConfigurationName).cpp"/></ItemGroup>`),
		"s-x64-Debug.cpp": "",
	}, "s.sln")
	if len(ps[0].conf["Debug|x64"].sources) != 1 {
		t.Fatal("the seeded properties were not used")
	}
}

func TestAMSBuildSolutionWithAByteOrderMarkIsRead(t *testing.T) {
	root := t.TempDir()
	m := newMSBuilder(root)
	if err := os.WriteFile(filepath.Join(root, "p.vcxproj"), []byte(vcx("")), 0o644); err != nil {
		t.Fatal(err)
	}
	u16 := []byte{0xFF, 0xFE}
	for _, r := range slnLine {
		u16 = append(u16, byte(r), byte(r>>8))
	}
	for name, b := range map[string][]byte{"utf16.sln": u16, "utf8.sln": append([]byte{0xEF, 0xBB, 0xBF}, slnLine...)} {
		if err := os.WriteFile(filepath.Join(root, name), b, 0o644); err != nil {
			t.Fatal(err)
		}
		ps, err := m.projectPaths(filepath.ToSlash(filepath.Join(root, name)))
		if err != nil || len(ps) != 1 {
			t.Errorf("%s: %v, %v", name, ps, err)
		}
	}
}

func TestAMSBuildCompilesNameTheNewerLanguageStandards(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"p.vcxproj": vcx(`<ItemDefinitionGroup><ClCompile><LanguageStandard>stdcpp23</LanguageStandard><LanguageStandard_C>stdclatest</LanguageStandard_C></ClCompile></ItemDefinitionGroup><ItemGroup><ClCompile Include="a.cpp"/><ClCompile Include="b.c"/></ItemGroup>`),
		"a.cpp":     "", "b.c": "",
	}
	for p, c := range files {
		if err := os.WriteFile(filepath.Join(root, p), []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cs, _, err := MSBuildCompiles(root, filepath.Join(root, "p.vcxproj"))
	if err != nil || len(cs) != 2 {
		t.Fatalf("%v, %v", cs, err)
	}
	if !slices.Contains(cs[0].Args, "/std:c++23preview") || !slices.Contains(cs[1].Args, "/std:clatest") {
		t.Fatalf("%v\n%v", cs[0].Args, cs[1].Args)
	}
}
