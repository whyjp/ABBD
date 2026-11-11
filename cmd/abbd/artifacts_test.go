package main

import (
	"strings"
	"testing"
)

const builds = `{"units":[
 {"ref":"libzstd","kind":"MOD","name":"libzstd","artifacts":["shared","static"],"anchors":[{"path":"billing/charge.go","symbol":"Charge"}]},
 {"ref":"zstd","kind":"MOD","name":"zstd","artifacts":["exe"],"anchors":[{"path":"api/http.go","symbol":"Serve"}]}
],"relations":[
 {"from":"zstd","to":"libzstd","kind":"depends_on","via":"link"}
]}`

func TestAModuleShowsWhatItBuildsAndTheLinkToIt(t *testing.T) {
	t.Parallel()
	p, exe := bare(t)
	p.extract(builds)
	p.setup(0, "synced: 2 unit(s)", "--", exe)
	out := p.want(0, "builds: static, shared (EXTRACTED)", "explain", "libzstd")
	contains(t, out, "MOD-0001 zstd depends_on (extracted, via link)")

	git(t, p.root, "add", "-A")
	git(t, p.root, "commit", "-qm", "base")
	base := rev(t, p.root)
	p.extract(strings.Replace(builds, `"artifacts":["shared","static"]`, `"artifacts":["static"]`, 1))
	p.want(0, "", "sync")
	p.want(0, `~ artifacts MOD-0002 libzstd: "static, shared" -> "static"`, "diff", "--base", base)
}

func TestOnlyTheScriptSaysWhatAModuleBuilds(t *testing.T) {
	t.Parallel()
	p, exe := bare(t)
	p.extract(strings.Replace(builds, `"kind":"MOD","name":"zstd"`, `"kind":"IF","name":"zstd"`, 1))
	p.setup(1, "only modules list artifacts", "--", exe)
	p.extract(strings.Replace(builds, `"exe"`, `"dll"`, 1))
	p.setup(1, `unknown artifact "dll"`, "--", exe)
	p.extract(builds)
	p.setup(1, "lists artifacts", "--supplement", p.file(t, "sup.json", `{"units":[
 {"ref":"libzstd","kind":"MOD","name":"libzstd","artifacts":["module"],"anchors":[{"path":"billing/charge.go","symbol":"Charge"}]}
]}`), "--", exe)
}

func TestExtractNamesTheBundledFrontEnds(t *testing.T) {
	t.Parallel()
	p, _ := bare(t)
	p.want(1, `unknown front-end "gradle": use cargo or cmake`, "extract", "gradle")
	p.want(1, "has no Cargo.toml", "extract", "cargo")
}
