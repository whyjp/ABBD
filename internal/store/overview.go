package store

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/whyjp/abbd/internal/model"
)

// OverviewFile is the one overview of a project, from the store's directory.
const OverviewFile = "overview.md"

// Overview is the overview's own record: its zones and the lines a reader
// chose. The primary flow it draws in stages is the stack marked primary
// (Figures.Primary), not part of this record. Each part keeps
// its own stamp, as a figure's does, so an edit near one part's quotes
// stales that part alone. The caption is the file's body. File is where the
// overview sits, "" when there is none. Known is false when the file is
// there and is no overview, and Why says why.
type Overview struct {
	Zones   []Part[model.Zone]
	Lines   []Part[model.Line]
	Caption string
	File    string
	Known   bool
	Why     string
}

// overviewYAML is the overview's frontmatter, its parts as Z and L: the
// claims alone in a draft, the stamped parts in the stored file.
type overviewYAML[Z, L any] struct {
	Format int `yaml:"format,omitempty"`
	Zones  []Z `yaml:"zones,omitempty"`
	Lines  []L `yaml:"lines,omitempty"`
}

type storedOverview = overviewYAML[Part[model.Zone], Part[model.Line]]

// decodeOverview reads an overview's frontmatter, refusing a key it does not
// know, so a misspelt field is no part silently dropped.
func decodeOverview(front []byte) (storedOverview, error) {
	var o storedOverview
	d := yaml.NewDecoder(bytes.NewReader(front))
	d.KnownFields(true)
	if err := d.Decode(&o); err != nil && !errors.Is(err, io.EOF) {
		return o, err
	}
	return o, nil
}

// ParseOverviewDraft reads the file a reader writes: the stored format, of
// which it keeps the parts and the caption. abbd sets the
// rest when it stores the overview.
func ParseOverviewDraft(b []byte) (Overview, error) {
	front, body, err := splitFrontmatter(b)
	if err != nil {
		return Overview{}, err
	}
	o, err := decodeOverview(front)
	if err != nil {
		return Overview{}, fmt.Errorf("the draft's frontmatter: %w", err)
	}
	out := Overview{Caption: CleanText(string(body)), Known: true}
	out.Zones = claimsOnly(o.Zones)
	out.Lines = claimsOnly(o.Lines)
	return out, nil
}

func claimsOnly[C model.Claim[C]](parts []Part[C]) []Part[C] {
	var out []Part[C]
	for _, p := range parts {
		out = append(out, Part[C]{Claim: p.Claim})
	}
	return out
}

// OverviewDraft is the stored overview as a draft to edit, without what
// abbd sets.
func OverviewDraft(o Overview) ([]byte, error) {
	var d overviewYAML[model.Zone, model.Line]
	d.Zones, d.Lines = ClaimsOf(o.Zones), ClaimsOf(o.Lines)
	front, err := yaml.Marshal(d)
	if err != nil {
		return nil, err
	}
	return []byte("---\n" + string(front) + "---\n\n" + o.Caption + "\n"), nil
}

// SaveOverview stamps each part with the lines around the quotes it cites,
// as the code is now, and writes the overview. It refuses a part that cites
// no code. The same overview on the same code writes the same bytes.
func SaveOverview(s Store, o Overview) error {
	var err error
	if o.Zones, err = stampParts(s, "zones", o.Zones); err != nil {
		return err
	}
	if o.Lines, err = stampParts(s, "lines", o.Lines); err != nil {
		return err
	}
	var f storedOverview
	f.Format = figureFormat
	f.Zones, f.Lines = plainQuotes(o.Zones), plainQuotes(o.Lines)
	front, err := yaml.Marshal(f)
	if err != nil {
		return err
	}
	return writeFile(s.path(OverviewFile), []byte("---\n"+string(front)+"---\n\n"+CleanText(o.Caption)+"\n"))
}

// LoadOverview reads the overview, each part checked against the code it
// cites now. A file abbd cannot read is no
// overview: it comes back with Known false and why, so one bad file stops
// no reader of the rest.
func (s Store) LoadOverview() (Overview, error) {
	file := Dir + "/" + OverviewFile
	b, err := os.ReadFile(s.path(OverviewFile))
	if os.IsNotExist(err) {
		return Overview{}, nil
	} else if err != nil {
		return Overview{}, err
	}
	front, body, err := splitFrontmatter(b)
	if errors.Is(err, errNoFrontmatter) {
		return Overview{File: file, Why: "has no frontmatter"}, nil
	} else if err != nil {
		return Overview{File: file, Why: "has frontmatter abbd cannot read (" + err.Error() + ")"}, nil
	}
	f, err := decodeOverview(front)
	if err != nil {
		return Overview{File: file, Why: "has frontmatter abbd cannot read (" + err.Error() + ")"}, nil
	}
	o := Overview{Zones: f.Zones, Lines: f.Lines, Caption: CleanText(string(body)), File: file, Known: true}
	readParts(s, o.Zones)
	readParts(s, o.Lines)
	return o, nil
}

// Gaps are what keep the overview's parts, taken together, from being true
// of the state: a unit in two zones, two zones of one name, or a line drawn
// twice.
// Each gap's parts are numbered within its own kind, which Part names.
func (o Overview) Gaps(units []model.Unit) []OverviewGap {
	var out []OverviewGap
	add := func(part string, gaps []model.Gap) {
		for _, g := range gaps {
			out = append(out, OverviewGap{Part: part, Gap: g})
		}
	}
	add("zone", model.Zone{}.Kind().Whole(nil, ClaimsOf(o.Zones), units))
	add("line", model.Line{}.Kind().Whole(nil, ClaimsOf(o.Lines), units))
	return out
}

// OverviewGap is a gap of the overview's parts of one kind.
type OverviewGap struct {
	Part string
	model.Gap
}
