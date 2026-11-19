package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const libsExtraction = `{"units":[
 {"key":"store","kind":"MOD","name":"store","anchors":[{"path":"store"}]},
 {"key":"cli","kind":"MOD","name":"cli","anchors":[{"path":"cli"}]}
],"relations":[{"from":"cli","to":"store","kind":"depends_on","via":"import"}]}`

// abbd libs adds each library the go.mod requires as one EXT unit, and sync
// keeps it like any extracted unit: the same ID on the next sync, a line in
// explain.
func TestLibsAddsExternalUnitsThatSyncKeeps(t *testing.T) {
	t.Parallel()
	p, exe := bare(t)
	p.code("go.mod", "module example.com/app\n\ngo 1.22\n\nrequire gopkg.in/yaml.v3 v3.0.1\n")
	p.code("store/store.go", "package store\n\nimport \"gopkg.in/yaml.v3\"\n\nvar _ = yaml.Marshal\n")
	p.code("cli/main.go", "package main\n\nimport _ \"example.com/app/store\"\n")
	p.extract(libsExtraction)
	var out, errb bytes.Buffer
	if code := run([]string{"libs", "--root", p.root, "--", exe}, nil, &out, &errb); code != 0 {
		t.Fatalf("abbd libs: exit %d\n%s", code, errb.String())
	}
	doc := out.String()
	if !strings.Contains(doc, `"kind": "EXT"`) || !strings.Contains(doc, `"to": "lib:go:gopkg.in/yaml.v3"`) {
		t.Fatalf("abbd libs printed no library:\n%s", doc)
	}
	p.extract(doc)
	p.setup(0, "synced", "--", exe)
	status := p.want(0, "1 EXT", "status")
	if strings.Contains(status, "system: EXT") || strings.Contains(status, "library: EXT") {
		t.Errorf("status reported the external library as one to leave out:\n%s", status)
	}
	explain := p.want(0, "gopkg.in/yaml.v3", "explain", "EXT-0001")
	if !strings.Contains(explain, "store") {
		t.Errorf("explain of the library names no module that uses it:\n%s", explain)
	}
	p.want(0, "", "sync")
	if _, err := os.Stat(filepath.Join(p.root, ".abbd", "architecture", "EXT-0001.md")); err != nil {
		t.Errorf("the library lost its ID on the next sync: %v", err)
	}
}

// Neither a reader nor the supplement makes an external library: the
// manifest declares it.
func TestRecordRefusesAnExternalUnit(t *testing.T) {
	t.Parallel()
	p, exe := bare(t)
	p.code("go.mod", "module example.com/app\n\nrequire gopkg.in/yaml.v3 v3.0.1\n")
	p.code("store/store.go", "package store\n")
	p.extract(libsExtraction)
	p.setup(0, "synced", "--", exe)
	p.want(1, "abbd libs", "record", "unit", "EXT", "yaml", "--evidence", "go.mod:3:require gopkg.in/yaml.v3 v3.0.1")
}
