package topology

import (
	"strings"
	"testing"

	"github.com/whyjp/abbd/internal/model"
)

// A link names the condition it holds under in its label, as explain
// prints it.
func TestALinkLabelSaysItsCondition(t *testing.T) {
	l := Link{Origin: model.Extracted, Via: "link", When: "PRIVATE"}
	if got := l.Label(); !strings.Contains(got, "via link, when PRIVATE") {
		t.Errorf("label %q does not say the condition", got)
	}
	if got := (Link{Origin: model.Extracted}).Label(); strings.Contains(got, "when") {
		t.Errorf("an unconditional link says a condition: %q", got)
	}
}
