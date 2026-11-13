package main

import (
	"strings"
	"testing"
)

const builds = `{"units":[
 {"ref":"libzstd","kind":"MOD","name":"libzstd","artifacts":["static","shared"],"anchors":[{"path":"billing/charge.go","symbol":"Charge"}]},
 {"ref":"zstd","kind":"MOD","name":"zstd","artifacts":["exe"],"anchors":[{"path":"api/http.go","symbol":"Serve"}]}
],"relations":[
 {"from":"zstd","to":"libzstd","kind":"depends_on","via":"link"}
]}`

func TestAModuleShowsWhatItBuildsAndTheLinkToIt(t *testing.T) {
	t.Parallel()
	p, exe := bare(t)
	p.extract(builds)
	p.setup(0, "synced: 2 unit(s)", "--", exe)
	out := p.want(0, "artifacts: shared, static", "explain", "libzstd")
	contains(t, out, "MOD-0001 zstd depends_on (extracted, via link)")

	git(t, p.root, "add", "-A")
	git(t, p.root, "commit", "-qm", "base")
	base := rev(t, p.root)
	p.extract(strings.Replace(builds, `"artifacts":["static","shared"]`, `"artifacts":["static"]`, 1))
	p.want(0, "", "sync")
	p.want(0, `~ artifact MOD-0002 libzstd: "shared,static" -> "static"`, "diff", "--base", base)
}

func TestExtractNamesTheBundledFrontEnds(t *testing.T) {
	t.Parallel()
	p, _ := bare(t)
	p.want(1, `unknown front-end "gradle": use cargo or cmake`, "extract", "gradle")
	p.want(1, "has no Cargo.toml", "extract", "cargo")
}

func TestTheScriptIsToldTheAbbdThatRunsIt(t *testing.T) {
	t.Parallel()
	p, exe := bare(t)
	p.extract(`{"units":[{"ref":"a","kind":"MOD","name":"{{ABBD_BIN}}","anchors":[{"path":"billing/charge.go","symbol":"Charge"}]}]}`)
	p.setup(0, "synced: 1 unit(s)", "--", exe)
	p.want(0, "MOD-0001 running abbd", "explain", "MOD-0001")
}
