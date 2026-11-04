package screen

import (
	"github.com/whyjp/abbd/internal/model"
)

// A stack answers what the layers of a part of the code are: bands from the
// top down, each a named layer with a one-line role and the stored units it
// holds. The page draws it as an HTML list, as it draws a flow, so every
// line wraps and the bands read in their order. A stack stores no unit and
// no relation. Each band names stored units, a click on one opens the inside
// that shows it, and a stored relation by which a lower band uses a higher
// one is drawn in the lower band as a breach.

// StackView is one stack as the page draws it. Badge is the weakest origin
// of its layers and Source the one every layer shares, if any. Stale counts
// its STALE layers, Reasons say why its layers taken together are not true
// of the state, and Breaches counts the relations that run up it.
type StackView struct {
	ID, Key, Question, Badge, Source string
	Caption                          []Block  `json:",omitempty"`
	Reasons                          []string `json:",omitempty"`
	Layers, Stale, Breaches          int
	Bands                            []Band
	held                             map[model.ID]int
}

// Band is one layer. N is its place from the top and ID its anchor. Badge
// and Source are set only where they differ from the stack's. Up are the
// breaches that leave this band for a higher one.
type Band struct {
	N                             int
	ID, Name, Role, Badge, Source string
	Units                         []UnitChip
	Up                            []BreachChip `json:",omitempty"`
	Quotes                        []string     `json:",omitempty"`
	Reasons                       []string     `json:",omitempty"`
	Stale                         bool         `json:",omitempty"`
}

// UnitChip is a unit a band names, linked to the inside that shows it.
type UnitChip struct{ ID, Name, Kind, Href string }

// BreachChip is a stored relation by which a unit this band holds uses a
// unit band Upper holds, named by its ends and the relation it rides.
type BreachChip struct {
	From, To, FromName, ToName, Verb, Via, Origin, Href, UpperName string
	Upper                                                          int
}

// StackRef is a stack as a module's inside lists it: the layers that hold
// the module's units.
type StackRef struct {
	ID, Question string
	Layers       []int
}

// stacks derives each stored stack's view, in key order.
func (o *overview) stacks(in Input) []StackView {
	var out []StackView
	for _, f := range in.Figures.Stacks {
		if !f.Known {
			continue
		}
		layers := f.Claims()
		breaches := model.Breaches(layers, in.Units)
		v := StackView{ID: figureID[model.Layer](f.Key), Key: string(f.Key), Question: f.Question, Badge: Badge(f.Origin()), Source: sharedSource(f), Caption: blocks(f.Caption),
			Layers: len(f.Parts), Breaches: len(breaches), held: model.Holders(layers, in.Units)}
		for _, g := range f.Gaps(in.Units) {
			v.Reasons = append(v.Reasons, g.Text)
		}
		for i, p := range f.Parts {
			b := Band{N: i + 1, ID: partID[model.Layer](f.Key, i+1), Name: p.Claim.Name, Role: p.Claim.Role, Reasons: p.Why(in.Units)}
			b.Badge, b.Source = partMarks(p.Stamp, v.Badge, v.Source)
			b.Stale = len(b.Reasons) > 0
			for _, id := range p.Claim.Units {
				b.Units = append(b.Units, UnitChip{ID: string(id), Name: o.unitName(id), Kind: kindWord(o.tree.units[id].Kind), Href: o.href(id)})
			}
			for _, e := range p.Claim.Evidence {
				b.Quotes = append(b.Quotes, cite(e))
			}
			for _, br := range breaches {
				if br.Lower == b.N {
					b.Up = append(b.Up, BreachChip{From: string(br.From), To: string(br.To), FromName: o.unitName(br.From), ToName: o.unitName(br.To),
						Verb: verbs[br.Kind], Via: string(br.Via), Origin: Badge(br.Origin), Href: o.href(br.To), Upper: br.Upper, UpperName: layers[br.Upper-1].Name})
				}
			}
			if b.Stale {
				v.Stale++
			}
			v.Bands = append(v.Bands, b)
		}
		out = append(out, v)
	}
	return out
}
