package main

import (
	"strings"
	"testing"
)

// Two pages share a graph component and each calls its own route of the
// api; a third page touches neither.
const pagesExtraction = `{"units":[
 {"key":"lib","kind":"MOD","name":"lib","anchors":[{"path":"web/lib/graph.ts","symbol":"Graph"}]},
 {"key":"server","kind":"MOD","name":"server","anchors":[{"path":"server/main.go","symbol":"main"}]},
 {"key":"api","kind":"IF","name":"http api","anchors":[{"path":"server/http.go","symbol":"Routes"}],
  "entries":[{"name":"GET /charges"},{"name":"GET /refunds"}]},
 {"key":"charges","kind":"FEAT","name":"charges page","anchors":[{"path":"web/charges.tsx","symbol":"Charges"}]},
 {"key":"chart","kind":"FEAT","name":"charges chart","parent":"charges","anchors":[{"path":"web/charges/chart.tsx","symbol":"Chart"}]},
 {"key":"refunds","kind":"FEAT","name":"refunds page","anchors":[{"path":"web/refunds.tsx","symbol":"Refunds"}]},
 {"key":"about","kind":"FEAT","name":"about page","anchors":[{"path":"web/about.tsx","symbol":"About"}]}
],"relations":[
 {"from":"server","to":"api","kind":"provides"},
 {"from":"chart","to":"lib","kind":"depends_on"},
 {"from":"refunds","to":"lib","kind":"depends_on"},
 {"from":"charges","to":"api","kind":"consumes","entries":["GET /charges"]},
 {"from":"refunds","to":"api","kind":"consumes","entries":["GET /refunds"]}
]}`

// impact --features groups what a change reaches by the pages it reaches,
// a part of a page under the page, and names the routes each page calls of
// the contracts the change reaches.
func TestImpactGroupsByFeature(t *testing.T) {
	t.Parallel()
	p, exe := bare(t)
	p.extract(pagesExtraction)
	p.setup(0, "synced", "--", exe)
	out := p.want(0, "", "impact", "MOD-0002,IF-0001", "--features")
	want := `feature FEAT-0002 charges page
  direct FEAT-0002 charges page: consumes IF-0001 (extracted)
  direct FEAT-0003 charges chart: depends_on MOD-0002 (extracted)
  uses IF-0001 http api: GET /charges
feature FEAT-0004 refunds page
  direct FEAT-0004 refunds page: depends_on MOD-0002 (extracted)
  uses IF-0001 http api: GET /refunds
no feature
  direct MOD-0002 lib: seed
  direct IF-0001 http api: seed
  direct MOD-0001 server: provides IF-0001 (extracted)
`
	if out != want {
		t.Fatalf("impact --features:\n%s\nwant:\n%s", out, want)
	}
	if strings.Contains(p.want(0, "", "impact", "MOD-0002,IF-0001"), "feature ") {
		t.Errorf("without --features impact lists its tiers as before")
	}
}

// A page's own view of its contracts names the routes it calls.
func TestExplainNamesTheEntriesAFeatureUses(t *testing.T) {
	t.Parallel()
	p, exe := bare(t)
	p.extract(pagesExtraction)
	p.setup(0, "synced", "--", exe)
	p.want(0, "IF-0001 http api (extracted): GET /charges", "explain", "FEAT-0002")
}

// A part of a page that uses a contract whole makes the page use it whole,
// whatever entries the page names itself.
func TestImpactKeepsAFeaturesWholeUse(t *testing.T) {
	t.Parallel()
	p, exe := bare(t)
	p.extract(strings.Replace(pagesExtraction, `{"from":"chart","to":"lib","kind":"depends_on"},`, `{"from":"chart","to":"lib","kind":"depends_on"},{"from":"chart","to":"api","kind":"consumes"},`, 1))
	p.setup(0, "synced", "--", exe)
	out := p.want(0, "uses IF-0001 http api, no entry named\nfeature FEAT-0004", "impact", "IF-0001", "--features")
	if strings.Contains(out, "uses IF-0001 http api: GET /charges") {
		t.Errorf("the chart's whole use is hidden by the page's route:\n%s", out)
	}
}
