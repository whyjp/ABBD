package extract

import (
	"strings"
	"testing"
)

func whenDoc(relations string) string {
	return `{"units":[{"ref":"a","kind":"MOD","name":"a","anchors":[{"path":"a"},{"path":"a/a.go"}]},{"ref":"b","kind":"MOD","name":"b","anchors":[{"path":"b"},{"path":"b/b.go"}]}],` +
		`"relations":[` + relations + `]}`
}

func whenRel(when string) string {
	return `{"from":"a","to":"b","kind":"depends_on","via":"import","when":"` + when + `"}`
}

// A relation may say the condition it holds under, on one short line.
func TestARelationSaysItsCondition(t *testing.T) {
	r, err := Parse(strings.NewReader(whenDoc(whenRel("PRIVATE"))))
	if err != nil || r.Relations[0].When != "PRIVATE" {
		t.Fatalf("a PRIVATE link: %v, %+v", err, r.Relations)
	}
	if _, err := Parse(strings.NewReader(whenDoc(whenRel(strings.Repeat("x", 61))))); err == nil {
		t.Error("a condition over 60 characters was accepted")
	}
	if _, err := Parse(strings.NewReader(whenDoc(whenRel(`cfg(unix)\ncfg(windows)`)))); err == nil || !strings.Contains(err.Error(), "one line") {
		t.Errorf("a condition over two lines was accepted: %v", err)
	}
}

// The store keeps one claim per edge and writer, so an edge listed twice
// with different conditions would lose one. Parse refuses it and says how
// to give the edge once.
func TestAnEdgeListedTwiceWithDifferentConditionsIsRefused(t *testing.T) {
	_, err := Parse(strings.NewReader(whenDoc(whenRel("cfg(unix)") + "," + whenRel("cfg(windows)"))))
	if err == nil || !strings.Contains(err.Error(), `joined as "a | b"`) {
		t.Fatalf("an edge listed under two conditions was accepted: %v", err)
	}
	if _, err := Parse(strings.NewReader(whenDoc(whenRel("dev") + "," + whenRel("")))); err == nil {
		t.Error("an edge listed with and without a condition was accepted")
	}
	_, err = Parse(strings.NewReader(whenDoc(whenRel("PRIVATE") + "," + whenRel("PUBLIC"))))
	if err == nil || !strings.Contains(err.Error(), "leave the scope out") || strings.Contains(err.Error(), `"a | b"`) {
		t.Errorf("two link scopes are refused without saying a scope is not a condition: %v", err)
	}
	r, err := Parse(strings.NewReader(whenDoc(whenRel("cfg(unix) | cfg(windows)"))))
	if err != nil || r.Relations[0].When != "cfg(unix) | cfg(windows)" {
		t.Errorf("the joined conditions: %v, %+v", err, r.Relations)
	}
}
