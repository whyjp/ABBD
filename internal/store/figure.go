package store

import (
	"bytes"
	"errors"
	"fmt"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/whyjp/abbd/internal/model"
)

// Figure is one stored question answered by ordered claims of one kind, such
// as a flow's steps. Each part keeps its own stamp, one hash per quote of the
// lines around it, so an edit near one part's quotes stales that part alone,
// and its own writer, so a reader who replaces a stale part keeps a human's
// word on the others. Retired are parts that went STALE and left the figure,
// each with the code it was written against, kept while that code still
// differs, so their words cannot come back on it unread. Columns are the
// head of a kind that has one, such as a table's. The caption is the file's
// body. InputsHash covers every kind's directory, so storing a figure stales
// the blueprint.
type Figure[C model.Claim[C]] struct {
	Question string
	// Primary marks the figure the repository's primary flow, its claims the
	// flow's stages in order. Only a kind whose FigureKind.Primary is set
	// holds it, and Figures.Primary reads it as one at most.
	Primary        bool
	Columns        []model.Column
	Parts, Retired []Part[C]
	Caption        string
	// Key is the file's name and File where it sits, from the project root.
	// Known is false when the name is no figure key; status reports it.
	Key   model.FigureKey
	File  string
	Known bool
}

// Flow is a question answered in ordered steps, Stack one answered in
// layers from the top down, and Table one answered in rows of cells under
// named columns.
type (
	Flow  = Figure[model.Step]
	Stack = Figure[model.Layer]
	Table = Figure[model.Row]
)

// Part is one claim of a figure with its stamp. Go cannot embed a type
// parameter, so the claim is a named field.
type Part[C model.Claim[C]] struct {
	Claim C `yaml:",inline"`
	Stamp `yaml:",inline"`
}

// Stamp is who wrote a part and the code it was written against: a human's
// part carries the Source they said it in. Changed lists the cited places
// whose code moved since, and Moved their files; loading fills both.
type Stamp struct {
	Writer  model.Writer      `yaml:"writer,omitempty"`
	Source  string            `yaml:"source,omitempty"`
	Code    map[string]string `yaml:"code"`
	Changed []string          `yaml:"-"`
	Moved   []string          `yaml:"-"`
}

// Stale reports whether the code the part cites changed since it was written.
func (s Stamp) Stale() bool { return len(s.Changed) > 0 }

// Why is every reason the part is STALE: the code it cites changed, or the
// state lost what it names.
func (p Part[C]) Why(units []model.Unit) []string {
	return append(slices.Clone(p.Changed), p.Claim.Gaps(units)...)
}

// Origin is the weakest origin of the figure's parts: USER_STATED only when
// a human wrote every part.
func (f Figure[C]) Origin() model.Origin {
	for _, p := range f.Parts {
		if p.Writer != model.Human {
			return model.Inferred
		}
	}
	return model.UserStated
}

// Claims are the figure's claims, in order.
func (f Figure[C]) Claims() []C {
	out := make([]C, len(f.Parts))
	for i, p := range f.Parts {
		out[i] = p.Claim
	}
	return out
}

// Gaps are what keep the figure's claims, taken together with its head, from
// being true of the state, each with the places of the parts it involves.
func (f Figure[C]) Gaps(units []model.Unit) []model.Gap {
	if whole := model.KindOf[C]().Whole; whole != nil {
		return whole(f.Columns, f.Claims(), units)
	}
	return nil
}

// figureFormat is the version of a figure file.
const figureFormat = 1

// FigureDir is the directory that holds the figures of kind C, from the
// project root.
func FigureDir[C model.Claim[C]]() string { return Dir + "/" + model.KindOf[C]().Dir }

// ParseFigureDraft reads the file a reader or a human writes: the stored
// format, of which it keeps the question, the head, the parts and the
// caption. abbd
// sets the rest when it stores the figure.
func ParseFigureDraft[C model.Claim[C]](b []byte) (Figure[C], error) {
	front, body, err := splitFrontmatter(b)
	if err != nil {
		return Figure[C]{}, err
	}
	f, err := decodeFigure[C](front)
	if err != nil {
		return Figure[C]{}, fmt.Errorf("the draft's frontmatter: %w", err)
	}
	out := Figure[C]{Question: strings.TrimSpace(f.Question), Primary: f.Primary, Columns: f.Columns, Caption: CleanText(string(body))}
	for _, p := range f.Parts {
		out.Parts = append(out.Parts, Part[C]{Claim: p.Claim})
	}
	return out, nil
}

// FigureDraft is the stored figure as a draft to edit: its question, head,
// claims and caption, without what abbd sets.
func FigureDraft[C model.Claim[C]](f Figure[C]) ([]byte, error) {
	front, err := encodeFigure(f, false)
	if err != nil {
		return nil, err
	}
	return []byte("---\n" + string(front) + "---\n\n" + f.Caption + "\n"), nil
}

// SaveFigure stamps each part with the lines around the quotes it cites, as
// the code is now, and writes the figure; a retired part keeps the stamp it
// was written with. It refuses a part that cites no code. The same figure on
// the same code writes the same bytes.
func SaveFigure[C model.Claim[C]](s Store, f Figure[C]) error {
	k := model.KindOf[C]()
	f.Parts = slices.Clone(f.Parts)
	for i := range f.Parts {
		f.Parts[i].Changed, f.Parts[i].Moved = nil, nil
		if _, f.Parts[i].Code = s.NoteStamp(f.Parts[i].Claim.Quotes()); len(f.Parts[i].Code) == 0 {
			return fmt.Errorf("%s %s %s %d cites no code to check it against", k.Noun, f.Key, k.Part, i+1)
		}
	}
	return writeFigure(s, f)
}

// writeFigure writes a stamped figure, each quote kept to its path, lines,
// symbol and words.
func writeFigure[C model.Claim[C]](s Store, f Figure[C]) error {
	f.Parts, f.Retired = slices.Clone(f.Parts), slices.Clone(f.Retired)
	for _, parts := range [][]Part[C]{f.Parts, f.Retired} {
		for i, p := range parts {
			ev := make([]model.Evidence, len(p.Claim.Quotes()))
			for j, e := range p.Claim.Quotes() {
				ev[j] = model.Evidence{Path: e.Path, Lines: e.Lines, Symbol: e.Symbol, Quote: e.Quote}
			}
			parts[i].Claim = p.Claim.Requote(ev)
		}
	}
	front, err := encodeFigure(f, true)
	if err != nil {
		return err
	}
	return writeFile(s.path(model.KindOf[C]().Dir, string(f.Key)+".md"), []byte("---\n"+string(front)+"---\n\n"+CleanText(f.Caption)+"\n"))
}

// encodeFigure writes a figure's frontmatter in the order a reader reads it:
// the question, then for a stored figure its format, then the primary mark
// when it is set, then the head of a kind that has one, then for a stored figure its parts under the kind's own key
// and its retired parts, and for a draft its claims alone.
func encodeFigure[C model.Claim[C]](f Figure[C], stored bool) ([]byte, error) {
	var m yaml.Node
	var head any = struct {
		Question string `yaml:"question"`
	}{f.Question}
	if stored {
		head = struct {
			Question string `yaml:"question"`
			Format   int    `yaml:"format"`
		}{f.Question, figureFormat}
	}
	if err := m.Encode(head); err != nil {
		return nil, err
	}
	add := func(key string, v any) error {
		var n yaml.Node
		if err := n.Encode(v); err != nil {
			return err
		}
		m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, &n)
		return nil
	}
	if f.Primary {
		if err := add("primary", true); err != nil {
			return nil, err
		}
	}
	if len(f.Columns) > 0 {
		if err := add("columns", f.Columns); err != nil {
			return nil, err
		}
	}
	parts := model.KindOf[C]().Parts
	if !stored {
		if err := add(parts, f.Claims()); err != nil {
			return nil, err
		}
		return yaml.Marshal(&m)
	}
	if err := add(parts, f.Parts); err != nil {
		return nil, err
	}
	if len(f.Retired) > 0 {
		if err := add("retired", f.Retired); err != nil {
			return nil, err
		}
	}
	return yaml.Marshal(&m)
}

// decodeFigure reads a figure's frontmatter by the keys encodeFigure writes.
func decodeFigure[C model.Claim[C]](front []byte) (Figure[C], error) {
	var f Figure[C]
	var m map[string]yaml.Node
	if err := yaml.Unmarshal(front, &m); err != nil {
		return f, err
	}
	for _, field := range []struct {
		key string
		v   any
	}{{"question", &f.Question}, {"primary", &f.Primary}, {"columns", &f.Columns}, {model.KindOf[C]().Parts, &f.Parts}, {"retired", &f.Retired}} {
		if n, ok := m[field.key]; ok {
			if err := n.Decode(field.v); err != nil {
				return f, err
			}
		}
	}
	return f, nil
}

// Stray is a markdown file beside the figures of a kind that is no figure,
// from the project root, and why abbd reads past it.
type Stray struct{ File, Why string }

// Figures is every stored figure, by kind, and the files beside them that
// are no figures.
type Figures struct {
	Flows  []Flow
	Stacks []Stack
	Tables []Table
	Strays []Stray
}

// Primary is the stack marked the repository's primary flow. ok is true only
// when exactly one known stack is marked, so a double mark lays out by
// neither.
func (fs Figures) Primary() (Stack, bool) {
	var marked []Stack
	for _, f := range fs.Stacks {
		if f.Known && f.Primary {
			marked = append(marked, f)
		}
	}
	if len(marked) != 1 {
		return Stack{}, false
	}
	return marked[0], true
}

// Marked are the keys of the known figures of figs marked the primary flow,
// in key order.
func Marked[C model.Claim[C]](figs []Figure[C]) []model.FigureKey {
	var out []model.FigureKey
	for _, f := range figs {
		if f.Known && f.Primary {
			out = append(out, f.Key)
		}
	}
	return out
}

// LoadKind reads every figure of kind C in key order.
func LoadKind[C model.Claim[C]](s Store) ([]Figure[C], error) {
	figs, _, err := loadKind[C](s)
	return figs, err
}

// LoadFigures reads every figure of every kind. It is the store's one list
// of kinds.
func (s Store) LoadFigures() (Figures, error) {
	var fs Figures
	var flows, stacks, tables []Stray
	var err error
	if fs.Flows, flows, err = loadKind[model.Step](s); err != nil {
		return fs, err
	}
	if fs.Stacks, stacks, err = loadKind[model.Layer](s); err != nil {
		return fs, err
	}
	if fs.Tables, tables, err = loadKind[model.Row](s); err != nil {
		return fs, err
	}
	fs.Strays = slices.Concat(flows, stacks, tables)
	return fs, nil
}

// LoadFigure reads one figure of kind C, false when there is none.
func LoadFigure[C model.Claim[C]](s Store, k model.FigureKey) (Figure[C], bool, error) {
	figs, _, err := loadKind[C](s)
	if err != nil {
		return Figure[C]{}, false, err
	}
	i := slices.IndexFunc(figs, func(f Figure[C]) bool { return f.Known && f.Key == k })
	if i < 0 {
		return Figure[C]{}, false, nil
	}
	return figs[i], true, nil
}

// loadKind reads every figure of kind C in key order, each part checked
// against the code it cites now. A markdown file there without frontmatter,
// or whose frontmatter abbd cannot read, is no figure: loadKind returns it
// among the strays, so one bad file stops no reader of the others.
func loadKind[C model.Claim[C]](s Store) ([]Figure[C], []Stray, error) {
	k := model.KindOf[C]()
	files, err := filepath.Glob(s.path(k.Dir, "*.md"))
	if err != nil {
		return nil, nil, err
	}
	slices.Sort(files)
	var out []Figure[C]
	var strays []Stray
	for _, f := range files {
		file := path.Join(FigureDir[C](), filepath.Base(f))
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, nil, err
		}
		front, body, err := splitFrontmatter(b)
		if errors.Is(err, errNoFrontmatter) {
			strays = append(strays, Stray{file, "has no frontmatter"})
			continue
		} else if err != nil {
			strays = append(strays, Stray{file, "has frontmatter abbd cannot read (" + err.Error() + ")"})
			continue
		}
		fig, err := decodeFigure[C](front)
		if err != nil {
			strays = append(strays, Stray{file, "has frontmatter abbd cannot read (" + err.Error() + ")"})
			continue
		}
		name := strings.TrimSuffix(filepath.Base(f), ".md")
		_, err = model.ParseFigureKey[C](name)
		fig.Key, fig.File, fig.Known, fig.Caption = model.FigureKey(name), file, err == nil, CleanText(string(body))
		for i, p := range fig.Parts {
			if p.Writer == "" {
				fig.Parts[i].Writer = model.Reader
			}
			fig.Parts[i].Changed, fig.Parts[i].Moved = s.changed(p.Claim.Quotes(), p.Code)
			if len(p.Claim.Quotes()) == 0 || len(p.Code) == 0 {
				fig.Parts[i].Changed = []string{"the " + k.Part + " cites no code abbd stamped, so nothing shows it is still true"}
			}
		}
		for i, p := range fig.Retired {
			fig.Retired[i].Changed, fig.Retired[i].Moved = s.changed(p.Claim.Quotes(), p.Code)
		}
		out = append(out, fig)
	}
	return out, strays, nil
}

// changed lists each cited place whose code differs from what was written
// against it, as why: its lines changed, or its quote is gone. files are
// the paths of those places.
func (s Store) changed(cited []model.Evidence, code map[string]string) (why, files []string) {
	_, now := s.NoteStamp(cited)
	if maps.Equal(now, code) {
		return nil, nil
	}
	for k, h := range now {
		if code[k] != h {
			reason := "the lines around it changed"
			if h == missing {
				reason = "the quote is gone, or no longer once in the file"
			}
			why = append(why, fmt.Sprintf("%s (%s)", keyPath(k), reason))
			files = append(files, keyPath(k))
		}
	}
	for k := range code {
		if _, ok := now[k]; !ok {
			files = append(files, keyPath(k))
		}
	}
	slices.Sort(why)
	slices.Sort(files)
	why, files = slices.Compact(why), slices.Compact(files)
	if len(why) == 0 {
		why = []string{"its stored code does not match its evidence"}
	}
	return why, files
}

// splitFrontmatter cuts a file into its frontmatter and its body.
func splitFrontmatter(b []byte) (front, body []byte, err error) {
	b = bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n"))
	rest, ok := bytes.CutPrefix(b, []byte("---\n"))
	if !ok {
		return nil, nil, errNoFrontmatter
	}
	front, body, ok = bytes.Cut(append(rest, '\n'), []byte("\n---\n"))
	if !ok {
		return nil, nil, fmt.Errorf("unterminated frontmatter")
	}
	return front, body, nil
}
