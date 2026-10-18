package ssot

import (
	"fmt"
	"strings"

	"github.com/whyjp/for-my-harness/internal/model"
)

// Summary renders the human-readable block of a Markdown original. It is derived from the
// frontmatter, so it is regenerated on every save.
func Summary(v any) string {
	var b strings.Builder
	switch o := v.(type) {
	case *model.Feature:
		fmt.Fprintf(&b, "# %s · %s\n\n", o.ID, o.Name)
		fmt.Fprintf(&b, "- 목적: %s `[%s]` (origin %s, %s)\n", orDash(o.Intent.Purpose.Value), o.Intent.Purpose.Badge(), o.Intent.Purpose.Origin, o.Intent.Purpose.Confirmation)
		fmt.Fprintf(&b, "- 상태: %s · 소유 모듈: %s\n", o.Status, joinOr(o.OwnedBy))
		for _, inv := range o.Intent.Invariants {
			fmt.Fprintf(&b, "- 불변조건: %s (origin %s, verification %s)\n", inv.Claim, inv.Origin, inv.Verification)
		}
		for _, e := range o.EntryPoints {
			fmt.Fprintf(&b, "- 진입점: %s `%s` %s\n", e.Kind, e.Selector, e.Contract)
		}
		for _, t := range o.Verification {
			fmt.Fprintf(&b, "- 검증: %s `%s` → %s\n", t.TestID, t.Command, t.Verification)
		}
		fmt.Fprintf(&b, "- last synced: %s\n", orDash(o.LastSynced.Revision))
	case *model.Module:
		fmt.Fprintf(&b, "# %s · %s (`%s`)\n\n", o.ID, o.Name, o.Path)
		fmt.Fprintf(&b, "- 책임: %s `[%s]`\n", orDash(o.Responsibility.Value), o.Responsibility.Badge())
		fmt.Fprintf(&b, "- 실제 의존: %s\n", joinOr(o.DependsOn))
		if len(o.DependencyPolicy.Allowed)+len(o.DependencyPolicy.Forbidden) > 0 {
			fmt.Fprintf(&b, "- 의존 정책: allowed %s · forbidden %s\n", joinOr(o.DependencyPolicy.Allowed), joinOr(o.DependencyPolicy.Forbidden))
		}
		if len(o.ExternalContracts) > 0 {
			b.WriteString("\n| 계약 | 종류 | 방향 | selector | origin | verification |\n|---|---|---|---|---|---|\n")
			for _, c := range o.ExternalContracts {
				fmt.Fprintf(&b, "| %s | %s | %s | `%s` | %s | %s |\n", c.ID, c.Kind, c.Direction, c.Selector, c.Origin, c.Verification)
			}
			b.WriteString("\n")
		}
		for _, s := range o.Serving {
			fmt.Fprintf(&b, "- 서빙: %s (%s, %s)\n", s.DeployUnit, s.Source, s.Origin)
		}
		fmt.Fprintf(&b, "- 심볼 %d개 · 테스트 %d개 · analyzer %s\n", len(o.Symbols), len(o.Tests), o.Analyzer)
	case *model.Test:
		fmt.Fprintf(&b, "# %s · %s\n\n- 위치: %s\n- 명령: `%s`\n- verification: %s %s\n", o.ID, o.Name, o.Location, o.Command, o.Verification, o.Evidence)
	case *model.Decision:
		fmt.Fprintf(&b, "# %s · %s (등급 %s)\n\n- 질문: %s\n- 기본값: %s · 선택: %s · 답변: %v\n- 답하지 않아도 진행되는 범위: %s\n", o.ID, o.Title, o.Tier, o.Question, o.Default, o.Chosen, o.Answered, orDash(o.ProceedsWithout))
		for _, op := range o.Options {
			fmt.Fprintf(&b, "  - `%s` %s — %s\n", op.Key, op.Label, op.Effect)
		}
	case *model.Change:
		fmt.Fprintf(&b, "# %s · %s\n\n- 상태: %s · intent_source: %s\n- revision: %s → %s\n- 영향: %s\n- 검증: %s %s\n- 동기화: map %s/%s · graph %s · html %s\n",
			o.ID, o.RequestSummary, o.State, o.IntentSource, o.BaseRevision, orDash(o.ResultRevision), joinOr(o.Impacted),
			o.Verification, joinOr(o.VerificationEvidence), o.SyncState.FeatureMap, o.SyncState.ArchitectureMap, o.SyncState.Graph, o.SyncState.HTML)
	case *model.Requirement:
		fmt.Fprintf(&b, "# %s\n\n- %s (origin %s)\n- from %s\n", o.ID, o.Statement.Value, o.Statement.Origin, o.Change)
	}
	return b.String()
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

func joinOr(s []string) string {
	if len(s) == 0 {
		return "—"
	}
	return strings.Join(s, ", ")
}
