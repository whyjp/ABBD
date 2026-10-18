// Package intent routes a request and links it to current knowledge (PRD ch.04 steps 1-3).
// Routing is a table of task types; resolution is token overlap with evidence. Both are simple
// heuristics on purpose: PRD appendix B says tune them only once evaluation data exists.
package intent

import (
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/ssot"
)

type TaskType struct {
	Name     string
	Keywords []string
	Tier     string
	Steps    []string
}

// Types is ordered: the first matching type wins, so specific types precede the default.
var Types = []TaskType{
	{"docs", []string{"문서", "readme", "docs", "주석", "documentation"}, "T0", []string{
		"원본 SSOT(md)인지 생성 HTML인지 식별", "원본 수정", "abbd render", "abbd check로 코드·계약 증거 점검", "abbd sync"}},
	{"bug-fix", []string{"버그", "bug", "fix", "고쳐", "오류", "에러", "error", "crash", "fail", "깨짐", "안됨"}, "T1", []string{
		"관련 feature/contract 조회 (abbd inspect)", "실패 재현 테스트 작성", "수정", "관련 테스트 (abbd verify --affected)", "abbd sync"}},
	{"perf", []string{"성능", "perf", "latency", "느림", "느려", "빠르게", "throughput", "최적화"}, "T2", []string{
		"병목·기준 환경 확인", "설계 불변조건 조회", "동일 조건 실측 루프", "비교 결과와 결정 기록", "abbd verify --affected", "abbd sync"}},
	{"interface-change", []string{"api 변경", "endpoint", "엔드포인트", "topic", "토픽", "schema", "스키마", "interface", "인터페이스", "호환", "breaking", "migration", "마이그레이션"}, "T2", []string{
		"구조 이해 (abbd inspect / graph neighbors)", "의도 충돌 확인", "abbd contracts impact (등급 B/C 트리거)", "인터페이스 계획", "코드", "폭넓은 회귀 (abbd verify --all)", "abbd sync"}},
	{"refactor", []string{"리팩터", "refactor", "rename", "이름 변경", "정리", "이동", "move", "분리", "extract"}, "T1", []string{
		"현재 경계와 의존 정책 조회", "동작 보존 범위 명시", "작은 단위 변경", "abbd verify --affected", "abbd sync (안정 ID 유지 확인)"}},
	{"feature", nil, "T1", []string{
		"기능 계약·시나리오·검증 계획 작성 (abbd intent feature)", "abbd impact", "코드·테스트 생성", "abbd verify --affected", "abbd sync (Feature 신규 ID와 Architecture 연결 확정)"}},
}

// riskWords raise the verification tier to T3 (state, concurrency, data).
var riskWords = []string{"동시성", "concurrency", "race", "재시도", "retry", "트랜잭션", "transaction", "데이터 삭제", "중복", "idempot", "lock"}

type Match struct {
	ID     string   `json:"id"`
	Kind   string   `json:"kind"`
	Label  string   `json:"label"`
	Score  int      `json:"score"`
	Tokens []string `json:"matched_tokens"`
}

type Resolution struct {
	Request    string   `json:"request"`
	TaskType   string   `json:"task_type"`
	Tier       string   `json:"verification_tier"`
	Steps      []string `json:"steps"`
	Matches    []Match  `json:"matches"`
	Ambiguous  bool     `json:"ambiguous"`
	NewFeature bool     `json:"new_feature"`
}

func Route(request string) TaskType {
	low := strings.ToLower(request)
	for _, t := range Types {
		for _, k := range t.Keywords {
			if strings.Contains(low, k) {
				return t
			}
		}
	}
	return Types[len(Types)-1]
}

func Resolve(request string, s *ssot.Snapshot) Resolution {
	t := Route(request)
	low := strings.ToLower(request)
	// Adding a new endpoint or topic is a new feature that adds a contract, not a change to
	// an existing contract; it keeps the wider T2 verification of an interface change.
	interfaceAdd := t.Name == "interface-change" && hasAny(low, addWords) && !hasAny(low, modificationWords)
	if interfaceAdd {
		t = Types[len(Types)-1]
	}
	r := Resolution{Request: request, TaskType: t.Name, Tier: t.Tier, Steps: t.Steps}
	if interfaceAdd {
		r.Tier = "T2"
	}
	for _, w := range riskWords {
		if strings.Contains(low, w) {
			r.Tier = "T3"
		}
	}
	tokens := Tokens(request)
	score := func(id, kind, label string, texts ...string) {
		hay := strings.ToLower(strings.Join(texts, " "))
		var hit []string
		for _, tok := range tokens {
			if strings.Contains(hay, tok) {
				hit = append(hit, tok)
			}
		}
		if len(hit) > 0 {
			r.Matches = append(r.Matches, Match{ID: id, Kind: kind, Label: label, Score: len(hit), Tokens: hit})
		}
	}
	for _, id := range model.SortedKeys(s.Features) {
		f := s.Features[id]
		if f.Status == model.ItemRemoved {
			continue
		}
		var sel []string
		for _, e := range f.EntryPoints {
			sel = append(sel, e.Selector)
		}
		score(id, "feature", f.Name, append([]string{f.Name, f.Intent.Purpose.Value}, sel...)...)
	}
	for _, id := range model.SortedKeys(s.Modules) {
		m := s.Modules[id]
		if m.Status == model.ItemRemoved {
			continue
		}
		score(id, "module", m.Name, m.Name, m.Path, m.Responsibility.Value)
		for _, c := range m.ExternalContracts {
			if c.Status != model.ItemRemoved {
				score(c.ID, "contract", c.Selector, c.Selector, c.Kind)
			}
		}
	}
	sort.SliceStable(r.Matches, func(i, j int) bool {
		if r.Matches[i].Score != r.Matches[j].Score {
			return r.Matches[i].Score > r.Matches[j].Score
		}
		return r.Matches[i].ID < r.Matches[j].ID
	})
	var feats []Match
	for _, m := range r.Matches {
		if m.Kind == "feature" {
			feats = append(feats, m)
		}
	}
	r.Ambiguous = len(feats) >= 2 && feats[0].Score == feats[1].Score && t.Name != "feature"
	// A modification request never invents a feature, even when nothing matched: the user is
	// changing something the map does not know yet, not stating a new purpose.
	r.NewFeature = t.Name == "feature" && !hasAny(low, modificationWords)
	if r.NewFeature && len(r.Matches) > 1 && r.Tier == "T1" {
		r.Tier = "T2"
	}
	return r
}

// ruleWords mark a request that sets a rule for the whole project rather than one change.
// A standing rule has a scope beyond this change AND an obligation. Either alone is ordinary
// wording ("절대 경로", "always_on 플래그", "모든 테스트가 통과하도록").
var (
	ruleScope = regexp.MustCompile(`(항상|모든\s|어떤 경로|어디서든|어떤 경우에도|앞으로|\balways\b|\bevery\b|\beverywhere\b|\bany (path|endpoint|route)\b|\ball (http|endpoints|routes|paths|errors)\b|\bfrom now on\b|\btoday or in anything\b|\bcurrent and future\b|\bincluding ones added later\b)`)
	// A scope this strong states a rule on its own ("ids are case-insensitive everywhere").
	ruleStrong = regexp.MustCompile(`(어디서든|어떤 경우에도|\beverywhere\b|\bfrom now on\b|\bincluding ones added later\b|\bcurrent and future\b)`)
	ruleDuty   = regexp.MustCompile(`(해야|하여야|돼야|되어야|어야 한|아야 한|하지 않|않는다|안 된다|금지|반드시|\bmust\b|\bshould\b|\bnever\b|\bshall\b|\bare the same\b|\bis rejected\b|\bmust not\b|\bonly\b)`)
)

// IsProjectRule reports whether a sentence reads as a standing rule.
func IsProjectRule(request string) bool {
	low := strings.ToLower(request)
	return ruleStrong.MatchString(low) || ruleScope.MatchString(low) && ruleDuty.MatchString(low)
}

// RuleSentences returns the sentences of a message that state a project-wide rule, so one
// message with several rules records each, and a rule buried in a feature request is kept.
func RuleSentences(message string) []string {
	var out []string
	for _, s := range splitSentences(message) {
		if IsProjectRule(s) {
			out = append(out, s)
		}
	}
	return out
}

func splitSentences(s string) []string {
	var out []string
	var cur strings.Builder
	runes := []rune(s)
	for i, r := range runes {
		cur.WriteRune(r)
		end := r == '\n' || ((r == '.' || r == '!' || r == '?' || r == '。') && (i+1 == len(runes) || runes[i+1] == ' ' || runes[i+1] == '\n'))
		if end {
			if t := strings.TrimSpace(cur.String()); t != "" {
				out = append(out, t)
			}
			cur.Reset()
		}
	}
	if t := strings.TrimSpace(cur.String()); t != "" {
		out = append(out, t)
	}
	return out
}

// SameRule: two rule texts that share most of their words are one rule stated twice.
func SameRule(a, b string) bool {
	ta, tb := Tokens(a), Tokens(b)
	if len(ta) == 0 || len(tb) == 0 {
		return false
	}
	set := map[string]bool{}
	for _, t := range tb {
		set[t] = true
	}
	common := 0
	for _, t := range ta {
		if set[t] {
			common++
		}
	}
	small := len(ta)
	if len(tb) < small {
		small = len(tb)
	}
	return float64(common) >= 0.7*float64(small)
}

var (
	modificationWords = []string{"변경", "수정", "개선", "바꿔", "change", "update", "modify", "improve"}
	addWords          = []string{"추가", "만들", "생성", "신규", "add", "new", "create", "implement", "구현", "지원"}
)

func hasAny(low string, words []string) bool {
	for _, w := range words {
		if strings.Contains(low, w) {
			return true
		}
	}
	return false
}

var stop = map[string]bool{"the": true, "and": true, "for": true, "with": true, "만들어줘": true, "해줘": true, "추가": true, "기본": true}

// Tokens lowercases and splits on anything that is not a letter or digit; Korean particles
// are trimmed by keeping tokens of two or more runes and stripping one trailing particle.
func Tokens(s string) []string {
	f := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	seen := map[string]bool{}
	var out []string
	for _, t := range f {
		t = trimParticle(t)
		if len([]rune(t)) < 2 || stop[t] || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	return out
}

var particles = []string{"으로", "에서", "와", "과", "을", "를", "이", "가", "은", "는", "의", "에", "로", "도"}

func trimParticle(t string) string {
	for _, p := range particles {
		if strings.HasSuffix(t, p) && len([]rune(t)) > len([]rune(p))+1 {
			return strings.TrimSuffix(t, p)
		}
	}
	return t
}
