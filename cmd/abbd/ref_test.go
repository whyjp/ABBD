package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// A unit's supplement ref names it as gitleaks' generic-api-key rule never
// reads a secret, since the rule keys on words such as "key". Stores and
// supplements that spell it key keep loading and pairing.
func TestASupplementRefPairsWithAUnitStoredUnderTheOldKey(t *testing.T) {
	t.Parallel()
	p := semanticProject(t)
	data := filepath.Join(p.root, ".abbd", "architecture", "DATA-0001.md")
	md := p.read("architecture/DATA-0001.md")
	if !strings.Contains(md, "supplement_ref: t\n") || strings.Contains(md, "supplement_key") {
		t.Fatalf("a semantic unit stores the supplement's ref as supplement_ref:\n%s", md)
	}
	write(t, data, strings.Replace(md, "supplement_ref: t\n", "supplement_key: t\n", 1))

	p.code("billing/schema.sql", "-- INSERT INTO charges (amount)\n")
	moved := strings.Replace(chargesTable, `{"key":"t","kind":"DATA","name":"charges table","parent":"b",
  "evidence":[{"path":"billing/charge.go","lines":"3","quote":"INSERT INTO charges"}]}`, `{"ref":"t","kind":"DATA","name":"charges table","parent":"b",
  "evidence":[{"path":"billing/schema.sql","lines":"1","quote":"INSERT INTO charges"}]}`, 1)
	if moved == chargesTable {
		t.Fatal("the fixture no longer holds the unit this test moves")
	}
	p.setup(0, "synced: 3 unit(s)", "--supplement", p.file(t, "ref.json", moved))
	md = p.read("architecture/DATA-0001.md")
	if !strings.Contains(md, "path: billing/schema.sql") || !strings.Contains(md, "supplement_ref: t\n") || strings.Contains(md, "supplement_key") {
		t.Fatalf("the ref pairs the moved unit with the one stored under supplement_key, and the store is rewritten as supplement_ref:\n%s", md)
	}
	if _, ok := units(t, p.root)["architecture/DATA-0002.md"]; ok {
		t.Fatalf("pairing on the ref must not mint a second unit: %v", units(t, p.root))
	}
}

func TestASupplementUnitMayNotSpellTwoRefs(t *testing.T) {
	t.Parallel()
	p, exe := bare(t)
	p.code("billing/charge.go", "package billing\n\nfunc Charge() {}\n")
	p.extract(ledgerTopo)
	two := `{"units":[{"ref":"b","key":"c","kind":"MOD","name":"billing","anchors":[{"path":"billing/charge.go","symbol":"Charge"}]}]}`
	p.setup(1, `"ref" "b" and "key" "c"`, "--supplement", p.file(t, "two.json", two), "--", exe)
}

func TestAStoredUnitMayNotSpellTwoSupplementRefs(t *testing.T) {
	t.Parallel()
	p := semanticProject(t)
	data := filepath.Join(p.root, ".abbd", "architecture", "DATA-0001.md")
	md := p.read("architecture/DATA-0001.md")
	write(t, data, strings.Replace(md, "supplement_ref: t\n", "supplement_ref: t\nsupplement_key: u\n", 1))
	p.want(1, `supplement_ref "t" and supplement_key "u"`, "status")
}
