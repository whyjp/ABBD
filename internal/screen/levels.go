package screen

import (
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"

	"github.com/whyjp/abbd/internal/model"
)

// The bounds of a level. The overview shows the parts an engineer names
// when explaining the system, a module's page shows that module's parts,
// and either draws few enough lines that each can be followed.
const (
	overviewFewest = 4
	overviewMost   = 12
	insideFewest   = 2
	// linesPerBox is the density past which a view gets focus views
	// (focus.go), and past which its parts are too fine.
	linesPerBox = 2
	// denseFrom is the fewest boxes a view must draw before its density
	// says anything about its parts.
	denseFrom = 4
	// hubFrom is the fewest users that make a box a shared layer, when
	// they are also at least half of the view's other boxes.
	hubFrom = 3
	// mostHubs is the most shared layers one view has. When as many boxes
	// qualify as do not, none stands out as the layer the rest stand on.
	mostHubs = 2
)

// Bounds are the numbers a level is held to, which the abbd-setup skill
// states to the agent that chooses the units.
type Bounds struct {
	OverviewFewest, OverviewMost, PageFewest, PageMost, LinesPerBox, LeafFiles int
}

// LevelBounds are the bounds Levels holds every view to.
func LevelBounds() Bounds {
	return Bounds{overviewFewest, overviewMost, insideFewest, viewBudget, linesPerBox, leafFiles}
}

// Level is how one full view stands as a level of abstraction: the boxes
// and the lines it draws, and what to change in the units when the level is
// off. It never fails lint, because only a reader's choice of units fixes it.
type Level struct {
	View  string `json:"view"`
	Boxes int    `json:"boxes"`
	Lines int    `json:"lines"`
	// Hubs are the boxes most of the view uses, such as shared types or a
	// shared UI kit. The lines into them do not count toward the density.
	Hubs   []string `json:"hubs,omitempty"`
	Advice []string `json:"advice,omitempty"`
	// Unseen is what the view leaves out of sight because of the units: the
	// relations a module's page cannot draw, a page with code and no parts,
	// and lines that say only how they arise. Unlike Advice, the first
	// lint --record waits for it whatever the view's breaches.
	Unseen []string `json:"unseen,omitempty"`
	// Kept is why the repository keeps the view at this level, as
	// .abbd/lint-debt.yaml records it.
	Kept string `json:"kept,omitempty"`
}

// Off says the view has a level line: advice or something it leaves unseen.
func (l Level) Off() bool { return len(l.Advice)+len(l.Unseen) > 0 }

// leafFiles is the fewest files a top-level module must hold before a page
// with no parts leaves its code unseen: two parts of several files each.
const leafFiles = 4

// fewestPhrased is the fewest relations a view must draw before the share
// of them that carries a phrase says anything.
const fewestPhrased = 4

// where a full view stands: the overview, the page of a top-level module,
// or the page of a module inside another.
type where int

const (
	atOverview where = iota
	atTop
	atNested
)

// Levels reads the overview and every module's inside as levels. Focus
// views are cut from them and are left out, and so are features' pages,
// which draw the units a feature reaches rather than parts of a module.
func Levels(m Model) []Level {
	unit := map[string]UnitRow{}
	kids := map[string][]string{}
	for _, u := range m.Units {
		unit[u.ID] = u
		kids[u.Parent] = append(kids[u.Parent], u.ID)
	}
	said := map[string]bool{}
	for _, e := range m.Edges {
		said[model.Edge{From: model.ID(e.From), To: model.ID(e.To), Kind: model.RelationKind(e.Kind)}.String()] = e.Phrase != ""
	}
	ov := level("overview", m.Overview, atOverview)
	ov.Unseen = unphrased(m.Overview, said)
	out := []Level{ov}
	for _, ins := range m.Insides {
		if unit[ins.ID].Kind != "MOD" {
			continue
		}
		at := atTop
		if unit[ins.ID].Parent != "" {
			at = atNested
		}
		l := level(ins.ID+" "+ins.Name, ins.Drawing, at)
		l.Unseen = append(ownCode(m, unit, kids, ins.ID, at), unphrased(ins.Drawing, said)...)
		out = append(out, l)
	}
	return out
}

// ownCode says what a module's page leaves unseen of the module's own code,
// the code it holds outside every part. A page draws its parts and what
// meets them, so a relation of the module's own code has no box to start or
// end at there. A top-level module of several files with no parts draws an
// empty frame.
func ownCode(m Model, unit map[string]UnitRow, kids map[string][]string, id string, at where) []string {
	if len(kids[id]) == 0 {
		if n := len(files(unit[id].paths)); at == atTop && n >= leafFiles {
			return []string{fmt.Sprintf("the page draws no box, though the module holds %d files, so it shows neither what the module holds nor what meets it. Give it 2 to %d parts, each a responsibility of several files, by the part rule of abbd-setup step 1", n, viewBudget)}
		}
		return nil
	}
	var meets []string
	for _, e := range m.Edges {
		if e.From == id || e.To == id {
			meets = append(meets, unit[e.From].Name+" "+e.Kind+" "+unit[e.To].Name)
		}
	}
	if len(meets) == 0 {
		return nil
	}
	held := map[string]bool{}
	var below func(string)
	below = func(k string) {
		for _, c := range kids[k] {
			for _, p := range unit[c].paths {
				held[p] = true
			}
			below(c)
		}
	}
	below(id)
	all := append(slices.Clone(unit[id].paths), slices.Collect(maps.Keys(held))...)
	var own []string
	for _, p := range files(all) {
		if slices.Contains(unit[id].paths, p) && !held[p] && !heldUnder(p, held) {
			own = append(own, p)
		}
	}
	verb := "meet"
	if len(meets) == 1 {
		verb = "meets"
	}
	s := fmt.Sprintf("%s %s the module itself rather than one of its parts, so the page draws none of them, such as %s", plural(len(meets), "relation"), verb, strings.Join(meets[:min(len(meets), 3)], "; "))
	if len(own) > 0 {
		s += fmt.Sprintf(". Its files %s", strings.Join(own[:min(len(own), 4)], ", "))
		if len(own) > 4 {
			s += fmt.Sprintf(" and %d more", len(own)-4)
		}
		s += " sit in no part, so place each in the part it serves"
	}
	return []string{s + ". Have the script end each coupling at the part whose file holds its code, at both ends (abbd-setup step 3)"}
}

// files are the paths that name no directory of another: a module's
// directory anchor holds its file anchors and is no file of its own.
func files(paths []string) []string {
	var out []string
	for _, p := range paths {
		dir := false
		for _, q := range paths {
			dir = dir || strings.HasPrefix(q, p+"/")
		}
		if !dir && !slices.Contains(out, p) {
			out = append(out, p)
		}
	}
	slices.Sort(out)
	return out
}

// heldUnder says a directory some part anchors holds p.
func heldUnder(p string, held map[string]bool) bool {
	for q := range held {
		if strings.HasPrefix(p, q+"/") {
			return true
		}
	}
	return false
}

// unphrased says a view's lines mostly say only how they arise: fewer than
// half of the relations it draws carry a phrase of what crosses them.
func unphrased(d Drawing, said map[string]bool) []string {
	drawn := map[string]bool{}
	for _, w := range d.Wires {
		for _, r := range w.Rels {
			drawn[r] = true
		}
	}
	n := 0
	for r := range drawn {
		if said[r] {
			n++
		}
	}
	if len(drawn) < fewestPhrased || 2*n >= len(drawn) {
		return nil
	}
	return []string{fmt.Sprintf("%d of the %s the view draws carry no phrase, so their lines say at most how they arise, such as 임포트. Give each coupling a phrase that names what crosses it, as abbd-setup step 4 says", len(drawn)-n, plural(len(drawn), "relation"))}
}

func level(name string, d Drawing, at where) Level {
	l := Level{View: name}
	// The parts are the view's boxes. A frame's chips stand for what lies
	// outside the module, and a contract no drawn module provides stands
	// loose on the overview, so neither is a part.
	part := map[string]bool{}
	for _, b := range d.Boxes {
		if at != atOverview || b.Class == "design" {
			part[b.ID] = true
		}
	}
	l.Boxes = len(part)
	// A line joins two parts once, whichever way it runs, and one between
	// two parts that use each other is one line.
	lines := map[[2]string]bool{}
	users := map[string]map[string]bool{}
	for _, r := range d.Relations {
		from, to := r[0], r[1]
		if !part[from] || !part[to] || from == to {
			continue
		}
		lines[[2]string{min(from, to), max(from, to)}] = true
		if users[to] == nil {
			users[to] = map[string]bool{}
		}
		users[to][from] = true
	}
	l.Lines = len(lines)
	hub := hubs(d.Boxes, part, users)
	for _, b := range d.Boxes {
		if hub[b.ID] {
			l.Hubs = append(l.Hubs, b.Name)
		}
	}
	own := 0
	for p := range lines {
		if !(hub[p[1]] && users[p[1]][p[0]]) && !(hub[p[0]] && users[p[0]][p[1]]) {
			own++
		}
	}
	n := boxCount(l.Boxes)
	switch {
	case at == atOverview && l.Boxes < overviewFewest:
		l.Advice = append(l.Advice, fmt.Sprintf("the overview draws %s, fewer than %d, so its modules are folders rather than parts. Make the parts an engineer names the top-level modules, such as the modules under a framework's app/, src/ or lib/, or the responsibilities of one root package", n, overviewFewest))
	case at == atOverview && l.Boxes > overviewMost:
		l.Advice = append(l.Advice, fmt.Sprintf("the overview draws %s, more than %d. Place each helper inside the module it serves, and group the rest by the responsibility they share", n, overviewMost))
	case at == atTop && l.Boxes == 1:
		l.Advice = append(l.Advice, "the page draws 1 box inside, which repeats the module. Fold that unit into the module, so the module stands as a leaf of the overview, or give it a second part")
	case at == atNested && l.Boxes < insideFewest:
		l.Advice = append(l.Advice, fmt.Sprintf("the page draws %s inside, fewer than %d, so the module is too small for a page. Fold its files into the module around it, so it is no module of its own, or give it two or more parts, each a responsibility of several files or the implementers of an interface it holds", n, insideFewest))
	case at != atOverview && l.Boxes > viewBudget:
		l.Advice = append(l.Advice, fmt.Sprintf("the page draws %s inside, more than %d. Split it by the responsibilities its parts share", n, viewBudget))
	}
	if l.Boxes >= denseFrom && own > linesPerBox*l.Boxes {
		besides := ""
		if len(l.Hubs) > 0 {
			besides = " besides those into the shared " + strings.Join(l.Hubs, ", ")
		}
		l.Advice = append(l.Advice, fmt.Sprintf("the view draws %d lines between %s%s, more than %d a box. Merge parts that change together into one responsibility, and make what most parts use, such as shared types, helpers or UI, one shared module, whose incoming lines do not count here", own, n, besides, linesPerBox))
	}
	return l
}

// hubs are the shared layers of a view: the parts used by at least hubFrom
// other parts and half of them, the most used first and at most mostHubs.
// When as many parts qualify as do not, none is a shared layer.
func hubs(boxes []Box, part map[string]bool, users map[string]map[string]bool) map[string]bool {
	var found []string
	for _, b := range boxes {
		if n := len(users[b.ID]); part[b.ID] && n >= hubFrom && 2*n >= len(part)-1 {
			found = append(found, b.ID)
		}
	}
	if 2*len(found) >= len(part) {
		return nil
	}
	sort.SliceStable(found, func(i, j int) bool { return len(users[found[i]]) > len(users[found[j]]) })
	hub := map[string]bool{}
	for _, id := range found[:min(len(found), mostHubs)] {
		hub[id] = true
	}
	return hub
}

func boxCount(n int) string {
	if n == 1 {
		return "1 box"
	}
	return fmt.Sprintf("%d boxes", n)
}
