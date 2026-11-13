package main

import (
	"fmt"
	"strings"

	"github.com/whyjp/abbd/internal/extract/runscan"
	"github.com/whyjp/abbd/internal/model"
)

// runtime prints the run-time work list: each catalog candidate the state
// does not hold yet, or, with --records, the scanner's records as JSON lines.
func (e env) runtime() error {
	rs, err := runscan.Scan(e.root)
	if err != nil {
		return err
	}
	units, err := e.st.LoadUnits()
	if err != nil {
		return err
	}
	if e.records {
		runscan.Own(rs, units)
		return runscan.WriteJSON(e.stdout, rs)
	}
	cs := runscan.Candidates(rs, units)
	open := 0
	for _, c := range cs {
		if !c.Stored {
			open++
		}
		if c.Stored && !e.verbose {
			continue
		}
		fmt.Fprintln(e.stdout, candidateLine(c))
	}
	fmt.Fprintf(e.stdout, "%d candidate(s), %d not stored; each is a lead: read its code, then store it with its run and quotes or leave it\n", len(cs), open)
	return nil
}

func candidateLine(c runscan.Candidate) string {
	state := map[bool]string{true: "stored", false: "open"}[c.Stored]
	who := func(id model.ID) string {
		if id == "" {
			return "-"
		}
		return string(id)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s %s", state, c.Fact, who(c.Owner))
	if c.Fact == runscan.PostsToFact || c.Other != "" {
		fmt.Fprintf(&b, " -> %s", who(c.Other))
	}
	fmt.Fprintf(&b, " %s", c.API)
	if c.Entry != "" {
		fmt.Fprintf(&b, " runs %s", c.Entry)
	}
	if c.Channel != "" {
		fmt.Fprintf(&b, " on %s", c.Channel)
	}
	for i, s := range c.Sites {
		sep := ":"
		if i > 0 {
			sep = " ->"
		}
		fmt.Fprintf(&b, "%s %s:%d %q", sep, s.Path, s.Line, s.Quote)
	}
	return b.String()
}

// runtimeLine is the status line for the run-time pass: how many catalog
// candidates the state does not hold yet. It is "" when the catalog finds
// no candidate, as in a library that starts no thread. An open candidate is a
// lead, not a debt, so it never fails a check.
func runtimeLine(root string, units []model.Unit) string {
	rs, err := runscan.Scan(root)
	if err != nil {
		return ""
	}
	cs := runscan.Candidates(rs, units)
	if len(cs) == 0 {
		return ""
	}
	n := map[runscan.Fact]int{}
	all := 0
	for _, c := range cs {
		all++
		if !c.Stored {
			n[c.Fact]++
		}
	}
	open := n[runscan.ExecFact] + n[runscan.PostsToFact] + n[runscan.RunsOnFact] + n[runscan.ProvidesFact]
	if open == 0 {
		return fmt.Sprintf("runtime: all %d candidate(s) stored", all)
	}
	parts := []string{fmt.Sprintf("%d spawn site(s) with no EXEC", n[runscan.ExecFact]), fmt.Sprintf("%d channel pair(s) with no posts_to", n[runscan.PostsToFact])}
	if k := n[runscan.RunsOnFact]; k > 0 {
		parts = append(parts, fmt.Sprintf("%d registration(s) with no runs_on", k))
	}
	if k := n[runscan.ProvidesFact]; k > 0 {
		parts = append(parts, fmt.Sprintf("%d impl(s) with no provides", k))
	}
	return fmt.Sprintf("runtime: %d of %d candidate(s) not stored: %s; abbd runtime lists them for the reader's run-time pass", open, all, strings.Join(parts, ", "))
}
