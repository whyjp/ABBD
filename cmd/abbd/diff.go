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
	was, err := e.st.UnitsAt(base)
	if err != nil {
		return err
	}
	now, err := e.st.LoadUnits()
	if err != nil {
		return err
	}
	d := topology.Compare(was, now)
	if e.unit != "" {
		id := model.ID(e.unit)
		_, isNow := presentUnit(now, id)
		_, wasThen := presentUnit(was, id)
		if !isNow && !wasThen {
			return fmt.Errorf("%s is in neither the base nor the current state", id)
		}
		d = d.Within(topology.Subtree(id, was, now))
	}
	if e.asJSON {
		b, err := json.MarshalIndent(struct {
			Base string `json:"base"`
			Unit string `json:"unit,omitempty"`
			topology.Diff
		}{base, e.unit, d}, "", "  ")
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
	}
	if e.page {
		page, err := e.comparePage(base, d)
		if err != nil {
			return err
		}
		fmt.Fprintln(e.stdout, "compare view:", page)
	}
	if e.check && d.Boundary() {
		return fmt.Errorf("the change reaches the boundary between modules: a contract's signature or one of its entries changed, a contract was added or removed, or a relation between top-level modules appeared or disappeared")
	}
	return nil
}

var compareTmpl = template.Must(template.New("compare").Parse(`<!doctype html>
<html lang="ko"><head><meta charset="utf-8"><title>{{.Of}} 비교 · ABBD</title>
<style>body{font:14px/1.5 system-ui,sans-serif;margin:16px}pre{background:#f6f6f6;padding:8px;white-space:pre-wrap}
.pair{display:grid;grid-template-columns:1fr 1fr;gap:8px}iframe{width:100%;height:80vh;border:1px solid #ccc}
@media (prefers-color-scheme:dark){body{background:#111;color:#ddd}pre{background:#222}}</style></head><body>
<h1>{{.Of}}: {{.Base}}와 지금</h1>
<p>청사진은 언제나 지금의 상태만 그립니다. 이 비교는 그 밖에 따로 둡니다.</p>
<pre>{{.Lines}}</pre>
<div class="pair"><div><h2>{{.Base}}</h2>{{if .Before}}<iframe src="{{.Before}}"></iframe>{{else}}<p>그때의 청사진에는 이 페이지가 없습니다.</p>{{end}}</div>
<div><h2>지금</h2>{{if .Now}}<iframe src="{{.Now}}"></iframe>{{else}}<p>지금의 청사진에는 이 페이지가 없습니다.</p>{{end}}</div></div>
</body></html>
`))

// comparePage writes the blueprint committed at base into the compare
// directory, and a page that sets one of its pages beside the current one
// with the changes listed. Git and the state's fingerprint both ignore the
// directory.
func (e env) comparePage(base string, d topology.Diff) (string, error) {
	dir := e.st.CompareDir()
	if err := os.RemoveAll(dir); err != nil {
		return "", err
	}
	then := filepath.Join(dir, "base")
	if err := os.MkdirAll(then, 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("*\n"), 0o644); err != nil {
		return "", err
	}
	files, err := gitx.Files(e.root, base, store.Dir+"/blueprint")
	if err != nil {
		return "", err
	}
	for _, f := range files {
		b, err := gitx.Show(e.root, base, f)
		if err != nil {
			return "", err
		}
		if err := os.WriteFile(filepath.Join(then, filepath.Base(f)), b, 0o644); err != nil {
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
	var b strings.Builder
	if err := compareTmpl.Execute(&b, map[string]string{
		"Of": of, "Base": short(base), "Lines": d.String(),
		"Before": exists("base/" + name), "Now": exists("../blueprint/" + name),
	}); err != nil {
		return "", err
	}
	page := filepath.Join(dir, "index.html")
	return page, os.WriteFile(page, []byte(b.String()), 0o644)
}

func short(rev string) string { return rev[:min(len(rev), 12)] }
