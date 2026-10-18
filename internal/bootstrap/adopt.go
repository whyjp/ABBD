package bootstrap

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/whyjp/for-my-harness/internal/model"
	"github.com/whyjp/for-my-harness/internal/ssot"
	"gopkg.in/yaml.v3"
)

// AdoptReport records what an existing document contributed. Adopt reconstructs, it does
// not import: structure is always re-extracted from code, and only meaning fields
// (purpose, responsibility) are taken from the document as ADOPTED (PRD ch.17 Q10 default).
type AdoptReport struct {
	From      string   `yaml:"from" json:"from"`
	Entries   int      `yaml:"entries" json:"entries"`
	Applied   []string `yaml:"applied" json:"applied"`
	Conflicts []string `yaml:"conflicts" json:"conflicts"`
	Unmatched []string `yaml:"unmatched" json:"unmatched"`
	Errors    []string `yaml:"errors" json:"errors"`
}

// entry is one row of a legacy document: a name or path plus a description.
type entry struct {
	Name, Path, Text string
}

func adopt(s *ssot.Snapshot, from string) *AdoptReport {
	rep := &AdoptReport{From: from}
	if from == "" {
		rep.Errors = append(rep.Errors, "--from 없음")
		return rep
	}
	entries, err := readEntries(from)
	if err != nil {
		rep.Errors = append(rep.Errors, err.Error())
		return rep
	}
	rep.Entries = len(entries)
	for _, e := range entries {
		label := firstNonEmpty(e.Name, e.Path)
		if applyToModule(s, e, rep) || applyToFeature(s, e, rep) {
			continue
		}
		rep.Unmatched = append(rep.Unmatched, label)
	}
	return rep
}

func firstNonEmpty(xs ...string) string {
	for _, x := range xs {
		if x != "" {
			return x
		}
	}
	return ""
}

func norm(s string) string {
	return strings.Trim(strings.ToLower(strings.TrimSpace(s)), "/`*")
}

func adoptInto(field *model.Semantic, id, text string, rep *AdoptReport) {
	switch {
	case field.Origin == model.UserStated || field.Confirmation == model.Confirmed:
		if norm(field.Value) != norm(text) {
			rep.Conflicts = append(rep.Conflicts, fmt.Sprintf("%s: 확정값 %q 와 문서 %q 불일치 (확정값 유지)", id, field.Value, text))
		}
	default:
		*field = model.MustSemantic(text, model.Adopted)
		rep.Applied = append(rep.Applied, id)
	}
}

func applyToModule(s *ssot.Snapshot, e entry, rep *AdoptReport) bool {
	for _, id := range model.SortedKeys(s.Modules) {
		m := s.Modules[id]
		if m.Status == model.ItemRemoved {
			continue
		}
		if (e.Path != "" && norm(e.Path) == norm(m.Path)) || (e.Name != "" && (norm(e.Name) == norm(m.Name) || norm(e.Name) == norm(filepath.Base(m.Path)))) {
			adoptInto(&m.Responsibility, id, e.Text, rep)
			return true
		}
	}
	return false
}

func applyToFeature(s *ssot.Snapshot, e entry, rep *AdoptReport) bool {
	key := norm(firstNonEmpty(e.Name, e.Path))
	for _, id := range model.SortedKeys(s.Features) {
		f := s.Features[id]
		if f.Status == model.ItemRemoved {
			continue
		}
		hit := norm(f.Name) == key
		for _, ep := range f.EntryPoints {
			hit = hit || norm(ep.Selector) == key || strings.Contains(norm(ep.Selector), key) && len(key) > 3
		}
		if hit {
			adoptInto(&f.Intent.Purpose, id, e.Text, rep)
			return true
		}
	}
	return false
}

var (
	tagRe    = regexp.MustCompile(`(?s)<[^>]+>`)
	rowRe    = regexp.MustCompile(`(?is)<tr[^>]*>(.*?)</tr>`)
	cellRe   = regexp.MustCompile(`(?is)<t[dh][^>]*>(.*?)</t[dh]>`)
	nameKeys = []string{"name", "이름", "모듈", "module", "기능", "feature", "id"}
	pathKeys = []string{"path", "경로", "package", "패키지", "selector", "entry", "진입점"}
	textKeys = []string{"responsibility", "책임", "purpose", "목적", "description", "설명", "역할"}
)

func readEntries(from string) ([]entry, error) {
	info, err := os.Stat(from)
	if err != nil {
		return nil, fmt.Errorf("adopt 입력을 읽을 수 없음: %w", err)
	}
	files := []string{from}
	if info.IsDir() {
		files = nil
		filepath.WalkDir(from, func(p string, d os.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				files = append(files, p)
			}
			return nil
		})
	}
	var out []entry
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		switch strings.ToLower(filepath.Ext(f)) {
		case ".yaml", ".yml":
			var v any
			if yaml.Unmarshal(b, &v) == nil {
				out = append(out, fromStructured(v)...)
			}
		case ".json":
			var v any
			if json.Unmarshal(b, &v) == nil {
				out = append(out, fromStructured(v)...)
			}
		case ".html", ".htm":
			out = append(out, fromHTMLTables(string(b))...)
		case ".md", ".markdown":
			out = append(out, fromMarkdownTables(string(b))...)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("adopt 입력에서 name/path + 설명 항목을 찾지 못함: %s", from)
	}
	return out, nil
}

func pick(m map[string]string, keys []string) string {
	for k, v := range m {
		lk := strings.ToLower(k)
		for _, want := range keys {
			if strings.Contains(lk, want) && strings.TrimSpace(v) != "" {
				return strings.TrimSpace(v)
			}
		}
	}
	return ""
}

func toEntry(m map[string]string) (entry, bool) {
	e := entry{Name: pick(m, nameKeys), Path: pick(m, pathKeys), Text: pick(m, textKeys)}
	return e, e.Text != "" && (e.Name != "" || e.Path != "")
}

func fromStructured(v any) []entry {
	var out []entry
	var walk func(any)
	walk = func(v any) {
		switch t := v.(type) {
		case map[string]any:
			flat := map[string]string{}
			for k, x := range t {
				if s, ok := x.(string); ok {
					flat[k] = s
				}
			}
			if e, ok := toEntry(flat); ok {
				out = append(out, e)
			}
			for _, x := range t {
				walk(x)
			}
		case []any:
			for _, x := range t {
				walk(x)
			}
		}
	}
	walk(v)
	return out
}

func rowsToEntries(header []string, rows [][]string) []entry {
	var out []entry
	for _, r := range rows {
		m := map[string]string{}
		for i, h := range header {
			if i < len(r) {
				m[h] = r[i]
			}
		}
		if e, ok := toEntry(m); ok {
			out = append(out, e)
		}
	}
	return out
}

func fromMarkdownTables(text string) []entry {
	var out []entry
	var header []string
	var rows [][]string
	flush := func() {
		if header != nil {
			out = append(out, rowsToEntries(header, rows)...)
		}
		header, rows = nil, nil
	}
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "|") {
			flush()
			continue
		}
		cells := strings.Split(strings.Trim(line, "|"), "|")
		for i := range cells {
			cells[i] = strings.TrimSpace(cells[i])
		}
		if strings.Trim(strings.Join(cells, ""), "-: ") == "" {
			continue
		}
		if header == nil {
			header = cells
		} else {
			rows = append(rows, cells)
		}
	}
	flush()
	return out
}

func fromHTMLTables(text string) []entry {
	var header []string
	var rows [][]string
	for _, tr := range rowRe.FindAllStringSubmatch(text, -1) {
		var cells []string
		for _, td := range cellRe.FindAllStringSubmatch(tr[1], -1) {
			cells = append(cells, strings.TrimSpace(tagRe.ReplaceAllString(td[1], "")))
		}
		if header == nil || strings.Contains(strings.ToLower(tr[1]), "<th") {
			header = cells
			continue
		}
		rows = append(rows, cells)
	}
	return rowsToEntries(header, rows)
}
