package screen

import (
	"bytes"
	"regexp"
	"strings"
	"testing"

	"github.com/whyjp/abbd/internal/model"
)

// The blueprint shows the final state only: nothing on it waits for a
// decision. The overview draws a reader's coupling with the reader's mark,
// and the inside of each module it touches lists it with its via and quote.
func TestTheBlueprintIsFinalStateOnly(t *testing.T) {
	units := append(ripgrep(), inside("DATA-0001", "MOD-0004"))
	units[0].Relations = append(units[0].Relations, guess("MOD-0005", model.DependsOn, "import"))
	units[2].Relations = append(units[2].Relations, guess("DATA-0001", model.Consumes, "convention"))
	var b bytes.Buffer
	if err := Render(&b, Build(Input{Units: units})); err != nil {
		t.Fatal(err)
	}
	html := b.String()
	for _, decision := range []string{"<form", "<button", "<input", "<script", "승인", "반려", "결정 대기", "/act"} {
		if strings.Contains(html, decision) {
			t.Errorf("the blueprint carries decision UI %q", decision)
		}
	}
	overview, insides, ok := strings.Cut(html, `<section class="inside"`)
	if !ok {
		t.Fatalf("the blueprint has no module insides")
	}
	_, svg, _ := strings.Cut(overview, `<section id="overview">`)
	if regexp.MustCompile(`class="w [^"]*guess|DATA-0001`).MatchString(svg) {
		t.Errorf("the overview dashes a reader's line or draws an inside unit:\n%s", svg)
	}
	if !regexp.MustCompile(`<path class="w bypass[^"]*" [^>]*><title>[^<]*ripgrep → grep-printer[^<]*판독`).MatchString(svg) || !strings.Contains(svg, `<circle class="origin inferred"`) {
		t.Errorf("the overview should draw the reader's coupling ripgrep → grep-printer with its mark:\n%s", svg)
	}
	for _, want := range []string{
		`ripgrep → grep-printer <span class="muted">(depends_on, via import)</span>`,
		`grep-searcher → DATA-0001 <span class="muted">(consumes, via convention)</span>`,
		`<q>src/a.rs:1 &#34;use b;&#34;</q>`,
	} {
		if !strings.Contains(insides, want) {
			t.Errorf("an inside should list the guess %s", want)
		}
	}
	if !strings.Contains(html, `id="freshness"`) {
		t.Errorf("the blueprint says whether it matches the working tree")
	}
}
