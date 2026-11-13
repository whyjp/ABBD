package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/topology"
)

// neighbours is how many units before or after a position names before it
// counts the rest.
const neighbours = 3

// flowWords say a position in the project's language.
type flowWords struct {
	flow, data, before, after, more string
	steps                           func(ns []string, of int) string
	inside                          func(n int, unit string) string
}

var flowLanguages = map[string]flowWords{
	"ko": {
		flow: "흐름", data: "데이터 흐름", before: "앞", after: "뒤", more: "외 %d개",
		steps:  func(ns []string, of int) string { return strings.Join(ns, "·") + "/" + strconv.Itoa(of) + "단계" },
		inside: func(n int, unit string) string { return fmt.Sprintf("%d단계 %s", n, unit) },
	},
	"en": {
		flow: "flow", data: "data flow", before: "before", after: "after", more: "and %d more",
		steps: func(ns []string, of int) string {
			if len(ns) == 1 {
				return fmt.Sprintf("step %s of %d", ns[0], of)
			}
			return fmt.Sprintf("steps %s of %d", strings.Join(ns, ", "), of)
		},
		inside: func(n int, unit string) string { return fmt.Sprintf("step %d %s", n, unit) },
	},
}

func wordsFor(t model.Language) flowWords {
	base, _, _ := strings.Cut(string(t), "-")
	if w, ok := flowLanguages[base]; ok {
		return w
	}
	return flowLanguages["en"]
}

// flowPositions are the unit's places in the stored flows, or in its
// posts_to and data relations when no stored flow holds it.
func (e env) flowPositions(units []model.Unit, id model.ID) ([]topology.FlowPosition, error) {
	figs, err := e.st.LoadFigures()
	if err != nil {
		return nil, err
	}
	var flows []topology.FlowSteps
	for _, f := range figs.Flows {
		if !f.Known {
			continue
		}
		fs := topology.FlowSteps{Key: string(f.Key)}
		for _, p := range f.Parts {
			fs.Units = append(fs.Units, p.Claim.Unit)
		}
		flows = append(flows, fs)
	}
	return topology.FlowPositions(units, flows, id), nil
}

// flowLines say each position on one line, such as
// "flow: sync step 3 of 13; before: MOD-0004 cmd; after: MOD-0017 extract".
func flowLines(ps []topology.FlowPosition, id model.ID, name map[model.ID]string, t model.Language) []string {
	w := wordsFor(t)
	unit := func(x model.ID) string { return strings.TrimSpace(string(x) + " " + name[x]) }
	side := func(label string, ids []model.ID) string {
		var s []string
		for _, x := range ids[:min(len(ids), neighbours)] {
			s = append(s, unit(x))
		}
		if len(ids) > neighbours {
			s = append(s, fmt.Sprintf(w.more, len(ids)-neighbours))
		}
		return label + ": " + strings.Join(s, ", ")
	}
	var out []string
	for _, p := range ps {
		head, parts := w.data, []string{}
		if p.Flow != "" {
			var ns, inside []string
			for _, s := range p.Steps {
				ns = append(ns, strconv.Itoa(s.N))
				if s.Unit != id {
					inside = append(inside, w.inside(s.N, unit(s.Unit)))
				}
			}
			at := p.Flow + " " + w.steps(ns, p.Of)
			if len(inside) > 0 {
				at += " (" + strings.Join(inside, ", ") + ")"
			}
			head, parts = w.flow, []string{at}
		}
		if len(p.Upstream) > 0 {
			parts = append(parts, side(w.before, p.Upstream))
		}
		if len(p.Downstream) > 0 {
			parts = append(parts, side(w.after, p.Downstream))
		}
		out = append(out, head+": "+strings.Join(parts, "; "))
	}
	return out
}

// flowText is the unit's flow lines in the project's language.
func (e env) flowText(units []model.Unit, id model.ID) ([]string, error) {
	ps, err := e.flowPositions(units, id)
	if err != nil || len(ps) == 0 {
		return nil, err
	}
	t, err := e.lang()
	if err != nil {
		return nil, err
	}
	return flowLines(ps, id, names(units), t), nil
}
