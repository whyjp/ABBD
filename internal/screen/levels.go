package screen

import "fmt"

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
)

// Level is how one full view stands as a level of abstraction: the boxes
// and the lines it draws, and what to change in the units when the level is
// off. It never fails lint, because only a reader's choice of units fixes it.
type Level struct {
	View   string   `json:"view"`
	Boxes  int      `json:"boxes"`
	Lines  int      `json:"lines"`
	Advice []string `json:"advice,omitempty"`
}

// Levels reads the overview and every module's inside as levels. Focus
// views are cut from them and are left out.
func Levels(m Model) []Level {
	out := []Level{level("overview", m.Overview, true)}
	for _, ins := range m.Insides {
		out = append(out, level(ins.ID+" "+ins.Name, ins.Drawing, false))
	}
	return out
}

func level(name string, d Drawing, overview bool) Level {
	l := Level{View: name}
	for _, b := range d.Boxes {
		// A contract no drawn module provides stands loose on the
		// overview. It is not one of the system's parts.
		if !overview || b.Class == "design" {
			l.Boxes++
		}
	}
	pairs := map[[2]string]bool{}
	for _, r := range d.Relations {
		pairs[r] = true
	}
	l.Lines = len(pairs)
	n := boxCount(l.Boxes)
	switch {
	case overview && l.Boxes < overviewFewest:
		l.Advice = append(l.Advice, fmt.Sprintf("the overview draws %s, fewer than %d, so its modules are folders rather than parts. Make the parts an engineer names the top-level modules, such as the modules under a framework's app/, src/ or lib/, or the responsibilities of one root package", n, overviewFewest))
	case overview && l.Boxes > overviewMost:
		l.Advice = append(l.Advice, fmt.Sprintf("the overview draws %s, more than %d. Place each helper inside the module it serves, and group the rest only along a grouping the project shows", n, overviewMost))
	case !overview && l.Boxes < insideFewest:
		l.Advice = append(l.Advice, fmt.Sprintf("the page draws %s inside, fewer than %d. Give the module two or more parts as units with it as parent, each a responsibility of several files or the implementers of an interface it holds, or make it a unit inside the module that uses it", n, insideFewest))
	case !overview && l.Boxes > viewBudget:
		l.Advice = append(l.Advice, fmt.Sprintf("the page draws %s inside, more than %d. Split it along a grouping the project shows", n, viewBudget))
	}
	if l.Boxes >= denseFrom && l.Lines > linesPerBox*l.Boxes {
		l.Advice = append(l.Advice, fmt.Sprintf("the view draws %d lines between %s, more than %d a box, so its parts are too fine. Make each part a responsibility of several files that change together, never one file, so the lines between parts are the few that matter", l.Lines, n, linesPerBox))
	}
	return l
}

func boxCount(n int) string {
	if n == 1 {
		return "1 box"
	}
	return fmt.Sprintf("%d boxes", n)
}
