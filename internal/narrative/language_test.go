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
	} {
		if got := ReadsIn(text, ko); got != want {
			s, n := Share(text, ko)
			t.Errorf("ReadsIn(%q, Hangul) = %v (share %.2f of %d letters), want %v", text, got, s, n, want)
		}
	}
	if ReadsIn("동기화는 상태를 저장하고 청사진을 다시 그린다.", []*unicode.RangeTable{unicode.Latin}) {
		t.Error("Latin reads a Korean sentence")
	}
}
