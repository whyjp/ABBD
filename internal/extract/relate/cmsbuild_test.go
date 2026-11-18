package relate

import (
	"slices"
	"strings"
	"testing"

	"github.com/whyjp/abbd/internal/extract"
	"github.com/whyjp/abbd/internal/extract/frontend"
	"github.com/whyjp/abbd/internal/model"
)

// cMSBuildFixture is testdata/msbuild, a Visual Studio solution: app and
// plugin call core's core_value, app reads core.h through an include
// directory and COMMON_LIMIT through a property sheet, app includes
// vendor_sdk.h, which no include path holds, and tests is left out.
const cMSBuildFixture = "testdata/msbuild"

func TestCMSBuildProjectsAreCompiledFromTheirVcxproj(t *testing.T) {
	needC(t, false)
	doc := extract.Doc{Units: []extract.DocUnit{mod("msbuild:app", "app"), mod("msbuild:core", "Core"), mod("msbuild:plugin", "plugin")}}
	out, unread := relateC(t, cMSBuildFixture, "c=app.sln", doc)
	var got []string
	for _, r := range out.Relations {
		if r.Kind == model.DependsOn {
			got = append(got, r.From+" -> "+r.To+" via "+string(r.Via))
		}
	}
	slices.Sort(got)
	want := []string{"msbuild:app -> msbuild:core via call", "msbuild:plugin -> msbuild:core via call"}
	if !slices.Equal(got, want) {
		t.Fatalf("relations = %q, want %q", got, want)
	}
	var lines []string
	for _, u := range unread {
		lines = append(lines, u.Lead+" "+strings.Join(u.Paths, ","))
	}
	all := strings.Join(lines, "\n")
	for _, w := range []string{"vendor_sdk.h (1 files)", "read under one configuration of each project: Debug|x64 for 3"} {
		if !strings.Contains(all, w) {
			t.Errorf("no status line holds %q:\n%s", w, all)
		}
	}
}

func TestCMSBuildFileThatIsNoProjectFails(t *testing.T) {
	s, err := ParseSpec("c=app")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := Relate(cMSBuildFixture, t.TempDir(), nil, extract.Doc{}, []frontend.Spec{s}); err == nil || !strings.Contains(err.Error(), "compile_commands.json or CMakeLists.txt") {
		t.Fatalf("err = %v", err)
	}
}

func TestCMSBuildSourcesPastTheLimitAreNamedUnreadOnce(t *testing.T) {
	needC(t, false)
	t.Setenv("ABBD_C_MAX_SOURCES", "2")
	doc := extract.Doc{Units: []extract.DocUnit{mod("msbuild:app", "app"), mod("msbuild:core", "Core"), mod("msbuild:plugin", "plugin")}}
	_, unread := relateC(t, cMSBuildFixture, "c=app.sln", doc)
	seen := map[string]bool{}
	var capped []string
	for _, u := range unread {
		if seen[u.Lead] {
			t.Errorf("the line is printed twice: %s", u.Lead)
		}
		seen[u.Lead] = true
		if strings.Contains(u.Lead, "past the limit of 2") {
			capped = u.Paths
		}
	}
	if len(capped) != 3 {
		t.Fatalf("past the limit: %v", capped)
	}
}
