package extract

import (
	"strings"
	"testing"
)

// A relation may say the condition it holds under, on one short line.
func TestARelationSaysItsCondition(t *testing.T) {
	doc := func(when string) string {
		return `{"units":[{"ref":"a","kind":"MOD","name":"a","anchors":[{"path":"a"},{"path":"a/a.go"}]},{"ref":"b","kind":"MOD","name":"b","anchors":[{"path":"b"},{"path":"b/b.go"}]}],` +
			`"relations":[{"from":"a","to":"b","kind":"depends_on","via":"import","when":"` + when + `"}]}`
	}
	r, err := Parse(strings.NewReader(doc("PRIVATE")))
	if err != nil || r.Relations[0].When != "PRIVATE" {
		t.Fatalf("a PRIVATE link: %v, %+v", err, r.Relations)
	}
	if _, err := Parse(strings.NewReader(doc(strings.Repeat("x", 61)))); err == nil {
		t.Error("a condition over 60 characters was accepted")
	}
}
