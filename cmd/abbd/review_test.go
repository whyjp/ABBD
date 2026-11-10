package main

import (
	"strings"
	"testing"
)

// review names both users of a new route and the feature that holds one,
// and the whole-contract user of a route the change removed,
// lists what the change reaches, puts a coupling only a reader found under
// what is worth a look, and exits 0 whatever it finds.
func TestReviewNamesTheUsersOfANewEntry(t *testing.T) {
	p, base := entriesProject(t)
	p.code("web/refund.ts", "export function Refund() {}\n")
	p.extract(strings.NewReplacer(
		`,{"name":"POST /refunds","shape":"Refund -> 201"}]}`, `,{"name":"PUT /charges/{id}"}]},
 {"key":"cli","kind":"MOD","name":"cli","anchors":[{"path":"web/refund.ts","symbol":"Cli"}]},
 {"key":"button","kind":"FEAT","name":"refund button","parent":"page","anchors":[{"path":"web/refund.ts","symbol":"Refund"}]}`,
		`"entries":["GET /charges"]}`, `"entries":["GET /charges","PUT /charges/{id}"]},
 {"from":"button","to":"api","kind":"consumes","entries":["PUT /charges/{id}"]},
 {"from":"cli","to":"api","kind":"consumes"}`,
	).Replace(entriesBase))
	p.want(0, "", "sync")
	p.want(0, "INFERRED", "record", "relation", "MOD-0001", "depends_on", "FEAT-0001", "--via", "call", "--evidence", `api/http.go:4:handle("GET /charges/{id}", charge)`)
	out := p.want(0, "review since "+base[:12], "review", "--base", base)
	contains(t, out,
		"+ entry IF-0001 http api PUT /charges/{id}",
		"IF-0001 http api PUT /charges/{id}: FEAT-0001 charges page consumes (EXTRACTED); FEAT-0002 refund button consumes (EXTRACTED, in FEAT-0001 charges page); MOD-0002 cli consumes the whole contract (EXTRACTED)",
		"IF-0001 http api POST /refunds: MOD-0002 cli consumes the whole contract (EXTRACTED)",
		"what the change reaches directly:",
		"worth a look:\n  a reader's coupling no human stated: MOD-0001 depends_on FEAT-0001",
	)
}
