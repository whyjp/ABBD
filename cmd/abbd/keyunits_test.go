package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func gitRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for p, s := range files {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, p)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, p), []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{{"init", "-q"}, {"add", "-A"}} {
		if out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return root
}

// The names a module declares rank by how many files outside it name them,
// not by how often: a name used three times in one file ranks below one
// used once in each of two files. Plain words, the module's own files and
// tests do not count.
func TestKeyUnitsRankByTheFilesOfOtherModulesThatNameThem(t *testing.T) {
	root := gitRepo(t, map[string]string{
		"billing/charge.go":  "package billing\n\ntype Ledger struct{}\n\nfunc ChargeCard() {}\n\nfunc RefundCard() {}\n\nfunc lock() {}\n",
		"billing/charge2.go": "package billing\n\nvar _ = ChargeCard\n",
		"web/a.go":           "package web\n\nvar _ = billing.ChargeCard\nvar _ = billing.ChargeCard\nvar _ = billing.ChargeCard\nvar _ billing.Ledger\n",
		"web/b.go":           "package web\n\nvar _ billing.Ledger\nfunc x() { lock() }\n",
		"web/b_test.go":      "package web\n\nvar _ = billing.RefundCard\n",
	})
	got := keyUnits(root, []string{"billing/charge.go", "billing/charge2.go"}, 8)
	if len(got) != 2 || got[0].Name != "Ledger" || got[0].Users != 2 || got[1].Name != "ChargeCard" || got[1].Users != 1 {
		t.Fatalf("names other modules use most: %+v", got)
	}
}

// A licence or readme beside the code holds prose the declaration patterns
// would misread, and a generic name says nothing of the module, so neither
// is a candidate.
func TestKeyUnitsReadOnlyCodeAndSkipGenericNames(t *testing.T) {
	root := gitRepo(t, map[string]string{
		"rtmp/COPYING":   "GNU Lesser General Public License (version 2.1)\nclass Covenant {\n",
		"rtmp/README.md": "type Widget struct\n",
		"rtmp/rtmp.c":    "struct Output {\nstruct RTMPPacket {\nint with_capacity(int n)\n",
		"app/main.c":     "Output o; RTMPPacket p; with_capacity(1); Widget w; Covenant c; License l;\n",
	})
	got := keyUnits(root, []string{"rtmp/COPYING", "rtmp/README.md", "rtmp/rtmp.c"}, 8)
	if len(got) != 1 || got[0].Name != "RTMPPacket" || got[0].File != "rtmp/rtmp.c" {
		t.Fatalf("the one candidate is RTMPPacket: %+v", got)
	}
}

// A Python dunder is no key unit; a snake-case function is.
func TestADunderIsNoKeyUnit(t *testing.T) {
	if distinctive("__init__") || distinctive("_private_") || !distinctive("charge_card") {
		t.Errorf("distinctive misjudges a dunder or a snake-case name")
	}
}
