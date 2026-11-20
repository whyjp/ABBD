package screen

import "strings"

// CommonFile is the page of the overview's common modules (common.go) and
// the third-party libraries its boxes share. The blueprint has it only when
// it has one of either, and the overview links it then.
const CommonFile = "common.html"

// CommonCard is one common module as its page draws it: the boxes that use
// it in a row over a band that stands for it, each user's words under its
// box, and the boxes that do not use it.
type CommonCard struct {
	CommonView
	Utility  bool
	Ref      Ref
	Users    []Ref
	NonUsers []Ref
	Words    []CommonWords
	Fig      CommonFig
}

// CommonWords are the words one user's relations into the module carry.
type CommonWords struct {
	From  Ref
	Words string
}

// CommonFig is the card's figure: the users' boxes in a row, a line down
// from each to the band, and each user's words under its box.
type CommonFig struct {
	W, H  int
	Boxes []CommonFigBox
	Band  rect
	Text  string
}

// CommonFigBox is one user's box on the figure and the words set under it.
type CommonFigBox struct {
	X, Y, W, H, CX, Low int
	// TX is where the line down to the band runs, near the box's left
	// edge, so the words beside it have the slot's width.
	TX         int
	Name, Href string
	Words      []Text
}

// LibraryRow is one shared library and the boxes that use it.
type LibraryRow struct {
	Name  string
	Users []Ref
}

// Figure sizes for the common page.
const (
	commonFigW    = 1180
	commonBoxH    = 40
	commonBoxMax  = 160
	commonWordGap = 16
	commonWords   = 3
)

// commonCards are the common modules of m as their page draws them.
func commonCards(m Model, ref func(string) Ref) []CommonCard {
	var out []CommonCard
	for _, c := range m.Common {
		card := CommonCard{CommonView: c, Utility: c.Utility(), Ref: ref(c.Unit)}
		words := map[string]string{}
		for _, p := range c.Phrases {
			words[p.From] = strings.Join(p.Words, ", ")
			card.Words = append(card.Words, CommonWords{From: ref(p.From), Words: words[p.From]})
		}
		for _, u := range c.Users {
			card.Users = append(card.Users, ref(u))
		}
		for _, u := range c.NonUsers {
			card.NonUsers = append(card.NonUsers, ref(u))
		}
		card.Fig = commonFig(card.Users, c.Phrases, len(c.Users)+len(c.NonUsers), c.Name)
		out = append(out, card)
	}
	return out
}

// commonFig lays the users out in one row, as many as fit, over a band.
func commonFig(users []Ref, phrases []CommonPhrase, others int, name string) CommonFig {
	n := max(1, len(users))
	slot := (commonFigW - 2*padX) / n
	w := min(commonBoxMax, slot-12)
	said := map[string][]string{}
	most := 0
	for _, p := range phrases {
		said[p.From] = p.Words
		most = max(most, min(len(p.Words), commonWords+1))
	}
	low := padY + commonBoxH + 18 + most*commonWordGap
	f := CommonFig{W: commonFigW}
	for i, u := range users {
		x := padX + i*slot + (slot-w)/2
		b := CommonFigBox{X: x, Y: padY, W: w, H: commonBoxH, CX: x + w/2, TX: x + 14, Name: fitName(u.Name, w-8), Href: u.Href}
		for j, s := range said[u.ID] {
			if j == commonWords {
				b.Words = append(b.Words, Text{S: "…", X: b.TX + 6, Y: padY + commonBoxH + 18 + j*commonWordGap})
				break
			}
			b.Words = append(b.Words, Text{S: fitName(s, slot-30), X: b.TX + 6, Y: padY + commonBoxH + 18 + j*commonWordGap})
		}
		f.Boxes = append(f.Boxes, b)
	}
	f.Band = rect{X: padX, Y: low + 24, W: commonFigW - 2*padX, H: 44}
	for i := range f.Boxes {
		f.Boxes[i].Low = f.Band.Y - 1
	}
	f.H = f.Band.Y + f.Band.H + padY
	f.Text = name + " · " + usedByText(len(users))
	return f
}

// fitName cuts s to what fits w px, a wide glyph 13px and any other 7px
// as the page sets them, with an ellipsis.
func fitName(s string, w int) string {
	width := func(rs []rune) int {
		n := 0
		for _, r := range rs {
			n += 7
			if r >= 0x1100 {
				n += 6
			}
		}
		return n
	}
	r := []rune(s)
	if width(r) <= w {
		return s
	}
	for len(r) > 1 && width(r)+7 > w {
		r = r[:len(r)-1]
	}
	return strings.TrimSpace(string(r)) + "…"
}

// libraryRows are the libraries two boxes or more of m share.
func libraryRows(m Model, ref func(string) Ref) []LibraryRow {
	var out []LibraryRow
	for _, l := range m.Libraries {
		r := LibraryRow{Name: l.Name}
		for _, u := range l.Users {
			r.Users = append(r.Users, ref(u))
		}
		out = append(out, r)
	}
	return out
}
