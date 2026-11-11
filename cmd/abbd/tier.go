package main

import (
	"fmt"
	"strings"

	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/narrative"
	"github.com/whyjp/abbd/internal/topology"
)

// A module at or past largeUnits inside units or largeFiles files is large:
// its text needs more room to name what it holds, so each section may run
// to narrative.MaxSectionLarge and Key Concepts to bulletsLarge bullets.
const (
	largeUnits   = 12
	largeFiles   = 15
	bulletsSmall = 10
	bulletsLarge = 15
)

// tier is how long a module's text may run, by how much the module holds.
type tier struct {
	Large          bool
	Chars, Bullets int
	Units, Files   int
}

// smallTier is the room of a small module, and of any note.
var smallTier = tier{Chars: narrative.MaxSection, Bullets: bulletsSmall}

// tierOf is the tier of a module's text, counted from its inside units and
// the files its text is stamped against.
func (e env) tierOf(units []model.Unit, id model.ID) tier {
	files, _ := e.st.Read(units, id)
	t := smallTier
	t.Units, t.Files = len(topology.Explain(units, id).Inside), len(files)
	if t.Units >= largeUnits || t.Files >= largeFiles {
		t.Large, t.Chars, t.Bullets = true, narrative.MaxSectionLarge, bulletsLarge
	}
	return t
}

func (t tier) String() string {
	size := "small"
	if t.Large {
		size = "large"
	}
	return fmt.Sprintf("%s module (%d inside units, %d files; large from %d units or %d files): each section up to %s characters, Key Concepts up to %d bullets", size, t.Units, t.Files, largeUnits, largeFiles, thousands(t.Chars), t.Bullets)
}

func thousands(n int) string {
	if n < 1000 {
		return fmt.Sprint(n)
	}
	return fmt.Sprintf("%d,%03d", n/1000, n%1000)
}

// bullets counts the top-level bullets of a text's Key Concepts section.
func bullets(text string) int {
	n, in := 0, false
	for _, l := range strings.Split(text, "\n") {
		switch {
		case strings.HasPrefix(l, "## "):
			in = strings.TrimSpace(strings.TrimPrefix(l, "## ")) == "Key Concepts"
		case in && (strings.HasPrefix(l, "- ") || strings.HasPrefix(l, "* ")):
			n++
		}
	}
	return n
}
