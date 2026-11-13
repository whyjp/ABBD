package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"strings"

	"github.com/whyjp/abbd/internal/gitx"
	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/screen"
	"github.com/whyjp/abbd/internal/store"
	"github.com/whyjp/abbd/internal/topology"
)

// diff prints how the stored topology differs from the one committed at the
// base revision, for the reply and the PR description. It reads only. With
// --check it exits 1 when the change reaches the boundary between modules,
// so a CI job can label the PR; ABBD itself decides nothing with it. With
// --unit it keeps one module's or feature's share, and with --page it
// writes the compare view; the blueprint itself never shows a change.
func (e env) diff() error {
	base := e.base
	if base == "" {
		var err error
		if base, err = gitx.MergeBase(e.root); err != nil {
			return err
		}
	}
	was, err := e.unitsAt(base)
	if err != nil {
		return err
	}
	now, err := e.st.LoadUnits()
	if err != nil {
		return err
	}
	d := topology.Compare(was, now)
	var in func(model.ID) bool
	if e.unit != "" {
		id := model.ID(e.unit)
		_, isNow := presentUnit(now, id)
		_, wasThen := presentUnit(was, id)
		if !isNow && !wasThen {
			return fmt.Errorf("%s is in neither the base nor the current state", id)
		}
		in = topology.Subtree(id, was, now)
		d = d.Within(in)
	}
	then, done, err := e.storeAt(base)
	defer done()
	if err != nil {
		return err
	}
	words, err := e.wordsSince(then, was, now, in)
	if err != nil {
		return err
	}
	if e.asJSON {
		b, err := json.MarshalIndent(struct {
			Base string `json:"base"`
			Unit string `json:"unit,omitempty"`
			topology.Diff
			Words []wordChange `json:"words"`
		}{base, e.unit, d, words}, "", "  ")
		if err != nil {
			return err
		}
		fmt.Fprintln(e.stdout, string(b))
	} else {
		of := ""
		if e.unit != "" {
			of = " inside " + e.unit
		}
		fmt.Fprintf(e.stdout, "topology%s since %s:\n%s", of, short(base), d)
		if len(words) > 0 {
			fmt.Fprintf(e.stdout, "words%s since %s:\n%s\n", of, short(base), strings.Join(lines(words), "\n"))
		}
	}
	if e.page {
		page, err := e.comparePage(base, then, d, words)
		if err != nil {
			return err
		}
		fmt.Fprintln(e.stderr, "compare view:", page)
	}
	if e.check && d.Boundary() {
		return fmt.Errorf("the change reaches the boundary between modules: a contract's signature or one of its entries changed, a contract was added or removed, or a relation between top-level modules appeared or disappeared")
	}
	return nil
}

var compareTmpl = template.Must(template.New("compare").Parse(`<!doctype html>
<html lang="ko"><head><meta charset="utf-8"><title>{{.Of}} 비교 · ABBD</title>
<style>{{.Style}}
body.compare{margin:16px;max-width:100vw;overflow-x:hidden}
.compare pre{white-space:pre-wrap;overflow-wrap:anywhere;max-height:40vh;overflow:auto}
.compare .pair{display:grid;grid-template-columns:minmax(0,1fr) minmax(0,1fr);gap:12px}
.compare .side{overflow:auto;border:1px solid var(--line);border-radius:6px;padding:6px}
.compare .side svg{max-width:100%;height:auto}</style></head><body class="compare">
<h1>{{.Of}}: {{.Base}}와 지금</h1>
<p>{{.Summary}}</p>
<p class="muted">청사진은 언제나 지금의 상태만 그립니다. 이 비교는 그 밖에 따로 둡니다. <a href="{{.NowPage}}">지금의 페이지</a>{{if .BasePage}} · <a href="{{.BasePage}}">그때의 페이지</a>{{end}}</p>
<div class="pair"><div><h2>{{.Base}}</h2><div class="side">{{if .Before}}{{.Before}}{{else}}<p>그때의 청사진에는 이 그림이 없습니다.</p>{{end}}</div></div>
<div><h2>지금</h2><div class="side">{{if .Now}}{{.Now}}{{else}}<p>지금의 청사진에는 이 그림이 없습니다.</p>{{end}}</div></div></div>
<details><summary>바뀐 것 {{.Count}}줄</summary><pre>{{.Lines}}</pre></details>
{{if .WordCount}}<details><summary>바뀐 글 {{.WordCount}}줄</summary><pre>{{.Words}}</pre></details>{{end}}
</body></html>
`))

// drawing cuts a page's drawing out of its HTML, the one in the section
// anchor names, and the page's style, so the compare view shows the two
// drawings side by side without the rest of either page.
func drawing(page []byte, anchor string) (svg, style string) {
	s := string(page)
	if a, b, ok := strings.Cut(s, "<style>"); ok && a != "" {
		style, _, _ = strings.Cut(b, "</style>")
	}
	// A unit with no inside of its own has a focus view as its page, under
	// another anchor, so the page's first drawing stands in for it.
	rest := s
	if _, after, ok := strings.Cut(s, `id="`+anchor+`"`); ok {
		rest = after
	}
	_, rest, ok := strings.Cut(rest, `<div class="mapbox">`)
	if !ok {
		return "", style
	}
	svg, _, _ = strings.Cut(rest, "</svg>")
	return svg + "</svg>", style
}

// summary says in one line how much changed, by kind.
func summary(d topology.Diff) string {
	var parts []string
	add := func(n int, what string) {
		if n > 0 {
			parts = append(parts, fmt.Sprintf("%s %d", what, n))
		}
	}
	add(len(d.UnitsAdded), "단위 추가")
	add(len(d.UnitsRemoved), "단위 제거")
	add(len(d.Changed), "단위 변경")
	add(len(d.Signatures)+len(d.Entries)+len(d.Listed), "계약 모양 변경")
	add(len(d.Code), "계약 코드 변경")
	add(len(d.RelationsAdded), "관계 추가")
	add(len(d.RelationsRemoved), "관계 제거")
	add(len(d.Conditions), "조건 변경")
	add(len(d.RunsAdded), "실행 관계 추가")
	add(len(d.RunsRemoved), "실행 관계 제거")
	add(len(d.InferredAdded), "판독한 결합 추가")
	add(len(d.InferredDead), "근거가 죽은 결합")
	if len(parts) == 0 {
		return "토폴로지에는 바뀐 것이 없습니다."
	}
	return strings.Join(parts, " · ")
}

// comparePage draws the blueprint of the state committed at base into the
// compare directory, and a page that sets one of its pages beside the
// current one with the changes listed. Git and the state's fingerprint both
// ignore the directory.
func (e env) comparePage(base string, at store.Store, d topology.Diff, words []wordChange) (string, error) {
	dir := e.st.CompareDir()
	then := filepath.Join(dir, "base")
	if err := os.RemoveAll(then); err != nil {
		return "", err
	}
	if err := os.MkdirAll(then, 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("*\n"), 0o644); err != nil {
		return "", err
	}
	pages, err := e.pagesAt(base, at)
	if err != nil {
		return "", err
	}
	for f, b := range pages {
		if err := os.WriteFile(filepath.Join(then, f), b, 0o644); err != nil {
			return "", err
		}
	}
	name, of := "index.html", "청사진"
	if e.unit != "" {
		name, of = e.unit+".html", e.unit
	}
	exists := func(p string) string {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(p))); err != nil {
			return ""
		}
		return p
	}
	anchor := "overview"
	if e.unit != "" {
		anchor = "m-" + e.unit
	}
	load := func(p string) []byte {
		if p == "" {
			return nil
		}
		b, _ := os.ReadFile(filepath.Join(dir, filepath.FromSlash(p)))
		return b
	}
	wasSVG, _ := drawing(load(exists("base/"+name)), anchor)
	nowSVG, style := drawing(load(exists("../blueprint/"+name)), anchor)
	var b strings.Builder
	if err := compareTmpl.Execute(&b, map[string]any{
		"Of": of, "Base": short(base), "Summary": summary(d), "Lines": d.String(), "Count": len(d.Lines()), "Words": strings.Join(lines(words), "\n"), "WordCount": len(lines(words)),
		"Style": template.CSS(style), "Before": template.HTML(wasSVG), "Now": template.HTML(nowSVG),
		"BasePage": exists("base/" + name), "NowPage": "../blueprint/" + name,
	}); err != nil {
		return "", err
	}
	page := filepath.Join(dir, "index.html")
	return page, os.WriteFile(page, []byte(b.String()), 0o644)
}

// pagesAt draws the blueprint of at, the state committed at base, so the
// base reads in the renderer of now, or reads the pages the kept copy holds.
func (e env) pagesAt(base string, at store.Store) (map[string][]byte, error) {
	if base == prev {
		kept := filepath.Join(e.st.PreviousDir(), store.Dir, "blueprint")
		es, err := os.ReadDir(kept)
		if err != nil {
			return nil, err
		}
		pages := map[string][]byte{}
		for _, en := range es {
			if pages[en.Name()], err = os.ReadFile(filepath.Join(kept, en.Name())); err != nil {
				return nil, err
			}
		}
		return pages, nil
	}
	in, err := screen.Load(at)
	if err != nil || len(in.Units) == 0 {
		return nil, err
	}
	return screen.Files(screen.Build(in))
}

// prev names the copy of .abbd that a write kept from before the first
// write since the last commit.
const prev = "prev"

// keepPrevious keeps .abbd as it was before a writing command's first
// change since the last commit. A failure only warns: the write goes on.
func (e env) keepPrevious(c command) {
	if !c.writes {
		return
	}
	if err := e.st.KeepPrevious(gitx.Head(e.root)); err != nil {
		fmt.Fprintln(e.stderr, "abbd: warning: could not keep the previous blueprint:", err)
	}
}

// unitsAt reads the units committed at base, or those of the kept copy.
func (e env) unitsAt(base string) ([]model.Unit, error) {
	if base != prev {
		return e.st.UnitsAt(base)
	}
	if _, err := os.Stat(filepath.Join(e.st.PreviousDir(), "HEAD")); err != nil {
		return nil, fmt.Errorf("no previous blueprint is kept yet; a write such as abbd sync keeps one before its first change since the last commit")
	}
	return store.Open(e.st.PreviousDir()).LoadUnits()
}

func short(rev string) string { return rev[:min(len(rev), 12)] }

// wordsSince is wordsDiff between then, the store at base, and now.
func (e env) wordsSince(then store.Store, was, now []model.Unit, in func(model.ID) bool) ([]wordChange, error) {
	before, err := writtenIn(then, was)
	if err != nil {
		return nil, err
	}
	after, err := writtenIn(e.st, now)
	if err != nil {
		return nil, err
	}
	return wordsDiff(before, after, in), nil
}
