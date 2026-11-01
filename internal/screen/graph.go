package screen

import (
	"sort"
	"strings"

	"github.com/whyjp/abbd/internal/model"
)

// Line is one live relation as graph.json lists it, with its origin and, for
// a reader's coupling, how it arises and the code it rests on.
type Line struct {
	From, To, Kind, Origin string
	Via                    string `json:",omitempty"`
	Quote                  string `json:",omitempty"`
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
			l := Line{From: string(u.ID), To: string(r.To), Kind: string(r.Kind), Origin: string(r.Origin), Via: string(r.Via)}
			if len(r.Evidence) > 0 {
				l.Quote = r.Evidence[0].String()
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
