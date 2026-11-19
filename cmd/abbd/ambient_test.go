package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// An ambient mark is the overview's own record: stored stamped with the
// rest, counted by status, given in graph.json, refused when it names a
// unit inside a box or a way of being ambient there is none of, and STALE
// once its code changes.
func TestAnAmbientMarkIsStoredCheckedAndGiven(t *testing.T) {
	t.Parallel()
	p := notesProject(t)
	mark := "\nambient:\n  - unit: MOD-0001\n    as: static\n    evidence:\n      - {path: billing/charge.go, lines: \"4\", symbol: Charge, quote: \"func Charge() {}\"}\n---\n\n장부는"
	marked := strings.Replace(chargeOverview, "\n---\n\n장부는", mark, 1)
	for _, c := range []struct{ from, to, want string }{
		{"as: static", "as: shared", `ambient MOD-0001 is "shared"; it is one of global, static, context`},
		{"unit: MOD-0001\n    as", "unit: DATA-0001\n    as", "DATA-0001 sits inside MOD-0001, which the overview draws as one box; mark MOD-0001"},
	} {
		p.overview(1, c.want, strings.Replace(marked, c.from, c.to, 1))
	}
	p.overview(0, "stored the overview: 1 zones, 1 lines, 1 ambient marks", marked)
	p.want(0, "overview: 1 zones, 1 lines, 1 ambient marks, 0 STALE", "status", "--check")
	if out := p.want(0, "ambient:", "overview"); !strings.Contains(out, "as: static") {
		t.Errorf("the draft gives the stored ambient mark back:\n%s", out)
	}
	var g struct {
		Ambient []struct{ Unit, As string }
	}
	if err := json.Unmarshal([]byte(p.read("blueprint/graph.json")), &g); err != nil {
		t.Fatal(err)
	}
	if len(g.Ambient) != 1 || g.Ambient[0].Unit != "MOD-0001" || g.Ambient[0].As != "static" {
		t.Errorf("graph.json gives the ambient marks: %+v", g.Ambient)
	}
	p.code("billing/charge.go", "package billing\n\n// moved\n\nfunc Charge() {}\n")
	p.resync()
	p.want(1, "overview ambient 1 (MOD-0001 static) is STALE", "status", "--check")
}
