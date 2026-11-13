package main

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/store"
)

// overview stores the overview's zones and lines from a draft, or prints a
// draft to write one from when no FILE is given (overviewDraft). Every check of a draft
// happens here, as it does for a figure, so the store and the screen trust
// what they read. The primary flow is the stack marked primary, stored with
// abbd stack.
func (e env) overview(args []string) error {
	if len(args) == 0 {
		return e.overviewDraft()
	}
	text, err := e.readInput(args[0], "pass an overview draft: frontmatter with zones and lines, and a caption")
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
	fmt.Fprintf(e.stdout, "stored the overview: %d zones, %d lines, each part stamped with the lines around the quotes it cites; its primary flow is the stack marked primary\n", len(o.Zones), len(o.Lines))
	return nil
}

// checkOverview refuses a draft that is no overview true of the state: too
// many zones or lines, a part that is not well formed, a quote the code does
// not hold once, a part the state does not bear out, or parts the state does
// not bear out taken together, such as a unit in two zones.
func checkOverview(e env, o *store.Overview, units []model.Unit) error {
	if err := partCount(len(o.Zones), model.Zone{}.Kind()); err != nil {
		return err
	}
	if err := partCount(len(o.Lines), model.Line{}.Kind()); err != nil {
		return err
	}
	for _, err := range []error{checkParts(e, o.Zones, units), checkParts(e, o.Lines, units)} {
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

// partCount refuses too many parts of one kind.
func partCount[C any](n int, k model.FigureKind[C]) error {
	if n > k.Max {
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

// overviewParts are the overview's parts in the file's order: zones, then
// lines.
func overviewParts(o store.Overview) []shownPart {
	return slices.Concat(partsShown(o.Zones), partsShown(o.Lines))
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
	fmt.Fprintf(e.stdout, "overview: %d zones, %d lines, %d STALE\n", len(o.Zones), len(o.Lines), len(stale))
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
var overviewWords = map[bool]struct{ stage, through, zone string }{
	true:  {"주 흐름 %s %d/%d단계 %s", ", 안의 %s로", "구역 %s"},
	false: {"primary flow %s, stage %d of %d: %s", ", through %s inside it", "zone: %s"},
}

// explainOverview prints the stage of the primary flow, the stack marked
// primary, and the zone of the overview that hold the unit, itself or
// through a unit that holds it. A unit no stage holds stands, as the
// overview draws its box, at the first stage that names a unit inside it,
// and explain names that unit.
func (e env) explainOverview(units []model.Unit, id model.ID) error {
	figs, err := e.st.LoadFigures()
	if err != nil {
		return err
	}
	o, err := e.st.LoadOverview()
	if err != nil {
		return err
	}
	p, flow := figs.Primary()
	if !flow && !o.Known {
		return nil
	}
	lang, err := e.lang()
	if err != nil {
		return err
	}
	words := overviewWords[strings.HasPrefix(string(lang)+"-", "ko-")]
	if flow {
		layers := p.Claims()
		if n, ok := model.Holders(layers, units)[id]; ok {
			fmt.Fprintf(e.stdout, words.stage+"\n", p.Key, n, len(layers), layers[n-1].Name)
		} else if n, inner := model.StandsAt(layers, units, id); n > 0 {
			fmt.Fprintf(e.stdout, words.stage+words.through+"\n", p.Key, n, len(layers), layers[n-1].Name, inner)
		}
	}
	if n, ok := model.Holders(store.ClaimsOf(o.Zones), units)[id]; ok {
		fmt.Fprintf(e.stdout, words.zone+"\n", o.Zones[n-1].Claim.Name)
	}
	return nil
}
