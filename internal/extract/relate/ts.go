package relate

import (
	"os"
	"path"
	"path/filepath"
	"strings"
)

// tsInstall says how to get typescript-language-server, which needs a
// TypeScript 5.x with tsserver.js beside it.
const tsInstall = "set ABBD_LSP_TS to its path, or install it: `npm i -g typescript-language-server typescript` (TypeScript 7 ships no tsserver.js, so 5.x is needed)"

var tsLang = scriptLang{
	exts: map[string]string{
		".ts": "typescript", ".mts": "typescript", ".cts": "typescript", ".tsx": "typescriptreact",
		".js": "javascript", ".mjs": "javascript", ".cjs": "javascript", ".jsx": "javascriptreact",
	},
	test: tsTest,
	skip: func(name string) bool { return name == "" || strings.HasPrefix(name, "<") },
}

// tsTest is whether a path names test code, which the relater leaves out
// by design: a .test or .spec file, or under a test, tests, __tests__ or
// __mocks__ directory.
func tsTest(p string) bool {
	if cTest(p) {
		return true
	}
	dirs := strings.Split(strings.ToLower(p), "/")
	for _, d := range dirs[:len(dirs)-1] {
		if d == "__tests__" || d == "__mocks__" || d == "e2e" {
			return true
		}
	}
	name := strings.TrimSuffix(dirs[len(dirs)-1], path.Ext(p))
	return strings.HasSuffix(name, ".test") || strings.HasSuffix(name, ".spec")
}

// typescript reads TypeScript and JavaScript with typescript-language-server,
// which runs the workspace's own tsconfig.json or jsconfig.json.
func typescript(t tree, o owners) (found, error) {
	bin, err := scriptServer("ABBD_LSP_TS", "typescript-language-server", tsInstall, true)
	if err != nil {
		return found{}, err
	}
	var init map[string]any
	if p := tsserverNear(bin); p != "" {
		init = map[string]any{"tsserver": map[string]any{"path": p}}
	}
	return scripting(t, o, tsLang, []string{bin, "--stdio"}, init)
}

// tsserverNear finds the tsserver.js of the typescript package installed
// beside the server, in the node_modules that holds it, so a global install
// reads the TypeScript it was installed with.
func tsserverNear(bin string) string {
	if r, err := filepath.EvalSymlinks(bin); err == nil {
		bin = r
	}
	for d := filepath.Dir(bin); ; d = filepath.Dir(d) {
		for _, rel := range []string{"typescript", filepath.Join("node_modules", "typescript")} {
			p := filepath.Join(d, rel, "lib", "tsserver.js")
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
		if filepath.Dir(d) == d {
			return ""
		}
	}
}
