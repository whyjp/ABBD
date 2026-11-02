package main

import (
	"strings"
	"testing"
)

// A module the extraction script does not know is a manifest no unit
// anchors code under. Sync names it, so a hand-kept list cannot hide it.
func TestSyncNamesAManifestNoUnitCovers(t *testing.T) {
	p, exe := bare(t)
	p.code("billing/charge.go", "package billing\n\nfunc Charge() {}\n")
	p.code("billing/go.mod", "module billing\n")
	p.code("ledger/book.go", "package ledger\n\nfunc Post() {}\n")
	p.code("api/go.mod", "module api\n")
	p.code("api/main.go", "package main\n")
	p.code("web/package.json", "{}\n")
	p.code("ledger/testdata/fixture/go.mod", "module fixture\n")
	p.code("web/node_modules/dep/package.json", "{}\n")
	p.extract(ledgerTopo)
	out := p.setup(0, "synced", "--", exe)
	out += p.want(0, "", "sync")
	contains(t, out, "uncovered: api/go.mod: no unit anchors code under api", "uncovered: web/package.json: no unit anchors code under web")
	for _, quiet := range []string{"billing/go.mod", "testdata", "node_modules"} {
		if strings.Contains(out, "uncovered: "+quiet) || strings.Contains(out, quiet+"/fixture") {
			t.Errorf("sync must not name %s, which a unit covers or which is test data or a dependency:\n%s", quiet, out)
		}
	}
	if code, _ := p.cmd("", "status", "--check"); code != 0 {
		t.Errorf("an uncovered manifest is a warning and must not fail status --check")
	}
}

// A manifest nested inside a module a unit anchors by its directory, such
// as an Android build under a mobile module, belongs to that module.
func TestSyncKeepsQuietOnAManifestUnderADirectoryAnchor(t *testing.T) {
	p, exe := bare(t)
	p.code("mobile/codec/index.ts", "export const codec = 1\n")
	p.code("mobile/codec/package.json", "{}\n")
	p.code("mobile/codec/android/build.gradle", "apply plugin: 'java'\n")
	p.code("mobile/other/android/build.gradle", "apply plugin: 'java'\n")
	p.extract(`{"units":[{"ref":"codec","kind":"MOD","name":"codec","anchors":[{"path":"mobile/codec"}]}]}`)
	out := p.setup(0, "synced", "--", exe)
	out += p.want(0, "", "sync")
	contains(t, out, "uncovered: mobile/other/android/build.gradle")
	for _, quiet := range []string{"mobile/codec/package.json", "mobile/codec/android/build.gradle"} {
		if strings.Contains(out, "uncovered: "+quiet) {
			t.Errorf("sync must not name %s, which sits under the unit anchored on mobile/codec:\n%s", quiet, out)
		}
	}
}
