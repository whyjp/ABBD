package main

import (
	"regexp"
	"strings"
	"testing"
)

func TestScreenFoldsTheCandidatesByKindAndTheQueueByUnit(t *testing.T) {
	p, exe := bare(t)
	p.extract(codeOnly)
	p.setup(0, "", "--supplement", p.file(t, "sup.json", supplement), "--", exe)
	s := p.screen()
	html := s.view(t)
	mods, ifs := strings.Index(html, `<summary>모듈 <span class="muted">2개</span>`), strings.Index(html, `<summary>인터페이스 <span class="muted">1개</span>`)
	if mods < 0 || ifs < mods {
		t.Fatalf("first-alignment candidates should fold by kind, modules first:\n%s", html)
	}

	s.decide(t, "do", "align", "id", "MOD-0002", "id", "IF-0001")
	html = s.view(t)
	design, below := strings.Index(html, `id="qg-MOD-0002"`), strings.Index(html, `id="qg-MOD-0001"`)
	if design < 0 || below < design {
		t.Fatalf("the queue should fold by unit, design units first:\n%s", html)
	}
	if !strings.Contains(html[design:below], "MOD-0002.purpose") || !strings.Contains(html[design:below], "MOD-0002.consumes.MOD-0001") || !strings.Contains(html[below:], "MOD-0001.responsibility") {
		t.Fatalf("each unit's items belong under its own group:\n%s", html)
	}
	if regexp.MustCompile(`<details class="group" open`).MatchString(html) {
		t.Fatalf("groups start folded so the first screen stays short")
	}
}
