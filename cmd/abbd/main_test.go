package main

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@t", "-c", "core.autocrlf=false"}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// syncWith registers the fake extraction script for root, has it report
// extraction, and runs abbd sync.
func syncWith(t *testing.T, root, extraction string) (code int, stdout, stderr string) {
	t.Helper()
	src := root + ".extraction.json"
	write(t, src, extraction)
	t.Setenv("ABBD_FAKE_EXTRACT", src)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(root, ".abbd", "project.yaml"), fmt.Sprintf("extract: [%q]\n", exe))
	var out, errb bytes.Buffer
	code = run([]string{"sync", "--verbose", "--root", root}, nil, &out, &errb)
	return code, out.String(), errb.String()
}

func ingestOK(t *testing.T, root, extraction string) string {
	t.Helper()
	code, out, errOut := syncWith(t, root, extraction)
	if code != 0 {
		t.Fatalf("sync exit %d: %s", code, errOut)
	}
	return out
}

func snapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	snap := map[string]string{}
	base := filepath.Join(root, ".abbd")
	filepath.WalkDir(base, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			t.Fatal(err)
		}
		if !d.IsDir() {
			b, _ := os.ReadFile(p)
			rel, _ := filepath.Rel(base, p)
			snap[filepath.ToSlash(rel)] = string(b)
		}
		return nil
	})
	return snap
}

func newRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	git(t, root, "init", "-q")
	write(t, filepath.Join(root, "billing", "charge.go"), "package billing\n\nfunc Charge() {}\n\nfunc Refund() {}\n")
	write(t, filepath.Join(root, "api", "http.go"), "package api\n\nfunc Serve() {}\n")
	git(t, root, "add", ".")
	git(t, root, "commit", "-qm", "init")
	return root
}

const extraction = `{"units":[
 {"kind":"MOD","name":"billing","anchors":[{"path":"billing/charge.go","symbol":"Charge"},{"path":"billing/charge.go","symbol":"Refund"}],"purpose":"takes payments"},
 {"kind":"IF","name":"http api","anchors":[{"path":"api/http.go","symbol":"Serve"}]}
]}`

func TestRerunPreservesIDsAndBytes(t *testing.T) {
	root := newRepo(t)
	first := ingestOK(t, root, extraction)
	if !strings.Contains(first, "MOD-0001 new") || !strings.Contains(first, "IF-0001 new") {
		t.Fatalf("first ingest should create MOD-0001 and IF-0001, got:\n%s", first)
	}
	before := snapshot(t, root)

	reordered := `{"units":[
 {"kind":"IF","name":"http api","anchors":[{"path":"api/http.go","symbol":"Serve"}]},
 {"kind":"MOD","name":"billing","anchors":[{"path":"billing/charge.go","symbol":"Refund"},{"path":"billing/charge.go","symbol":"Charge"}],"purpose":"takes payments"}
]}`
	second := ingestOK(t, root, reordered)
	if !strings.Contains(second, "MOD-0001 matched") || !strings.Contains(second, "IF-0001 matched") || strings.Contains(second, " new\n") {
		t.Fatalf("re-ingest should only match existing ids, got:\n%s", second)
	}
	after := snapshot(t, root)
	if len(before) != len(after) {
		t.Fatalf("file set changed: %v vs %v", keysOf(before), keysOf(after))
	}
	for k, v := range before {
		if after[k] != v {
			t.Errorf("%s changed on re-ingest\nbefore:\n%s\nafter:\n%s", k, v, after[k])
		}
	}
}

func TestExternalFileMoveKeepsIDs(t *testing.T) {
	for _, commit := range []bool{false, true} {
		name := "staged"
		if commit {
			name = "committed"
		}
		t.Run(name, func(t *testing.T) {
			root := newRepo(t)
			ingestOK(t, root, extraction)

			git(t, root, "mv", "billing", "payments")
			git(t, root, "mv", "api/http.go", "api/server.go")
			if commit {
				git(t, root, "commit", "-qm", "move")
			}

			moved := `{"units":[
 {"kind":"MOD","name":"payments","anchors":[{"path":"payments/charge.go","symbol":"Charge"},{"path":"payments/charge.go","symbol":"Refund"}]},
 {"kind":"IF","name":"http api","anchors":[{"path":"api/server.go","symbol":"Serve"}]}
]}`
			out := ingestOK(t, root, moved)
			if !strings.Contains(out, "MOD-0001 matched") || !strings.Contains(out, "IF-0001 matched") || strings.Contains(out, " new\n") {
				t.Fatalf("moved files should keep their ids, got:\n%s", out)
			}
			mod := snapshot(t, root)["architecture/MOD-0001.md"]
			if !strings.Contains(mod, "payments/charge.go") || strings.Contains(mod, "billing/charge.go") {
				t.Fatalf("MOD-0001 anchors should follow the move:\n%s", mod)
			}
			if !strings.Contains(mod, "takes payments") {
				t.Fatalf("purpose recorded before the move should survive:\n%s", mod)
			}
		})
	}
}

func TestSingleSharedAnchorBecomesCandidate(t *testing.T) {
	root := newRepo(t)
	ingestOK(t, root, extraction)
	split := `{"units":[
 {"kind":"MOD","name":"billing","anchors":[{"path":"billing/charge.go","symbol":"Charge"},{"path":"billing/charge.go","symbol":"Void"}]},
 {"kind":"IF","name":"http api","anchors":[{"path":"api/http.go","symbol":"Serve"}]}
]}`
	out := ingestOK(t, root, split)
	if !strings.Contains(out, "MOD-0002 new") || !strings.Contains(out, "MOD-0002 candidate-of MOD-0001") {
		t.Fatalf("one shared anchor should mint a new id and a merge candidate, got:\n%s", out)
	}
	again := ingestOK(t, root, split)
	if !strings.Contains(again, "MOD-0002 matched") || !strings.Contains(again, "MOD-0002 candidate-of MOD-0001") {
		t.Fatalf("the candidate should persist and the new id should stay stable, got:\n%s", again)
	}
}

func TestReservedDomainKindIsRejected(t *testing.T) {
	root := newRepo(t)
	if code, _, errOut := syncWith(t, root, `{"units":[{"kind":"STATE","name":"s","anchors":[{"path":"a.go"}]}]}`); code == 0 || !strings.Contains(errOut, "reserved") {
		t.Fatalf("STATE kind should be rejected as reserved, exit %d: %s", code, errOut)
	}
}

func keysOf(m map[string]string) []string {
	var ks []string
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}

func ingestErr(t *testing.T, root, extraction string) (string, string) {
	t.Helper()
	code, out, errOut := syncWith(t, root, extraction)
	if code != 0 {
		t.Fatalf("sync exit %d: %s", code, errOut)
	}
	return out, errOut
}

func unitsByName(t *testing.T, root string) map[string]string {
	t.Helper()
	got := map[string]string{}
	for path, body := range snapshot(t, root) {
		if strings.HasSuffix(path, ".md") {
			name := body[strings.Index(body, "\n# ")+3:]
			got[name[:strings.Index(name, "\n")]] = path
		}
	}
	return got
}

func TestReorderedInputSharingFirstAnchorKeepsIDs(t *testing.T) {
	root := newRepo(t)
	big := `{"kind":"MOD","name":"big","anchors":[{"path":"billing/charge.go","symbol":"Charge"},{"path":"billing/charge.go","symbol":"Refund"},{"path":"billing/charge.go","symbol":"Void"}]}`
	small := `{"kind":"MOD","name":"small","anchors":[{"path":"billing/charge.go","symbol":"Charge"},{"path":"billing/charge.go","symbol":"Refund"}]}`
	ingestOK(t, root, `{"units":[`+big+`,`+small+`]}`)
	before := unitsByName(t, root)
	ingestOK(t, root, `{"units":[`+small+`,`+big+`]}`)
	after := unitsByName(t, root)
	if before["big"] != after["big"] || before["small"] != after["small"] {
		t.Fatalf("reordering swapped ids: before %v after %v", before, after)
	}
}

func TestSecondMoveAfterUncommittedFirstMoveKeepsID(t *testing.T) {
	root := newRepo(t)
	ingestOK(t, root, extraction)
	git(t, root, "mv", "billing", "payments")
	ingestOK(t, root, strings.ReplaceAll(extraction, "billing/", "payments/"))
	git(t, root, "commit", "-qm", "first move")
	git(t, root, "mv", "payments", "pay")
	git(t, root, "commit", "-qm", "second move")
	out := ingestOK(t, root, strings.ReplaceAll(extraction, "billing/", "pay/"))
	if !strings.Contains(out, "MOD-0001 matched") || strings.Contains(out, " new\n") {
		t.Fatalf("second move should keep MOD-0001, got:\n%s", out)
	}
}

func TestPlainMoveWithoutGitKeepsID(t *testing.T) {
	root := newRepo(t)
	ingestOK(t, root, extraction)
	if err := os.Rename(filepath.Join(root, "billing"), filepath.Join(root, "payments")); err != nil {
		t.Fatal(err)
	}
	out := ingestOK(t, root, strings.ReplaceAll(extraction, "billing/", "payments/"))
	if !strings.Contains(out, "MOD-0001 matched") || strings.Contains(out, " new\n") {
		t.Fatalf("a plain mv should keep MOD-0001, got:\n%s", out)
	}
}

func TestStatedPurposeAndNotesSurviveReingest(t *testing.T) {
	root := newRepo(t)
	ingestOK(t, root, extraction)
	p := filepath.Join(root, ".abbd", "architecture", "MOD-0001.md")
	b, _ := os.ReadFile(p)
	stated := "value: posts entries\n    writer: human\n    source: conversation 2026-11-01\n"
	edited := strings.Replace(string(b), "value: takes payments\n    writer: supplement\n", stated, 1) + "\n## Notes\n\nkeep this\n"
	write(t, p, edited)
	ingestOK(t, root, extraction)
	b, _ = os.ReadFile(p)
	if !strings.Contains(string(b), stated) || !strings.Contains(string(b), "## Notes\n\nkeep this\n") {
		t.Fatalf("a stated purpose and notes should survive re-ingest:\n%s", b)
	}
}

func TestDeletedIDIsNeverReused(t *testing.T) {
	root := newRepo(t)
	ingestOK(t, root, extraction)
	split := strings.Replace(extraction, `"symbol":"Refund"`, `"symbol":"Void"`, 1)
	ingestOK(t, root, split)
	if err := os.Remove(filepath.Join(root, ".abbd", "architecture", "MOD-0002.md")); err != nil {
		t.Fatal(err)
	}
	out := ingestOK(t, root, `{"units":[{"kind":"MOD","name":"other","anchors":[{"path":"x.go","symbol":"X"},{"path":"y.go","symbol":"Y"}]}]}`)
	if !strings.Contains(out, "MOD-0003 new") || strings.Contains(out, "MOD-0002") {
		t.Fatalf("a deleted id must not be reused or keep its candidate, got:\n%s", out)
	}
}

func TestLostSnapshotWarnsAndContinues(t *testing.T) {
	root := newRepo(t)
	ingestOK(t, root, extraction)
	p := filepath.Join(root, ".abbd", "state.yaml")
	b, _ := os.ReadFile(p)
	lines := strings.Split(string(b), "\n")
	for i, l := range lines {
		if strings.HasPrefix(l, "fingerprint:") {
			lines[i] = "fingerprint: 0123456789012345678901234567890123456789"
		}
	}
	write(t, p, strings.Join(lines, "\n"))
	out, errOut := ingestErr(t, root, extraction)
	if !strings.Contains(errOut, "is gone") || !strings.Contains(out, "MOD-0001 matched") {
		t.Fatalf("a missing snapshot should warn and still ingest, stdout:\n%s\nstderr:\n%s", out, errOut)
	}
}

func TestEquivalentPathSpellingsMatch(t *testing.T) {
	root := newRepo(t)
	ingestOK(t, root, extraction)
	odd := strings.Replace(extraction, `"billing/charge.go","symbol":"Charge"`, `"billing\\charge.go","symbol":"Charge"`, 1)
	odd = strings.Replace(odd, `"billing/charge.go","symbol":"Refund"`, `"./billing/charge.go","symbol":" Refund "`, 1)
	out := ingestOK(t, root, odd)
	if !strings.Contains(out, "MOD-0001 matched") || strings.Contains(out, " new\n") {
		t.Fatalf("path spellings should normalise to the same anchor, got:\n%s", out)
	}
}

func TestSplitLeftoverBecomesCandidate(t *testing.T) {
	root := newRepo(t)
	ingestOK(t, root, `{"units":[{"kind":"MOD","name":"all","anchors":[{"path":"a.go","symbol":"A"},{"path":"a.go","symbol":"B"},{"path":"a.go","symbol":"C"},{"path":"a.go","symbol":"D"}]}]}`)
	out := ingestOK(t, root, `{"units":[{"kind":"MOD","name":"ab","anchors":[{"path":"a.go","symbol":"A"},{"path":"a.go","symbol":"B"}]},{"kind":"MOD","name":"cd","anchors":[{"path":"a.go","symbol":"C"},{"path":"a.go","symbol":"D"}]}]}`)
	if !strings.Contains(out, "MOD-0002 candidate-of MOD-0001") {
		t.Fatalf("the split-off half should be a candidate of the original, got:\n%s", out)
	}
}

func TestUnitInWrongFolderIsRejected(t *testing.T) {
	root := newRepo(t)
	ingestOK(t, root, extraction)
	a := filepath.Join(root, ".abbd", "architecture", "MOD-0001.md")
	b, _ := os.ReadFile(a)
	write(t, filepath.Join(root, ".abbd", "features", "MOD-0001.md"), string(b))
	if code, _, errOut := syncWith(t, root, extraction); code == 0 || !strings.Contains(errOut, "belongs in architecture/") {
		t.Fatalf("a MOD file under features/ should be rejected, exit %d: %s", code, errOut)
	}
}
