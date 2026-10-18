package intent

import (
	"testing"

	"github.com/whyjp/for-my-harness/internal/ssot"
)

func TestRouting(t *testing.T) {
	s := ssot.NewSnapshot(t.TempDir())
	cases := []struct {
		req, typ, tier string
		newFeature     bool
	}{
		{"로그인 버그 고쳐줘", "bug-fix", "T1", false},
		{"README 문서 갱신", "docs", "T0", false},
		{"GET /health 엔드포인트 추가", "feature", "T2", true},
		{"heartbeat-events 토픽 스키마 변경", "interface-change", "T2", false},
		{"재시도 정책을 동적 설정 가능하게 변경", "feature", "T3", false},
		{"메시지 수신 API 만들어줘", "feature", "T1", true},
	}
	for _, c := range cases {
		r := Resolve(c.req, s)
		if r.TaskType != c.typ || r.Tier != c.tier || r.NewFeature != c.newFeature {
			t.Errorf("%q: got %s/%s/new=%v, want %s/%s/new=%v", c.req, r.TaskType, r.Tier, r.NewFeature, c.typ, c.tier, c.newFeature)
		}
	}
}

func TestTokensTrimKoreanParticles(t *testing.T) {
	got := Tokens("메시지를 토픽에 발행")
	want := map[string]bool{"메시지": true, "토픽": true, "발행": true}
	for _, g := range got {
		delete(want, g)
	}
	if len(want) > 0 {
		t.Fatalf("tokens %v missing %v", got, want)
	}
}
