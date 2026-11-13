package topology

import (
	"strings"
	"testing"

	"github.com/whyjp/abbd/internal/model"
)

// A link names the condition it holds under in its label, as explain
// prints it, and names a link scope as its scope, never as a condition.
func TestALinkLabelSaysItsCondition(t *testing.T) {
	l := Link{Origin: model.Extracted, Via: "link", When: `feature = "zstd"`}
	if got := l.Label(); !strings.Contains(got, `via link, when feature = "zstd"`) {
		t.Errorf("label %q does not say the condition", got)
	}
	l.When = "PRIVATE"
	if got := l.Label(); !strings.Contains(got, "via link, scope PRIVATE") || strings.Contains(got, "when") {
		t.Errorf("label %q does not say the link scope as a scope", got)
	}
	if got := (Link{Origin: model.Extracted}).Label(); strings.Contains(got, "when") {
		t.Errorf("an unconditional link says a condition: %q", got)
	}
}
