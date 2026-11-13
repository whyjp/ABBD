package main

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

// nested is the shape abbd-setup recommends: a package module anchored on its
// directory and every file in it, and a module inside it anchored the same way.
const nested = `{"units":[
 {"key":"app","kind":"MOD","name":"app","anchors":[{"path":"app"},{"path":"app/main.py"},{"path":"app/sub"}]},
 {"key":"app/sub","kind":"MOD","name":"app/sub","parent":"app","anchors":[{"path":"app/sub"},{"path":"app/sub/x.py"}]}
]}`

func nestedProject(t *testing.T, sup string) (*project, string) {
	t.Helper()
	p, exe := bare(t)
	p.code("app/main.py", "def serve():\n    return 'GET /x'\n")
	p.code("app/sub/x.py", "INSERT INTO charges\n")
	p.extract(nested)
	var o, e bytes.Buffer
	code := run([]string{"setup", "--root", p.root, "--supplement", p.file(t, "sup.json", sup), "--", exe}, neverRead{t}, &o, &e)
	out := o.String() + e.String()
	return p, fmt.Sprintf("exit %d\n%s", code, out)
}

func (p *project) setupCode(out string) int {
	if strings.HasPrefix(out, "exit 0\n") {
		return 0
	}
	return 1
}

// A reader's unit may sit inside a module only the script reports, named by
// the script's key, without restating that module in the supplement.
func TestSupplementUnitTakesAScriptModuleAsItsParent(t *testing.T) {
	t.Parallel()
	p, out := nestedProject(t, `{"units":[
 {"key":"charges","kind":"DATA","name":"charges table","parent":"app/sub",
  "evidence":[{"path":"app/sub/x.py","lines":"1","quote":"INSERT INTO charges"}]}]}`)
	if p.setupCode(out) != 0 {
		t.Fatalf("setup should accept a parent that is a script key:\n%s", out)
	}
	contains(t, p.want(0, "", "explain", "charges table"), "inside MOD-0002 app/sub\n")
}

// Restating a script module by one anchor its parent also holds names the
// inner module, the one the anchor is most specific to.
func TestSupplementMatchesTheInnermostModuleOnATie(t *testing.T) {
	t.Parallel()
	p, out := nestedProject(t, `{"units":[
 {"key":"s","kind":"MOD","name":"app/sub","anchors":[{"path":"app/sub"}],"responsibility":"Writes charges."}]}`)
	if p.setupCode(out) != 0 {
		t.Fatalf("setup should match the inner module:\n%s", out)
	}
	contains(t, p.want(0, "", "explain", "app/sub"), "responsibility: Writes charges.")
	if strings.Contains(p.want(0, "", "explain", "MOD-0001"), "Writes charges.") {
		t.Fatalf("the parent must not take the inner module's responsibility")
	}
}

// Two unrelated modules that share the anchor stay ambiguous, and the error
// names both.
func TestSupplementTieBetweenUnrelatedModulesNamesThem(t *testing.T) {
	t.Parallel()
	p, exe := bare(t)
	p.code("lib/a.py", "a\n")
	p.extract(`{"units":[
 {"key":"a","kind":"MOD","name":"alpha","anchors":[{"path":"lib"},{"path":"lib/a.py"}]},
 {"key":"b","kind":"MOD","name":"beta","anchors":[{"path":"lib"}]}]}`)
	p.setup(1, `MOD "alpha" and MOD "beta"`, "--supplement", p.file(t, "sup.json", `{"units":[
 {"key":"x","kind":"MOD","name":"lib","anchors":[{"path":"lib"}],"purpose":"p"}]}`), "--", exe)
}

// A reader finds a module inside a package the script anchors on its
// directory and files, so its quote sits on one of the parent's anchors. It
// names that parent, so it is code inside the parent, not the parent itself.
func TestSupplementInsideModuleUnderADirectoryAnchoredParent(t *testing.T) {
	t.Parallel()
	for _, symbol := range []string{"", `"symbol":"serve",`} {
		p, out := nestedProject(t, `{"units":[
 {"key":"app","kind":"MOD","name":"app","anchors":[{"path":"app/main.py"}]},
 {"key":"serve","kind":"MOD","name":"serve","parent":"app",
  "evidence":[{"path":"app/main.py","lines":"2",`+symbol+`"quote":"return 'GET /x'"}]}]}`)
		if p.setupCode(out) != 0 {
			t.Fatalf("setup should accept an inside module under the parent it names (%s):\n%s", symbol, out)
		}
		contains(t, p.want(0, "", "explain", "serve"), "serve (MOD, INFERRED)\ninside MOD-0001 app\n")
		p.want(0, "", "sync")
		contains(t, p.want(0, "", "explain", "serve"), "inside MOD-0001 app\n")
		contains(t, p.want(0, "", "explain", "MOD-0001"), "app (MOD, EXTRACTED)")
	}
	_, out := nestedProject(t, `{"units":[
 {"key":"serve","kind":"MOD","name":"serve",
  "evidence":[{"path":"app/main.py","lines":"2","quote":"return 'GET /x'"}]}]}`)
	contains(t, out, "exit 1\n", `would merge into it`)
}

// When the parent's anchors are exactly the unit's, nothing tells the two
// apart, so the unit is still refused.
func TestSupplementInsideModuleNeedsAnAnchorItsParentLacks(t *testing.T) {
	t.Parallel()
	p, exe := bare(t)
	p.code("app/main.py", "def serve():\n    return 'GET /x'\n")
	p.extract(`{"units":[{"key":"app","kind":"MOD","name":"app","anchors":[{"path":"app/main.py"}]}]}`)
	p.setup(1, "would merge into it", "--supplement", p.file(t, "sup.json", `{"units":[
 {"key":"serve","kind":"MOD","name":"serve","parent":"app",
  "evidence":[{"path":"app/main.py","lines":"2","quote":"return 'GET /x'"}]}]}`), "--", exe)
}

// A contract only a reader found has no signature in the code, so the one
// the reader documents is the only one there is. It stays, INFERRED.
func TestReaderFoundContractKeepsItsSignature(t *testing.T) {
	t.Parallel()
	p, out := nestedProject(t, `{"units":[
 {"key":"app","kind":"MOD","name":"app","anchors":[{"path":"app/main.py"}]},
 {"key":"api","kind":"IF","name":"http api","parent":"app","signature":"GET /x",
  "evidence":[{"path":"app/main.py","lines":"2","quote":"return 'GET /x'"}]}]}`)
	if p.setupCode(out) != 0 || strings.Contains(out, "conflict") {
		t.Fatalf("setup should keep the reader's signature without a conflict:\n%s", out)
	}
	contains(t, p.want(0, "", "explain", "http api"), "signature: GET /x (INFERRED)")
	p.want(0, "", "sync")
	contains(t, p.want(0, "", "explain", "http api"), "signature: GET /x (INFERRED)")
}

// The same holds for a contract the script reports without a signature. A
// signature the script does read still wins, and a different one the
// supplement documents is a conflict.
func TestScriptContractWithoutSignatureTakesTheReaders(t *testing.T) {
	t.Parallel()
	p, exe := bare(t)
	p.extract(codeOnly)
	sup := strings.Replace(supplement, `"name":"http api",`, `"name":"http api","signature":"GET /charges?all",`, 1)
	contains(t, p.setup(0, "", "--supplement", p.file(t, "sup.json", sup), "--", exe), `documents signature "GET /charges?all", code has "GET /charges"`)
	contains(t, p.want(0, "", "explain", "http api"), "signature: GET /charges (EXTRACTED)")

	p.extract(strings.Replace(codeOnly, `"signature":"GET /charges",`, "", 1))
	out := p.setup(0, "", "--supplement", p.file(t, "sup2.json", sup))
	if strings.Contains(out, "conflict") {
		t.Fatalf("a signature the script cannot read is no conflict:\n%s", out)
	}
	contains(t, p.want(0, "", "explain", "http api"), "signature: GET /charges?all (INFERRED)")
	p.want(0, "", "sync")
	contains(t, p.want(0, "", "explain", "http api"), "signature: GET /charges?all (INFERRED)")
}

// A flat package's script reports one module per file, and the reader groups
// them into responsibilities. The grouping quotes its members' code, since no
// other code is there to quote, and its members name it as their parent, so
// it stays a module of its own and no member merges into it, through setup
// and the syncs after. A grouping quoting code of a module that does not name
// it is still refused.
func TestSupplementGroupingQuotesItsMembersCode(t *testing.T) {
	t.Parallel()
	p, exe := bare(t)
	p.code("pkg/globals.py", "from .ctx import _cv_app\ncurrent_app = _cv_app.get\n")
	p.code("pkg/ctx.py", "_cv_app = ContextVar('flask.app_ctx')\n")
	p.code("pkg/app.py", "class Flask:\n    pass\n")
	p.extract(`{"units":[
 {"key":"f:globals","kind":"MOD","name":"globals","anchors":[{"path":"pkg/globals.py"}]},
 {"key":"f:ctx","kind":"MOD","name":"ctx","anchors":[{"path":"pkg/ctx.py"}]},
 {"key":"f:app","kind":"MOD","name":"app","anchors":[{"path":"pkg/app.py"}]}]}`)
	member := func(key, file, quote string) string {
		return `{"key":"` + key + `","kind":"MOD","name":"` + key[2:] + `","anchors":[{"path":"pkg/` + file + `"}],"parent":"grp:context",
  "parent_evidence":[{"path":"pkg/` + file + `","quote":"` + quote + `"}]}`
	}
	group := `{"key":"grp:context","kind":"MOD","name":"request context",
  "evidence":[{"path":"pkg/globals.py","lines":"2","quote":"current_app = _cv_app.get"}]}`
	sup := `{"units":[` + group + `,
 ` + member("f:globals", "globals.py", "from .ctx import _cv_app") + `,
 ` + member("f:ctx", "ctx.py", "_cv_app = ContextVar('flask.app_ctx')") + `]}`
	p.setup(0, "", "--supplement", p.file(t, "sup.json", sup), "--", exe)
	check := func(when string) {
		t.Helper()
		g := p.want(0, "", "explain", "request context")
		contains(t, g, "request context (MOD, INFERRED)", "inside:\n", "globals (MOD)", "ctx (MOD)")
		contains(t, p.want(0, "", "explain", "globals"), "globals (MOD, EXTRACTED)", "inside MOD-0004 request context")
		if n := len(units(t, p.root)); n != 4 {
			t.Fatalf("%s: want 4 units, the grouping beside its three file modules, got %d", when, n)
		}
	}
	check("after setup")
	p.want(0, "", "sync")
	check("after sync")
	p.want(0, "", "sync")
	check("after a second sync")

	q, exe := bare(t)
	q.code("pkg/globals.py", "from .ctx import _cv_app\ncurrent_app = _cv_app.get\n")
	q.code("pkg/ctx.py", "_cv_app = ContextVar('flask.app_ctx')\n")
	q.code("pkg/app.py", "class Flask:\n    pass\n")
	q.extract(`{"units":[
 {"key":"f:globals","kind":"MOD","name":"globals","anchors":[{"path":"pkg/globals.py"}]},
 {"key":"f:ctx","kind":"MOD","name":"ctx","anchors":[{"path":"pkg/ctx.py"}]}]}`)
	q.setup(1, "would merge into it", "--supplement", q.file(t, "sup.json", `{"units":[
 {"key":"grp:context","kind":"MOD","name":"request context",
  "evidence":[{"path":"pkg/globals.py","lines":"2","quote":"current_app = _cv_app.get"}]},
 `+member("f:ctx", "ctx.py", "_cv_app = ContextVar('flask.app_ctx')")+`]}`), "--", exe)

	r, exe := bare(t)
	r.code("pkg/globals.py", "from .ctx import _cv_app\ncurrent_app = _cv_app.get\n")
	r.code("pkg/ctx.py", "_cv_app = ContextVar('flask.app_ctx')\n")
	r.extract(`{"units":[
 {"key":"f:globals","kind":"MOD","name":"globals","anchors":[{"path":"pkg/globals.py"}]},
 {"key":"f:ctx","kind":"MOD","name":"ctx","anchors":[{"path":"pkg/ctx.py"}]}]}`)
	r.setup(1, "would merge into it", "--supplement", r.file(t, "sup.json", `{"units":[`+group+`,
 `+member("f:ctx", "ctx.py", "_cv_app = ContextVar('flask.app_ctx')")+`,
 {"key":"f:globals","kind":"MOD","name":"globals","anchors":[{"path":"pkg/globals.py"}]},
 {"key":"f:globals2","kind":"MOD","name":"globals again","anchors":[{"path":"pkg/globals.py"}],"parent":"grp:context",
  "parent_evidence":[{"path":"pkg/globals.py","quote":"from .ctx import _cv_app"}]}]}`), "--", exe)

	s2, exe := bare(t)
	s2.code("pkg/globals.py", "from .ctx import _cv_app\ncurrent_app = _cv_app.get\n")
	s2.code("pkg/ctx.py", "_cv_app = ContextVar('flask.app_ctx')\n")
	s2.extract(`{"units":[
 {"key":"f:globals","kind":"MOD","name":"globals","anchors":[{"path":"pkg/globals.py"}]},
 {"key":"f:ctx","kind":"MOD","name":"ctx","anchors":[{"path":"pkg/ctx.py"}]}]}`)
	s2.setup(1, "would merge into it", "--supplement", s2.file(t, "sup.json", `{"units":[`+group+`,
 `+member("f:globals", "globals.py", "from .ctx import _cv_app")+`]}`), "--", exe)
}
