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
	write(t, root, "src/mod.rs", searcher)
	return root
}

func write(t *testing.T, root, p, body string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(p))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
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

// Each row is one case from this repository's history: an edited line, a
// function moved to another file, a deleted file, and a quote now twice in
// its file. Only a gone file is dead at once; the others drift, and a sync
// at a later revision kills what is still drifted.
func TestAnItemIsLiveDriftedOrDead(t *testing.T) {
	root := t.TempDir()
	long := "renames, ok, err := gitx.Renames(e.root, state.Fingerprint, tree)"
	write(t, root, "cmd/main.go", strings.Repeat("// pad\n", 40)+long+"\n")
	write(t, root, "internal/evidence.go", "func checkEvidence() {\n\tc.Check(ev, within)\n}\n")
	write(t, root, "internal/query.go", "package topology\n")
	write(t, root, "internal/path.go", "func Path(units []model.Unit, a, b model.ID) (hops []Hop, ok bool) {\n")
	write(t, root, "internal/dup.go", long+"\n// pad\n"+long+"\n")
	for _, tc := range []struct {
		name string
		e    model.Evidence
		want model.Status
	}{
		{"pushed 38 lines down in its file", model.Evidence{Path: "cmd/main.go", Lines: "3", Quote: long}, model.Live},
		{"the quoted line was edited", model.Evidence{Path: "internal/evidence.go", Lines: "2", Quote: "r := c.Check(ev)"}, model.Drifted},
		{"the function moved to another file", model.Evidence{Path: "internal/query.go", Lines: "12", Quote: "func Path(units []model.Unit, a, b model.ID) (hops []Hop, ok bool) {"}, model.Drifted},
		{"twice in its file, far from its lines", model.Evidence{Path: "internal/dup.go", Lines: "90", Quote: long}, model.Drifted},
		{"its file is gone", model.Evidence{Path: "internal/gone.go", Quote: "func Path("}, model.Dead},
	} {
		c := New(root)
		now, _ := c.Status(tc.e, "r1", true)
		if now.Status != tc.want {
			t.Errorf("%s: %q, want %q", tc.name, now.Status, tc.want)
			continue
		}
		if tc.want != model.Drifted {
			continue
		}
		if now.Since != "r1" {
			t.Errorf("%s: drifted since %q, want the sync's revision r1", tc.name, now.Since)
		}
		if same, _ := c.Status(now, "r1", true); same.Status != model.Drifted {
			t.Errorf("%s: a sync at the same revision killed it: %+v", tc.name, same)
		}
		later, _ := c.Status(now, "r2", true)
		if later.Status != model.Dead || later.Reason == "" {
			t.Errorf("%s: a sync at a later revision still finding it drifted left %+v", tc.name, later)
		}
	}
}

// Dead is terminal for a reader's item, so a dead coupling never comes back
// by itself. The script restates its evidence on every sync, so its items
// follow the tree both ways.
func TestADeadReaderItemStaysDeadAndAScriptItemFollowsTheTree(t *testing.T) {
	root := tree(t)
	ev := model.Evidence{Path: "src/mod.rs", Quote: "impl Searcher", Status: model.Dead, Reason: "src/mod.rs is gone"}
	if now, _ := New(root).Status(ev, "r2", true); now.Status != model.Dead {
		t.Errorf("a dead reader item came back: %+v", now)
	}
	if now, _ := New(root).Status(ev, "r2", false); now.Status != model.Live {
		t.Errorf("the script's quote is in the file, yet its item is %q", now.Status)
	}
	gone := model.Evidence{Path: "src/mod.rs", Quote: "a quote the script no longer finds"}
	if now, _ := New(root).Status(gone, "r2", false); now.Status != model.Dead {
		t.Errorf("the script's evidence drifts instead of dying: %+v", now)
	}
}

func units() []model.Unit {
	return []model.Unit{
		{ID: "MOD-0001", Kind: model.Module, Name: model.Said(model.Script, "searcher"), Relations: []model.Relation{
			{To: "DATA-0001", Kind: model.Consumes, Writer: model.Reader, Via: "convention", Evidence: []model.Evidence{{Path: "src/mod.rs", Lines: "7", Quote: "matcher_line_term != self.config.line_term"}}},
			{To: "MOD-0002", Kind: model.DependsOn, Writer: model.Reader, Evidence: []model.Evidence{{Path: "src/mod.rs", Quote: "a quote nobody wrote"}}},
		}},
		{ID: "DATA-0001", Kind: model.Data, Name: model.Said(model.Reader, "line terminator"), Evidence: []model.Evidence{{Path: "src/mod.rs", Quote: "MismatchedLineTerminators"}}},
		{ID: "DATA-0002", Kind: model.Data, Name: model.Said(model.Reader, "ghost"), Evidence: []model.Evidence{{Path: "src/gone.rs", Quote: "x"}}},
	}
}

// The state rests on this check, so the same tree must give the same answer
// every time, whatever was checked before.
func TestApplyGivesTheSameAnswerThreeTimes(t *testing.T) {
	root := tree(t)
	first := New(root).Apply(units(), "r1")
	for range 2 {
		if again := New(root).Apply(units(), "r1"); !reflect.DeepEqual(first, again) {
			t.Fatalf("a second check on the same tree differs:\n%+v\n%+v", first, again)
		}
	}
	if r := first[0].Relations; r[0].Dead || r[0].Digest == "" || r[1].Dead || !r[1].Drifted() {
		t.Fatalf("the cited coupling should be live with a digest and the miscited one drifted, still counting: %+v", r)
	}
	if first[1].Absent || !first[2].Absent || first[2].AbsentReason != model.EvidenceDead {
		t.Fatalf("a semantic unit is present while its evidence is, and absent with a reason after: %+v", first[1:])
	}
}

func TestEditingTheQuotedLinesChangesTheDigestOnly(t *testing.T) {
	root := tree(t)
	before := New(root).Apply(units(), "r1")[0].Relations[0]
	shifted := "// a new first line\n" + searcher
	os.WriteFile(filepath.Join(root, "src", "mod.rs"), []byte(shifted), 0o644)
	if moved := New(root).Apply(units(), "r1")[0].Relations[0]; moved.Dead || moved.Digest != before.Digest {
		t.Fatalf("a quote one line lower is the same evidence: %+v vs %+v", moved, before)
	}
	edited := []byte(strings.Replace(searcher, "line_term {", "line_term || strict {", 1))
	os.WriteFile(filepath.Join(root, "src", "mod.rs"), edited, 0o644)
	if after := New(root).Apply(units(), "r1")[0].Relations[0]; after.Dead || after.Digest == before.Digest {
		t.Fatalf("an edit to the quoted line keeps the evidence but changes its digest: %+v vs %+v", after, before)
	}
}
