package screen

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/store"
)

// rest is what a label adds after the names it gives.
var rest = regexp.MustCompile(` (등|외 \d+)$`)

// mentions says whether label s names phrase p: p's comma-separated names stand
// together in s, or s keeps p's leading names when p is too wide.
func mentions(s, p string) bool {
	ls, ps := strings.Split(rest.ReplaceAllString(s, ""), ", "), strings.Split(p, ", ")
	for i := 0; i+len(ps) <= len(ls); i++ {
		if slices.Equal(ls[i:i+len(ps)], ps) {
			return true
		}
	}
	return len(ls) < len(ps) && slices.Equal(ls, ps[:len(ls)])
}

// misstated says what is wrong with the word s a line's label says about
// the relations it stands for, or "" when s is true of them: "minority" when
// s names phrases that fewer than half of them carry without saying how many
// it leaves out, and "hidden" when s names no phrase although one of them
// carries a phrase.
func misstated(t tree, rels []string, s string) string {
	n := map[string]int{}
	for _, r := range rels {
		if p := t.phrases[r].text; p != "" {
			n[p]++
		}
	}
	named, lead := 0, 0
	ls := strings.Split(rest.ReplaceAllString(s, ""), ", ")
	for p, c := range n {
		if s != "" && mentions(s, p) {
			named += c
		}
		if ps := strings.Split(p, ", "); len(ls) <= len(ps) && slices.Equal(ps[:len(ls)], ls) {
			lead += c
		}
	}
	switch {
	case named > 0 && 2*named < len(rels) && !strings.HasSuffix(s, fmt.Sprintf(" 외 %d", len(rels)-lead)) && !strings.HasSuffix(s, fmt.Sprintf(" 외 %d", len(rels)-n[strings.Join(ls, ", ")])):
		return "minority"
	case named == 0 && len(n) > 0:
		return "hidden"
	}
	return ""
}

func viaWord(s string) bool {
	for _, v := range []model.Via{"import", "trait", "callback", "macro", "config", "build-feature", "data", "convention", "call"} {
		if v.Word() == s {
			return true
		}
	}
	return false
}

// TestLabelAudit counts, over every line of every view of each fixture, of
// this repository's own .abbd, whose couplings carry phrases while the
// fixtures' carry none, and of each root in ABBD_LABEL_ROOTS, the lines whose
// word misstates their relations (misstated), and how many words, and
// labels drawn, name a phrase or a via. It fails on a line that names a
// phrase most of its relations do not carry without saying how many. With
// ABBD_LABEL_DUMP set it writes one row per misstated line there.
func TestLabelAudit(t *testing.T) {
	roots := map[string]string{"repo": filepath.Join("..", "..")}
	for _, f := range fixtures {
		roots[f] = filepath.Join("testdata", f)
	}
	for _, r := range filepath.SplitList(os.Getenv("ABBD_LABEL_ROOTS")) {
		if r != "" {
			roots[filepath.Base(r)] = r
		}
	}
	keys := make([]string, 0, len(roots))
	for k := range roots {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	var rows []string
	t.Logf("%-15s %5s %5s | word: %5s %5s %5s %5s | drawn: %5s %5s %5s %5s | overview drawn: %4s %4s %4s | 외 by ×N %3s", "root", "lines", "phrsd", "phr", "via", "minor", "hidn", "phr", "via", "minor", "hidn", "phr", "via", "bad", "n")
	for _, name := range keys {
		units, err := store.Open(roots[name]).LoadUnits()
		if err != nil {
			t.Fatal(err)
		}
		tr := newTree(units)
		m := Build(Input{Units: units})
		var lines, phrased int
		word, drawn, over := map[string]int{}, map[string]int{}, map[string]int{}
		for _, v := range views(m) {
			for _, w := range v.d.Wires {
				if len(w.Rels) == 0 {
					continue
				}
				lines++
				if slices.ContainsFunc(w.Rels, func(r string) bool { return tr.phrases[r].text != "" }) {
					phrased++
				}
				s := wordOf(tr, w.Rels)
				kind := ""
				switch {
				case viaWord(s):
					kind = "via"
				case s != "":
					kind = "phrase"
				}
				bad := misstated(tr, w.Rels, s)
				counted := ""
				if strings.Contains(s, " 외 ") && slices.ContainsFunc(v.d.Wires, func(o Wire) bool { return o.Count > 1 && (o.D == w.D || w.Net != 0 && o.Net == w.Net) }) {
					counted = "counted"
				}
				for _, k := range []string{kind, bad, counted} {
					if k == "" {
						continue
					}
					word[k]++
					if w.Label != nil {
						drawn[k]++
						if v.name == "overview" {
							over[k]++
						}
					}
				}
				if bad != "" {
					rows = append(rows, fmt.Sprintf("%s\t%s\t%s\t%v\t%q\t%d\t%s", name, bad, v.name, w.Label != nil, s, len(w.Rels), strings.Join(said(tr, w.Rels), "; ")))
				}
			}
		}
		t.Logf("%-15s %5d %5d | word: %5d %5d %5d %5d | drawn: %5d %5d %5d %5d | overview drawn: %4d %4d %4d | 외 by ×N %3d", name, lines, phrased,
			word["phrase"], word["via"], word["minority"], word["hidden"], drawn["phrase"], drawn["via"], drawn["minority"], drawn["hidden"],
			over["phrase"], over["via"], over["minority"]+over["hidden"], drawn["counted"])
		if word["minority"] > 0 {
			t.Errorf("%s: %d lines name a phrase fewer than half their relations carry without saying how many it leaves out", name, word["minority"])
		}
	}
	if p := os.Getenv("ABBD_LABEL_DUMP"); p != "" {
		os.WriteFile(p, []byte(strings.Join(rows, "\n")+"\n"), 0o644)
	}
}

func wordOf(t tree, rels []string) string {
	s, _ := word(t, rels)
	return s
}
