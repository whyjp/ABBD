package screen

import (
	"strings"
	"testing"
)

// A page without its text reads as its drawing and its unit names. It says
// once that the module has no text, and no unit repeats that it has none.
func TestAPageWithoutTextSaysSoOnce(t *testing.T) {
	m := Build(Input{Units: append(ripgrep(), inside("DATA-0001", "MOD-0004"))})
	for name, page := range pages(t, m) {
		if strings.Contains(page, "책임 문장이 아직") {
			t.Errorf("%s repeats a missing responsibility line per unit", name)
		}
		if n := strings.Count(page, "이 모듈의 설명은 아직 없습니다"); n > 1 {
			t.Errorf("%s says %d times that a text is missing", name, n)
		}
	}
}
