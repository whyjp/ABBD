// Package bootstrap is the Zero-Map Bootstrap of PRD ch.01A: Discover, Observe, Infer
// candidates, (do not ask), Seed, Handoff. It never reads input; every uncertain item becomes
// INFERRED and UNCONFIRMED and goes on the confirm queue.
package bootstrap

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/whyjp/abbd/internal/analysis"
	"github.com/whyjp/abbd/internal/html"
	"github.com/whyjp/abbd/internal/identity"
	"github.com/whyjp/abbd/internal/knowledge"
	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/ssot"
	"github.com/whyjp/abbd/internal/verify"
	"github.com/whyjp/abbd/internal/workspace"
)

const (
	Greenfield = "greenfield"
	CodeOnly   = "code-only"
	Adopt      = "adopt"
)

type Options struct {
	Mode            string
	From            string
	VerifyAvailable bool
	Preview         bool
}

type Report struct {
	Mode              string             `yaml:"mode" json:"mode"`
	Revision          string             `yaml:"revision" json:"revision"`
	Fingerprint       string             `yaml:"fingerprint" json:"fingerprint"`
	GeneratedAt       string             `yaml:"generated_at" json:"generated_at"`
	Rerun             bool               `yaml:"rerun" json:"rerun"`
	Languages         []string           `yaml:"languages" json:"languages"`
	Counts            map[string]int     `yaml:"counts" json:"counts"`
	Knowledge         knowledge.Report   `yaml:"knowledge" json:"knowledge"`
	VerificationScope []string           `yaml:"verification_scope" json:"verification_scope"`
	NotRun            []string           `yaml:"not_run" json:"not_run"`
	Unconfirmed       []html.PendingItem `yaml:"unconfirmed" json:"unconfirmed"`
	UnconfirmedCount  int                `yaml:"unconfirmed_count" json:"unconfirmed_count"`
	Unknowns          []string           `yaml:"unknowns" json:"unknowns"`
	Adopt             *AdoptReport       `yaml:"adopt,omitempty" json:"adopt,omitempty"`
	QuestionsAsked    int                `yaml:"questions_asked" json:"questions_asked"`
	Explorer          string             `yaml:"explorer" json:"explorer"`
	NextTurn          string             `yaml:"next_turn" json:"next_turn"`
}

// DetectMode chooses the entry path when the user did not: adopt needs an explicit source,
// any source code means code-only, otherwise greenfield.
func DetectMode(opt Options, res *analysis.Result) string {
	switch {
	case opt.Mode != "":
		return opt.Mode
	case opt.From != "":
		return Adopt
	case len(res.Modules) > 0:
		return CodeOnly
	default:
		return Greenfield
	}
}

func Run(root string, opt Options) (*Report, error) {
	root, _ = filepath.Abs(root)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	ws, err := workspace.Observe(root)
	if err != nil {
		return nil, fmt.Errorf("discover: %w", err)
	}
	res, err := analysis.Analyze(root)
	if err != nil {
		return nil, fmt.Errorf("discover: %w", err)
	}
	mode := DetectMode(opt, res)
	switch mode {
	case Greenfield, CodeOnly, Adopt:
	default:
		return nil, fmt.Errorf("unknown mode %q (greenfield|code-only|adopt)", mode)
	}

	rerun := ssot.Exists(root)
	var s *ssot.Snapshot
	if rerun {
		if s, err = ssot.Load(root); err != nil {
			return nil, err
		}
		// Re-seeding mid-turn would move the sync watermark under unverified edits.
		if s.State.ActiveTurn != "" {
			return nil, fmt.Errorf("turn %s is active; finish it with sync or turn abort before re-running bootstrap", s.State.ActiveTurn)
		}
	} else {
		s = ssot.NewSnapshot(root)
		s.Project = ssot.DefaultProject(filepath.Base(root), mode)
		s.Project.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	s.Project.Mode = mode
	s.Project.Languages = res.Languages
	if len(res.TestCommands) > 0 {
		s.Project.Verification.TestCommands = res.TestCommands
	}
	if len(res.BuildCommands) > 0 {
		s.Project.Verification.BuildCommands = res.BuildCommands
	}

	renames := map[string]string{}
	if rerun {
		renames = identity.ContentRenames(s.State.Files, ws.Files)
		for k, v := range workspace.Renames(root, s.State.Revision) {
			renames[k] = v
		}
	}
	krep := knowledge.Apply(s, res, knowledge.Options{Revision: ws.Revision, Renames: renames, SeedFeatures: true})

	rep := &Report{
		Mode: mode, Revision: ws.Revision, Fingerprint: ws.Fingerprint, Rerun: rerun,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339), Languages: res.Languages,
		Knowledge: krep, Unknowns: res.Unknowns,
	}
	if mode == Adopt {
		// Adopt failure never blocks the base bootstrap (AC-BOOT-05).
		rep.Adopt = adopt(s, opt.From)
	}

	if opt.VerifyAvailable {
		for _, cmd := range append(append([]string{}, s.Project.Verification.BuildCommands...), s.Project.Verification.TestCommands...) {
			ev := verify.Run(root, cmd, ws.Revision, ws.Fingerprint, "")
			if !opt.Preview {
				if err := verify.Save(root, ev); err != nil {
					return nil, err
				}
			}
			verify.ApplyResults(s, ev)
			rep.VerificationScope = append(rep.VerificationScope, fmt.Sprintf("%s → %s %s", cmd, ev.Verification, ev.ID))
		}
	} else {
		rep.NotRun = append(rep.NotRun, "검증 명령을 실행하지 않음 (--verify-available 없음): 모든 관찰은 verification NOT_RUN")
	}
	if len(s.Project.Verification.TestCommands) == 0 {
		rep.NotRun = append(rep.NotRun, "탐지된 테스트 명령 없음: 첫 개발 턴에서 검증 계약을 만든다")
	}

	rep.Counts = map[string]int{
		"features": countLive(s.Features, func(f *model.Feature) model.ItemStatus { return f.Status }),
		"modules":  countLive(s.Modules, func(m *model.Module) model.ItemStatus { return m.Status }),
		"tests":    countLive(s.Tests, func(t *model.Test) model.ItemStatus { return t.Status }),
	}
	contracts := 0
	for _, m := range s.Modules {
		for _, c := range m.ExternalContracts {
			if c.Status != model.ItemRemoved {
				contracts++
			}
		}
	}
	rep.Counts["contracts"] = contracts
	rep.Counts["candidates"] = len(krep.Candidates)
	rep.Unconfirmed = html.Pending(s)
	rep.UnconfirmedCount = len(rep.Unconfirmed)
	rep.Explorer = filepath.ToSlash(filepath.Join(ssot.Dir, html.ExplorerFile))
	rep.NextTurn = nextTurnHint(mode)
	if opt.Preview {
		return rep, nil
	}

	if _, err := knowledge.Publish(s, res, ws, true, "bootstrap "+mode); err != nil {
		return nil, fmt.Errorf("seed: %w", err)
	}
	if err := ssot.WriteYAMLFile(root, "bootstrap-report.yaml", rep); err != nil {
		return nil, err
	}
	return rep, ssot.WriteText(root, "bootstrap-report.md", Markdown(rep))
}

func countLive[T any](m map[string]*T, status func(*T) model.ItemStatus) int {
	n := 0
	for _, v := range m {
		if status(v) != model.ItemRemoved {
			n++
		}
	}
	return n
}

func nextTurnHint(mode string) string {
	if mode == Greenfield {
		return `abbd run "<첫 기능 요청>" — 요청이 REQ/FEAT 초안(USER_STATED)이 되고 구현·검증·동기화로 이어진다`
	}
	return `abbd run "<수정 의도>" — 생성된 Map·코드·외부 계약 뷰를 근거로 계획한다`
}

func Markdown(r *Report) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# BootstrapReport\n\n- 모드: %s (재실행: %v)\n- revision: `%s` · fingerprint `%s`\n- 생성: %s\n- 언어: %s\n- 질문: %d회 (bootstrap은 묻지 않는다)\n\n",
		r.Mode, r.Rerun, r.Revision, r.Fingerprint, r.GeneratedAt, strings.Join(r.Languages, ", "), r.QuestionsAsked)
	b.WriteString("## 생성된 지식\n\n")
	for _, k := range model.SortedKeys(r.Counts) {
		fmt.Fprintf(&b, "- %s: %d\n", k, r.Counts[k])
	}
	fmt.Fprintf(&b, "- 신규 %d · 갱신 %d · 제거 %d · 병합 후보 %d\n\n", len(r.Knowledge.New), len(r.Knowledge.Updated), len(r.Knowledge.Removed), len(r.Knowledge.Candidates))
	b.WriteString("## 검증 범위\n\n")
	for _, v := range r.VerificationScope {
		fmt.Fprintf(&b, "- 실행함: %s\n", v)
	}
	for _, v := range r.NotRun {
		fmt.Fprintf(&b, "- 실행 못함: %s\n", v)
	}
	fmt.Fprintf(&b, "\n## 확인 대기 (UNCONFIRMED %d)\n\n", r.UnconfirmedCount)
	for _, p := range r.Unconfirmed {
		fmt.Fprintf(&b, "- `%s` %s: %s (origin %s)\n", p.ID, p.Field, p.Value, p.Origin)
	}
	if len(r.Unknowns) > 0 {
		b.WriteString("\n## UNKNOWN (분석하지 못한 것)\n\n")
		for _, u := range r.Unknowns {
			fmt.Fprintf(&b, "- %s\n", u)
		}
	}
	if r.Adopt != nil {
		fmt.Fprintf(&b, "\n## adopt\n\n- 출처: %s\n- 적용: %d · 충돌: %d · 매칭 실패: %d\n", r.Adopt.From, len(r.Adopt.Applied), len(r.Adopt.Conflicts), len(r.Adopt.Unmatched))
		for _, e := range r.Adopt.Errors {
			fmt.Fprintf(&b, "- 오류(기본 bootstrap은 계속): %s\n", e)
		}
		for _, c := range r.Adopt.Conflicts {
			fmt.Fprintf(&b, "- 충돌: %s\n", c)
		}
	}
	fmt.Fprintf(&b, "\n## 다음 턴\n\n- HTML Explorer: `%s`\n- %s\n", r.Explorer, r.NextTurn)
	return b.String()
}
