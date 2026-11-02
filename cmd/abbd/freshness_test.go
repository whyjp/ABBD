package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The blueprint is CURRENT only when its files are the ones the last sync
// rendered: any file missing, added, edited or left from an older render
// reads STALE, whatever the state says it was rendered from.
func TestABlueprintThatIsNotTheRenderedOneReadsStale(t *testing.T) {
	bp := func(p *project, name string) string { return filepath.Join(p.root, ".abbd", "blueprint", name) }
	for name, tamper := range map[string]func(p *project){
		"page deleted":  func(p *project) { os.Remove(bp(p, "index.html")) },
		"graph deleted": func(p *project) { os.Remove(bp(p, "graph.json")) },
		"module page deleted": func(p *project) {
			pages, _ := filepath.Glob(bp(p, "MOD-*.html"))
			if len(pages) == 0 {
				p.t.Fatal("the render wrote no module page")
			}
			os.Remove(pages[0])
		},
		"page edited":   func(p *project) { write(p.t, bp(p, "index.html"), p.read("blueprint/index.html")+"<p>hand edit</p>") },
		"graph emptied": func(p *project) { write(p.t, bp(p, "graph.json"), "{}") },
		"file added":    func(p *project) { write(p.t, bp(p, "extra.html"), "<p>stray</p>") },
		"older render": func(p *project) {
			old := map[string]string{"index.html": p.read("blueprint/index.html"), "graph.json": p.read("blueprint/graph.json")}
			p.extract(strings.ReplaceAll(ledgerTopo, `"name":"ledger"`, `"name":"books"`))
			p.resync()
			for f, b := range old {
				write(p.t, bp(p, f), b)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			p := semanticProject(t)
			p.want(0, "blueprint: CURRENT", "status", "--check")
			tamper(p)
			p.want(1, "the blueprint is STALE", "status", "--check")
			p.resync()
			p.want(0, "blueprint: CURRENT", "status", "--check")
		})
	}
}

// A checkout that converts line endings holds the same render.
func TestACRLFCheckoutOfTheBlueprintIsCurrent(t *testing.T) {
	p := semanticProject(t)
	for _, f := range []string{"index.html", "graph.json"} {
		path := filepath.Join(p.root, ".abbd", "blueprint", f)
		b, _ := os.ReadFile(path)
		write(t, path, strings.ReplaceAll(string(b), "\n", "\r\n"))
	}
	p.want(0, "blueprint: CURRENT", "status", "--check")
}
