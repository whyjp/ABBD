package screen

import (
	"bytes"
	"fmt"
	"github.com/whyjp/abbd/internal/store"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
)

func traced(t *testing.T, d Drawing) Trace {
	t.Helper()
	var b bytes.Buffer
	if err := site.ExecuteTemplate(&b, "drawing", d); err != nil {
		t.Fatal(err)
	}
	s, err := readSVG(b.String())
	if err != nil {
		t.Fatal(err)
	}
	return trace(s, d.Relations)
}

// Every relation a drawing stands for can be traced along its ink. With
// ABBD_METRICS set to a file, the path measures of every view go there as a
// table.
func TestTraceEveryRealDrawing(t *testing.T) {
	t.Parallel()
	var rows []string
	for _, name := range fixtures {
		m := fixture(t, name)
		for _, v := range views(m) {
			got := traced(t, v.d)
			ms := measured(t, v.d)
			if got.Lost > 0 {
				t.Errorf("%s %s: %d relations no ink carries", name, v.name, got.Lost)
			}
			if got.UnmarkedT > 0 {
				t.Errorf("%s %s: rule 17: %d splits or merges without a dot", name, v.name, got.UnmarkedT)
			}
			if got.SizeSpread > maxSpread {
				t.Errorf("%s %s: rule 21: the tallest box is %.2f times the median, limit %.1f", name, v.name, got.SizeSpread, maxSpread)
			}
			t.Logf("%s %s: %s", name, v.name, got)
			logJogs(t, got)
			rows = append(rows, fmt.Sprintf("%s\t%s\t%d\t%d\t%d\t%.1f\t%d\t%d\t%.1f\t%d\t%d\t%d\t%.2f\t%.3f\t%d\t%d\t%d\t%d\t%d\t%d\t%d\t%d\t%d\t%d\t%d", name, v.name, got.Relations, got.Lost, got.Phantom, median(got.Bends), maxOf(got.Bends), sum(got.Jogs), median(got.Crossings), maxOf(got.Crossings), got.UnmarkedT, got.CountAmbiguity, got.SizeSpread, got.Beside, ms.Crossings, ms.Bends, ms.Back, ms.BackLength, ms.Length, ms.MaxDetour, ms.Braid, over2(got.Bends), ms.Width, sum(got.Bends), steps(got)))
		}
	}
	if f := os.Getenv("ABBD_METRICS"); f != "" {
		head := "fixture\tview\trelations\tlost\tphantom\tbends_med\tbends_max\tjogs\tcross_med\tcross_max\tunmarked_t\tcount_amb\tsize_spread\tbeside\tcrossings\tbends\tback\tback_ink\tink\tmax_detour\tbraid\tpaths_over2\twidth\ttraced_bends\tsteps\n"
		if err := os.WriteFile(f, []byte(head+strings.Join(rows, "\n")+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// With ABBD_JOGS set, logJogs lists every traced path that jogs, and every
// relation no ink carries and pair of places the ink joins wrongly.
func logJogs(t *testing.T, got Trace) {
	if os.Getenv("ABBD_JOGS") == "" {
		return
	}
	ways := slices.Clone(got.Ways)
	sort.Slice(ways, func(i, j int) bool { return ways[i].from+ways[i].to < ways[j].from+ways[j].to })
	for _, w := range ways {
		if n := jogs(w.pts); n > 0 {
			t.Logf("    jogs %d: %s to %s at %s", n, w.from, w.to, pointList(w.pts))
		}
	}
	for _, r := range got.Missing {
		t.Logf("    lost: %s to %s", r[0], r[1])
	}
	for _, w := range got.Phantoms {
		t.Logf("    phantom: %s to %s at %s", w.from, w.to, pointList(w.pts))
	}
}

// The hand-laid reference drawing of this repository's overview,
// eval/render-r1/reference-self.svg, lays out the same boxes and relations
// the way a person would in draw.io. The path measures must rate it at
// least as well as the renderer does, or they measure the wrong thing.
func TestReferenceRanksFirst(t *testing.T) {
	t.Parallel()
	b, err := os.ReadFile(filepath.Join("..", "..", "eval", "render-r1", "reference-self.svg"))
	if err != nil {
		t.Fatal(err)
	}
	s, err := readSVG(string(b))
	if err != nil {
		t.Fatal(err)
	}
	m := fixture(t, "self")
	ref, got := trace(s, m.Overview.Relations), traced(t, m.Overview)
	t.Logf("reference: %s", ref)
	t.Logf("renderer:  %s", got)
	if ref.Lost > 0 || ref.Relations != got.Relations {
		t.Fatalf("the reference draws %d of %d relations, %d lost", ref.Relations, got.Relations, ref.Lost)
	}
	for _, c := range []struct {
		name     string
		ref, got float64
	}{
		{"phantom", float64(ref.Phantom), float64(got.Phantom)},
		{"bends median", median(ref.Bends), median(got.Bends)},
		{"bends max", float64(maxOf(ref.Bends)), float64(maxOf(got.Bends))},
		{"path crossings median", median(ref.Crossings), median(got.Crossings)},
		{"unmarked T", float64(ref.UnmarkedT), float64(got.UnmarkedT)},
		{"count ambiguity", float64(ref.CountAmbiguity), float64(got.CountAmbiguity)},
		{"size spread", ref.SizeSpread, got.SizeSpread},
		{"beside", ref.Beside, got.Beside},
	} {
		if c.ref > c.got {
			t.Errorf("%s rates the renderer (%.2f) above the hand-laid reference (%.2f)", c.name, c.got, c.ref)
		}
	}
	// Roads take out every jog the reference draws, so the renderer now
	// draws fewer; it must still draw no more.
	if r, g := sum(ref.Jogs), sum(got.Jogs); g > r {
		t.Errorf("jogs: the renderer draws %d, the hand-laid reference %d", g, r)
	}
	// The renderer's busiest path crosses fewer others than the reference's
	// 6, so the most crossings are pinned at what it draws now.
	if g := maxOf(got.Crossings); g > 4 {
		t.Errorf("path crossings max: the renderer's busiest path crosses %d others, pinned at 4", g)
	}
}

// With ABBD_TRACE_STORE set to a repository's root, the path
// measures of its views are logged, for a state the fixtures do not hold.
func TestTraceStore(t *testing.T) {
	dir := os.Getenv("ABBD_TRACE_STORE")
	if dir == "" {
		t.Skip("ABBD_TRACE_STORE is not set")
	}
	units, err := store.Open(dir).LoadUnits()
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range views(Build(Input{Units: units})) {
		got := traced(t, v.d)
		t.Logf("%s: %s", v.name, got)
		logJogs(t, got)
	}
}

// sketch reads a hand-written drawing: boxes as "id x y w h", then the
// rest of the SVG body.
func sketch(t *testing.T, boxes []string, body string) sheet {
	t.Helper()
	var b strings.Builder
	b.WriteString(`<svg xmlns="http://www.w3.org/2000/svg" width="200" height="200">`)
	for _, bx := range boxes {
		var id string
		var x, y, w, h int
		fmt.Sscan(bx, &id, &x, &y, &w, &h)
		fmt.Fprintf(&b, `<g class="box design" data-id="%s"><rect x="%d" y="%d" width="%d" height="%d"/></g>`, id, x, y, w, h)
	}
	b.WriteString(body + `</svg>`)
	s, err := readSVG(b.String())
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// A relation whose end the drawing has no shape for is lost, not left
// uncounted.
func TestRelationWithoutItsShapeIsLost(t *testing.T) {
	s := sketch(t, []string{"A 0 0 40 20"}, ``)
	got := trace(s, [][2]string{{"A", "port|gone"}})
	if got.Relations != 1 || got.Lost != 1 {
		t.Fatalf("relations %d, lost %d; want 1 and 1", got.Relations, got.Lost)
	}
}

// One chip that stands for several frame ports carries each port's key, so
// a line to it traces as the relation it draws.
func TestSharedChipTracesEachKey(t *testing.T) {
	s := sketch(t, []string{"A 0 0 40 20", "B 100 0 40 20"},
		`<g class="port other entry" data-id="port|x port|y"><rect x="40" y="150" width="40" height="17"/></g>`+
			`<path class="w bypass" d="M20 20 V160 H40"/><path class="w bypass" d="M120 20 V160 H80"/>`)
	got := trace(s, [][2]string{{"A", "port|x"}, {"B", "port|y"}})
	if got.Relations != 2 || got.Lost != 0 || got.Phantom != 0 {
		t.Fatalf("%s; want 2 relations, none lost or phantom", got)
	}
}

// Two lines of one colour that cross where a dot of another colour sits do
// not join: the dot marks a split of its own colour's ink only.
func TestDotJoinsOnlyItsOwnColour(t *testing.T) {
	s := sketch(t, []string{"T 40 0 20 20", "B 40 180 20 20", "L 0 90 20 20", "R 180 90 20 20"},
		`<path class="w bypass" d="M50 20 V180"/><path class="w bypass" d="M20 100 H180"/>`+
			`<circle class="junction contract" cx="50" cy="100" r="3"/>`)
	got := trace(s, [][2]string{{"T", "B"}, {"L", "R"}})
	if got.Phantom != 0 || got.Lost != 0 {
		t.Fatalf("%s; want no phantom pair through the other colour's dot", got)
	}
}

// A four-way join marked only by a dot of another colour is unmarked.
func TestSplitNeedsADotOfItsColour(t *testing.T) {
	s := sketch(t, []string{"T 40 0 20 20", "B 40 180 20 20", "L 0 90 20 20", "R 180 90 20 20"},
		`<path class="w bypass" d="M50 20 V180"/><path class="w bypass joins" d="M20 100 H50"/>`+
			`<path class="w bypass joins" d="M180 100 H50"/><circle class="junction contract" cx="50" cy="100" r="3"/>`)
	if got := trace(s, nil); got.UnmarkedT != 1 {
		t.Fatalf("unmarked T %d; want 1", got.UnmarkedT)
	}
}

// A T says by its shape that one line ends on the other, so it needs no
// dot.
func TestATNeedsNoDot(t *testing.T) {
	s := sketch(t, []string{"T 40 0 20 20", "B 40 180 20 20", "L 0 90 20 20"},
		`<path class="w bypass" d="M50 20 V180"/><path class="w bypass joins" d="M20 100 H50"/>`)
	if got := trace(s, nil); got.UnmarkedT != 0 || len(got.Splits) != 1 {
		t.Fatalf("unmarked T %d, splits %v; want 0 and the one split", got.UnmarkedT, got.Splits)
	}
}

// Two undotted four-way joins a few pixels apart are two, not one.
func TestCloseSplitsCountApart(t *testing.T) {
	s := sketch(t, []string{"T 40 0 20 20", "B 40 180 20 20", "L 0 90 20 20", "R 180 90 20 20"},
		`<path class="w bypass" d="M50 20 V180"/><path class="w bypass joins" d="M20 100 H50"/>`+
			`<path class="w bypass joins" d="M180 100 H50"/><path class="w bypass joins" d="M20 103 H50"/>`+
			`<path class="w bypass joins" d="M180 103 H50"/>`)
	if got := trace(s, nil); got.UnmarkedT != 2 {
		t.Fatalf("unmarked T %d; want 2", got.UnmarkedT)
	}
}

// The checker expects the relations a view stands for, whatever the
// routing drew.
func TestExpectedRelationsDoNotComeFromTheWires(t *testing.T) {
	d := Drawing{Width: 200, Height: 200, Relations: [][2]string{{"A", "B"}},
		Boxes: []Box{{ID: "A", W: 40, H: 20}, {ID: "B", X: 100, W: 40, H: 20}}}
	if got := traced(t, d); got.Relations != 1 || got.Lost != 1 {
		t.Fatalf("%s; want the one undrawn relation lost", got)
	}
}

// Every place a view's relations join is a shape on its drawing, a frame
// port that shares its chip with others included.
func TestEveryRelationEndIsDrawn(t *testing.T) {
	t.Parallel()
	for _, name := range fixtures {
		for _, v := range views(fixture(t, name)) {
			var b bytes.Buffer
			if err := site.ExecuteTemplate(&b, "drawing", v.d); err != nil {
				t.Fatal(err)
			}
			s, err := readSVG(b.String())
			if err != nil {
				t.Fatal(err)
			}
			drawn := map[string]bool{}
			for _, sh := range s.shapes {
				for _, id := range strings.Fields(sh.id) {
					drawn[id] = true
				}
			}
			missing := 0
			for _, r := range v.d.Relations {
				if !drawn[r[0]] || !drawn[r[1]] {
					missing++
				}
			}
			if missing > 0 {
				t.Errorf("%s %s: %d relations end on no shape", name, v.name, missing)
			}
		}
	}
}

func TestDoglegs(t *testing.T) {
	for _, c := range []struct {
		shape string
		pts   []fpt
		want  int
	}{
		{"straight", []fpt{{0, 0}, {0, 100}}, 0},
		{"straight through a repeated point", []fpt{{0, 0}, {0, 20}, {0, 20}, {0, 100}}, 0},
		{"L with a 20px lead", []fpt{{0, 0}, {0, 20}, {100, 20}}, 1},
		{"L with 50px legs", []fpt{{0, 0}, {0, 50}, {50, 50}}, 0},
		{"Z with a 10px middle", []fpt{{0, 0}, {0, 50}, {10, 50}, {10, 100}}, 1},
	} {
		if got := doglegs(c.pts); got != c.want {
			t.Errorf("%s: %d doglegs, want %d", c.shape, got, c.want)
		}
	}
}
