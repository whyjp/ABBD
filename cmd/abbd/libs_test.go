package main

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/whyjp/abbd/internal/model"
)

const libsExtraction = `{"units":[
 {"key":"store","kind":"MOD","name":"store","anchors":[{"path":"store"}]},
 {"key":"cli","kind":"MOD","name":"cli","anchors":[{"path":"cli"}]}
],"relations":[{"from":"cli","to":"store","kind":"depends_on","via":"import"}]}`

// abbd libs adds each library the go.mod requires as one EXT unit, and sync
// keeps it like any extracted unit: the same ID on the next sync, a line in
// explain, and a chip on the overview.
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
	if !strings.Contains(p.blueprint(), "yaml.v3: 외부 라이브러리. 1개 상자가 사용") {
		t.Errorf("the overview does not draw the library as a chip one box uses")
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

// An external library is described by its name: lint and status never ask
// it for a purpose.
func TestAnExternalLibraryNeedsNoPurpose(t *testing.T) {
	t.Parallel()
	us := []model.Unit{
		{ID: "MOD-0001", Kind: model.Module, Name: model.Said(model.Script, "store")},
		{ID: "EXT-0001", Kind: model.External, Name: model.Said(model.Script, "yaml")},
	}
	if got := undescribed(us); !slices.Equal(got, []model.ID{"MOD-0001"}) {
		t.Errorf("undescribed %v, want only the module", got)
	}
}
