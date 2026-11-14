package narrative

import (
	"testing"
	"unicode"
)

func TestReadsInJudgesProseAndLeavesCodePathsAndQuotesOut(t *testing.T) {
	ko := []*unicode.RangeTable{unicode.Hangul}
	for text, want := range map[string]bool{
		"Owns the sync pipeline and writes state.yaml.":                                                   false,
		"## Overview\n\n`abbd sync`가 `internal/store/store.go`의 LoadProject를 호출해 상태를 저장한다.":               true,
		"sync 파이프라인은 blueprint를 다시 그린다.":                                                                  true,
		"상태는 \"the state always rests on the deterministic extraction alone\" 라고 적힌 대로 저장된다.":             true,
		"```go\nfunc Sync() error { return nil } // writes the whole state\n```\n동기화는 상태를 쓴다.":            true,
		"`cmd/abbd/main.go` MOD-0004 runScript syncActual":                                                true,
		"한 줄: the extraction runs, the reader adds evidence, and every sync checks what went stale today": false,
		"store는 unit files와 lint debt를 저장한다.":                                                             true,
		"**drawn versus listed relation rule**(`DATA-0010`)이 `Counts`에 있다.":                               true,
		"읽기 동사는 explain, brief, path, impact, lint다.":                                                     true,
		"lint는 3개 view와 2번 sync를 본다.":                                                                     true,
		"Stores unit files and the lint debt.":                                                            false,
		"Draws the overview with 3 views.":                                                                false,
	} {
		if got := ReadsIn(text, ko); got != want {
			s, n := Share(text, ko)
			t.Errorf("ReadsIn(%q, Hangul) = %v (share %.2f of %d letters), want %v", text, got, s, n, want)
		}
	}
	if ReadsIn("동기화는 상태를 저장하고 청사진을 다시 그린다.", []*unicode.RangeTable{unicode.Latin}) {
		t.Error("Latin reads a Korean sentence")
	}
	if s, n := Share("lint는 3개 view를 본다.", ko); n != 4 || s != 1 {
		t.Errorf("a Korean counter must count as a Korean word: share %.2f of %d words, want 1 of 4", s, n)
	}
	if !ReadsIn("Stores unit files and the lint debt.", nil) {
		t.Error("a language with no known script must not be judged")
	}
}

func TestReadsAsWordFailsABareIdentifier(t *testing.T) {
	ko := []*unicode.RangeTable{unicode.Hangul}
	for text, want := range map[string]bool{
		"EnvironBuilder":           false,
		"FLASK_DEBUG":              false,
		"SESSION_COOKIE_*":         false,
		"Request, Response":        false,
		"EnvironBuilder로 요청 환경 생성": true,
		"JSON 직렬화":                 true,
		"abbd sync 실행":             true,
		"404":                      true,
	} {
		if got := ReadsAsWord(text, ko); got != want {
			t.Errorf("ReadsAsWord(%q, Hangul) = %v, want %v", text, got, want)
		}
	}
	if !ReadsAsWord("EnvironBuilder", nil) {
		t.Error("with no script to judge by every word reads")
	}
}
