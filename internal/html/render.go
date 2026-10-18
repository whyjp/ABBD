// Package html renders the Markdown originals into the generated HTML Explorer (PRD ch.08).
// The output is one dependency-free file per page; it opens without any other file (AC-BOOT-04).
package html

import (
	"fmt"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/whyjp/for-my-harness/internal/graph"
	"github.com/whyjp/for-my-harness/internal/model"
	"github.com/whyjp/for-my-harness/internal/ssot"
)

const (
	ExplorerFile = "generated/explorer.html"
	QueueFile    = "generated/confirm-queue.html"
)

type Meta struct {
	Revision    string
	Fingerprint string
	SSOTHash    string
	Freshness   model.Freshness
	Note        string
}

func (m Meta) watermark() string { return m.Revision + "|" + m.Fingerprint + "|" + m.SSOTHash }

var watermarkRe = regexp.MustCompile(`<meta name="fmh-watermark" content="([^"]*)">`)

// ReadWatermark returns revision, fingerprint and SSOT hash embedded in a rendered page.
func ReadWatermark(root string) (rev, fp, hash string, ok bool) {
	b, err := os.ReadFile(ssot.Path(root, ExplorerFile))
	if err != nil {
		return "", "", "", false
	}
	m := watermarkRe.FindSubmatch(b)
	if m == nil {
		return "", "", "", false
	}
	parts := strings.Split(string(m[1]), "|")
	if len(parts) != 3 {
		return "", "", "", false
	}
	return parts[0], parts[1], parts[2], true
}

// PendingItem is one entry of the 확인 대기 list.
type PendingItem struct {
	ID     string `json:"id"`
	Field  string `json:"field"`
	Value  string `json:"value"`
	Origin string `json:"origin"`
	Kind   string `json:"kind"`
}

// Pending lists every UNCONFIRMED semantic item, decision and merge candidate.
func Pending(s *ssot.Snapshot) []PendingItem {
	var out []PendingItem
	for _, id := range model.SortedKeys(s.Features) {
		f := s.Features[id]
		if f.Status == model.ItemRemoved {
			continue
		}
		if f.Intent.Purpose.Pending() {
			out = append(out, PendingItem{id, "purpose", f.Intent.Purpose.Value, string(f.Intent.Purpose.Origin), "feature"})
		}
		for _, inv := range f.Intent.Invariants {
			if inv.Origin != model.UserStated && inv.Confirmation == model.Unconfirmed {
				out = append(out, PendingItem{id, "invariant", inv.Claim, string(inv.Origin), "feature"})
			}
		}
	}
	for _, id := range model.SortedKeys(s.Modules) {
		m := s.Modules[id]
		if m.Status == model.ItemRemoved {
			continue
		}
		if m.Responsibility.Pending() {
			out = append(out, PendingItem{id, "responsibility", m.Responsibility.Value, string(m.Responsibility.Origin), "module"})
		}
		for _, c := range m.ExternalContracts {
			if c.Status == model.ItemRemoved {
				continue
			}
			if c.Compatibility.Pending() {
				out = append(out, PendingItem{c.ID, "compatibility", c.Compatibility.Value, string(c.Compatibility.Origin), "contract"})
			}
			if c.ExternalConsumers == nil && (c.Direction == "exposes" || c.Direction == "produces") {
				out = append(out, PendingItem{c.ID, "external_consumers", "미상", string(model.UnknownOrigin), "contract"})
			}
		}
	}
	for _, id := range model.SortedKeys(s.Decisions) {
		d := s.Decisions[id]
		if d.Confirmation == model.Unconfirmed {
			out = append(out, PendingItem{id, "decision", d.Question + " → " + d.Chosen + " (기본값 진행)", string(d.Rationale.Origin), "decision"})
		}
	}
	for _, c := range s.Candidates {
		if c.Confirmation == model.Unconfirmed {
			out = append(out, PendingItem{c.ID, "merge", c.Name + " ↔ " + c.CandidateOf, "anchors " + fmt.Sprint(len(c.Matched)) + "/" + fmt.Sprint(len(c.Anchors)), "candidate"})
		}
	}
	return out
}

func Render(s *ssot.Snapshot, g *graph.Graph, meta Meta) error {
	pending := Pending(s)
	var b strings.Builder
	page(&b, "For My Harness Explorer · "+s.Project.Name, meta, func() {
		nav(&b)
		banner(&b, s, meta, len(pending))
		section(&b, "pending", "확인 대기 (UNCONFIRMED)", func() { pendingTable(&b, pending) })
		section(&b, "architecture", "Architecture View", func() { architecture(&b, s) })
		section(&b, "contracts", "외부 계약 뷰 (IF / EVT / DATA / CFG / 서빙)", func() { contracts(&b, s) })
		section(&b, "features", "Feature View", func() { features(&b, s) })
		section(&b, "changes", "Change View", func() { changes(&b, s) })
		section(&b, "graph", "Graph View", func() { graphView(&b, s, g) })
		section(&b, "agent", "Agent View", func() { agentView(&b, s) })
	})
	var q strings.Builder
	page(&q, "확인 대기 · "+s.Project.Name, meta, func() {
		banner(&q, s, meta, len(pending))
		fmt.Fprintf(&q, "<p>틈날 때 <code>myharness confirm ID</code> 또는 <code>myharness confirm ID --value \"...\"</code>로 정리한다. 건드리지 않는 추정은 묻지 않는다.</p>")
		pendingTable(&q, pending)
	})
	if err := write(s.Root, ExplorerFile, b.String()); err != nil {
		return err
	}
	return write(s.Root, QueueFile, q.String())
}

// write renders into a staging file and renames it, so a failed render never leaves a
// half-written page that claims a new watermark (PRD ch.07 atomicity).
func write(root, rel, content string) error {
	p := ssot.Path(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	tmp := p + ".staging"
	if err := os.WriteFile(tmp, []byte(content), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

var e = html.EscapeString

const css = `:root{--bg:#fbfaf7;--fg:#1d1d1b;--muted:#6b6a65;--line:#e4e1d8;--card:#fff;--ok:#1f7a4d;--warn:#a05a00;--bad:#b3261e;--info:#2952a3}
@media (prefers-color-scheme:dark){:root{--bg:#161614;--fg:#ecebe6;--muted:#9a988f;--line:#33322d;--card:#1f1f1c;--ok:#5cc28f;--warn:#e0a24a;--bad:#f07a70;--info:#86a8ef}}
*{box-sizing:border-box}body{margin:0;background:var(--bg);color:var(--fg);font:14px/1.55 system-ui,-apple-system,"Segoe UI","Noto Sans KR",sans-serif}
main{max-width:1180px;margin:0 auto;padding:16px}h1{font-size:22px;margin:8px 0}h2{font-size:17px;margin:28px 0 8px;border-bottom:1px solid var(--line);padding-bottom:4px}
nav{position:sticky;top:0;background:var(--bg);padding:8px 0;border-bottom:1px solid var(--line);display:flex;gap:12px;flex-wrap:wrap;z-index:1}nav a{color:var(--info);text-decoration:none}
input{width:100%;padding:8px;border:1px solid var(--line);border-radius:6px;background:var(--card);color:var(--fg);margin:8px 0}
table{border-collapse:collapse;width:100%;background:var(--card);display:block;overflow-x:auto}th,td{border:1px solid var(--line);padding:5px 8px;text-align:left;vertical-align:top}th{color:var(--muted);font-weight:600}
code{font:12px ui-monospace,Consolas,monospace}.b{display:inline-block;padding:0 6px;border-radius:9px;font-size:12px;border:1px solid currentColor;white-space:nowrap}
.ok{color:var(--ok)}.warn{color:var(--warn)}.bad{color:var(--bad)}.muted{color:var(--muted)}.banner{padding:10px;border:1px solid var(--line);border-radius:8px;background:var(--card)}
details{margin:4px 0}.copy{cursor:pointer}`

const js = `document.addEventListener('input',function(ev){if(ev.target.id!=='q')return;var q=ev.target.value.toLowerCase();document.querySelectorAll('[data-s]').forEach(function(r){r.style.display=r.textContent.toLowerCase().indexOf(q)<0?'none':''})});
document.addEventListener('click',function(ev){var t=ev.target.closest('.copy');if(t&&navigator.clipboard){navigator.clipboard.writeText(t.dataset.id)}});`

func page(b *strings.Builder, title string, meta Meta, body func()) {
	fmt.Fprintf(b, "<!doctype html>\n<html lang=\"ko\"><head><meta charset=\"utf-8\"><meta name=\"viewport\" content=\"width=device-width,initial-scale=1\">\n")
	fmt.Fprintf(b, "<meta name=\"fmh-watermark\" content=\"%s\">\n<title>%s</title><style>%s</style></head><body><main>\n", e(meta.watermark()), e(title), css)
	fmt.Fprintf(b, "<h1>%s</h1>\n<input id=\"q\" placeholder=\"ID·이름·selector 검색\">\n", e(title))
	body()
	fmt.Fprintf(b, "</main><script>%s</script></body></html>\n", js)
}

func nav(b *strings.Builder) {
	b.WriteString(`<nav><a href="#pending">확인 대기</a><a href="#architecture">Architecture</a><a href="#contracts">외부 계약</a><a href="#features">Features</a><a href="#changes">Changes</a><a href="#graph">Graph</a><a href="#agent">Agent</a></nav>`)
}

func section(b *strings.Builder, id, title string, body func()) {
	fmt.Fprintf(b, "<h2 id=\"%s\">%s</h2>\n", id, e(title))
	body()
}

func freshClass(f model.Freshness) string {
	switch f {
	case model.Current:
		return "ok"
	case model.Draft:
		return "warn"
	default:
		return "bad"
	}
}

func banner(b *strings.Builder, s *ssot.Snapshot, meta Meta, pending int) {
	fmt.Fprintf(b, "<div class=\"banner\"><b>최신성</b> <span class=\"b %s\">%s</span> · revision <code>%s</code> · fingerprint <code>%s</code> · md <code>%s</code> · 모드 %s · 확인 대기 <b>%d</b>",
		freshClass(meta.Freshness), meta.Freshness, e(meta.Revision), e(meta.Fingerprint), e(meta.SSOTHash), e(s.Project.Mode), pending)
	if c := s.ActiveChange(); c != nil {
		fmt.Fprintf(b, "<br><span class=\"warn\">작업 중 초안 DRAFT: %s · %s · 상태 %s</span> — 아래 DRAFT 항목은 마지막 확정 버전이 아니다.", e(c.ID), e(c.RequestSummary), c.State)
	}
	if meta.Note != "" {
		fmt.Fprintf(b, "<br>%s", e(meta.Note))
	}
	b.WriteString("</div>\n")
}

func semBadge(sem model.Semantic) string {
	cls := "warn"
	switch sem.Badge() {
	case model.BadgeConfirmed:
		cls = "ok"
	case model.BadgeRejected:
		cls = "bad"
	}
	return fmt.Sprintf("<span class=\"b %s\" title=\"origin %s · %s\">%s</span>", cls, sem.Origin, sem.Confirmation, sem.Badge())
}

func verBadge(v model.Verification) string {
	cls := map[model.Verification]string{model.Passed: "ok", model.Failed: "bad", model.Unverifiable: "warn"}[v]
	if cls == "" {
		cls = "muted"
	}
	return fmt.Sprintf("<span class=\"b %s\">%s</span>", cls, v.Badge())
}

func originBadge(o model.Origin) string {
	cls := "muted"
	if o == model.UnknownOrigin || o == model.Inferred {
		cls = "warn"
	}
	return fmt.Sprintf("<span class=\"b %s\">%s</span>", cls, o)
}

func statusBadge(st model.ItemStatus) string {
	switch st {
	case model.ItemDraft:
		return "<span class=\"b warn\">DRAFT</span>"
	case model.ItemRemoved:
		return "<span class=\"b bad\">removed</span>"
	}
	return ""
}

func pendingTable(b *strings.Builder, items []PendingItem) {
	if len(items) == 0 {
		b.WriteString("<p class=\"ok\">확인 대기 항목 없음.</p>\n")
		return
	}
	b.WriteString("<table><tr><th>ID</th><th>필드</th><th>값</th><th>origin</th><th>확인 명령</th></tr>\n")
	for _, p := range items {
		fmt.Fprintf(b, "<tr data-s><td><code>%s</code></td><td>%s</td><td>%s</td><td>%s</td><td><code>myharness confirm %s</code></td></tr>\n", e(p.ID), e(p.Field), e(p.Value), e(p.Origin), e(p.ID))
	}
	b.WriteString("</table>\n")
}

func architecture(b *strings.Builder, s *ssot.Snapshot) {
	b.WriteString("<table><tr><th>ID</th><th>모듈</th><th>책임</th><th>실제 의존</th><th>허용 / 금지</th><th>위반</th><th>계약</th><th>서빙</th><th>테스트</th></tr>\n")
	for _, id := range model.SortedKeys(s.Modules) {
		m := s.Modules[id]
		var violations []string
		forbidden := map[string]bool{}
		for _, f := range m.DependencyPolicy.Forbidden {
			forbidden[f] = true
		}
		for _, d := range m.DependsOn {
			if forbidden[d] {
				violations = append(violations, d)
			}
		}
		var serving []string
		for _, sv := range m.Serving {
			serving = append(serving, sv.DeployUnit)
		}
		fmt.Fprintf(b, "<tr data-s><td><code>%s</code> %s</td><td>%s<br><code class=\"muted\">%s</code></td><td>%s %s</td><td>%s</td><td>%s / %s</td><td class=\"bad\">%s</td><td>%d</td><td>%s</td><td>%d</td></tr>\n",
			e(id), statusBadge(m.Status), e(m.Name), e(m.Path), e(m.Responsibility.Value), semBadge(m.Responsibility),
			e(strings.Join(m.DependsOn, ", ")), e(strings.Join(m.DependencyPolicy.Allowed, ", ")), e(strings.Join(m.DependencyPolicy.Forbidden, ", ")),
			e(strings.Join(violations, ", ")), len(m.ExternalContracts), e(strings.Join(serving, ", ")), len(m.Tests))
	}
	b.WriteString("</table>\n")
}

func contracts(b *strings.Builder, s *ssot.Snapshot) {
	b.WriteString("<table><tr><th>ID</th><th>종류</th><th>방향</th><th>selector</th><th>소유 모듈</th><th>origin</th><th>verification</th><th>호환성</th><th>외부 소비자</th><th>위치</th></tr>\n")
	for _, mid := range model.SortedKeys(s.Modules) {
		m := s.Modules[mid]
		for _, c := range m.ExternalContracts {
			consumers := "미상"
			if c.ExternalConsumers != nil {
				consumers = fmt.Sprint(*c.ExternalConsumers)
			}
			fmt.Fprintf(b, "<tr data-s><td><code>%s</code> %s</td><td>%s</td><td>%s</td><td><code>%s</code></td><td>%s</td><td>%s</td><td>%s <span class=\"muted\">%s</span></td><td>%s %s</td><td>%s</td><td><code>%s</code></td></tr>\n",
				e(c.ID), statusBadge(c.Status), e(c.Kind), e(c.Direction), e(c.Selector), e(mid), originBadge(c.Origin), verBadge(c.Verification), e(c.VerificationNote),
				e(c.Compatibility.Value), semBadge(c.Compatibility), e(consumers), e(c.Location))
		}
		for _, sv := range m.Serving {
			fmt.Fprintf(b, "<tr data-s><td>—</td><td>deploy-unit</td><td>serves</td><td><code>%s</code></td><td>%s</td><td>%s</td><td>—</td><td>—</td><td>—</td><td><code>%s</code></td></tr>\n",
				e(sv.DeployUnit), e(mid), originBadge(sv.Origin), e(sv.Source))
		}
	}
	b.WriteString("</table>\n<p class=\"muted\">도메인 상태 층(DOM/STATE/TRN/INV)은 Phase 3 이후. 운영 중인 실제 상태는 코드로 추측하지 않는다.</p>\n")
}

func features(b *strings.Builder, s *ssot.Snapshot) {
	b.WriteString("<table><tr><th>ID</th><th>기능</th><th>목적</th><th>진입점</th><th>소유</th><th>관찰 (what)</th><th>검증</th><th>synced</th></tr>\n")
	for _, id := range model.SortedKeys(s.Features) {
		f := s.Features[id]
		var entries, obs, tests []string
		for _, ep := range f.EntryPoints {
			entries = append(entries, ep.Kind+" "+ep.Selector)
		}
		for _, o := range f.Observations {
			obs = append(obs, fmt.Sprintf("%s %s", e(o.Behavior), verBadge(o.Verification)))
		}
		for _, inv := range f.Intent.Invariants {
			obs = append(obs, fmt.Sprintf("불변: %s %s", e(inv.Claim), verBadge(inv.Verification)))
		}
		for _, t := range f.Verification {
			tests = append(tests, fmt.Sprintf("<code>%s</code> %s", e(t.TestID), verBadge(t.Verification)))
		}
		fmt.Fprintf(b, "<tr data-s><td><code>%s</code> %s</td><td>%s</td><td>%s %s</td><td><code>%s</code></td><td>%s</td><td>%s</td><td>%s</td><td><code>%s</code></td></tr>\n",
			e(id), statusBadge(f.Status), e(f.Name), e(f.Intent.Purpose.Value), semBadge(f.Intent.Purpose), e(strings.Join(entries, ", ")),
			e(strings.Join(f.OwnedBy, ", ")), strings.Join(obs, "<br>"), strings.Join(tests, "<br>"), e(f.LastSynced.Revision))
	}
	b.WriteString("</table>\n")
}

func changes(b *strings.Builder, s *ssot.Snapshot) {
	b.WriteString("<table><tr><th>CHG</th><th>요청</th><th>출처</th><th>상태</th><th>수정 파일</th><th>영향</th><th>검증</th><th>동기화</th><th>결정</th><th>계측</th></tr>\n")
	ids := model.SortedKeys(s.Changes)
	for i := len(ids) - 1; i >= 0; i-- {
		c := s.Changes[ids[i]]
		var decisions []string
		for _, d := range c.Decisions {
			decisions = append(decisions, fmt.Sprintf("%s(%s,%v)", d.ADR, d.Tier, d.Answered))
		}
		fmt.Fprintf(b, "<tr data-s><td><code>%s</code></td><td>%s</td><td>%s</td><td>%s</td><td><code>%s</code></td><td>%s</td><td>%s <code>%s</code></td><td>map %s · graph %s · html %s</td><td>%s</td><td>model %d · graph %d · test %d · 질문 %d · %ds</td></tr>\n",
			e(c.ID), e(c.RequestSummary), e(c.IntentSource), c.State, e(strings.Join(c.FilesChanged, ", ")), e(strings.Join(c.Impacted, ", ")),
			verBadge(c.Verification), e(strings.Join(c.VerificationEvidence, ", ")), c.SyncState.FeatureMap, c.SyncState.Graph, c.SyncState.HTML,
			e(strings.Join(decisions, ", ")), c.Telemetry.ModelCalls, c.Telemetry.GraphQueries, c.Telemetry.TestRuns, c.Telemetry.QuestionsAsked, c.Telemetry.DurationSec)
	}
	b.WriteString("</table>\n")
}

// graphView shows only the bounded neighborhood of each feature, never the whole graph.
func graphView(b *strings.Builder, s *ssot.Snapshot, g *graph.Graph) {
	if g == nil {
		b.WriteString("<p class=\"bad\">그래프 없음 (UNKNOWN). <code>myharness sync</code>로 생성.</p>\n")
		return
	}
	fmt.Fprintf(b, "<p class=\"muted\">노드 %d · 간선 %d · graph revision <code>%s</code>. 기능별 2단계 이웃만 펼친다.</p>\n", len(g.Nodes), len(g.Edges), e(g.Revision))
	for _, id := range model.SortedKeys(s.Features) {
		sub := g.Neighbors(id, 2, 40)
		fmt.Fprintf(b, "<details data-s><summary><code>%s</code> %s (%d edges)</summary><table><tr><th>from</th><th>type</th><th>to</th><th>origin</th><th>confidence</th><th>source</th></tr>", e(id), e(s.Features[id].Name), len(sub.Edges))
		for _, ed := range sub.Edges {
			fmt.Fprintf(b, "<tr><td><code>%s</code></td><td>%s</td><td><code>%s</code></td><td>%s</td><td>%s</td><td><code>%s</code></td></tr>", e(ed.From), e(ed.Type), e(ed.To), originBadge(ed.Origin), e(ed.Confidence), e(ed.SourceLocation))
		}
		if sub.Truncated {
			b.WriteString("<tr><td colspan=6 class=\"muted\">… 생략 (myharness graph neighbors로 조회)</td></tr>")
		}
		b.WriteString("</table></details>\n")
	}
}

func agentView(b *strings.Builder, s *ssot.Snapshot) {
	b.WriteString("<p class=\"muted\">ID를 누르면 복사된다. MCP: get_feature_context, get_architecture_context, trace_implementation.</p><p>")
	for _, id := range s.AllIDs() {
		fmt.Fprintf(b, "<code class=\"copy\" data-s data-id=\"%s\">%s</code> ", e(id), e(id))
	}
	b.WriteString("</p>\n")
}
