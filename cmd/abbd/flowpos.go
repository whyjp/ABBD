package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/screen"
	"github.com/whyjp/abbd/internal/topology"
)

// named is how many items a list of units or steps names before it counts
// the rest.
const named = 3

// flowWords say a position in the project's language.
type flowWords struct {
	// lead heads the flow the overview draws bold, flow any other stored
	// flow, and handoffs a position read from relations.
	lead, flow, handoffs string
	roles                map[topology.Role]string
	more                 string
	// steps says step numbers, with of the flow's length or 0 for none,
	// and sep joins the numbers.
	steps func(ns string, many bool, of int) string
	sep   string
}

var flowLanguages = map[string]flowWords{
	"ko": {
		lead: "주 흐름", flow: "흐름", handoffs: "주고받기", more: "외 %d개", sep: "·",
		roles: map[topology.Role]string{
			topology.Before: "앞", topology.After: "뒤", topology.WorkFrom: "일 받음", topology.WorkTo: "일 넘김",
			topology.Reads: "읽음", topology.ReadBy: "읽는 쪽", topology.ProvidedBy: "제공",
		},
		steps: func(ns string, _ bool, of int) string {
			if of > 0 {
				return ns + "/" + strconv.Itoa(of) + "단계"
			}
			return ns + "단계"
		},
	},
	"en": {
		lead: "primary flow", flow: "flow", handoffs: "hand-offs", more: "and %d more", sep: ", ",
		roles: map[topology.Role]string{
			topology.Before: "before", topology.After: "after", topology.WorkFrom: "work from", topology.WorkTo: "work to",
			topology.Reads: "reads", topology.ReadBy: "read by", topology.ProvidedBy: "provided by",
		},
		steps: func(ns string, many bool, of int) string {
			s := "step " + ns
			if many {
				s = "steps " + ns
			}
			if of > 0 {
				s += " of " + strconv.Itoa(of)
			}
			return s
		},
	},
}

func wordsFor(t model.Language) flowWords {
	base, _, _ := strings.Cut(string(t), "-")
	if w, ok := flowLanguages[base]; ok {
		return w
	}
	return flowLanguages["en"]
}

// flowPositions are the unit's places in the stored flows, the one the
// overview draws bold first, or in its posts_to and data relations when no
// stored flow holds it. Only with two stored flows or more is there a lead
// to tell apart, and only then is the overview drawn to find it.
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
	ps := topology.FlowPositions(units, flows, id)
	if len(flows) < 2 || len(ps) == 0 || ps[0].Flow == "" {
		return ps, nil
	}
	in, err := screen.Load(e.st)
	if err != nil {
		return nil, err
	}
	lead := screen.LeadFlow(in)
	for i := range flows {
		flows[i].Lead = flows[i].Key == lead
	}
	return topology.FlowPositions(units, flows, id), nil
}

// flowLines say each position on one line, such as
// "flow: sync steps 3–5 of 13; before: MOD-0004 cmd; after: MOD-0017 extract".
func flowLines(ps []topology.FlowPosition, id model.ID, name map[model.ID]string, t model.Language) []string {
	w := wordsFor(t)
	unit := func(x model.ID) string { return strings.TrimSpace(string(x) + " " + name[x]) }
	var out []string
	for _, p := range ps {
		head, parts := w.handoffs, []string{}
		if p.Flow != "" {
			head = w.flow
			if p.Lead {
				head = w.lead
			}
			var all []int
			var inside []model.ID
			at := map[model.ID][]int{}
			for _, s := range p.Steps {
				all = append(all, s.N)
				if s.Unit != id {
					if at[s.Unit] == nil {
						inside = append(inside, s.Unit)
					}
					at[s.Unit] = append(at[s.Unit], s.N)
				}
			}
			ns, many := w.numbers(all)
			step := p.Flow + " " + w.steps(ns, many, p.Of)
			var groups []string
			for _, u := range inside {
				ns, many := w.numbers(at[u])
				groups = append(groups, unit(u)+" "+w.steps(ns, many, 0))
			}
			if len(groups) > 0 {
				step += " (" + w.capped(groups) + ")"
			}
			parts = append(parts, step)
		}
		for _, s := range p.Sides {
			var us []string
			for _, x := range s.Units {
				us = append(us, unit(x))
			}
			parts = append(parts, w.roles[s.Role]+": "+w.capped(us))
		}
		out = append(out, head+": "+strings.Join(parts, "; "))
	}
	return out
}

// capped names the first items and counts the rest: "A, B, C 외 2개".
func (w flowWords) capped(items []string) string {
	s := strings.Join(items[:min(len(items), named)], ", ")
	if len(items) > named {
		s += " " + fmt.Sprintf(w.more, len(items)-named)
	}
	return s
}

// numbers says ascending step numbers with every run of three or more as
// its ends, "2·3·5–9", and whether they are more than one step.
func (w flowWords) numbers(ns []int) (string, bool) {
	var out []string
	for i := 0; i < len(ns); {
		j := i
		for j+1 < len(ns) && ns[j+1] == ns[j]+1 {
			j++
		}
		if j-i >= 2 {
			out = append(out, strconv.Itoa(ns[i])+"–"+strconv.Itoa(ns[j]))
		} else {
			for k := i; k <= j; k++ {
				out = append(out, strconv.Itoa(ns[k]))
			}
		}
		i = j + 1
	}
	return strings.Join(out, w.sep), len(ns) > 1
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
