package review

import (
	"fmt"
	"html"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/ssot"
)

const File = "generated/review.html"

// Cell geometry of the SVG grid.
const (
	cellW, cellH = 200, 96
	nodeW, nodeH = 168, 56
	pad          = 24
)

type node struct {
	ID, Kind, Label, Sub string
	Inferred, Unknown    bool
	Href                 string
	cell                 Cell
	removed              bool
}

type edge struct {
	From, To, Kind    string
	Forbidden, Dashed bool
	Unknown, Removed  bool
	key               string
}

type view struct {
	Name, Title string
	Nodes       []node
	Edges       []edge
}

// Target picks the turn the review screen overlays: the active turn, else the latest turn with
// structural deltas. Empty when no turn has any.
func Target(s *ssot.Snapshot) *model.Change {
	if c := s.ActiveChange(); c != nil && len(c.StructuralDeltas) > 0 {
		return c
	}
	ids := model.SortedKeys(s.Changes)
	for i := len(ids) - 1; i >= 0; i-- {
		if len(s.Changes[ids[i]].StructuralDeltas) > 0 {
			return s.Changes[ids[i]]
		}
	}
	return nil
}

// Write renders review.html for the target turn and persists the layout. revision is the
// workspace fingerprint; it decides how long a removed node keeps its cell.
func Write(s *ssot.Snapshot, revision string) error {
	l := LoadLayout(s.Root)
	page := Render(s, Target(s), l, revision, "")
	if err := l.Save(s.Root); err != nil {
		return err
	}
	p := ssot.Path(s.Root, File)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	tmp := p + ".staging"
	if err := os.WriteFile(tmp, []byte(page), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

// Render is deterministic: the same SSOT, layout and revision give the same page (FR-30).
// The agent never writes topology; it is drawn from data. token, when set, is the abbd serve
// session token that lets the page post decisions.
func Render(s *ssot.Snapshot, target *model.Change, l *Layout, revision, token string) string {
	var deltas []model.StructuralDelta
	if target != nil {
		deltas = target.StructuralDeltas
	}
	views := []view{systemView(s), moduleView(s), featureView(s)}
	var b strings.Builder
	b.WriteString("<!doctype html>\n<html lang=\"ko\"><head><meta charset=\"utf-8\"><meta name=\"viewport\" content=\"width=device-width,initial-scale=1\">\n")
	fmt.Fprintf(&b, "<title>ABBD Review</title><link rel=\"icon\" href=\"data:,\"><meta name=\"abbd-revision\" content=\"%s\">", esc(revision))
	if target != nil {
		fmt.Fprintf(&b, "<meta name=\"abbd-review\" content=\"%s\">", esc(target.ID))
	}
	if token != "" {
		fmt.Fprintf(&b, "<meta name=\"abbd-serve-token\" content=\"%s\">", esc(token))
	}
	fmt.Fprintf(&b, "<style>%s</style></head><body><main>\n", reviewCSS)
	fmt.Fprintf(&b, "<header><h1>ABBD Review · %s</h1>", esc(s.Project.Name))
	if target == nil {
		b.WriteString("<p class=\"muted\">구조 변경이 기록된 턴이 없다. 토폴로지는 현재 설계도다.</p>")
	} else {
		fmt.Fprintf(&b, "<p><b>%s</b> %s · 상태 %s · 구조 변경 %d건</p>", esc(target.ID), esc(target.RequestSummary), target.State, len(deltas))
	}
	b.WriteString("<div class=\"legend\"><span class=\"lg ext\">EXTRACTED 실선</span><span class=\"lg inf\">추정 점선</span><span class=\"lg unk\">UNKNOWN ?</span><span class=\"lg forb\">금지 의존</span><span class=\"lg add\">추가</span><span class=\"lg rem\">삭제</span><span class=\"lg chg\">변경</span></div>")
	b.WriteString("<nav class=\"zoom\">")
	for i, v := range views {
		cls := ""
		if i == 1 {
			cls = " on"
		}
		fmt.Fprintf(&b, "<button class=\"zb%s\" data-view=\"%s\">%s</button>", cls, v.Name, esc(v.Title))
	}
	b.WriteString("<label><input type=\"checkbox\" id=\"showall\"> 전체 보기</label></nav></header>\n<div class=\"wrap\"><section class=\"canvas\">")
	affected := affectedNodes(deltas)
	for i, v := range views {
		renderView(&b, v, l, revision, deltas, affected, i == 1)
	}
	b.WriteString("</section><aside class=\"cards\">")
	cards(&b, target, deltas)
	b.WriteString("</aside></div></main>")
	fmt.Fprintf(&b, "<script>%s</script></body></html>\n", reviewJS)
	return b.String()
}

func esc(s string) string { return html.EscapeString(s) }

func systemView(s *ssot.Snapshot) view {
	v := view{Name: "system", Title: "시스템"}
	units := map[string][]string{}
	for _, id := range model.SortedKeys(s.Modules) {
		m := s.Modules[id]
		if m.Status == model.ItemRemoved {
			continue
		}
		for _, sv := range m.Serving {
			units[sv.DeployUnit] = append(units[sv.DeployUnit], id)
		}
	}
	unitOf := map[string][]string{}
	for u, mods := range units {
		v.Nodes = append(v.Nodes, node{ID: "unit:" + u, Kind: "unit", Label: u, Sub: strings.Join(mods, " ")})
		for _, m := range mods {
			unitOf[m] = append(unitOf[m], "unit:"+u)
		}
	}
	for _, mid := range model.SortedKeys(s.Modules) {
		m := s.Modules[mid]
		if m.Status == model.ItemRemoved {
			continue
		}
		holders := unitOf[mid]
		for _, c := range m.ExternalContracts {
			if c.Status == model.ItemRemoved {
				continue
			}
			if len(holders) == 0 {
				holders = []string{mid}
				v.Nodes = appendOnce(v.Nodes, node{ID: mid, Kind: "module", Label: m.Name, Sub: m.Path, Inferred: m.Responsibility.Pending()})
			}
			v.Nodes = appendOnce(v.Nodes, contractNode(c))
			for _, h := range holders {
				from, to := h, c.ID
				if c.Direction == "consumes" {
					from, to = c.ID, h
				}
				v.Edges = append(v.Edges, edge{From: from, To: to, Kind: c.Direction, Dashed: c.Origin == model.Inferred, Unknown: c.Origin == model.UnknownOrigin})
			}
		}
	}
	return v
}

func contractNode(c model.Contract) node {
	return node{ID: c.ID, Kind: "contract-" + contractShape(c.Kind), Label: c.Selector, Sub: c.ID + " · " + c.Kind,
		Inferred: c.Compatibility.Pending(), Unknown: c.Origin == model.UnknownOrigin, Href: c.Location}
}

func contractShape(kind string) string {
	switch kind {
	case "kafka-topic", "queue", "webhook":
		return "topic"
	case "db-table", "db-view", "cache-key":
		return "data"
	case "env-var", "config-key":
		return "config"
	default:
		return "api"
	}
}

func appendOnce(ns []node, n node) []node {
	for _, x := range ns {
		if x.ID == n.ID {
			return ns
		}
	}
	return append(ns, n)
}

func moduleView(s *ssot.Snapshot) view {
	v := view{Name: "module", Title: "모듈"}
	for _, id := range model.SortedKeys(s.Modules) {
		m := s.Modules[id]
		if m.Status == model.ItemRemoved {
			continue
		}
		v.Nodes = append(v.Nodes, node{ID: id, Kind: "module", Label: m.Name, Sub: id + " · " + m.Path, Inferred: m.Responsibility.Pending(), Href: m.Path})
		for _, d := range m.DependsOn {
			v.Edges = append(v.Edges, edge{From: id, To: d, Kind: "depends", Forbidden: forbidden(m, d), key: EdgeKey(id, d)})
		}
		for _, c := range m.ExternalContracts {
			if c.Status == model.ItemRemoved {
				continue
			}
			v.Nodes = append(v.Nodes, contractNode(c))
			v.Edges = append(v.Edges, edge{From: id, To: c.ID, Kind: c.Direction, Dashed: c.Origin == model.Inferred, Unknown: c.Origin == model.UnknownOrigin})
		}
	}
	return v
}

// featureView draws one lane per feature: feature -> modules -> contracts -> tests. Lane nodes
// are keyed per feature so a module shared by two features appears in both lanes.
func featureView(s *ssot.Snapshot) view {
	v := view{Name: "feature", Title: "기능 경로"}
	for _, fid := range model.SortedKeys(s.Features) {
		f := s.Features[fid]
		if f.Status == model.ItemRemoved {
			continue
		}
		v.Nodes = append(v.Nodes, node{ID: fid, Kind: "feature", Label: f.Name, Sub: fid, Inferred: f.Intent.Purpose.Pending()})
		prev := []string{fid}
		var mods []string
		for _, mid := range f.OwnedBy {
			if m := s.Modules[mid]; m != nil {
				key := fid + "|" + mid
				v.Nodes = append(v.Nodes, node{ID: key, Kind: "module", Label: m.Name, Sub: mid})
				v.Edges = append(v.Edges, edge{From: fid, To: key, Kind: "owned_by"})
				mods = append(mods, key)
			}
		}
		if len(mods) > 0 {
			prev = mods
		}
		var cs []string
		for _, cid := range f.ExternalContracts {
			if c, _ := s.Contract(cid); c != nil {
				key := fid + "|" + cid
				n := contractNode(*c)
				n.ID = key
				v.Nodes = append(v.Nodes, n)
				for _, p := range prev {
					v.Edges = append(v.Edges, edge{From: p, To: key, Kind: c.Direction})
				}
				cs = append(cs, key)
			}
		}
		if len(cs) > 0 {
			prev = cs
		}
		for _, t := range f.Verification {
			key := fid + "|" + t.TestID
			label := t.TestID
			if tt := s.Tests[t.TestID]; tt != nil {
				label = tt.Name
			}
			v.Nodes = append(v.Nodes, node{ID: key, Kind: "test " + strings.ToLower(string(t.Verification)), Label: label, Sub: t.TestID + " · " + string(t.Verification)})
			v.Edges = append(v.Edges, edge{From: prev[0], To: key, Kind: "verified_by"})
		}
	}
	return v
}

// wants assigns preferred cells for new nodes: module layers by dependency depth, contracts in
// rows under their owner, feature lanes one row per feature.
func wants(v view) []Want {
	var out []Want
	switch v.Name {
	case "module":
		depth := moduleDepths(v)
		maxD := 0
		for _, d := range depth {
			if d > maxD {
				maxD = d
			}
		}
		col := map[string]int{}
		var mods []string
		for _, n := range v.Nodes {
			if n.Kind == "module" {
				mods = append(mods, n.ID)
			}
		}
		sort.Strings(mods)
		for i, m := range mods {
			col[m] = i
			out = append(out, Want{ID: m, Row: depth[m], Col: i})
		}
		owner := map[string]string{}
		for _, e := range v.Edges {
			if e.Kind != "depends" {
				owner[e.To] = e.From
			}
		}
		for _, n := range v.Nodes {
			if n.Kind != "module" {
				out = append(out, Want{ID: n.ID, Row: maxD + 1 + shapeRow(n.Kind), Col: col[owner[n.ID]]})
			}
		}
	case "system":
		for _, n := range v.Nodes {
			row := 0
			if strings.HasPrefix(n.Kind, "contract") {
				row = 1 + shapeRow(n.Kind)
			}
			out = append(out, Want{ID: n.ID, Row: row})
		}
	default:
		lane, col := -1, 0
		for _, n := range v.Nodes {
			if n.Kind == "feature" {
				lane++
				col = 0
			} else {
				col++
			}
			out = append(out, Want{ID: n.ID, Row: lane, Col: col})
		}
	}
	return out
}

func shapeRow(kind string) int {
	switch kind {
	case "contract-api", "contract-topic":
		return 0
	default:
		return 1
	}
}

// moduleDepths layers modules so that a module sits above the modules it depends on.
func moduleDepths(v view) map[string]int {
	deps := map[string][]string{}
	for _, e := range v.Edges {
		if e.Kind == "depends" {
			deps[e.To] = append(deps[e.To], e.From)
		}
	}
	depth := map[string]int{}
	var visit func(id string, seen map[string]bool) int
	visit = func(id string, seen map[string]bool) int {
		if d, ok := depth[id]; ok {
			return d
		}
		if seen[id] {
			return 0
		}
		seen[id] = true
		d := 0
		for _, up := range deps[id] {
			if x := visit(up, seen) + 1; x > d {
				d = x
			}
		}
		depth[id] = d
		return d
	}
	for _, n := range v.Nodes {
		if n.Kind == "module" {
			visit(n.ID, map[string]bool{})
		}
	}
	return depth
}

func affectedNodes(deltas []model.StructuralDelta) map[string]string {
	out := map[string]string{}
	for _, d := range deltas {
		cls := "chg"
		switch d.Kind {
		case model.ModuleAdded, model.ContractAdded:
			cls = "add"
		case model.ModuleRemoved, model.ContractRemoved:
			cls = "rem"
		}
		for i, n := range d.Nodes {
			if i == 0 || out[n] == "" {
				if i == 0 {
					out[n] = cls
				} else {
					out[n] = "touch"
				}
			}
		}
	}
	return out
}

func renderView(b *strings.Builder, v view, l *Layout, revision string, deltas []model.StructuralDelta, affected map[string]string, visible bool) {
	cells, gone := l.Place(v.Name, wants(v), revision)
	edgeClass := map[string]string{}
	var removedEdges []edge
	for _, d := range deltas {
		switch d.Kind {
		case model.DependencyAdded, model.ForbiddenDependency:
			edgeClass[d.Subject] = "add"
		case model.DependencyRemoved:
			parts := strings.SplitN(d.Subject, "->", 2)
			removedEdges = append(removedEdges, edge{From: parts[0], To: parts[1], Kind: "depends", Removed: true})
		}
	}
	// Neighbours of affected nodes stay lit; everything else dims unless "전체 보기" is on.
	lit := map[string]bool{}
	for id := range affected {
		lit[id] = true
	}
	for _, e := range v.Edges {
		if affected[base(e.From)] != "" || affected[base(e.To)] != "" {
			lit[e.From], lit[e.To] = true, true
		}
	}
	maxC, maxR := 0, 0
	for _, c := range cells {
		maxC, maxR = max(maxC, c.Col), max(maxR, c.Row)
	}
	for _, c := range gone {
		maxC, maxR = max(maxC, c.Col), max(maxR, c.Row)
	}
	w, h := pad*2+(maxC+1)*cellW, pad*2+(maxR+1)*cellH
	hidden := ""
	if !visible {
		hidden = " hidden"
	}
	fmt.Fprintf(b, "<div class=\"view\"%s data-view=\"%s\"><svg viewBox=\"0 0 %d %d\" width=\"%d\" height=\"%d\" role=\"img\" aria-label=\"%s 토폴로지\">", hidden, v.Name, w, h, w, h, esc(v.Title))
	b.WriteString(`<defs><marker id="ar-` + v.Name + `" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse"><path d="M0,0 L10,5 L0,10 z" class="arrow"/></marker></defs>`)
	center := func(c Cell) (int, int) { return pad + c.Col*cellW + nodeW/2, pad + c.Row*cellH + nodeH/2 }
	dim := len(affected) > 0
	drawEdge := func(e edge, cls string) {
		fc, ok1 := cells[e.From]
		tc, ok2 := cells[e.To]
		if !ok1 {
			fc, ok1 = gone[e.From]
		}
		if !ok2 {
			tc, ok2 = gone[e.To]
		}
		if !ok1 || !ok2 {
			return
		}
		x1, y1 := center(fc)
		x2, y2 := center(tc)
		classes := []string{"edge", "k-" + e.Kind}
		if e.Forbidden {
			classes = append(classes, "forb")
		}
		if e.Dashed {
			classes = append(classes, "inf")
		}
		if e.Unknown {
			classes = append(classes, "unk")
		}
		if cls != "" {
			classes = append(classes, cls)
		}
		if dim && !(lit[e.From] && lit[e.To]) {
			classes = append(classes, "dim")
		}
		fmt.Fprintf(b, "<line class=\"%s\" data-from=\"%s\" data-to=\"%s\" x1=\"%d\" y1=\"%d\" x2=\"%d\" y2=\"%d\" marker-end=\"url(#ar-%s)\"/>", strings.Join(classes, " "), esc(base(e.From)), esc(base(e.To)), x1, y1, x2, y2, v.Name)
		if e.Unknown {
			fmt.Fprintf(b, "<text class=\"unkq\" x=\"%d\" y=\"%d\">?</text>", (x1+x2)/2, (y1+y2)/2)
		}
	}
	for _, e := range v.Edges {
		drawEdge(e, edgeClass[e.key])
	}
	for _, e := range removedEdges {
		drawEdge(e, "rem")
	}
	draw := func(n node, c Cell, removed bool) {
		x, y := pad+c.Col*cellW, pad+c.Row*cellH
		classes := []string{"node", strings.ReplaceAll(n.Kind, " ", " t-")}
		if n.Inferred {
			classes = append(classes, "inf")
		}
		if n.Unknown {
			classes = append(classes, "unk")
		}
		if a := affected[base(n.ID)]; a != "" && a != "touch" {
			classes = append(classes, a)
		}
		if removed {
			classes = append(classes, "rem")
		}
		if dim && !lit[n.ID] && !lit[base(n.ID)] && !removed {
			classes = append(classes, "dim")
		}
		fmt.Fprintf(b, "<g class=\"%s\" data-node=\"%s\" transform=\"translate(%d %d)\"><title>%s</title>", strings.Join(classes, " "), esc(base(n.ID)), x, y, esc(n.Sub))
		b.WriteString(shape(n.Kind))
		fmt.Fprintf(b, "<text class=\"lbl\" x=\"%d\" y=\"24\">%s</text><text class=\"sub\" x=\"%d\" y=\"42\">%s</text>", nodeW/2, esc(trunc(n.Label, 22)), nodeW/2, esc(trunc(n.Sub, 26)))
		if n.Inferred {
			b.WriteString(`<g class="badge"><rect x="118" y="-9" width="44" height="18" rx="9"/><text x="140" y="4">추정</text></g>`)
		}
		b.WriteString("</g>")
	}
	for _, n := range v.Nodes {
		draw(n, cells[n.ID], false)
	}
	for _, id := range model.SortedKeys(gone) {
		draw(node{ID: id, Kind: "gone", Label: base(id), Sub: "삭제됨"}, gone[id], true)
	}
	b.WriteString("</svg></div>")
}

// base strips the feature-lane prefix so lane copies highlight with their original node.
func base(id string) string {
	if i := strings.Index(id, "|"); i >= 0 {
		return id[i+1:]
	}
	return id
}

func shape(kind string) string {
	switch {
	case kind == "unit":
		return fmt.Sprintf(`<rect class="sh" x="-6" y="-6" width="%d" height="%d" rx="14"/>`, nodeW+12, nodeH+12)
	case kind == "contract-topic":
		return fmt.Sprintf(`<rect class="sh" width="%d" height="%d" rx="28"/>`, nodeW, nodeH)
	case kind == "contract-data":
		return fmt.Sprintf(`<path class="sh" d="M0,10 Q%d,-6 %d,10 L%d,%d Q%d,%d 0,%d Z"/>`, nodeW/2, nodeW, nodeW, nodeH-6, nodeW/2, nodeH+8, nodeH-6)
	case kind == "contract-config":
		return fmt.Sprintf(`<path class="sh" d="M12,0 L%d,0 L%d,%d L%d,%d L0,%d Z"/>`, nodeW, nodeW-12, nodeH, 0, nodeH, nodeH/2)
	case kind == "contract-api":
		return fmt.Sprintf(`<path class="sh" d="M14,0 L%d,0 L%d,%d L%d,%d L0,%d Z"/>`, nodeW-14, nodeW, nodeH/2, nodeW-14, nodeH, nodeH/2)
	case kind == "feature":
		return fmt.Sprintf(`<rect class="sh" width="%d" height="%d" rx="6"/><rect class="bar" width="6" height="%d" rx="3"/>`, nodeW, nodeH, nodeH)
	default:
		return fmt.Sprintf(`<rect class="sh" width="%d" height="%d" rx="4"/>`, nodeW, nodeH)
	}
}

func trunc(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

var kindLabel = map[model.DeltaKind]string{
	model.ModuleAdded: "모듈 추가", model.ModuleRemoved: "모듈 삭제", model.ModuleSplit: "모듈 분할", model.ModuleMerged: "모듈 병합",
	model.DependencyAdded: "의존 추가", model.DependencyRemoved: "의존 제거", model.ForbiddenDependency: "금지 의존",
	model.ContractAdded: "계약 추가", model.ContractRemoved: "계약 삭제", model.ContractCompatChange: "계약 호환성 변경",
	model.OwnershipMoved: "소유 이동", model.DeployMappingChanged: "배포 매핑 변경", model.FeaturePathChanged: "기능 경로 변경",
}

// cards: B and C items are cards with decisions; A items are an applied list with a revert
// request (PRD v1.5 Q21 default).
func cards(b *strings.Builder, target *model.Change, deltas []model.StructuralDelta) {
	if target == nil || len(deltas) == 0 {
		b.WriteString("<h2>구조 변경 0건</h2><p class=\"muted\">이번 턴은 모듈 안쪽만 바뀌었다. 결정할 카드가 없다.</p>")
		return
	}
	group := func(title string, tier model.Tier) {
		var ds []model.StructuralDelta
		for _, d := range deltas {
			if d.Tier == tier {
				ds = append(ds, d)
			}
		}
		if len(ds) == 0 {
			return
		}
		fmt.Fprintf(b, "<h2>%s · %d</h2>", esc(title), len(ds))
		for _, d := range ds {
			card(b, target, d)
		}
	}
	group("C · 승인해야 실행", model.TierC)
	group("B · 기본값으로 적용됨, 결정 대기", model.TierB)
	group("A · 적용됨", model.TierA)
	b.WriteString(`<div id="pending" hidden><p>결정 <b id="pcount">0</b>건이 아직 하네스에 반영되지 않았다.</p><button id="download">결정 파일 내려받기</button><p class="muted">반영: <code>abbd review --apply abbd-decisions.json</code></p></div>`)
}

func card(b *strings.Builder, target *model.Change, d model.StructuralDelta) {
	fmt.Fprintf(b, "<article class=\"card tier-%s st-%s\" data-delta=\"%s\" data-nodes=\"%s\">", d.Tier, d.Status, esc(d.ID), esc(strings.Join(d.Nodes, " ")))
	fmt.Fprintf(b, "<header><span class=\"tier\">%s</span> <b>%s</b> <code>%s</code> <span class=\"status\">%s</span></header>", d.Tier, esc(kindLabel[d.Kind]), esc(d.Subject), esc(statusLabel(d.Status)))
	if d.Before != "" || d.After != "" {
		fmt.Fprintf(b, "<p class=\"diff\"><del>%s</del> → <ins>%s</ins></p>", esc(orDash(d.Before)), esc(orDash(d.After)))
	}
	fmt.Fprintf(b, "<p>%s", esc(d.Reason))
	if d.Inferred {
		b.WriteString(" · <span class=\"inftag\">새 추정 항목</span>")
	}
	b.WriteString("</p>")
	fmt.Fprintf(b, "<p class=\"why\">왜: %s</p>", esc(strings.Join(target.IntentDelta.Additions, " / ")))
	fmt.Fprintf(b, "<p class=\"ev\">근거: %s %s</p>", target.Verification, esc(strings.Join(target.VerificationEvidence, ", ")))
	if dec := d.Decision; dec != nil {
		fmt.Fprintf(b, "<p class=\"dec\">결정 %s · %s · %s · revision %s", esc(dec.Decision), esc(dec.Channel), esc(dec.At), esc(dec.Revision))
		if dec.Reason != "" {
			fmt.Fprintf(b, " · 사유: %s", esc(dec.Reason))
		}
		if dec.Invalidated != "" {
			fmt.Fprintf(b, " · <b>무효</b>: %s에서 같은 항목이 다시 바뀜", esc(dec.Invalidated))
		}
		b.WriteString("</p>")
	}
	if open(d) {
		b.WriteString("<div class=\"act\">")
		switch d.Tier {
		case model.TierA:
			b.WriteString(`<button data-decision="revert">되돌리기 요청</button>`)
		case model.TierC:
			b.WriteString(`<button data-decision="approve" class="pri">승인 (실행)</button><button data-decision="reject">거부</button>`)
		default:
			b.WriteString(`<button data-decision="approve" class="pri">승인</button><button data-decision="reject">거부</button>`)
			if d.Kind == model.ContractCompatChange || d.Kind == model.ContractRemoved {
				b.WriteString(`<button data-decision="option:compat">호환 경로 유지</button>`)
			}
		}
		b.WriteString(`<input class="reason" placeholder="사유 (거부 시 다음 턴이 받는다)"></div>`)
	}
	b.WriteString("</article>")
}

func open(d model.StructuralDelta) bool {
	switch d.Status {
	case model.DeltaApplied, model.DeltaDefaultApplied, model.DeltaPending:
		return true
	}
	return false
}

func statusLabel(s model.DeltaStatus) string {
	return map[model.DeltaStatus]string{
		model.DeltaApplied: "적용됨", model.DeltaDefaultApplied: "기본값 적용, 결정 대기", model.DeltaPending: "대기 중 (미실행)",
		model.DeltaApproved: "승인됨", model.DeltaRejected: "거부됨", model.DeltaRevertAsked: "되돌리기 요청됨", model.DeltaSuperseded: "무효 (다시 바뀜)",
	}[s]
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

const reviewCSS = `:root{--bg:#f7f6f2;--fg:#1d1d1b;--muted:#6b6a65;--line:#d9d6cc;--card:#fff;--node:#fff;--edge:#8a877d;--add:#1f8a4c;--rem:#c0392b;--chg:#d9822b;--inf:#7a6f9b;--forb:#c0392b}
@media (prefers-color-scheme:dark){:root:not([data-theme=light]){--bg:#151513;--fg:#ecebe6;--muted:#9a988f;--line:#38362f;--card:#1e1e1b;--node:#22221f;--edge:#8f8c82;--add:#4cc27f;--rem:#ef6b5e;--chg:#f0a24e;--inf:#a99ad6;--forb:#ef6b5e}}
:root[data-theme=dark]{--bg:#151513;--fg:#ecebe6;--muted:#9a988f;--line:#38362f;--card:#1e1e1b;--node:#22221f;--edge:#8f8c82;--add:#4cc27f;--rem:#ef6b5e;--chg:#f0a24e;--inf:#a99ad6;--forb:#ef6b5e}
*{box-sizing:border-box}body{margin:0;background:var(--bg);color:var(--fg);font:14px/1.5 system-ui,"Segoe UI","Noto Sans KR",sans-serif}main{padding:16px}h1{font-size:20px;margin:0 0 4px}h2{font-size:15px;margin:16px 0 8px}
.muted{color:var(--muted)}.legend{display:flex;gap:10px;flex-wrap:wrap;font-size:12px;margin:6px 0}.lg{padding:1px 8px;border-radius:9px;border:1px solid var(--line)}.lg.inf{border-style:dashed;color:var(--inf)}.lg.unk{color:var(--muted)}.lg.forb,.lg.rem{color:var(--rem)}.lg.add{color:var(--add)}.lg.chg{color:var(--chg)}
.zoom{display:flex;gap:6px;align-items:center;margin:8px 0}.zb{border:1px solid var(--line);background:var(--card);color:var(--fg);padding:4px 12px;border-radius:6px;cursor:pointer}.zb.on{border-color:var(--fg);font-weight:600}
.wrap{display:flex;gap:16px;align-items:flex-start}.canvas{flex:1;min-width:0;overflow:auto;border:1px solid var(--line);border-radius:8px;background:var(--card)}.cards{width:380px;max-width:100%}
@media (max-width:900px){.wrap{flex-direction:column}.cards{width:100%}}
.view[hidden]{display:none}svg{display:block}.arrow{fill:var(--edge)}
.node .sh{fill:var(--node);stroke:var(--fg);stroke-width:1.4}.node.inf .sh{stroke-dasharray:6 4;stroke:var(--inf)}.node.unit .sh{fill:none;stroke-width:2}.node.feature .bar{fill:var(--inf)}
.node .lbl{font-size:13px;font-weight:600;text-anchor:middle;fill:var(--fg)}.node .sub{font-size:11px;text-anchor:middle;fill:var(--muted)}
.node .badge rect{fill:var(--inf)}.node .badge text{font-size:10px;fill:#fff;text-anchor:middle}
.node.t-passed .sh{stroke:var(--add)}.node.t-failed .sh{stroke:var(--rem)}
.node.add .sh{stroke:var(--add);stroke-width:3}.node.chg .sh{stroke:var(--chg);stroke-width:3}.node.rem .sh{stroke:var(--rem);stroke-dasharray:4 3}.node.rem .lbl{text-decoration:line-through;fill:var(--rem)}
.edge{stroke:var(--edge);stroke-width:1.4}.edge.inf{stroke-dasharray:6 4;stroke:var(--inf)}.edge.unk{stroke:var(--muted);stroke-dasharray:2 4}.edge.forb{stroke:var(--forb);stroke-width:2.4}
.edge.add{stroke:var(--add);stroke-width:3}.edge.rem{stroke:var(--rem);stroke-dasharray:5 4;stroke-width:2.4}.unkq{fill:var(--muted);font-weight:700}
.dim{opacity:.25}body.all .dim{opacity:1}.hl .sh{stroke:var(--chg)!important;stroke-width:4!important}line.hl{stroke:var(--chg)!important;stroke-width:4}
.card{border:1px solid var(--line);border-radius:8px;background:var(--card);padding:10px;margin:8px 0;cursor:pointer}.card.sel{outline:2px solid var(--chg)}.card header{display:flex;gap:6px;align-items:center;flex-wrap:wrap}
.tier{display:inline-block;min-width:20px;text-align:center;border-radius:4px;color:#fff;background:var(--muted);font-weight:700}.tier-C .tier{background:var(--rem)}.tier-B .tier{background:var(--chg)}.tier-A .tier{background:var(--add)}
.status{margin-left:auto;font-size:12px;color:var(--muted)}.diff del{color:var(--rem)}.diff ins{color:var(--add);text-decoration:none}.why,.ev,.dec{font-size:12px;color:var(--muted);margin:4px 0}.inftag{color:var(--inf)}
.act{display:flex;gap:6px;flex-wrap:wrap;margin-top:6px}.act button{border:1px solid var(--line);background:var(--bg);color:var(--fg);border-radius:6px;padding:4px 10px;cursor:pointer}.act button.pri{border-color:var(--fg);font-weight:600}.act .reason{flex:1;min-width:160px;padding:4px 8px;border:1px solid var(--line);border-radius:6px;background:var(--bg);color:var(--fg)}
.st-approved,.st-rejected,.st-superseded,.st-revert_requested{opacity:.7}code{font:12px ui-monospace,Consolas,monospace}`

const reviewJS = `(function(){
var token=(document.querySelector('meta[name=abbd-serve-token]')||{}).content;
var change=(document.querySelector('meta[name=abbd-review]')||{}).content;
var pending=[];
document.querySelectorAll('.zb').forEach(function(b){b.addEventListener('click',function(){
 document.querySelectorAll('.zb').forEach(function(x){x.classList.toggle('on',x===b)});
 document.querySelectorAll('.view').forEach(function(v){v.hidden=v.dataset.view!==b.dataset.view});});});
var all=document.getElementById('showall');if(all)all.addEventListener('change',function(){document.body.classList.toggle('all',all.checked)});
function highlight(ids){document.querySelectorAll('.hl').forEach(function(e){e.classList.remove('hl')});
 ids.forEach(function(id){document.querySelectorAll('[data-node="'+id+'"]').forEach(function(e){e.classList.add('hl')});
 document.querySelectorAll('line[data-from="'+id+'"],line[data-to="'+id+'"]').forEach(function(e){if(ids.indexOf(e.dataset.from)>=0&&ids.indexOf(e.dataset.to)>=0)e.classList.add('hl')});});}
document.querySelectorAll('.card').forEach(function(c){c.addEventListener('click',function(ev){
 if(ev.target.closest('button,input'))return;
 document.querySelectorAll('.card.sel').forEach(function(x){x.classList.remove('sel')});c.classList.add('sel');
 highlight((c.dataset.nodes||'').split(' ').filter(Boolean));});});
function send(d){
 if(token){fetch('/api/decide',{method:'POST',headers:{'Content-Type':'application/json','X-ABBD-Token':token},body:JSON.stringify(d)})
  .then(function(r){return r.text().then(function(t){if(!r.ok)throw new Error(t);location.reload()})}).catch(function(e){alert(e.message)});return;}
 pending=pending.filter(function(p){return p.delta!==d.delta});pending.push(d);
 var box=document.getElementById('pending');box.hidden=false;document.getElementById('pcount').textContent=pending.length;}
document.querySelectorAll('.act button').forEach(function(b){b.addEventListener('click',function(){
 var card=b.closest('.card');var reason=card.querySelector('.reason').value;
 if(b.dataset.decision==='reject'&&!reason){alert('거부에는 사유가 필요하다');return;}
 send({delta:card.dataset.delta,decision:b.dataset.decision,reason:reason});
 card.querySelector('.status').textContent='보냄: '+b.dataset.decision;});});
var dl=document.getElementById('download');if(dl)dl.addEventListener('click',function(){
 var blob=new Blob([JSON.stringify({change:change,decisions:pending},null,2)],{type:'application/json'});
 var a=document.createElement('a');a.href=URL.createObjectURL(blob);a.download='abbd-decisions.json';a.click();});
})();`
