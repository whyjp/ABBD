package screen

import (
	"cmp"
	"slices"
	"strings"

	"github.com/whyjp/abbd/internal/model"
)

// A line's phrase says what crosses it, and may name the symbols in the code
// that carry that (a function, a type, a channel). Those are listed in text
// beside the neighbor or dependent they belong to, so a reader who asks how
// one box reaches another finds the name to look for without opening the
// code. A relation whose phrase names no symbol adds nothing, so a page of a
// repository that records none is as it was.

// Carry is what one relation's phrase says and the symbols that carry it.
type Carry struct {
	Phrase  string
	Symbols []string
}

// carriesOf are the symbol-bearing phrases of the live relations from one unit
// to another by which the first depends on or consumes the second, in phrase
// order.
func (t tree) carriesOf(from, to model.ID) []Carry {
	var out []Carry
	for _, kind := range []model.RelationKind{model.DependsOn, model.Consumes} {
		if s := t.symbols[key(from, kind, to)]; len(s) > 0 {
			out = append(out, Carry{t.phrases[key(from, kind, to)].text, slices.Clone(s)})
		}
	}
	return sortedCarries(out)
}

// carriesBetween are the symbol-bearing phrases of the relations a view's lines
// stand for, by the pair of boxes of the view the relation joins: the unit at
// each end stands on the box that holds it, and a contract on the boxes that
// provide it. boxes are the view's boxes by ID.
func (o *overview) carriesBetween(boxes map[string]bool) map[[2]string][]Carry {
	t := o.tree
	boxOf := func(id model.ID) string {
		for _, c := range slices.Backward(t.chain(id)) {
			if boxes[string(c)] {
				return string(c)
			}
		}
		return ""
	}
	providers := map[model.ID][]model.ID{}
	for id, u := range t.units {
		for _, r := range u.Relations {
			if r.Kind == model.Provides && !r.Dead {
				providers[r.To] = append(providers[r.To], id)
			}
		}
	}
	out := map[[2]string][]Carry{}
	for k, symbols := range t.symbols {
		e, err := model.ParseEdge(k)
		if err != nil || e.Kind != model.DependsOn && e.Kind != model.Consumes {
			continue
		}
		from := boxOf(e.From)
		tos := []model.ID{e.To}
		if t.units[e.To].Kind.IsContract() {
			tos = providers[e.To]
		}
		for _, to := range tos {
			if b := boxOf(to); from != "" && b != "" && b != from {
				pair := [2]string{from, b}
				out[pair] = append(out[pair], Carry{t.phrases[k].text, slices.Clone(symbols)})
			}
		}
	}
	for pair := range out {
		out[pair] = sortedCarries(out[pair])
	}
	return out
}

// sortedCarries orders carries by phrase and then symbols, each once.
func sortedCarries(cs []Carry) []Carry {
	slices.SortFunc(cs, func(a, b Carry) int {
		return cmp.Or(strings.Compare(a.Phrase, b.Phrase), slices.Compare(a.Symbols, b.Symbols))
	})
	return slices.CompactFunc(cs, func(a, b Carry) bool { return a.Phrase == b.Phrase && slices.Equal(a.Symbols, b.Symbols) })
}
