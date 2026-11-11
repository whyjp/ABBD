package screen

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/whyjp/abbd/internal/store"
)

// rest is a list or count a label adds after a name.
var rest = regexp.MustCompile(` (등|외 \d+)$`)

// misstated says what is wrong with label s, a phrase when phrase is set,
// on wire w, or "" when s is true of the lines it speaks for:
//   - "hidden": s is no phrase although at least half of them carry one
//     whose first name fits;
//   - "listed": s gives more than one name, or adds 등 or 외 N;
//   - "unknown": s is not the first name of a phrase any of them carries;
//   - "minority": s names a phrase, or a via word, fewer than half of them
//     carry.
func misstated(t tree, w Wire, s string, phrase bool) string {
	us := units(w.lines)
	n := carried(us, func(r string) string { return t.phrases[r].text })
	if !phrase {
		for p, c := range n {
			if 2*c >= len(us) && textWidth(phraseNames(p)[0], labelPx) <= labelMax {
				return "hidden"
			}
		}
		if s != "" && 2*carried(us, func(r string) string { return t.vias[r].Word() })[s] < len(us) {
			return "minority"
		}
		return ""
	}
	if rest.MatchString(s) || len(phraseNames(s)) > 1 {
		return "listed"
	}
	c := -1
	for p, k := range n {
		if phraseNames(p)[0] == s {
			c = max(c, k)
		}
	}
	switch {
	case c < 0:
		return "unknown"
	case 2*c < len(us):
		return "minority"
	}
	return ""
}

// TestLabelAudit checks, over every line of every view of each fixture, of
// this repository's own .abbd, whose couplings carry phrases while the
// fixtures' carry none, and of each root in ABBD_LABEL_ROOTS, the word
// each line says (see misstated) against the lines it speaks for, and that
// no line holds a relation twice. It fails on any misstated word, drawn or
// not, and on a doubled relation. It logs how many words, and
// labels drawn, name a phrase or a via, and with ABBD_LABEL_DUMP set it
// writes one row per misstated line there.
func TestLabelAudit(t *testing.T) {
	t.Parallel()
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
	t.Logf("%-15s %5s %5s | word: %5s %5s %5s | drawn: %5s %5s %5s | overview drawn: %4s %4s %4s | dup %3s", "root", "lines", "phrsd", "phr", "via", "bad", "phr", "via", "bad", "phr", "via", "bad", "n")
	for _, name := range keys {
		loaded, err := store.Open(roots[name]).LoadUnits()
		if err != nil {
			t.Fatal(err)
		}
		tr := newTree(loaded)
		m := rootModel(t, name, roots[name], loaded)
		var lines, phrased, dups int
		words, drawn, over := map[string]int{}, map[string]int{}, map[string]int{}
		for _, v := range views(m) {
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
				kind := func(s string, phrase bool) string {
					switch {
					case phrase:
						return "phrase"
					case s != "":
						return "via"
					}
					return ""
				}
				tally := func(m map[string]int, s string, phrase bool) string {
					bad := misstated(tr, w, s, phrase)
					if k := kind(s, phrase); k != "" {
						m[k]++
					}
					if bad != "" {
						m["bad"]++
					}
					return bad
				}
				s, ph := word(tr, w.lines)
				if bad := tally(words, s, ph); bad != "" {
					t.Errorf("%s %s: the word %q is %s (said: %s)", name, v.name, s, bad, strings.Join(said(tr, w.lines), "; "))
					rows = append(rows, fmt.Sprintf("%s	%s	%s	%q	%d	%s", name, bad, v.name, s, len(w.lines), strings.Join(said(tr, w.lines), "; ")))
				}
				if w.Label == nil {
					continue
				}
				if bad := tally(drawn, w.Label.S, w.Label.Said); bad != "" {
					t.Errorf("%s %s: the label %q is %s (said: %s)", name, v.name, w.Label.S, bad, strings.Join(said(tr, w.lines), "; "))
				}
				if v.name == "overview" {
					tally(over, w.Label.S, w.Label.Said)
				}
			}
		}
		t.Logf("%-15s %5d %5d | word: %5d %5d %5d | drawn: %5d %5d %5d | overview drawn: %4d %4d %4d | dup %3d", name, lines, phrased,
			words["phrase"], words["via"], words["bad"], drawn["phrase"], drawn["via"], drawn["bad"], over["phrase"], over["via"], over["bad"], dups)
	}
	if p := os.Getenv("ABBD_LABEL_DUMP"); p != "" {
		os.WriteFile(p, []byte(strings.Join(rows, "\n")+"\n"), 0o644)
	}
}

func wordOf(t tree, rels []string) string {
	s, _ := word(t, [][]string{rels})
	return s
}

// A phrase is said once along one track, however far its ink runs: on this
// repository's overview no two labels of one phrase stand on wires of one
// bundle or on touching ink that leaves a place in common, as the three
// Compare revisions along one track did, each just past repeatNear.
func TestAPhraseIsSaidOncePerTrack(t *testing.T) {
	t.Parallel()
	in, err := Load(store.Open(filepath.Join("..", "..")))
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range views(Build(in)) {
		d := v.d
		track := tracks(d.Wires, inkTouches(d))
		at := map[[2]any]Label{}
		for i, w := range d.Wires {
			if l := w.Label; l != nil && l.Said {
				k := [2]any{l.S, track(i)}
				if o, ok := at[k]; ok {
					t.Errorf("%s: %q said at %d,%d and again at %d,%d on one track", v.name, l.S, o.X, o.Y, l.X, l.Y)
				}
				at[k] = *l
			}
		}
	}
}
