package relate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/whyjp/abbd/internal/extract"
)

func TestCDatabaseKeepsTheFirstSourcesByPathPastTheLimit(t *testing.T) {
	root := t.TempDir()
	var entries []cEntry
	// Listed in an order that is not the path order, as a build may list them.
	for _, n := range []string{"d", "b", "e", "a", "c"} {
		f := filepath.Join(root, "src", n+".cpp")
		entries = append(entries, cEntry{f, map[string]any{"directory": root, "file": f, "arguments": []string{"cl.exe", "/c", f}}})
	}
	o := index([]extract.DocUnit{mod("m", "src")})
	cache := t.TempDir()
	dir, compiled, capped, err := cDatabase(tree{root: root, dir: root, cache: cache}, o, entries, 3)
	if err != nil {
		t.Fatal(err)
	}
	if got := slices.Sorted(func(yield func(string) bool) {
		for k := range compiled {
			if !yield(k) {
				return
			}
		}
	}); !slices.Equal(got, []string{"src/a.cpp", "src/b.cpp", "src/c.cpp"}) {
		t.Fatalf("compiled = %v", got)
	}
	if !slices.Equal(capped, []string{"src/d.cpp", "src/e.cpp"}) {
		t.Fatalf("capped = %v", capped)
	}
	b, err := os.ReadFile(filepath.Join(dir, "compile_commands.json"))
	if err != nil {
		t.Fatal(err)
	}
	var db []map[string]any
	if err := json.Unmarshal(b, &db); err != nil || len(db) != 3 {
		t.Fatalf("the database holds %d entries, %v", len(db), err)
	}
	if _, _, capped, _ := cDatabase(tree{root: root, dir: root, cache: cache}, o, entries, 0); len(capped) != 0 {
		t.Fatalf("a limit of 0 capped %v", capped)
	}
}

func TestCMaxSourcesDefaultsByKindOfDatabaseAndTakesTheEnvironment(t *testing.T) {
	t.Setenv("ABBD_C_MAX_SOURCES", "")
	if n, err := cMaxSources(tree{dir: "/r/app.sln"}); err != nil || n != cMSBuildMaxSources {
		t.Errorf("a solution: %d, %v", n, err)
	}
	if n, err := cMaxSources(tree{dir: "/r/build"}); err != nil || n != 0 {
		t.Errorf("a CMake project: %d, %v", n, err)
	}
	t.Setenv("ABBD_C_MAX_SOURCES", "7")
	if n, err := cMaxSources(tree{dir: "/r/build"}); err != nil || n != 7 {
		t.Errorf("set: %d, %v", n, err)
	}
	t.Setenv("ABBD_C_MAX_SOURCES", "many")
	if _, err := cMaxSources(tree{dir: "/r/app.sln"}); err == nil || !strings.Contains(err.Error(), "ABBD_C_MAX_SOURCES") {
		t.Errorf("err = %v", err)
	}
}
