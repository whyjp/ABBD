package intent

import (
	"testing"

	"github.com/whyjp/abbd/internal/ssot"
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

func TestRuleSentencesNeedScopeAndDuty(t *testing.T) {
	for _, s := range []string{
		"절대 경로로 바꿔줘",
		"always_on 플래그 추가",
		"모든 테스트가 통과하도록 고쳐줘",
		"every test file was updated",
	} {
		if IsProjectRule(s) {
			t.Errorf("not a standing rule: %q", s)
		}
	}
	for _, s := range []string{
		"앞으로 모든 HTTP 에러 응답은 JSON 형식이어야 한다",
		"Every route must appear in GET /routes.",
		"ids are case-insensitive everywhere in the service.",
		"모든 엔드포인트는 인증을 거쳐야 한다",
		"모든 응답에 trace id를 붙여줘야 한다",
		"앞으로 모든 로그는 JSON 형식을 써야 한다",
		"all endpoints require auth",
		"every request needs a trace id",
	} {
		if !IsProjectRule(s) {
			t.Errorf("a standing rule: %q", s)
		}
	}
}
