package bootstrap

import (
	"fmt"
	"strings"

	"github.com/whyjp/abbd/internal/confirm"
	"github.com/whyjp/abbd/internal/model"
)

// Report is what setup tells the human. It holds no timestamps, so rerunning
// setup on unchanged code rewrites the same bytes.
type Report struct {
	Mode      string
	Extract   []string
	Digest    string
	Units     []model.Unit
	Queue     []confirm.Item
	Overlays  map[string]Findings
	Failures  map[string]string
	NextSteps []string
}

func (r Report) Markdown() []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "# Bootstrap report\n\nMode: %s.\n", r.Mode)
	if len(r.Extract) > 0 {
		fmt.Fprintf(&b, "Extraction script: `%s`. Two runs printed identical output (sha256 %s).\n", strings.Join(r.Extract, " "), r.Digest)
	}
	fmt.Fprintf(&b, "\n## Design unit candidates\n\n")
	for _, u := range r.Units {
		fmt.Fprintf(&b, "- %s %s (%s), %d anchor(s)", u.ID, u.Name.Value, u.Name.Origin, len(u.Anchors))
		if u.Signature != nil {
			fmt.Fprintf(&b, ", signature `%s`", u.Signature.Value)
		}
		if u.Purpose != nil {
			fmt.Fprintf(&b, ", purpose %s: %s", u.Purpose.Origin, u.Purpose.Value)
		}
		b.WriteString("\n")
	}
	for _, name := range []string{Supplement.Name, Adopt.Name} {
		if msg, ok := r.Failures[name]; ok {
			fmt.Fprintf(&b, "\n## %s\n\nNot applied: %s\nThe code-only bootstrap above is unaffected.\n", name, msg)
			continue
		}
		f, ok := r.Overlays[name]
		if !ok {
			continue
		}
		fmt.Fprintf(&b, "\n## %s\n\n%d unit(s) matched the code.\n", name, f.Matched)
		list(&b, "Conflicts", f.Conflicts)
		list(&b, "Gaps", f.Gaps)
	}
	fmt.Fprintf(&b, "\n## Confirm queue\n\n%d item(s) wait for a human. None of them blocks alignment.\n", len(r.Queue))
	for _, it := range r.Queue {
		fmt.Fprintf(&b, "- %s\n", it)
	}
	b.WriteString("\n## Blind spots\n\nCouplings the script cannot see, such as a shared table or an execution order, may exist. Register one by confirming it from the queue, then have the script learn it.\n")
	if len(r.NextSteps) > 0 {
		b.WriteString("\n## Next\n\n")
		for _, s := range r.NextSteps {
			fmt.Fprintf(&b, "- `%s`\n", s)
		}
	}
	return []byte(b.String())
}

func list(b *strings.Builder, title string, items []string) {
	if len(items) == 0 {
		fmt.Fprintf(b, "%s: none.\n", title)
		return
	}
	fmt.Fprintf(b, "%s:\n\n", title)
	for _, s := range items {
		fmt.Fprintf(b, "- %s\n", s)
	}
}
