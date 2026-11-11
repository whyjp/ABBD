package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// The units a module declares rank by how often the rest of the repository
// names them, plain words and macros left out, and a text that never names
// one is told so.
func TestKeyUnitsRankByFanIn(t *testing.T) {
	root := t.TempDir()
	write := func(p, s string) {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, p)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, p), []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("billing/charge.go", "package billing\n\ntype Ledger struct{}\n\nfunc ChargeCard() {}\n\nfunc RefundCard() {}\n\nfunc lock() {}\n")
	write("web/a.go", "package web\n\nvar _ = billing.ChargeCard\nvar _ billing.Ledger\nvar _ = billing.ChargeCard\n")
	write("web/b.go", "package web\n\nvar _ = billing.ChargeCard\nfunc x() { lock() }\n")
	for _, args := range [][]string{{"init", "-q"}, {"add", "-A"}} {
		if out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	got := keyUnits(root, []string{"billing/charge.go"}, 8)
	if len(got) != 2 || got[0].Name != "ChargeCard" || got[0].Uses != 3 || got[1].Name != "Ledger" {
		t.Fatalf("key units: %+v", got)
	}
	if u := unnamed("The **Ledger** keeps charges.", got); len(u) != 1 || u[0].Name != "ChargeCard" {
		t.Errorf("unnamed: %+v", u)
	}
}

// A Python dunder is no key unit; a snake-case function is.
func TestADunderIsNoKeyUnit(t *testing.T) {
	if distinctive("__init__") || distinctive("_private_") || !distinctive("charge_card") {
		t.Errorf("distinctive misjudges a dunder or a snake-case name")
	}
}
