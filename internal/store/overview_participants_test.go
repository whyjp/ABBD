package store

import (
	"strings"
	"testing"
)

// A draft's participants are read as claims and written back unchanged.
func TestADraftKeepsItsParticipants(t *testing.T) {
	draft := "---\nparticipants:\n    - unit: EXT-0001\n      evidence:\n        - path: a/x.go\n          lines: \"3\"\n          quote: use lib\n---\n\ncaption\n"
	o, err := ParseOverviewDraft([]byte(draft))
	if err != nil {
		t.Fatal(err)
	}
	if len(o.Participants) != 1 || o.Participants[0].Claim.Unit != "EXT-0001" {
		t.Fatalf("participants: %+v", o.Participants)
	}
	back, err := OverviewDraft(o)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(back), "participants:") || !strings.Contains(string(back), "EXT-0001") {
		t.Errorf("draft: %s", back)
	}
	if _, err := ParseOverviewDraft([]byte(strings.Replace(draft, "participants:", "participant:", 1))); err == nil {
		t.Error("a misspelt key is refused")
	}
}
