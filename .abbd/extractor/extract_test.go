package main

import (
	"go/token"
	"os"
	"path/filepath"
	"testing"
)

// A package dependency draws as a call where the dependent calls one of the
// other package's functions or methods, and as an import where it names
// only types or values, a type that shares a method's name among them.
func TestViaReadsCallsFromCode(t *testing.T) {
	const a = `package a

type T struct{}

type Kind int

func F() {}

func (T) M() {}

func (T) Kind() Kind { return 0 }

var V = 1
`
	for _, c := range []struct {
		name, use, want string
	}{
		{"a type", `var _ a.T`, "import"},
		{"a value", `var _ = a.V`, "import"},
		{"a type named like a method", `var _ a.Kind`, "import"},
		{"a package function", `func f() { a.F() }`, "call"},
		{"a method", `func f(t a.T) { t.M() }`, "call"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			write(t, "internal/a/a.go", a)
			write(t, "internal/b/b.go", "package b\n\nimport \"github.com/whyjp/abbd/internal/a\"\n\n"+c.use+"\n")
			fset, files, decls = token.NewFileSet(), map[string]*srcFile{}, map[string]map[string]string{}
			methods, funcs, types = map[string]bool{}, map[string]bool{}, map[string]typeDecl{}
			if err := parseTree("internal"); err != nil {
				t.Fatal(err)
			}
			refs := references(map[string]string{"internal/a/a.go": "mod-a", "internal/b/b.go": "mod-b"})
			r := relation{"mod-b", "mod-a", "depends_on"}
			if got := via(r, nil, map[relation]bool{r: refs["mod-b"]["mod-a"]}); got != c.want {
				t.Errorf("b naming %s draws via %s, want %s", c.name, got, c.want)
			}
		})
	}
}

func write(t *testing.T, p, src string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
}
