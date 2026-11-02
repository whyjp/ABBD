package main

import (
	"os"
	"path/filepath"
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
// children stays current across the upgrade only when that digest matches
// and read every file the new one reads. other is one file, so its text
// stays current. app's old digest never read the files under app/web, so
// its text goes STALE. A text that was STALE stays STALE.
func TestSyncUpgradesATextWrittenAgainstTheOldDigest(t *testing.T) {
	for _, c := range []struct{ id, digest, says, stale string }{
		{"MOD-0004", "596dd225dceb4742", "it stays current", "0"},
		{"MOD-0001", "73de2c949d70057d", "it is STALE until pstack how reads", "1"},
		{"MOD-0001", "0123456789abcdef", "", "1"},
	} {
		p := deepProject(t)
		write(t, p.root+"/.abbd/understanding/"+c.id+".md", "---\nunit: "+c.id+"\norigin: INFERRED\ndigest: \""+c.digest+"\"\n---\n\nthe module.\n")
		out := p.resync()
		if c.says == "" && strings.Contains(out, "now covers") || !strings.Contains(out, c.says) {
			t.Fatalf("%s at %s: sync should say %q:\n%s", c.id, c.digest, c.says, out)
		}
		p.stale(c.stale)
	}
}

// A restamp sees a file the module lost because it was deleted, under a
// directory anchor or as the only anchor of a unit that went absent.
func TestRestampIsRefusedWhenTheModuleLostAFile(t *testing.T) {
	p := deepProject(t)
	p.want(0, "understood", "understand", "MOD-0001", p.file(t, "how.md", "app serves pages.\n"))
	if err := os.Remove(filepath.Join(p.root, "app", "web", "page.tsx")); err != nil {
		t.Fatal(err)
	}
	p.resync()
	p.stale("1")
	p.want(1, "app/web (code under it)", "understand", "MOD-0001", "--restamp")
}

// sync upgrades only texts the old binary stored. A text stored now whose
// module later gains a deeper unit stays STALE even when the old digest,
// which never saw that unit, still matches.
func TestSyncDoesNotUpgradeATextStoredNow(t *testing.T) {
	p, exe := bare(t)
	p.code("app/main.go", "package app\n\nfunc Main() {}\n")
	p.code("app/web/page.go", "package web\n\nfunc Page() {}\n")
	p.code("app/web/deep/g.go", "package deep\n\nfunc G() {}\n")
	flat := `{"units":[
 {"key":"app","kind":"MOD","name":"app","anchors":[{"path":"app/main.go","symbol":"Main"}]},
 {"key":"web","kind":"MOD","name":"web","parent":"app","anchors":[{"path":"app/web/page.go","symbol":"Page"}]}`
	p.extract(flat + "]}")
	p.setup(0, "synced", "--", exe)
	p.want(0, "understood", "understand", "MOD-0001", p.file(t, "how.md", "app serves pages.\n"))
	p.extract(flat + `,
 {"key":"g","kind":"MOD","name":"deep","parent":"web","anchors":[{"path":"app/web/deep/g.go","symbol":"G"}]}]}`)
	if out := p.resync(); strings.Contains(out, "now covers") {
		t.Fatalf("a text stored by this binary must not be upgraded:\n%s", out)
	}
	p.stale("1")
}

// vendorProject holds a submodule at vendor/lib and an ignored generated
// directory gen/. Code a git tree never shows still belongs to a module.
func vendorProject(t *testing.T, topo string) *project {
	t.Helper()
	lib := newRepo(t)
	write(t, filepath.Join(lib, "lib.go"), "package lib\n\nfunc Lib() {}\n")
	write(t, filepath.Join(lib, "pkg", "p.go"), "package pkg\n\nfunc P() {}\n")
	git(t, lib, "add", ".")
	git(t, lib, "commit", "-qm", "lib")
	p, exe := bare(t)
	git(t, p.root, "-c", "protocol.file.allow=always", "submodule", "add", "-q", lib, "vendor/lib")
	p.code(".gitignore", "gen/\n")
	p.code("app/main.go", "package app\n\nfunc Main() {}\n")
	p.code("gen/api.go", "package gen\n\nfunc API() {}\n")
	git(t, p.root, "add", "-A")
	git(t, p.root, "commit", "-qm", "vendor")
	p.extract(topo)
	p.setup(0, "synced", "--", exe)
	return p
}

const vendorTopo = `{"units":[
 {"key":"app","kind":"MOD","name":"app","anchors":[{"path":"app/main.go","symbol":"Main"}]},
 {"key":"lib","kind":"MOD","name":"lib","parent":"app","anchors":[{"path":"vendor/lib/lib.go","symbol":"Lib"}]},
 {"key":"gen","kind":"MOD","name":"gen","parent":"app","anchors":[{"path":"gen/api.go","symbol":"API"}]}`

// A restamp compares the files the digest reads, so an edit inside a
// submodule or under an ignored directory blocks it like any other.
func TestRestampSeesCodeInASubmoduleAndInAnIgnoredFile(t *testing.T) {
	for _, edit := range []string{"vendor/lib/lib.go", "gen/api.go"} {
		p := vendorProject(t, vendorTopo+"]}")
		p.want(0, "understood", "understand", "MOD-0001", p.file(t, "how.md", "app wraps lib.\n"))
		p.code(edit, "package x\n\nfunc Changed() {}\n")
		p.extract(vendorTopo + `,
 {"key":"x","kind":"MOD","name":"x","anchors":[{"path":"app/main.go","symbol":"Main"}]}]}`)
		p.resync()
		p.stale("1")
		p.want(1, edit, "understand", "MOD-0001", "--restamp")
		p.stale("1")
	}
}

// A directory anchored at a submodule stands for the commit the
// superproject records for it, so a new pin stales the text.
func TestADirectoryAnchorOnASubmoduleCountsItsPin(t *testing.T) {
	p := vendorProject(t, `{"units":[
 {"key":"lib","kind":"MOD","name":"lib","anchors":[{"path":"vendor/lib"}]}]}`)
	out := p.want(0, "understood", "understand", "MOD-0001", p.file(t, "how.md", "lib is vendored.\n"))
	if strings.Contains(out, "against code \n") {
		t.Fatalf("a module with code must not get the empty digest:\n%s", out)
	}
	sub := filepath.Join(p.root, "vendor", "lib")
	write(t, filepath.Join(sub, "lib.go"), "package lib\n\nfunc Lib() { panic(1) }\n")
	git(t, sub, "commit", "-qam", "bump")
	git(t, p.root, "add", "vendor/lib")
	p.resync()
	p.stale("1")
}

// An old text was checked only against its module's direct children. When
// the wider digest sees code the old one never covered, sync cannot show
// the text still describes it, so the text goes STALE and sync says why.
func TestSyncLeavesAnOldTextStaleWhenTheWiderScopeSeesNewCode(t *testing.T) {
	p := deepProject(t)
	write(t, p.root+"/.abbd/understanding/MOD-0001.md", "---\nunit: MOD-0001\norigin: INFERRED\ndigest: \"73de2c949d70057d\"\n---\n\napp serves pages.\n")
	p.code("app/web/games/board.tsx", "export function Board() { return 1 }\n")
	out := p.resync()
	if !strings.Contains(out, "MOD-0001") || !strings.Contains(out, "STALE") {
		t.Fatalf("sync must say the old text went STALE because the scope widened:\n%s", out)
	}
	p.stale("1")
}

// Outside git a directory anchor still stands for the files under it, so a
// module with code never gets the empty digest.
func TestADirectoryAnchorCountsItsFilesOutsideGit(t *testing.T) {
	p := deepProject(t)
	os.RemoveAll(filepath.Join(p.root, ".git"))
	p.want(0, "understood", "understand", "MOD-0001", p.file(t, "how.md", "app serves pages.\n"))
	p.code("app/web/page.tsx", "export const Page = 2\n")
	p.stale("1")
}

// A directory anchor git lists nothing under still stands for code: inside
// a submodule it is that submodule's pin, and an ignored directory is its
// files on disk. Neither may leave a text current after its code moved.
func TestADirectoryAnchorGitListsNothingUnderStillStalesItsText(t *testing.T) {
	t.Run("inside a submodule", func(t *testing.T) {
		p := vendorProject(t, `{"units":[{"key":"pkg","kind":"MOD","name":"pkg","anchors":[{"path":"vendor/lib/pkg"}]}]}`)
		p.want(0, "understood", "understand", "MOD-0001", p.file(t, "how.md", "pkg is vendored.\n"))
		sub := filepath.Join(p.root, "vendor", "lib")
		write(t, filepath.Join(sub, "pkg", "p.go"), "package pkg\n\nfunc P() { panic(1) }\n")
		git(t, sub, "commit", "-qam", "bump")
		git(t, p.root, "add", "vendor/lib")
		p.resync()
		p.stale("1")
	})
	t.Run("ignored", func(t *testing.T) {
		p := vendorProject(t, `{"units":[{"key":"gen","kind":"MOD","name":"gen","anchors":[{"path":"gen"}]}]}`)
		p.want(0, "understood", "understand", "MOD-0001", p.file(t, "how.md", "gen is generated.\n"))
		p.code("gen/api.go", "package gen\n\nfunc API() { panic(1) }\n")
		p.stale("1")
	})
}

// A restamp compares what the digest reads, so an edit to a quoted file far
// from the quote, which leaves the digest alone, does not block it.
func TestRestampIgnoresAnEditTheDigestIgnores(t *testing.T) {
	p := deepProject(t)
	p.want(0, "understood", "understand", "MOD-0001", p.file(t, "how.md", "app follows the repo rules.\n"))
	p.code("CLAUDE.md", claudeLines("keep it small")+"line added far below\n")
	p.extract(strings.Replace(deepTopo, `"parent":"app",`, ``, 1))
	p.resync()
	p.stale("1")
	p.want(0, "restamped MOD-0001", "understand", "MOD-0001", "--restamp")
	p.stale("0")
}

// A text stored with the format 1 manifest, one hash per file, stays
// current across the upgrade when its digest still matches the code, and
// is stored again in the current format.
func TestSyncUpgradesAFormat1Text(t *testing.T) {
	p := deepProject(t)
	write(t, p.root+"/.abbd/understanding/MOD-0004.md", "---\nunit: MOD-0004\norigin: INFERRED\ndigest: 596dd225dceb4742\ncode:\n    other/x.go: 1ec0203f385af158\n---\n\nthe module.\n")
	if out := p.resync(); !strings.Contains(out, "it stays current") {
		t.Fatalf("sync should carry the format 1 text over:\n%s", out)
	}
	p.stale("0")
	if text := p.read("understanding/MOD-0004.md"); !strings.Contains(text, "format: 3") {
		t.Fatalf("the upgraded text should record the current format:\n%s", text)
	}
}

// A gitfile marks a worktree or a submodule checkout; the files beside it
// are still code under an ignored directory anchor.
func TestAGitfileUnderAnIgnoredAnchorHidesNoCode(t *testing.T) {
	p := vendorProject(t, `{"units":[{"key":"gen","kind":"MOD","name":"gen","anchors":[{"path":"gen"}]}]}`)
	p.code("gen/dep/.git", "gitdir: ../../.git/modules/dep\n")
	p.code("gen/dep/src/a.go", "package src\n")
	p.want(0, "understood", "understand", "MOD-0001", p.file(t, "how.md", "gen is generated.\n"))
	p.code("gen/dep/src/a.go", "package src\n\nfunc A() {}\n")
	p.stale("1")
}

// git lists an untracked repository inside the project as one directory;
// its files on disk are still the code of a module anchored there.
func TestAnUntrackedNestedRepositoryCountsItsFiles(t *testing.T) {
	p, exe := bare(t)
	p.code("app/main.go", "package app\n")
	p.code("nested/a.go", "package nested\n")
	nested := filepath.Join(p.root, "nested")
	git(t, nested, "init", "-q")
	git(t, nested, "add", ".")
	git(t, nested, "commit", "-qm", "nested")
	p.extract(`{"units":[{"key":"n","kind":"MOD","name":"nested","anchors":[{"path":"nested"}]}]}`)
	p.setup(0, "synced", "--", exe)
	p.want(0, "understood", "understand", "MOD-0001", p.file(t, "how.md", "nested is vendored.\n"))
	p.code("nested/a.go", "package nested\n\nfunc A() {}\n")
	p.stale("1")
}

func twiceQuoted(edit int) string {
	var b strings.Builder
	for i := 1; i <= 30; i++ {
		switch i {
		case 7, 25:
			b.WriteString("\treturn nil\n")
		case edit:
			b.WriteString("edited\n")
		default:
			b.WriteString("line\n")
		}
	}
	return b.String()
}

// twiceProject's app holds two units that quote "return nil" in
// lib/util.go, at lines 7 and 25.
func twiceProject(t *testing.T) *project {
	t.Helper()
	p, exe := bare(t)
	p.code("app/main.go", "package app\n")
	p.code("lib/util.go", twiceQuoted(0))
	p.extract(`{"units":[{"key":"app","kind":"MOD","name":"app","anchors":[{"path":"app/main.go"}]}]}`)
	rules := `{"units":[
 {"key":"app","kind":"MOD","name":"app","anchors":[{"path":"app/main.go"}]},
 {"key":"a","kind":"CFG","name":"early exit","parent":"app","evidence":[{"path":"lib/util.go","lines":"7","quote":"return nil"}]},
 {"key":"b","kind":"CFG","name":"late exit","parent":"app","evidence":[{"path":"lib/util.go","lines":"25","quote":"return nil"}]}
]}`
	p.setup(0, "synced", "--supplement", p.file(t, "rules.json", rules), "--", exe)
	return p
}

// Two units quoting the same words at two places in one file each count
// by the lines around their own quote.
func TestTheSameQuoteCitedTwiceCountsAtBothPlaces(t *testing.T) {
	for _, edit := range []int{8, 24} {
		p := twiceProject(t)
		p.want(0, "understood", "understand", "MOD-0001", p.file(t, "how.md", "app exits early.\n"))
		p.code("lib/util.go", twiceQuoted(edit))
		p.resync()
		p.stale("1")
	}
}

// Format 2 read one place of a quote cited at several. A text stored then
// stays current only when its module cites no quote twice; otherwise sync
// cannot show the other place unchanged and says the text is STALE. The
// digests are what ABBD stored in format 2 for these projects.
func TestSyncUpgradesAFormat2TextOnlyWhenItReadEveryPlace(t *testing.T) {
	for _, c := range []struct {
		name, digest, says, stale string
		project                   func(*testing.T) *project
	}{
		{"one place", "b59a0a674660efef", "it stays current", "0", deepProject},
		{"two places", "2b10416f927695bf", "it is STALE until pstack how reads", "1", twiceProject},
	} {
		t.Run(c.name, func(t *testing.T) {
			p := c.project(t)
			write(t, p.root+"/.abbd/understanding/MOD-0001.md", "---\nunit: MOD-0001\norigin: INFERRED\ndigest: "+c.digest+"\nformat: 2\n---\n\nthe module.\n")
			if out := p.resync(); !strings.Contains(out, c.says) {
				t.Fatalf("sync should say %q:\n%s", c.says, out)
			}
			p.stale(c.stale)
		})
	}
}

// Format 1 read one place of a quote cited at several too.
func TestSyncLeavesAFormat1TextStaleWhenAQuoteIsCitedTwice(t *testing.T) {
	p := twiceProject(t)
	write(t, p.root+"/.abbd/understanding/MOD-0001.md", "---\nunit: MOD-0001\norigin: INFERRED\ndigest: 1912b5ee72f158bd\ncode:\n    app/main.go: 75d99e22087438b6\n---\n\nthe module.\n")
	if out := p.resync(); !strings.Contains(out, "it is STALE until pstack how reads") {
		t.Fatalf("sync should say the text is STALE:\n%s", out)
	}
	p.stale("1")
}
