package parts

import (
	"slices"

	"testing"
)

func file(p, text string) File { return File{Path: p, Node: p, Text: text} }

func partOf(r Result, node string) *Part {
	for i := range r.Parts {
		if slices.Contains(r.Parts[i].Nodes, node) {
			return &r.Parts[i]
		}
	}
	return nil
}

func together(t *testing.T, r Result, nodes ...string) *Part {
	t.Helper()
	p := partOf(r, nodes[0])
	if p == nil {
		t.Fatalf("%s is in no part: %+v", nodes[0], r)
	}
	for _, n := range nodes[1:] {
		if !slices.Contains(p.Nodes, n) {
			t.Fatalf("%s and %s should share a part: %+v", nodes[0], n, r.Parts)
		}
	}
	return p
}

func apart(t *testing.T, r Result, a, b string) {
	t.Helper()
	if p := partOf(r, a); p != nil && slices.Contains(p.Nodes, b) {
		t.Fatalf("%s and %s should be in different parts: %+v", a, b, r.Parts)
	}
}

// Files that each construct the type another file declares are one family,
// even though none of them uses another.
func TestFilesThatConstructOneTypeAreOneFamily(t *testing.T) {
	r := Propose([]File{
		file("engine.go", "package web\n\ntype HandlerFunc func(*Request)\n\ntype Engine struct{ chain []HandlerFunc }\n\nfunc (e *Engine) Use(h HandlerFunc) { e.chain = append(e.chain, h) }\n"),
		file("request.go", "package web\n\ntype Request struct{ path string }\n\nfunc (r *Request) Next() {}\n"),
		file("auth.go", "package web\n\nfunc BasicAuth(user string) HandlerFunc {\n\treturn func(r *Request) { r.Next() }\n}\n"),
		file("logger.go", "package web\n\nfunc Logger() HandlerFunc {\n\treturn func(r *Request) { r.Next() }\n}\n"),
		file("recovery.go", "package web\n\nfunc Recovery() HandlerFunc {\n\treturn func(r *Request) { r.Next() }\n}\n"),
	})
	p := together(t, r, "auth.go", "logger.go", "recovery.go")
	if p.Family != "HandlerFunc" || p.Name != "handler func family" {
		t.Fatalf("the family part is named by its type: %+v", p)
	}
	apart(t, r, "auth.go", "engine.go")
}

// A helper only one file uses sits in that file's part; a subdirectory is a
// part of its own and keeps the files only it uses.
func TestAHelperSitsInsideThePartThatAloneUsesIt(t *testing.T) {
	r := Propose([]File{
		{Path: "app/sessions.py", Node: "app/sessions.py", Text: "from .cookies import CookiePolicy\n\n\nclass Session:\n    policy = CookiePolicy()\n    body = 1\n    more = 2\n"},
		{Path: "app/cookies.py", Node: "app/cookies.py", Text: "class CookiePolicy:\n    pass\n"},
		{Path: "app/client.py", Node: "app/client.py", Text: "from .sessions import Session\nfrom .output import write\n\n\ndef send():\n    write(Session())\n"},
		{Path: "app/output/writer.py", Node: "app/output/", Text: "from ..formatting import Formatter\n\n\ndef write(m):\n    Formatter().format(m)\n"},
		{Path: "app/output/streams.py", Node: "app/output/", Text: "class Stream:\n    pass\n"},
		{Path: "app/formatting.py", Node: "app/formatting.py", Text: "class Formatter:\n    def format(self, m):\n        return m\n"},
	})
	together(t, r, "app/sessions.py", "app/cookies.py")
	together(t, r, "app/output/", "app/formatting.py")
	apart(t, r, "app/output/", "app/sessions.py")
	if p := partOf(r, "app/sessions.py"); p.Name != "session" || len(p.Links) == 0 || p.Links[0].Quote != "from .cookies import CookiePolicy" {
		t.Fatalf("the part is named by its main type and cites the line that holds it together: %+v", p)
	}
}

// The single files several parts use are one shared layer.
func TestFilesSeveralPartsUseAreTheSharedLayer(t *testing.T) {
	r := Propose([]File{
		file("util.js", "function escape(s) { return s }\nmodule.exports.escape = escape\n"),
		file("compat.js", "const isWindows = false\nmodule.exports.isWindows = isWindows\n"),
		file("a/index.js", "const { escape } = require('../util')\nconst { isWindows } = require('../compat')\n"),
		{Path: "b/index.js", Node: "b/", Text: "const { escape } = require('../util')\nconst { isWindows } = require('../compat')\n"},
		{Path: "c/index.js", Node: "c/", Text: "const { escape } = require('../util')\nconst { isWindows } = require('../compat')\n"},
	})
	p := together(t, r, "util.js", "compat.js")
	if !p.Shared || p.Name != "shared layer" {
		t.Fatalf("util and compat are the shared layer: %+v", p)
	}
}

func TestProposalIsTheSameForAnyInputOrder(t *testing.T) {
	fs := []File{
		file("a.go", "package x\n\ntype A struct{}\n\nfunc NewA() *A { return &A{} }\n"),
		file("b.go", "package x\n\nfunc useA() { _ = NewA() }\n"),
		file("c.go", "package x\n\ntype C struct{ a *A }\n"),
	}
	want := Propose(fs)
	slices.Reverse(fs)
	got := Propose(fs)
	if len(got.Parts) != len(want.Parts) {
		t.Fatalf("order changed the proposal: %+v vs %+v", got, want)
	}
	for i := range got.Parts {
		if !slices.Equal(got.Parts[i].Nodes, want.Parts[i].Nodes) || got.Parts[i].Name != want.Parts[i].Name {
			t.Fatalf("order changed the proposal: %+v vs %+v", got.Parts, want.Parts)
		}
	}
}

func TestWordsSplitCodeNames(t *testing.T) {
	for in, want := range map[string]string{
		"RouterGroup":       "router group",
		"HTTPSAdapter":      "https adapter",
		"outlier_detection": "outlier detection",
		"loading-dots":      "loading dots",
		"parseURLPath":      "parse url path",
	} {
		if got := words(in); got != want {
			t.Errorf("words(%q) = %q, want %q", in, got, want)
		}
	}
}

// A mixed-case word three or more types start with, such as the project's
// name, is left out of the part names; an ordinary first word stays.
func TestABrandPrefixIsLeftOutOfNames(t *testing.T) {
	var srcs []*source
	for _, c := range []string{"XYZioHTTPAdapter", "XYZioCertificate", "XYZioCookiePolicy", "RouterGroup"} {
		srcs = append(srcs, load(file(c+".py", "class "+c+":\n    pass\n")))
	}
	ps := []Part{{Symbol: "XYZioCertificate", Name: words("XYZioCertificate")}, {Symbol: "RouterGroup", Name: words("RouterGroup")}}
	stripBrand(ps, srcs)
	if ps[0].Name != "certificate" || ps[1].Name != "router group" {
		t.Fatalf("only the shared brand word goes: %q, %q", ps[0].Name, ps[1].Name)
	}
}
