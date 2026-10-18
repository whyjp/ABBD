package identity

import (
	"testing"

	"github.com/whyjp/for-my-harness/internal/model"
)

var (
	sym   = model.Anchor{Kind: model.AnchorSymbol, Ref: "go:svc.Consumer.Run"}
	file  = model.Anchor{Kind: model.AnchorPath, Ref: "src/consumer.go"}
	entry = model.Anchor{Kind: model.AnchorEntry, Ref: "kafka-topic:hb"}
)

func TestMatchPolicy(t *testing.T) {
	existing := []Existing{{ID: "FEAT-001", Anchors: []model.Anchor{sym, file, entry}}}
	cases := []struct {
		name    string
		anchors []model.Anchor
		want    Verdict
	}{
		{"two anchors keep the ID", []model.Anchor{sym, entry, {Kind: model.AnchorPath, Ref: "moved.go"}}, Same},
		{"one anchor is a merge candidate", []model.Anchor{entry, {Kind: model.AnchorSymbol, Ref: "go:other"}}, Candidate},
		{"no anchor is new", []model.Anchor{{Kind: model.AnchorEntry, Ref: "x"}}, New},
	}
	for _, c := range cases {
		if got := Resolve(c.anchors, existing, nil, nil); got.Verdict != c.want {
			t.Errorf("%s: verdict %v, want %v", c.name, got.Verdict, c.want)
		}
	}
}

func TestRenameKeepsPathAnchor(t *testing.T) {
	existing := []Existing{{ID: "MOD-001", Anchors: []model.Anchor{{Kind: model.AnchorPath, Ref: "a/old"}, {Kind: model.AnchorName, Ref: "go:x"}}}}
	fresh := []model.Anchor{{Kind: model.AnchorPath, Ref: "b/new"}, {Kind: model.AnchorName, Ref: "go:x"}}
	if Resolve(fresh, existing, nil, nil).Verdict != Candidate {
		t.Fatal("without rename info only the name matches")
	}
	if Resolve(fresh, existing, map[string]string{"a/old": "b/new"}, nil).Verdict != Same {
		t.Fatal("rename detection restores the path anchor")
	}
}

func TestContentRenamesDetectMovedFilesAndDirs(t *testing.T) {
	r := ContentRenames(map[string]string{"a/x.go": "h1", "a/y.go": "h2"}, map[string]string{"b/x.go": "h1", "a/y.go": "h2"})
	if r["a/x.go"] != "b/x.go" || r["a"] != "b" {
		t.Fatalf("renames %v", r)
	}
}
