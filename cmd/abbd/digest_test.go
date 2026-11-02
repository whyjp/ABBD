package main

import (
	"strings"
	"testing"
)

// deepTopo puts app's code two levels down: app holds web, a directory, and
// web holds games. Only a supplement unit cites the root CLAUDE.md.
const deepTopo = `{"units":[
 {"key":"app","kind":"MOD","name":"app","anchors":[{"path":"app/main.go","symbol":"Main"}]},
 {"key":"web","kind":"MOD","name":"web","parent":"app","anchors":[{"path":"app/web"}]},
 {"key":"games","kind":"MOD","name":"games","parent":"web","anchors":[{"path":"app/web/games/board.tsx","symbol":"Board"}]},
 {"key":"other","kind":"MOD","name":"other","anchors":[{"path":"other/x.go","symbol":"X"}]}
]}`

const deepRules = `{"units":[
 {"key":"r","kind":"CFG","name":"repo rules","parent":"app",
  "evidence":[{"path":"CLAUDE.md","lines":"12","quote":"never commit secrets"}]},
 {"key":"app","kind":"MOD","name":"app","anchors":[{"path":"app/main.go","symbol":"Main"}]}
]}`

func claudeLines(near string) string {
	var b strings.Builder
	for i := 1; i <= 30; i++ {
		switch i {
		case 12:
			b.WriteString("never commit secrets\n")
		case 13:
			b.WriteString(near + "\n")
		default:
			b.WriteString("line\n")
		}
	}
	return b.String()
}

func deepProject(t *testing.T) *project {
	t.Helper()
	p, exe := bare(t)
	p.code("app/main.go", "package app\n\nfunc Main() {}\n")
	p.code("app/web/page.tsx", "export const Page = 1\n")
	p.code("app/web/games/board.tsx", "export function Board() {}\n")
	p.code("other/x.go", "package other\n\nfunc X() {}\n")
	p.code("CLAUDE.md", claudeLines("keep it small"))
	p.extract(deepTopo)
	p.setup(0, "synced", "--supplement", p.file(t, "rules.json", deepRules), "--", exe)
	return p
}

func (p *project) stale(n string) {
	p.t.Helper()
	p.want(0, "understanding: 1 module text(s), "+n+" STALE", "status")
}

// A module's text describes all of its code: what its packages two levels
// down hold, and every file under a directory it is anchored at.
func TestATextGoesStaleWhenAnyCodeUnderItsModuleChanges(t *testing.T) {
	p := deepProject(t)
	n := 0
	text := func() string {
		n++
		return p.file(t, "how.md", strings.Repeat("app serves pages.\n", n))
	}
	p.want(0, "understood MOD-0001 app", "understand", "MOD-0001", text())
	p.stale("0")

	p.code("app/web/games/board.tsx", "export function Board() { return 1 }\n")
	p.resync()
	p.stale("1")
	p.want(0, "understood", "understand", "MOD-0001", text())

	p.code("app/web/menu.tsx", "export const Menu = 1\n")
	p.resync()
	p.stale("1")
	p.want(0, "understood", "understand", "MOD-0001", text())

	p.code("other/x.go", "package other\n\nfunc X() { panic(1) }\n")
	p.resync()
	p.stale("0")
}

// A file the module only quotes counts by the lines around the quote, so an
// edit elsewhere in a shared file such as CLAUDE.md leaves the text current.
func TestAQuotedFileCountsOnlyAroundTheQuote(t *testing.T) {
	p := deepProject(t)
	p.want(0, "understood", "understand", "MOD-0001", p.file(t, "how.md", "app follows the repo rules.\n"))
	p.code("CLAUDE.md", "# added at the top\n"+claudeLines("keep it small")+"line added far below\n")
	p.resync()
	p.stale("0")
	p.code("CLAUDE.md", "# added at the top\n"+claudeLines("keep it tiny")+"line added far below\n")
	p.resync()
	p.stale("1")
}

// When only the extraction moved what a module holds, an agent restamps the
// text explicitly; abbd allows it only while none of the module's code
// changed since the text was written.
func TestRestampFollowsAnExtractionChangeButNotACodeChange(t *testing.T) {
	p := deepProject(t)
	p.want(0, "understood", "understand", "MOD-0001", p.file(t, "how.md", "app serves pages.\n"))
	p.want(1, "is current", "understand", "MOD-0001", "--restamp")

	p.extract(strings.Replace(deepTopo, `"parent":"app",`, ``, 1))
	p.resync()
	p.stale("1")
	p.want(0, "restamped MOD-0001", "understand", "MOD-0001", "--restamp")
	p.stale("0")

	p.code("app/main.go", "package app\n\nfunc Main() { run() }\n")
	p.extract(deepTopo)
	p.resync()
	p.want(1, "app/main.go", "understand", "MOD-0001", "--restamp")
	p.stale("1")
	p.want(1, "or --restamp without one", "understand", "MOD-0001", p.file(t, "how2.md", "x\n"), "--restamp")
}

// A text stored by an ABBD whose digest covered only a module's direct
// children keeps its state across the upgrade: sync restamps it when it was
// current then, and leaves it STALE when it was not.
func TestSyncUpgradesATextWrittenAgainstTheOldDigest(t *testing.T) {
	for _, c := range []struct{ digest, stale string }{{"73de2c949d70057d", "0"}, {"0123456789abcdef", "1"}} {
		p := deepProject(t)
		write(t, p.root+"/.abbd/understanding/MOD-0001.md", "---\nunit: MOD-0001\norigin: INFERRED\ndigest: \""+c.digest+"\"\n---\n\napp serves pages.\n")
		out := p.resync()
		if strings.Contains(out, "now covers") != (c.stale == "0") {
			t.Fatalf("sync should restamp only a text current under the old digest:\n%s", out)
		}
		p.stale(c.stale)
	}
}
