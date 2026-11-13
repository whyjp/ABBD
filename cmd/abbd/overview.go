package main

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/store"
)

// overview stores the overview from a draft, or prints the stored one as a
// draft when no FILE is given. Every check of a draft happens here, as it
// does for a figure, so the store and the screen trust what they read.
func (e env) overview(args []string) error {
	if len(args) == 0 {
		prev, err := e.st.LoadOverview()
		switch {
		case err != nil:
			return err
		case prev.File == "":
			return fmt.Errorf("there is no overview; store one with abbd overview FILE")
		case !prev.Known:
			return fmt.Errorf("%s %s, so it is no overview; store one with abbd overview FILE", prev.File, prev.Why)
		}
		for _, l := range overviewStale(prev, nil) {
			fmt.Fprintln(e.stderr, "abbd:", l.text)
		}
		b, err := store.OverviewDraft(prev)
		if err != nil {
			return err
		}
		_, err = e.stdout.Write(b)
		return err
	}
	text, err := e.readInput(args[0], "pass an overview draft: frontmatter with flow (its name and stages), zones and lines, and a caption")
	if err != nil {
		return err
	}
	o, err := store.ParseOverviewDraft([]byte(text))
	if err != nil {
		return err
	}
	units, err := e.st.LoadUnits()
	if err != nil {
		return err
	}
	if err := checkOverview(e, &o, units); err != nil {
		return err
	}
	if err := store.SaveOverview(e.st, o); err != nil {
		return err
	}
	fmt.Fprintf(e.stdout, "stored the overview: primary flow %s in %d stages, %d zones, %d lines, each part stamped with the lines around the quotes it cites\n", o.Flow, len(o.Stages), len(o.Zones), len(o.Lines))
	return nil
}

// checkOverview refuses a draft that is no overview true of the state: a
// flow with no name or too few or too many stages, too many zones or lines,
// a part that is not well formed, a quote the code does not hold once, a
// part the state does not bear out, or parts the state does not bear out
// taken together, such as a unit in two stages.
func checkOverview(e env, o *store.Overview, units []model.Unit) error {
	if o.Flow == "" || strings.ContainsAny(o.Flow, "\r\n") {
		return fmt.Errorf("the overview's flow has a name, on one line")
	}
	if err := partCount(len(o.Stages), model.Stage{}.Kind()); err != nil {
		return err
	}
	if err := partCount(len(o.Zones), model.Zone{}.Kind()); err != nil {
		return err
	}
	if err := partCount(len(o.Lines), model.Line{}.Kind()); err != nil {
		return err
	}
	for _, err := range []error{checkParts(e, o.Stages, units), checkParts(e, o.Zones, units), checkParts(e, o.Lines, units)} {
		if err != nil {
			return err
		}
	}
	var texts []string
	for _, g := range o.Gaps(units) {
		texts = append(texts, g.Text)
	}
	if len(texts) > 0 {
		return errors.New(strings.Join(texts, "; "))
	}
	return nil
}

// partCount refuses too few or too many parts of one kind.
func partCount[C any](n int, k model.FigureKind[C]) error {
	switch {
	case n < k.Min:
		return fmt.Errorf("an overview has %d %s or more, and this one has %d", k.Min, k.Parts, n)
	case n > k.Max:
		return fmt.Errorf("an overview has at most %d %s, and this one has %d; keep the ones a person reads first", k.Max, k.Parts, n)
	}
	return nil
}

// staleLine is why one part of the overview, or its parts taken together,
// is STALE, and what a reader does about it: read the code again when the
// code it cites changed, else draw it again from the state as it is now.
type staleLine struct {
	text string
	code bool
}

func (l staleLine) fix() string {
	if l.code {
		return "read that code again and store the overview with abbd overview FILE"
	}
	return "draw it again from the state as it is now and store the overview with abbd overview FILE"
}

// overviewStale lists why each part of the overview is STALE: the code it
// cites changed, or, when units is given, the state no longer bears it out.
func overviewStale(o store.Overview, units []model.Unit) []staleLine {
	var out []staleLine
	for _, p := range overviewParts(o) {
		why := p.changed
		if units != nil {
			why = p.why(units)
		}
		if len(why) > 0 {
			out = append(out, staleLine{fmt.Sprintf("%s is STALE: %s", p, strings.Join(why, ", ")), len(p.changed) > 0})
		}
	}
	if units != nil {
		for _, g := range o.Gaps(units) {
			out = append(out, staleLine{"overview is STALE: " + g.Text, false})
		}
	}
	return out
}

// shownPart is one part of the overview as status and the evidence check
// name it: its kind, its place among its kind from 1, and its label.
type shownPart struct {
	kind    string
	n       int
	label   string
	quotes  []model.Evidence
	changed []string
	why     func([]model.Unit) []string
}

func (p shownPart) String() string { return fmt.Sprintf("overview %s %d (%s)", p.kind, p.n, p.label) }

// overviewParts are the overview's parts in the file's order: stages, zones,
// then lines.
func overviewParts(o store.Overview) []shownPart {
	return slices.Concat(partsShown(o.Stages), partsShown(o.Zones), partsShown(o.Lines))
}

func partsShown[C model.Claim[C]](parts []store.Part[C]) []shownPart {
	var out []shownPart
	for i, p := range parts {
		out = append(out, shownPart{model.KindOf[C]().Part, i + 1, p.Claim.Label(), p.Claim.Quotes(), p.Changed, p.Why})
	}
	return out
}

// statusOverview reports the overview: its parts, which are STALE and why,
// and whether the file is no overview. It returns what the check fails on,
// or "" when every part is current.
func (e env) statusOverview(units []model.Unit, o store.Overview) string {
	switch {
	case o.File == "":
		return ""
	case !o.Known:
		fmt.Fprintf(e.stdout, "warning: %s %s, so it is no overview and abbd reads past it; fix its frontmatter to clear this\n", o.File, o.Why)
		return ""
	}
	stale := overviewStale(o, units)
	fmt.Fprintf(e.stdout, "overview: primary flow %s in %d stages, %d zones, %d lines, %d STALE\n", o.Flow, len(o.Stages), len(o.Zones), len(o.Lines), len(stale))
	for _, l := range stale {
		fmt.Fprintln(e.stdout, "stale:", l.text+"; "+l.fix())
	}
	if len(stale) == 0 {
		return ""
	}
	return fmt.Sprintf("%d overview part(s) STALE, read the code each cites again and store it with abbd overview FILE", len(stale))
}

// overviewWords are the words explain prints for a unit's place in the
// overview, by the project's language: Korean, else English.
var overviewWords = map[bool]struct{ stage, zone string }{
	true:  {"주 흐름 %s %d/%d단계 %s", "구역 %s"},
	false: {"primary flow %s, stage %d of %d: %s", "zone: %s"},
}

// explainOverview prints the stage of the primary flow and the zone that
// hold the unit, itself or through a unit that holds it.
func (e env) explainOverview(units []model.Unit, id model.ID) error {
	o, err := e.st.LoadOverview()
	if err != nil || !o.Known {
		return err
	}
	lang, err := e.lang()
	if err != nil {
		return err
	}
	words := overviewWords[strings.HasPrefix(string(lang)+"-", "ko-")]
	if n, ok := model.Holders(store.ClaimsOf(o.Stages), units)[id]; ok {
		fmt.Fprintf(e.stdout, words.stage+"\n", o.Flow, n, len(o.Stages), o.Stages[n-1].Claim.Name)
	}
	if n, ok := model.Holders(store.ClaimsOf(o.Zones), units)[id]; ok {
		fmt.Fprintf(e.stdout, words.zone+"\n", o.Zones[n-1].Claim.Name)
	}
	return nil
}
