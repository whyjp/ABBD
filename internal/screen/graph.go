package screen

import (
	"fmt"
	"sort"
	"strings"

	"github.com/whyjp/abbd/internal/model"
)

// Line is one live relation as graph.json lists it, with its origin and, for
// a reader's coupling, how it arises and the code it rests on. Phrase is
// what crosses it while its note is live, and PhraseOrigin whose words they
// are, which need not be the line's origin.
type Line struct {
	From, To, Kind, Origin string
	Via                    string `json:",omitempty"`
	When                   string `json:",omitempty"`
	Phrase                 string `json:",omitempty"`
	PhraseOrigin           string `json:",omitempty"`
	Quote                  string `json:",omitempty"`
}

// cite names the code a quote rests on by its file and its words. The
// blueprint leaves out the lines, which an edit above the quote moves
// though the quote still says the same, so such an edit rewrites no page.
// The store and abbd evidence keep the lines.
func cite(e model.Evidence) string {
	return fmt.Sprintf("%s %q", e.Path, e.Quote)
}

func lines(units []model.Unit) []Line {
	present := map[model.ID]bool{}
	for _, u := range units {
		present[u.ID] = !u.Absent
	}
	var out []Line
	for _, u := range model.Present(units) {
		for _, r := range u.Relations {
			if !present[r.To] || r.Dead {
				continue
			}
			l := Line{From: string(u.ID), To: string(r.To), Kind: string(r.Kind), Origin: string(r.Origin), Via: string(r.Via), When: r.When}
			if n, ok := livePhrase(u, r.Kind, r.To); ok {
				l.Phrase, l.PhraseOrigin = n.Text, string(n.Writer.Origin())
			}
			if len(r.Evidence) > 0 {
				l.Quote = cite(r.Evidence[0])
			}
			out = append(out, l)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		return strings.Join([]string{a.From, a.To, a.Kind}, " ") < strings.Join([]string{b.From, b.To, b.Kind}, " ")
	})
	return out
}
