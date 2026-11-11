package main

import "testing"

func TestADraftIsTheFirstSentenceOfTheCommentThatDeclaresTheSymbol(t *testing.T) {
	goSrc := "// Copyright 2024 The Authors. Licensed MIT.\n\n// Package billing charges cards. It retries twice.\npackage billing\n\nfunc use() { scope() }\n\n// scope declares what a task touches. More here.\nfunc (e env) scope(list string) error { return nil }\n"
	for _, c := range []struct {
		name, text, symbol string
		md                 bool
		want               string
	}{
		{"declaration over first use", goSrc, "scope", false, "scope declares what a task touches."},
		{"file comment past the licence", goSrc, "", false, "Package billing charges cards."},
		{"python", "#!/usr/bin/env python\n# Loads the config.\nimport os\n", "", false, "Loads the config."},
		{"rust doc", "/// Parses one line\npub fn parse(s: &str) {}\n", "parse", false, "Parses one line"},
		{"no comment", "func scope() {}\n", "scope", false, ""},
		{"markdown", "---\nname: x\n---\n# Title\n\nThe skill reads the blueprint. Then more.\n", "", true, "The skill reads the blueprint."},
	} {
		if got := firstSentence(docComment(c.text, c.symbol, c.md)); got != c.want {
			t.Errorf("%s: drafted %q, want %q", c.name, got, c.want)
		}
	}
}
