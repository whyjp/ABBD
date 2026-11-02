package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestPerfLargeTree times the commands that digest module texts on a tree of
// 20,000 files under 200 directory anchors in four modules, and reports the
// size of a stored text. It runs only with ABBD_PERF set.
func TestPerfLargeTree(t *testing.T) {
	if os.Getenv("ABBD_PERF") == "" {
		t.Skip("set ABBD_PERF=1 to time a large tree")
	}
	const modules, dirs, files = 4, 50, 100
	p, exe := bare(t)
	var topo []string
	for m := range modules {
		top := fmt.Sprintf("m%d", m)
		p.code(top+"/main.go", "package "+top+"\n\nfunc Main() {}\n")
		topo = append(topo, fmt.Sprintf(`{"key":%q,"kind":"MOD","name":%q,"anchors":[{"path":"%s/main.go","symbol":"Main"}]}`, top, top, top))
		for d := range dirs {
			dir := fmt.Sprintf("%s/d%03d", top, d)
			for f := range files {
				p.code(fmt.Sprintf("%s/f%03d.go", dir, f), fmt.Sprintf("package d\n\n// %s %d\nfunc F%d() {}\n", dir, f, f))
			}
			topo = append(topo, fmt.Sprintf(`{"key":"%s","kind":"MOD","name":"%s","parent":%q,"anchors":[{"path":%q}]}`, dir, dir, top, dir))
		}
	}
	git(t, p.root, "add", "-A")
	git(t, p.root, "commit", "-qm", "large")
	p.extract(`{"units":[` + strings.Join(topo, ",\n") + `]}`)
	timed := func(what string, f func()) {
		start := time.Now()
		f()
		t.Logf("%-12s %v", what, time.Since(start).Round(time.Millisecond))
	}
	timed("setup", func() { p.setup(0, "synced", "--", exe) })
	// The first open of a file just written is slow on Windows, where the
	// virus scanner reads it; reading every file once leaves that out.
	filepath.WalkDir(p.root, func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			os.ReadFile(path)
		}
		return nil
	})
	text := p.file(t, "how.md", "the module.\n")
	var first string
	for k, v := range units(t, p.root) {
		if strings.Contains(v, "name:\n    value: m") && !strings.Contains(v, "parents:") {
			id := strings.TrimSuffix(strings.TrimPrefix(k, "architecture/"), ".md")
			timed("understand", func() { p.want(0, "understood", "understand", id, text) })
			if strings.Contains(v, "m0/main.go") {
				first = id
			}
		}
	}
	timed("status", func() { p.want(0, "0 STALE", "status") })
	p.code("m0/d000/f000.go", "package d\n\nfunc Changed() {}\n")
	timed("sync", func() { p.want(0, "", "sync") })
	timed("status", func() { p.want(0, "1 STALE", "status") })
	fi, err := os.Stat(filepath.Join(p.root, ".abbd", "understanding", first+".md"))
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("text size    %d bytes", fi.Size())
}
