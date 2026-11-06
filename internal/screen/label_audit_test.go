package screen

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/whyjp/abbd/internal/store"
)

// rest is what a label adds after the names it gives.
var rest = regexp.MustCompile(` (등|외 (\d+))$`)

// mentions says whether label s names phrase p: p's names stand together in
// s, or s keeps p's leading names.
func mentions(s, p string) bool {
	ls, ps := phraseNames(rest.ReplaceAllString(s, "")), phraseNames(p)
	for i := 0; i+len(ps) <= len(ls); i++ {
		if slices.Equal(ls[i:i+len(ps)], ps) {
			return true
		}
	}
	return len(ls) < len(ps) && slices.Equal(ls, ps[:len(ls)])
}

// misstated says what is wrong with label s, a phrase when phrase is set,
// on wire w beside a bus count of count lines, or "" when s is true of the
// lines it speaks for:
//   - "hidden": s is no phrase although one of them carries a phrase;
//   - "unknown": s names a phrase none of them carries;
//   - "overstated": s names a phrase alone that not every relation carries;
//   - "minority": s says 등 for a phrase fewer than half the units carry;
//   - "miscount": s says 외 N for a phrase at least half the units carry,
//     or N is not how many units do not carry it, or s keeps more than the
//     phrase's first name before 외, which reads as a list;
//   - "count": s says 외 N beside a ×count, and the units that carry its
//     phrase and N do not add up to count.
func misstated(t tree, w Wire, s string, phrase bool, count int) string {
	us := units(w.lines)
	n := carried(us, func(r string) string { return t.phrases[r].text })
	if !phrase {
		if len(n) > 0 {
			return "hidden"
		}
		return ""
	}
	m := rest.FindStringSubmatch(s)
	ls := phraseNames(rest.ReplaceAllString(s, ""))
	p, c := "", 0
	for q, k := range n {
		if qs := phraseNames(q); len(ls) <= len(qs) && slices.Equal(qs[:len(ls)], ls) && k > c {
			p, c = q, k
		}
	}
	switch {
	case p == "":
		return "unknown"
	case m == nil:
		if len(ls) < len(phraseNames(p)) || !every(w.lines, func(r string) bool { return t.phrases[r].text == p }) {
			return "overstated"
		}
	case m[1] == "등":
		if 2*c < len(us) {
			return "minority"
		}
	default:
		if k, _ := strconv.Atoi(m[2]); 2*c >= len(us) || k != len(us)-c || len(ls) > 1 {
			return "miscount"
		} else if count > 1 && c+k != count {
			return "count"
		}
	}
	return ""
}

// TestLabelAudit checks, over every line of every view of each fixture, of
// this repository's own .abbd, whose couplings carry phrases while the
// fixtures' carry none, and of each root in ABBD_LABEL_ROOTS, the label
// each line draws (see misstated) against the lines it speaks for and the
// ×N of its bus, and that no line holds a relation twice. It fails on any
// misstated drawn label or doubled relation, and on a word, drawn or not,
// that passes a minority phrase for the line. It logs how many words, and
// labels drawn, name a phrase or a via, and with ABBD_LABEL_DUMP set it
// writes one row per misstated line there.
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
	t.Logf("%-15s %5s %5s | word: %5s %5s %5s %5s | drawn: %5s %5s %5s %5s %5s | overview drawn: %4s %4s %4s | 외 by ×N %3s %3s | dup %3s", "root", "lines", "phrsd", "phr", "via", "minor", "hidn", "phr", "via", "minor", "hidn", "other", "phr", "via", "bad", "n", "c>1", "n")
	for _, name := range keys {
		loaded, err := store.Open(roots[name]).LoadUnits()
		if err != nil {
			t.Fatal(err)
		}
		tr := newTree(loaded)
		m := Build(Input{Units: loaded})
		var lines, phrased, dups int
		words, drawn, over := map[string]int{}, map[string]int{}, map[string]int{}
		for _, v := range views(m) {
			counts := map[int][]int{}
			for _, w := range v.d.Wires {
				if w.Net != 0 && w.Count > 1 {
					counts[w.Net] = append(counts[w.Net], w.Count)
				}
			}
			for _, w := range v.d.Wires {
				if len(w.Rels) == 0 {
					continue
				}
				lines++
				if len(slices.Compact(slices.Sorted(slices.Values(w.Rels)))) < len(w.Rels) {
					dups++
					t.Errorf("%s %s: a line holds a relation twice: %v", name, v.name, w.Rels)
				}
				if slices.ContainsFunc(w.Rels, func(r string) bool { return tr.phrases[r].text != "" }) {
					phrased++
				}
				// A path of a bus with one count speaks for the lines it counts
				// where it carries them all, as a bus drawn as one path does;
				// a branch of a road carries only its own.
				count := w.Count
				if cs := counts[w.Net]; count < 2 && len(cs) == 1 && len(w.lines) == cs[0] {
					count = cs[0]
				}
				kind := func(s string, phrase bool) string {
					switch {
					case phrase:
						return "phrase"
					case s != "":
						return "via"
					}
					return ""
				}
				s, ph := word(tr, w.lines)
				bad := misstated(tr, w, s, ph, count)
				for _, k := range []string{kind(s, ph), bad} {
					if k != "" {
						words[k]++
					}
				}
				if bad == "minority" {
					t.Errorf("%s %s: the word %q passes a phrase fewer than half its lines carry for the line", name, v.name, s)
				}
				if w.Label == nil {
					continue
				}
				l := w.Label
				drew := misstated(tr, w, l.S, l.Said, count)
				for _, k := range []string{kind(l.S, l.Said), drew} {
					if k == "" {
						continue
					}
					if k != "phrase" && k != "via" && k != "minority" && k != "hidden" {
						k = "other"
					}
					drawn[k]++
					if v.name == "overview" {
						over[k]++
					}
				}
				if strings.Contains(l.S, " 외 ") && count > 1 {
					drawn["counted"]++
					if p := phraseNames(strings.Split(l.S, " 외 ")[0])[0]; carried(units(w.lines), func(r string) string {
						if q := tr.phrases[r].text; q != "" && phraseNames(q)[0] == p {
							return p
						}
						return ""
					})[p] > 1 {
						drawn["c>1"]++
					}
				}
				if drew != "" {
					t.Errorf("%s %s: the label %q is %s (said: %s, ×%d)", name, v.name, l.S, drew, strings.Join(said(tr, w.lines), "; "), count)
					rows = append(rows, fmt.Sprintf("%s\t%s\t%s\t%q\t%d\t%d\t%s", name, drew, v.name, l.S, len(w.lines), count, strings.Join(said(tr, w.lines), "; ")))
				}
			}
		}
		t.Logf("%-15s %5d %5d | word: %5d %5d %5d %5d | drawn: %5d %5d %5d %5d %5d | overview drawn: %4d %4d %4d | 외 by ×N %3d %3d | dup %3d", name, lines, phrased,
			words["phrase"], words["via"], words["minority"], words["hidden"], drawn["phrase"], drawn["via"], drawn["minority"], drawn["hidden"], drawn["other"],
			over["phrase"], over["via"], over["minority"]+over["hidden"]+over["other"], drawn["counted"], drawn["c>1"], dups)
	}
	if p := os.Getenv("ABBD_LABEL_DUMP"); p != "" {
		os.WriteFile(p, []byte(strings.Join(rows, "\n")+"\n"), 0o644)
	}
}

func wordOf(t tree, rels []string) string {
	s, _ := word(t, [][]string{rels})
	return s
}
