package screen

import (
	"slices"
	"sort"

	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/topology"
)

// Line is one relation as graph.json lists it, with what it means for the
// design: kept or lost from the intended topology, new across the boundary,
// below the line, or a guess with how it arises and the code it rests on.
type Line struct {
	From, To, Kind, Class, Title string
	Via                          string `json:",omitempty"`
	Quote                        string `json:",omitempty"`
}

func lines(units []model.Unit, in *topology.Intended, items []topology.Item) []Line {
	present := map[model.ID]bool{}
	for _, u := range units {
		present[u.ID] = !u.Absent
	}
	effect := map[string]topology.Effect{}
	for _, it := range items {
		effect[it.Key()] = it.Effect
	}
	var intended []topology.Edge
	if in != nil {
		intended = in.Relations
	}
	actual := map[topology.Edge]bool{}
	var out []Line
	add := func(e topology.Edge, class, title string) *Line {
		out = append(out, Line{From: string(e.From), To: string(e.To), Kind: string(e.Kind), Class: class, Title: e.String() + ": " + title})
		return &out[len(out)-1]
	}
	for _, u := range units {
		if u.Absent {
			continue
		}
		for _, r := range u.Relations {
			e := topology.Edge{From: u.ID, To: r.To, Kind: r.Kind}
			switch {
			case !present[r.To] || r.Dead:
			case r.Counts():
				actual[e] = true
			case r.Guess() || (r.Origin == model.Adopted && r.Confirmation != model.Rejected):
				title := "추정, 미확인"
				if r.Via != "" {
					title += ", via " + string(r.Via)
				}
				if len(r.Evidence) > 0 {
					title += ": " + r.Evidence[0].String()
				}
				l := add(e, "guess", title)
				if len(r.Evidence) > 0 {
					l.Via, l.Quote = string(r.Via), r.Evidence[0].String()
				}
			}
		}
	}
	for _, e := range intended {
		if actual[e] {
			add(e, "kept", "의도")
		} else {
			add(e, "removed", "의도했으나 코드에서 사라짐")
		}
	}
	var fresh []topology.Edge
	for e := range actual {
		if !slices.Contains(intended, e) {
			fresh = append(fresh, e)
		}
	}
	sort.Slice(fresh, func(i, j int) bool { return fresh[i].String() < fresh[j].String() })
	for _, e := range fresh {
		switch effect["new-relation:"+e.String()] {
		case topology.Blocked:
			add(e, "blocked", "새 결합, 의도한 경계를 넘음")
		case topology.Realign:
			add(e, "realign", "새 결합, 설계 단위에 닿음")
		default:
			add(e, "below", "선 아래")
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Class != out[j].Class {
			return out[i].Class < out[j].Class
		}
		return out[i].Title < out[j].Title
	})
	return out
}
