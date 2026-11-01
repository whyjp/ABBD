package screen

import (
	"fmt"
	"strings"

	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/topology"
)

// Understanding is a module's text from pstack's how, split into blocks the
// page renders without trusting any markup in it.
type Understanding struct {
	By     string
	Stale  bool
	Blocks []Block
}

type Block struct {
	// Kind is "h" for a heading, "li" for a list item, "p" for a paragraph.
	Kind  string
	Parts []Span
}

type Span struct {
	Text string
	Code bool
	// Mark highlights the words a change touched.
	Mark bool `json:",omitempty"`
}

func spans(s string) []Span {
	var out []Span
	for i, part := range strings.Split(s, "`") {
		if part != "" {
			out = append(out, Span{Text: part, Code: i%2 == 1})
		}
	}
	return out
}

func blocks(text string) []Block {
	var out []Block
	var para []string
	flush := func() {
		if len(para) > 0 {
			out = append(out, Block{Kind: "p", Parts: spans(strings.Join(para, " "))})
			para = nil
		}
	}
	for _, line := range strings.Split(text, "\n") {
		t := strings.TrimSpace(line)
		switch {
		case t == "":
			flush()
		case strings.HasPrefix(t, "#"):
			flush()
			out = append(out, Block{Kind: "h", Parts: spans(strings.TrimSpace(strings.TrimLeft(t, "#")))})
		case strings.HasPrefix(t, "- "), strings.HasPrefix(t, "* "):
			flush()
			out = append(out, Block{Kind: "li", Parts: spans(t[2:])})
		case len(t) > 2 && t[0] >= '0' && t[0] <= '9' && strings.Contains(t[:min(4, len(t))], ". "):
			flush()
			_, rest, _ := strings.Cut(t, ". ")
			out = append(out, Block{Kind: "li", Parts: spans(rest)})
		default:
			para = append(para, t)
		}
	}
	flush()
	return out
}

// Question is a grade C card in a human's words: what changes, why ABBD
// asks, what each answer does, and what to choose when unsure. The key that
// the CLI takes is folded under it.
type Question struct {
	Title, Why, Approve, Reject, Default string
	// Diff shows a contract's signature change member by member.
	Diff []DiffLine
	// Same counts the members the change left alone.
	Same int
}

type DiffLine struct {
	// Op is "+" for an added member, "-" for a removed one.
	Op    string
	Parts []Span
}

func relationWords(k model.RelationKind) string {
	return map[model.RelationKind]string{
		model.DependsOn: "에 직접 의존합니다", model.Consumes: "을(를) 씁니다", model.Provides: "을(를) 제공합니다",
	}[k]
}

func ask(it topology.Item, name func(model.ID) string, users []string) Question {
	switch it.Change {
	case topology.ContractChanged:
		q := Question{
			Title:   fmt.Sprintf("계약 %s의 모양이 바뀌었습니다.", name(it.Unit)),
			Why:     "의도한 계약은 사람이 승인한 모양입니다. 이 계약을 쓰는 곳이 함께 깨질 수 있어서 묻습니다.",
			Approve: "새 모양을 의도한 계약으로 받아들입니다. 쓰는 쪽을 맞추는 일은 이 턴에 남습니다.",
			Reject:  "받아들이지 않습니다. 에이전트가 반려 이유를 읽고 계약을 되돌립니다.",
			Default: "요청이 이 계약을 바꾸라고 했을 때만 승인하세요. 아니면 반려가 안전합니다.",
		}
		if len(users) > 0 {
			q.Why = fmt.Sprintf("의도한 계약은 사람이 승인한 모양입니다. 이 계약을 쓰는 %s이(가) 함께 깨질 수 있어서 묻습니다.", strings.Join(users, ", "))
		}
		q.Diff, q.Same = diffSignature(it.Old, it.New)
		return q
	case topology.NewRelation:
		e := it.Edge
		return Question{
			Title:   fmt.Sprintf("%s이(가) 이제 %s%s", name(e.From), name(e.To), relationWords(e.Kind)),
			Why:     "의도한 경계를 넘는 새 결합입니다. 두 설계 단위를 새로 묶거나 계약에 새로 붙습니다.",
			Approve: "이 결합을 의도한 토폴로지에 넣습니다. 이후로는 경계의 일부로 지켜집니다.",
			Reject:  "받아들이지 않습니다. 에이전트가 이 결합 없이 다시 구현합니다.",
			Default: "요청이 두 곳을 새로 잇는 일이었다면 승인하세요. 아니면 반려가 안전합니다.",
		}
	case topology.RemovedRelation:
		e := it.Edge
		return Question{
			Title:   fmt.Sprintf("%s이(가) 더 이상 %s%s", name(e.From), name(e.To), strings.Replace(relationWords(e.Kind), "니다", "지 않습니다", 1)),
			Why:     "의도한 토폴로지에 있던 결합이 코드에서 사라졌습니다. 계약에 붙은 결합이라 사람이 정합니다.",
			Approve: "의도에서 이 결합을 지웁니다.",
			Reject:  "결합을 되살려야 한다는 뜻입니다. 에이전트가 이유를 읽고 되돌립니다.",
			Default: "정리나 분리가 요청이었다면 승인하세요. 아니면 반려가 안전합니다.",
		}
	case topology.UnitRemoved:
		return Question{
			Title:   fmt.Sprintf("%s이(가) 코드에서 사라졌습니다.", name(it.Unit)),
			Why:     "의도한 계약이 없어지면 그 계약을 믿는 쪽이 모두 영향을 받습니다.",
			Approve: "의도에서 이 단위와 그 결합을 지웁니다.",
			Reject:  "단위를 되살려야 한다는 뜻입니다. 에이전트가 이유를 읽고 되돌립니다.",
			Default: "삭제가 요청이었다면 승인하세요. 아니면 반려가 안전합니다.",
		}
	}
	return Question{Title: it.Key(), Why: "의도한 경계에 닿는 변경입니다."}
}

// diffSignature compares two signatures member by member. A trait's members
// end in ";" and a record's fields in ",", so a one-method change shows as one
// line rather than two copies of the whole contract.
func diffSignature(old, new string) ([]DiffLine, int) {
	if !strings.Contains(old+new, ";") {
		return around(old, new), 0
	}
	a, b := members(old), members(new)
	// Longest common subsequence over members.
	n, m := len(a), len(b)
	lcs := make([][]int, n+1)
	for i := range lcs {
		lcs[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	var out []DiffLine
	same := 0
	i, j := 0, 0
	for i < n || j < m {
		switch {
		case i < n && j < m && a[i] == b[j]:
			same++
			i++
			j++
		case i < n && (j == m || lcs[i+1][j] >= lcs[i][j+1]):
			out = append(out, DiffLine{Op: "-", Parts: []Span{{Text: a[i]}}})
			i++
		default:
			out = append(out, DiffLine{Op: "+", Parts: []Span{{Text: b[j]}}})
			j++
		}
	}
	// A member that was replaced shows as a removal followed by an addition;
	// marking the words between their common start and end points the eye at
	// the change instead of at two near-identical lines.
	for k := 0; k+1 < len(out); k++ {
		if out[k].Op == "-" && out[k+1].Op == "+" {
			out[k].Parts, out[k+1].Parts = mark(out[k].Parts[0].Text, out[k+1].Parts[0].Text)
			k++
		}
	}
	return out, same
}

// around diffs a signature with no member separator, such as a field list,
// by its common start and end, and shows only the changed middle with a
// little context on each side.
func around(a, b string) []DiffLine {
	ra, rb := []rune(a), []rune(b)
	pre := 0
	for pre < len(ra) && pre < len(rb) && ra[pre] == rb[pre] {
		pre++
	}
	suf := 0
	for suf < len(ra)-pre && suf < len(rb)-pre && ra[len(ra)-1-suf] == rb[len(rb)-1-suf] {
		suf++
	}
	const ctx = 24
	line := func(r []rune, op string) DiffLine {
		from, to := max(0, pre-ctx), min(len(r), len(r)-suf+ctx)
		var parts []Span
		if from > 0 {
			parts = append(parts, Span{Text: "…"})
		}
		parts = append(parts, Span{Text: string(r[from:pre])}, Span{Text: string(r[pre : len(r)-suf]), Mark: true}, Span{Text: string(r[len(r)-suf : to])})
		if to < len(r) {
			parts = append(parts, Span{Text: "…"})
		}
		return DiffLine{Op: op, Parts: parts}
	}
	return []DiffLine{line(ra, "-"), line(rb, "+")}
}

func mark(a, b string) ([]Span, []Span) {
	x, y := strings.Fields(a), strings.Fields(b)
	pre := 0
	for pre < len(x) && pre < len(y) && x[pre] == y[pre] {
		pre++
	}
	suf := 0
	for suf < len(x)-pre && suf < len(y)-pre && x[len(x)-1-suf] == y[len(y)-1-suf] {
		suf++
	}
	split := func(w []string) []Span {
		var out []Span
		add := func(ws []string, marked bool) {
			if len(ws) > 0 {
				out = append(out, Span{Text: strings.Join(ws, " ") + " ", Mark: marked})
			}
		}
		add(w[:pre], false)
		add(w[pre:len(w)-suf], true)
		add(w[len(w)-suf:], false)
		return out
	}
	return split(x), split(y)
}

func members(sig string) []string {
	var out []string
	sep := ','
	if strings.Contains(sig, ";") {
		sep = ';'
	}
	depth, start := 0, 0
	for i, r := range sig {
		switch {
		case r == '>' && i > 0 && sig[i-1] == '-':
			continue
		}
		switch r {
		case '(', '<', '[':
			depth++
		case ')', '>', ']':
			depth = max(0, depth-1)
		case '{':
			depth++
			if depth == 1 {
				if head := strings.TrimSpace(sig[start:i]); head != "" {
					out = append(out, head+" {")
				}
				start = i + 1
			}
		case '}':
			if depth == 1 {
				if m := strings.TrimSpace(sig[start:i]); m != "" {
					out = append(out, m)
				}
				start = i + 1
			}
			depth = max(0, depth-1)
		case sep:
			if depth == 1 {
				if m := strings.TrimSpace(sig[start:i]); m != "" {
					out = append(out, m)
				}
				start = i + 1
			}
		}
	}
	if m := strings.TrimSpace(sig[start:]); m != "" {
		out = append(out, m)
	}
	return out
}
