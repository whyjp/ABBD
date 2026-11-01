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
	p, exe := bare(t)
	p.code("lib/a.py", "a\n")
	p.extract(`{"units":[
 {"key":"a","kind":"MOD","name":"alpha","anchors":[{"path":"lib"},{"path":"lib/a.py"}]},
 {"key":"b","kind":"MOD","name":"beta","anchors":[{"path":"lib"}]}]}`)
	p.setup(1, `MOD "alpha" and MOD "beta"`, "--supplement", p.file(t, "sup.json", `{"units":[
 {"key":"x","kind":"MOD","name":"lib","anchors":[{"path":"lib"}],"purpose":"p"}]}`), "--", exe)
}
