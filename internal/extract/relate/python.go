package relate

import (
	"path"
	"strings"
)

// pythonInstall says how to get pyright's language server.
const pythonInstall = "set ABBD_LSP_PYTHON to its path, or install it: `npm i -g pyright` or `pip install pyright`, which provide pyright-langserver"

var pythonLang = scriptLang{
	exts: map[string]string{".py": "python"},
	test: pyTest,
	// A name with a leading underscore is private by convention.
	skip: func(name string) bool { return name == "" || strings.HasPrefix(name, "_") },
}

// pyTest is whether a path names test code, which the relater leaves out
// by design.
func pyTest(p string) bool {
	return cTest(p) || path.Base(p) == "conftest.py"
}

// python reads Python with pyright's language server over the directory as
// its project root.
func python(t tree, o owners) (found, error) {
	bin, err := scriptServer("ABBD_LSP_PYTHON", "pyright-langserver", pythonInstall, false)
	if err != nil {
		return found{}, err
	}
	return scripting(t, o, pythonLang, []string{bin, "--stdio"}, nil)
}
