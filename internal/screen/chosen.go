package screen

import (
	"slices"

	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/store"
)

// ZoneView is one zone of the overview: its name, the units it names, and
// why it is STALE, if it is.
type ZoneView struct {
	Name    string   `json:"name"`
	Units   []string `json:"units"`
	Reasons []string `json:"reasons,omitempty"`
}

// ChosenLine is one line the overview's reader chose, with its word, and
// what it draws: the kinds and vias of the live relations under it, and
// how many there are. A line none runs under is STALE and says why.
type ChosenLine struct {
	From      string   `json:"from"`
	To        string   `json:"to"`
	Word      string   `json:"word"`
	Kinds     []string `json:"kinds,omitempty"`
	Vias      []string `json:"vias,omitempty"`
	Relations int      `json:"relations"`
	Reasons   []string `json:"reasons,omitempty"`
}

// chosen is the stored overview as graph.json gives it, and the zone of
// each unit a zone holds, itself or through a unit that holds it. It is
// empty when there is no overview abbd can read.
type chosen struct {
	zones []ZoneView
	lines []ChosenLine
	zone  map[model.ID]string
}

func readOverview(in Input) chosen {
	o := in.Overview
	if !o.Known {
		return chosen{}
	}
	whole := map[string]map[int][]string{}
	for _, g := range o.Gaps(in.Units) {
		if whole[g.Part] == nil {
			whole[g.Part] = map[int][]string{}
		}
		for _, n := range g.Parts {
			whole[g.Part][n] = append(whole[g.Part][n], g.Text)
		}
	}
	reasons := func(part string, n int, why []string) []string {
		return slices.Concat(why, whole[part][n])
	}
	c := chosen{zone: map[model.ID]string{}}
	for i, z := range o.Zones {
		c.zones = append(c.zones, ZoneView{Name: z.Claim.Name, Units: idStrings(z.Claim.Units), Reasons: reasons("zone", i+1, z.Why(in.Units))})
	}
	for i, l := range o.Lines {
		cl := ChosenLine{From: string(l.Claim.From), To: string(l.Claim.To), Word: l.Claim.Word, Reasons: reasons("line", i+1, l.Why(in.Units))}
		for _, r := range l.Claim.Carries(in.Units) {
			cl.Relations++
			cl.Kinds = append(cl.Kinds, string(r.Kind))
			if r.Via != "" {
				cl.Vias = append(cl.Vias, string(r.Via))
			}
		}
		slices.Sort(cl.Kinds)
		slices.Sort(cl.Vias)
		cl.Kinds, cl.Vias = slices.Compact(cl.Kinds), slices.Compact(cl.Vias)
		c.lines = append(c.lines, cl)
	}
	for id, n := range model.Holders(store.ClaimsOf(o.Zones), in.Units) {
		c.zone[id] = o.Zones[n-1].Claim.Name
	}
	return c
}

func idStrings(us []model.ID) []string {
	out := make([]string, len(us))
	for i, u := range us {
		out[i] = string(u)
	}
	return out
}
