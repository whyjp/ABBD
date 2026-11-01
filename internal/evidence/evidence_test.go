package evidence

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/whyjp/abbd/internal/model"
)

const searcher = "impl Searcher {\n    fn check_config<M: Matcher>(&self, matcher: M) -> Result<(), ConfigError> {\n        let matcher_line_term = match matcher.line_terminator() {\n            None => return Ok(()),\n            Some(line_term) => line_term,\n        };\n        if matcher_line_term != self.config.line_term {\r\n            return Err(ConfigError::MismatchedLineTerminators {\n"

func tree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "mod.rs"), []byte(searcher), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestAQuoteIsFoundNearItsLinesOrAsItsFilesOnlyOccurrence(t *testing.T) {
	c := New(tree(t))
	for _, tc := range []struct {
		e    model.Evidence
		live bool
		at   string
	}{
		{model.Evidence{Path: "src/mod.rs", Lines: "7", Quote: "if matcher_line_term != self.config.line_term {"}, true, "7"},
		{model.Evidence{Path: "src/mod.rs", Lines: "4", Quote: "if   matcher_line_term !=\tself.config.line_term"}, true, "7"},
		{model.Evidence{Path: "src/mod.rs", Quote: "None => return Ok(()), Some(line_term) => line_term,"}, true, "4-5"},
		{model.Evidence{Path: "src/mod.rs", Lines: "20-22", Quote: "if matcher_line_term != self.config.line_term {"}, true, "7"},
		{model.Evidence{Path: "src/mod.rs", Quote: "matcher_line_term == self.config.line_term"}, false, ""},
		{model.Evidence{Path: "src/gone.rs", Quote: "impl Searcher"}, false, ""},
	} {
		r := c.Check(tc.e)
		if r.Live != tc.live || r.At != tc.at {
			t.Errorf("%s: got live=%v at %q, want live=%v at %q", tc.e, r.Live, r.At, tc.live, tc.at)
		}
	}
}

func units() []model.Unit {
	return []model.Unit{
		{ID: "MOD-0001", Kind: model.Module, Name: model.Fact{Value: "searcher", Origin: model.Extracted}, Relations: []model.Relation{
			{To: "DATA-0001", Kind: model.Consumes, Origin: model.Inferred, Via: "convention", Evidence: []model.Evidence{{Path: "src/mod.rs", Lines: "7", Quote: "matcher_line_term != self.config.line_term"}}},
			{To: "MOD-0002", Kind: model.DependsOn, Origin: model.Inferred, Evidence: []model.Evidence{{Path: "src/mod.rs", Quote: "a quote nobody wrote"}}},
		}},
		{ID: "DATA-0001", Kind: model.Data, Name: model.Fact{Value: "line terminator", Origin: model.Inferred}, Evidence: []model.Evidence{{Path: "src/mod.rs", Quote: "MismatchedLineTerminators"}}},
		{ID: "DATA-0002", Kind: model.Data, Name: model.Fact{Value: "ghost", Origin: model.Inferred}, Evidence: []model.Evidence{{Path: "src/gone.rs", Quote: "x"}}},
	}
}

// The gate rests on this check, so the same tree must give the same answer
// every time, whatever was checked before.
func TestApplyGivesTheSameAnswerThreeTimes(t *testing.T) {
	root := tree(t)
	first := New(root).Apply(units())
	for range 2 {
		if again := New(root).Apply(units()); !reflect.DeepEqual(first, again) {
			t.Fatalf("a second check on the same tree differs:\n%+v\n%+v", first, again)
		}
	}
	if r := first[0].Relations; r[0].Dead || r[0].Digest == "" || !r[1].Dead {
		t.Fatalf("the cited coupling should be live with a digest and the uncited one dead: %+v", r)
	}
	if first[1].Absent || !first[2].Absent {
		t.Fatalf("a semantic unit is present exactly while its evidence is: %+v", first[1:])
	}
}

func TestEditingTheQuotedLinesChangesTheDigestOnly(t *testing.T) {
	root := tree(t)
	before := New(root).Apply(units())[0].Relations[0]
	shifted := "// a new first line\n" + searcher
	os.WriteFile(filepath.Join(root, "src", "mod.rs"), []byte(shifted), 0o644)
	if moved := New(root).Apply(units())[0].Relations[0]; moved.Dead || moved.Digest != before.Digest {
		t.Fatalf("a quote one line lower is the same evidence: %+v vs %+v", moved, before)
	}
	edited := []byte(strings.Replace(searcher, "line_term {", "line_term || strict {", 1))
	os.WriteFile(filepath.Join(root, "src", "mod.rs"), edited, 0o644)
	if after := New(root).Apply(units())[0].Relations[0]; after.Dead || after.Digest == before.Digest {
		t.Fatalf("an edit to the quoted line keeps the evidence but changes its digest: %+v vs %+v", after, before)
	}
}

// Code moves between syncs: a merge can push a quote many lines down, and a
// refactor can move it to another file. The quote follows only when there is
// exactly one place it can be, so the same tree still gives the same answer.
func TestAQuoteFollowsItsCodeOnlyToAUniquePlace(t *testing.T) {
	root := t.TempDir()
	write := func(p, body string) {
		t.Helper()
		full := filepath.Join(root, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	long := "renames, ok, err := gitx.Renames(e.root, state.Fingerprint, tree)"
	write("cmd/main.go", strings.Repeat("// pad\n", 40)+long+"\n")
	write("internal/path.go", "func Path(units []model.Unit, a, b model.ID) (hops []Hop, ok bool) {\n")
	write("internal/twice_a.go", "x := shared.Helper(a, b, c, d, e)\n")
	write("internal/twice_b.go", "x := shared.Helper(a, b, c, d, e)\n")
	write("internal/dup.go", long+"\n// pad\n"+long+"\n")
	c := New(root)
	for _, tc := range []struct {
		name string
		e    model.Evidence
		live bool
		at   string
		path string
	}{
		{"pushed 38 lines down in the same file", model.Evidence{Path: "cmd/main.go", Lines: "3", Quote: long}, true, "41", ""},
		{"moved to another file", model.Evidence{Path: "internal/query.go", Lines: "12", Quote: "func Path(units []model.Unit, a, b model.ID) (hops []Hop, ok bool) {"}, true, "1", "internal/path.go"},
		{"two places to go", model.Evidence{Path: "internal/gone.go", Quote: "x := shared.Helper(a, b, c, d, e)"}, false, "", ""},
		{"twice in its own file, far from its lines", model.Evidence{Path: "internal/dup.go", Lines: "90", Quote: long}, false, "", ""},
		{"too short to follow out of its file", model.Evidence{Path: "internal/gone.go", Quote: "func Path("}, false, "", ""},
	} {
		r := c.Check(tc.e)
		if r.Live != tc.live || r.At != tc.at || r.Path != tc.path {
			t.Errorf("%s: got live=%v at %q path %q, want live=%v at %q path %q", tc.name, r.Live, r.At, r.Path, tc.live, tc.at, tc.path)
		}
	}
}
