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

// An entries table opens only when it is short and has a note to show, and
// a table with no notes has no note column.
func TestAnEntriesTableFoldsWhenItHasNothingToSay(t *testing.T) {
	render := func(e Entries) string {
		t.Helper()
		var b strings.Builder
		if err := site.ExecuteTemplate(&b, "entries", e); err != nil {
			t.Fatal(err)
		}
		return b.String()
	}
	rows := func(n int) []EntryRow {
		var out []EntryRow
		for i := range n {
			out = append(out, EntryRow{Key: string(rune('a' + i%26)), Name: "e"})
		}
		return out
	}
	bare := render(Entries{ID: "IF-0001", Shape: "시그니처", Rows: rows(3)})
	if strings.Contains(bare, " open>") || strings.Contains(bare, "<th>설명</th>") {
		t.Errorf("a table with no notes should start folded and drop its note column:\n%s", bare)
	}
	noted := rows(3)
	noted[0].Note = &Note{}
	if short := render(Entries{ID: "IF-0001", Shape: "시그니처", Rows: noted, Notes: true}); !strings.Contains(short, `<details class="entries" open>`) || !strings.Contains(short, "<th>설명</th>") {
		t.Errorf("a short table with a note should open with its note column:\n%s", short)
	}
	long := append(rows(21), noted...)
	if s := render(Entries{ID: "IF-0001", Shape: "시그니처", Rows: long, Notes: true}); strings.Contains(s, " open>") {
		t.Errorf("a table of more than 20 rows should start folded")
	}
}
