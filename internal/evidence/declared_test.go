package evidence

import "testing"

const goSource = `package store

import "fmt"

// Load reads the file.
// It never writes.
func (s *Store) Load(
	path string,
) (string, error) {
	return fmt.Sprint(Save), nil
}

func Save() {}

const (
	limit = 3
	tries = 5
)

type Flow struct {
	Steps []string
}
`

const pySource = `import os

@cache
def load(path):
    if path:
        return os.read(path)

    return None

def save():
    pass
`

// A symbol's code is its declaration, the comments right above it, and the
// lines indented under it to its closing brace; a use of the name elsewhere
// is no declaration.
func TestASymbolsCodeIsWhereTheFileDeclaresIt(t *testing.T) {
	root := tree(t)
	write(t, root, "store.go", goSource)
	write(t, root, "load.py", pySource)
	c := New(root)
	for _, w := range []struct {
		path, symbol string
		first, last  int
	}{
		{"store.go", "Load", 5, 11},
		{"store.go", "Save", 13, 13},
		{"store.go", "tries", 17, 17},
		{"store.go", "Flow", 20, 22},
		{"load.py", "load", 3, 8},
		{"load.py", "save", 10, 11},
	} {
		first, last, ok := c.Declared(w.path, w.symbol)
		if !ok || first != w.first || last != w.last {
			t.Errorf("%s declares %s at %d-%d, got %d-%d (%v)", w.path, w.symbol, w.first, w.last, first, last, ok)
		}
	}
	for _, s := range []string{"Sprint", "Missing", "path"} {
		if _, _, ok := c.Declared("store.go", s); ok {
			t.Errorf("store.go declares no %s", s)
		}
	}
	if _, _, ok := c.Declared("gone.go", "Load"); ok {
		t.Error("a gone file declares nothing")
	}
}
