package screen

import (
	"slices"

	"github.com/whyjp/abbd/internal/model"
)

// The canonical method words every line (C4, the exemplars). Which lines
// say a word at rest, and where each word comes from, is a diagnostic lint
// reports and never gates on: the overview record's chosen lines give
// their own word to the line between their boxes, a relation's phrase
// gives the word a reader wrote on it, and with neither the line's via
// names it when it is no plain call. So a repository with no phrases and
// no record leaves most lines unsaid, and the fill shows what the record's
// words add.

// wordFrom is where a line's word at rest comes from.
type wordFrom int

const (
	fromVia wordFrom = iota
	fromPhrase
	fromRecord
)

// WordFill is how many of a view's routes a reader finds a word on at rest,
// and from where each word comes, a route taking the word of the first
// labelled wire that speaks for it by record, then phrase, then via. On the
// overview, RecordPairs are the pairs of boxes the stored record gives a
// word, and RecordSaid those whose word labels a drawn route.
type WordFill struct {
	Routes      int `json:"routes"`
	Labelled    int `json:"labelled"`
	Record      int `json:"record"`
	Phrase      int `json:"phrase"`
	Via         int `json:"via"`
	RecordPairs int `json:"record_pairs,omitempty"`
	RecordSaid  int `json:"record_said,omitempty"`
}

// Add sums two fills, as lint's totals do.
func (f WordFill) Add(o WordFill) WordFill {
	return WordFill{f.Routes + o.Routes, f.Labelled + o.Labelled, f.Record + o.Record, f.Phrase + o.Phrase, f.Via + o.Via, f.RecordPairs + o.RecordPairs, f.RecordSaid + o.RecordSaid}
}

// wordFill reads d's words at rest: a route is labelled as labelled reads
// it, and the record's pairs from the words the drawing was given.
func wordFill(d Drawing) WordFill {
	f := WordFill{Routes: len(d.ways)}
	speaks := func(w Wire, k int, r Route) bool {
		return w.Label != nil && (slices.Contains(w.rides, k) || slices.ContainsFunc(w.lines, func(l []string) bool {
			return slices.ContainsFunc(l, func(rel string) bool { return slices.Contains(r.rels, rel) })
		}))
	}
	for k, r := range d.ways {
		best := -1
		for _, w := range d.Wires {
			if speaks(w, k, r) {
				best = max(best, int(w.from))
			}
		}
		switch wordFrom(best) {
		case fromRecord:
			f.Record++
		case fromPhrase:
			f.Phrase++
		case fromVia:
			f.Via++
		}
		if best >= 0 {
			f.Labelled++
		}
	}
	f.RecordPairs = len(d.said)
	said := map[pair]bool{}
	for _, w := range d.Wires {
		if w.from == fromRecord && w.Label != nil && len(w.ends) == 2 {
			said[pairOf(model.ID(w.ends[0]), model.ID(w.ends[1]))] = true
		}
	}
	f.RecordSaid = len(said)
	return f
}
