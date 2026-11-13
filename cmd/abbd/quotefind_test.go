package main

import (
	"strings"
	"testing"
)

// quoteProject is a repository whose app box runs to a ledger box that
// anchors a directory and a file of it with no symbol, with the app's
// files as given: the extraction anchors app by its directory.
func quoteProject(t *testing.T, ext string, files map[string]string) string {
	t.Helper()
	p, exe := bare(t)
	for f, body := range files {
		p.code(f, body)
	}
	p.code("billing/ledger/book."+ext, "x\n")
	p.code("billing/ledger/entry."+ext, "x\n")
	p.code("app/start."+ext, "x\n")
	p.extract(`{"units":[
 {"key":"app","kind":"MOD","name":"app","anchors":[{"path":"app"},{"path":"app/start.` + ext + `"}]},
 {"key":"ledger","kind":"MOD","name":"posting","anchors":[{"path":"billing/ledger"},{"path":"billing/ledger/book.` + ext + `"}]}
],"relations":[
 {"from":"app","to":"ledger","kind":"depends_on","via":"import"}
]}`)
	p.setup(0, "synced: 2 unit(s)", "--", exe)
	return p.want(0, "", "overview")
}

// A line of a test fixture, a page of prose or a test is no code of the box,
// so the draft never quotes it, even where it is the only line that names
// what the box runs to; the same line in the box's code is found.
func TestTheOverviewDraftQuotesNoFixtureOrProse(t *testing.T) {
	t.Parallel()
	line := "import \"example.com/shop/billing/ledger\"\n"
	only := map[string]string{
		"app/testdata/flows/sync.md": "- path: billing/ledger/book.go\n" + line,
		"app/fixtures/cases.go":      "package fixtures\n\n" + line,
		"app/README.md":              "Runs ledger.Post() " + line,
		"app/main_test.go":           "package app\n\n" + line,
		"app/data.json":              `{"import": "billing/ledger"}` + "\n",
	}
	draft := quoteProject(t, "go", only)
	contains(t, draft, "no line of app names what it runs to")
	for _, bad := range []string{"testdata", "fixtures", "README", "main_test", "data.json"} {
		if strings.Contains(draft, "path: \""+"app/"+bad) || strings.Contains(draft, "path: \"app/"+bad) {
			t.Errorf("the draft quotes %s:\n%s", bad, draft)
		}
	}
	only["app/main.go"] = "package app\n\n" + line
	contains(t, quoteProject(t, "go", only), `evidence: [{path: "app/main.go", lines: "3", quote: "import \"example.com/shop/billing/ledger\""}]`)
}

// Whatever language the box is written in, the draft finds the line that
// reaches the unit it runs to by the unit's place in the tree: the import
// line that spells its directory as a path, dotted or with ::, or a
// qualified use of its last directory, which is a plain word and so no
// identifier to search for.
func TestTheOverviewDraftFindsAQuoteByTheUnitsPlaceInTheTree(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ lang, ext, line, want string }{
		{"go import", "go", `import "example.com/shop/billing/ledger"`, `names billing/ledger`},
		{"go qualified use", "go", "\tledger.Post(1)", "names ledger."},
		{"js import", "js", "const l = require('../billing/ledger')", "names billing/ledger"},
		{"python import", "py", "from billing.ledger import post", "names billing/ledger"},
		{"python qualified use", "py", "    ledger.post(1)", "names ledger."},
		{"rust import", "rs", "use crate::billing::ledger;", "names billing/ledger"},
		{"rust qualified use", "rs", "    ledger::post(1);", "names ledger."},
		{"c include", "c", `#include "billing/ledger/book.h"`, "names billing/ledger"},
	} {
		t.Run(c.lang, func(t *testing.T) {
			t.Parallel()
			draft := quoteProject(t, c.ext, map[string]string{"app/main." + c.ext: "\n" + c.line + "\n"})
			contains(t, draft, "quote: "+quoteOf(c.line), c.want)
		})
	}
}

// A comment that names a unit, and the line that defines a function of the
// same name, are no use of it.
func TestTheOverviewDraftQuotesNoCommentOrDefinition(t *testing.T) {
	t.Parallel()
	draft := quoteProject(t, "go", map[string]string{"app/main.go": "package app\n\n// ledger.Post posts\nfunc Wrap() {}\n// see \"example.com/shop/billing/ledger\"\n"})
	contains(t, draft, "no line of app names what it runs to")
}

func quoteOf(line string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(strings.TrimSpace(line)) + `"`
}
