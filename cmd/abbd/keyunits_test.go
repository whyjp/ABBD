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

// A name a language or its library spells everywhere, or a plain word a help
// text starts a line with, is no key unit; a distinctive type is.
func TestAGenericNameIsNoKeyUnit(t *testing.T) {
	for _, line := range []string{"type names", "    type that the", "struct Debug;", "struct File;", "pub fn as_bytes(&self) -> &[u8] {", "pub fn into_bytes(self) -> Vec<u8> {"} {
		if got := declaration(line); got != "" {
			t.Errorf("%q declares %q, a name too generic to count", line, got)
		}
	}
	for line, want := range map[string]string{"pub struct HiArgs {": "HiArgs", "typedef struct lua_State {": "lua_State", "class SearchWorker:": "SearchWorker",
		"pub struct Event {": "Event", "class Node:": "Node"} {
		if got := declaration(line); got != want {
			t.Errorf("%q declares %q, want %q", line, got, want)
		}
	}
}

// A type a language spells in lower case is a key unit, because what makes a
// line a declaration is the shape after the name, not the name's case: Go's
// unexported types and C's structs are most of what those modules declare.
func TestALowerCaseTypeDeclarationStaysAKeyUnit(t *testing.T) {
	for line, want := range map[string]string{
		"type parser struct {":          "parser",
		"type state map[ID]Unit":        "state",
		"type env struct {":             "env",
		"type visitor interface {":      "visitor",
		"struct client {":               "client",
		"typedef struct dict {":         "dict",
		"struct inode {":                "inode",
		"type handler = Box<dyn Fn()>;": "handler",
		"struct ImGuiContext":           "ImGuiContext",
		"class Widget":                  "Widget",
		"type Kind string":              "Kind",
		"type Duration int64":           "Duration",
		"type Handler http.Handler":     "Handler",
		"type Names []*Name":            "Names",
	} {
		if got := declaration(line); got != want {
			t.Errorf("%q declares %q, want %q", line, got, want)
		}
	}
	for _, line := range []string{"type names", "type names that the", "  type names  ", "struct that is", "type names and kinds"} {
		if got := declaration(line); got != "" {
			t.Errorf("%q is prose, and declares %q", line, got)
		}
	}
}
