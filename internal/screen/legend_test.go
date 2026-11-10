package screen

import (
	"strings"
	"testing"
)

// The legend's explanations sit once on legend.html. A module's page links
// there and repeats none of them, so its own content is what it shows.
func TestTheLegendIsSaidOnce(t *testing.T) {
	ps := pages(t, Build(Input{Units: ripgrep()}))
	explained := []string{"테두리의 칸은 이 모듈의 계약입니다", "굵은 테의 상자가 중심입니다", "상자 테두리와 옅은 바탕", "계약을 거친 사용", "abbd status"}
	for _, w := range explained {
		if !strings.Contains(ps[LegendFile], w) {
			t.Errorf("legend.html should explain %q", w)
		}
		if strings.Contains(ps["MOD-0003.html"], w) {
			t.Errorf("a module page repeats the legend's %q", w)
		}
	}
	if !strings.Contains(ps["MOD-0003.html"], `href="legend.html"`) {
		t.Errorf("a module page should link the legend")
	}
}
