package workflow

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/review"
	"github.com/whyjp/abbd/internal/ssot"
)

// carryDecisions keeps decisions already made on this turn's cards when sync recomputes the
// deltas (for example after a tier C approval), matched by kind and subject.
func carryDecisions(fresh, prev []model.StructuralDelta) []model.StructuralDelta {
	for i := range fresh {
		for _, p := range prev {
			if p.Kind != fresh[i].Kind || p.Subject != fresh[i].Subject || p.Decision == nil {
				continue
			}
			// A decision covers the change it saw. The same item changed again within the turn
			// is a new card (08B §6), so an approval of v2 never authorizes v3.
			if p.Before != fresh[i].Before || p.After != fresh[i].After {
				fresh[i].Reason += fmt.Sprintf(" (이전 결정 %s 무효: 같은 항목이 다시 바뀜)", p.Decision.Decision)
				continue
			}
			fresh[i].Decision, fresh[i].ADR = p.Decision, p.ADR
			fresh[i].Status = p.Status
		}
	}
	return fresh
}

// changedAgain reports whether an earlier card of this turn on the same item saw a different
// change; then an ADR approval naming the item no longer applies.
func changedAgain(prev []model.StructuralDelta, d model.StructuralDelta) bool {
	for _, p := range prev {
		if p.Kind == d.Kind && p.Subject == d.Subject && (p.Before != d.Before || p.After != d.After) {
			return true
		}
	}
	return false
}

// pendingTierC lists tier C deltas without approval. An approval is either a decision on the
// card or a tier C ADR answered "approve" that names the item (`abbd decide --tier C`).
func pendingTierC(s *ssot.Snapshot, chg *model.Change, prev, deltas []model.StructuralDelta) []string {
	var out []string
	for i := range deltas {
		d := &deltas[i]
		if d.Tier != model.TierC {
			continue
		}
		if d.Status == model.DeltaPending && !changedAgain(prev, *d) && approved(s, chg, d.Subject, model.TierC) {
			d.Status = model.DeltaApproved
		}
		switch d.Status {
		case model.DeltaApproved:
		case model.DeltaRejected:
			out = append(out, fmt.Sprintf("%s 거부됨: 변경을 되돌려야 함 (%s)", d.ID, d.Decision.Reason))
		default:
			out = append(out, fmt.Sprintf("%s %s %s (%s)", d.ID, d.Kind, d.Subject, d.Reason))
		}
	}
	return out
}

// recordDeltaDecisions gives every open tier B card an UNCONFIRMED ADR with its default (the
// turn ends NEEDS_USER until someone decides), and invalidates earlier decisions on the same
// item: a decision made before the item changed again no longer holds (CR-5, AC-REV-06).
func recordDeltaDecisions(s *ssot.Snapshot, chg *model.Change) {
	for i := range chg.StructuralDeltas {
		d := &chg.StructuralDeltas[i]
		invalidateEarlier(s, chg.ID, d.Subject)
		if d.Tier != model.TierB || d.ADR != "" {
			continue
		}
		in := DecideInput{
			Title: string(d.Kind) + " " + d.Subject, Tier: model.TierB,
			Question: fmt.Sprintf("%s: %s (%s → %s). 유지할까?", d.ID, d.Reason, orDash(d.Before), orDash(d.After)),
			Options:  deltaOptions(*d), Default: "approve", Subjects: []string{d.Subject},
			ProceedsWithout: "구조 변경은 기본값으로 적용된 채 유지. 결정은 리뷰 화면에서",
		}
		d.ADR = recordDecision(s, chg, in, false).ID
	}
}

func deltaOptions(d model.StructuralDelta) []model.Option {
	opts := []model.Option{{Key: "approve", Label: "승인"}, {Key: "reject", Label: "거부 (다음 턴이 되돌리거나 재작업)"}}
	if d.Kind == model.ContractCompatChange || d.Kind == model.ContractRemoved {
		opts = append(opts, model.Option{Key: "compat", Label: "호환 경로 유지"})
	}
	return opts
}

func invalidateEarlier(s *ssot.Snapshot, current, subject string) {
	for _, id := range model.SortedKeys(s.Changes) {
		if id == current {
			continue
		}
		c := s.Changes[id]
		for i := range c.StructuralDeltas {
			d := &c.StructuralDeltas[i]
			awaitingRework := (d.Status == model.DeltaRejected || d.Status == model.DeltaRevertAsked) && d.Decision != nil && d.Decision.Consumed == ""
			if d.Subject == subject && d.Decision != nil && d.Decision.Invalidated == "" && d.Status != model.DeltaSuperseded && !awaitingRework {
				d.Decision.Invalidated = current
				d.Status = model.DeltaSuperseded
			}
		}
	}
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

type DeltaDecisionInput struct {
	// ByUser is required on the CLI channel for B/C cards; serve and the decision file are user
	// channels, MCP is the agent's.
	ByUser   bool   `json:"-"`
	Delta    string `json:"delta"`
	Decision string `json:"decision"` // approve | reject | revert | option:<key>
	Reason   string `json:"reason"`
	Channel  string `json:"-"`
}

// DecideDelta records one decision on one structural change card. Every channel (serve,
// decision file, MCP, CLI) goes through here and writes the same record.
func (e *Engine) DecideDelta(in DeltaDecisionInput) (*model.StructuralDelta, error) {
	w, err := e.load()
	if err != nil {
		return nil, err
	}
	d, _, err := e.decideIn(w, in)
	if err != nil {
		return nil, err
	}
	return d, e.republish(w, "결정 반영 "+d.ID)
}

func findDelta(s *ssot.Snapshot, id string) (*model.StructuralDelta, *model.Change) {
	chgID, _, _ := strings.Cut(id, "#")
	c := s.Changes[chgID]
	if c == nil {
		return nil, nil
	}
	for i := range c.StructuralDeltas {
		if c.StructuralDeltas[i].ID == id {
			return &c.StructuralDeltas[i], c
		}
	}
	return nil, c
}

func (e *Engine) decideIn(w *world, in DeltaDecisionInput) (*model.StructuralDelta, *model.Change, error) {
	d, chg := findDelta(w.s, in.Delta)
	if d == nil {
		return nil, nil, fmt.Errorf("structural change %s not found", in.Delta)
	}
	if d.Status == model.DeltaSuperseded {
		return nil, nil, fmt.Errorf("%s changed again in %s after it was decided; decide the newer card", d.ID, d.Decision.Invalidated)
	}
	decision := in.Decision
	key := strings.TrimPrefix(decision, "option:")
	switch {
	case decision == "approve" || decision == "reject":
	case decision == "revert" && d.Tier == model.TierA:
	case strings.HasPrefix(decision, "option:") && d.Tier == model.TierB && hasOption(deltaOptions(*d), key):
	default:
		return nil, nil, fmt.Errorf("%s (tier %s) does not take decision %q", d.ID, d.Tier, decision)
	}
	if (decision == "reject" || decision == "revert") && strings.TrimSpace(in.Reason) == "" {
		return nil, nil, fmt.Errorf("a %s needs a reason: the next turn works from it", decision)
	}
	if in.Channel == "" {
		in.Channel = "cli"
	}
	// The plugin's own MCP server is called by the agent, so it is an agent channel like the CLI
	// without --user; the user's channels are the served review page and the decision file.
	if (in.Channel == "cli" || in.Channel == "mcp") && !in.ByUser && d.Tier != model.TierA {
		return nil, nil, fmt.Errorf("deciding %s records the user's decision; pass --user only with the user's own answer, or let the user decide on the review page", d.ID)
	}
	d.Decision = &model.ItemDecision{Decision: decision, Reason: in.Reason, At: now(), Revision: w.ws.Fingerprint, Channel: in.Channel}
	switch decision {
	case "approve":
		d.Status = model.DeltaApproved
		confirmSubject(w.s, d.Subject, chg.ID)
	case "reject":
		d.Status = model.DeltaRejected
	case "revert":
		d.Status = model.DeltaRevertAsked
	default:
		d.Status = model.DeltaApproved
	}
	if d.Tier != model.TierA {
		if d.ADR == "" {
			d.ADR = recordDecision(w.s, chg, DecideInput{Title: string(d.Kind) + " " + d.Subject, Tier: d.Tier, Question: d.ID + ": " + d.Reason,
				Options: deltaOptions(*d), Default: "approve", Subjects: []string{d.Subject}}, true).ID
		}
		if adr := w.s.Decisions[d.ADR]; adr != nil {
			answer(adr, key, chg)
			if in.Reason != "" {
				adr.Rationale = model.MustSemantic(in.Reason, model.UserStated)
			}
		}
		d.Decision.ADR = d.ADR
	}
	return d, chg, nil
}

func hasOption(opts []model.Option, key string) bool {
	for _, o := range opts {
		if o.Key == key {
			return true
		}
	}
	return false
}

// confirmSubject: approving a card confirms the meaning field of the item it is about.
func confirmSubject(s *ssot.Snapshot, subject, where string) {
	for _, id := range strings.Split(subject, "->") {
		if sem, err := field(s, id); err == nil && sem.Value != "" && sem.Confirmation == model.Unconfirmed {
			sem.Confirmation, sem.ConfirmedIn = model.Confirmed, where
		}
	}
}

// rejectedOnly: a blocked turn whose every tier C card was rejected cannot proceed, and the
// rejection means "revert", which is the next turn's work.
func rejectedOnly(c *model.Change) bool {
	found := false
	for _, d := range c.StructuralDeltas {
		if d.Tier == model.TierC {
			if d.Status != model.DeltaRejected {
				return false
			}
			found = true
		}
	}
	return found
}

// markConsumed names the turn that took rejections staged under the placeholder consumer.
func markConsumed(s *ssot.Snapshot, chgID string) {
	for _, c := range s.Changes {
		for i := range c.StructuralDeltas {
			if d := c.StructuralDeltas[i].Decision; d != nil && d.Consumed == pendingConsumer {
				d.Consumed = chgID
			}
		}
	}
}

const pendingConsumer = "next-turn"

// consumeRejections turns rejected and revert-requested cards into the new turn's IntentDelta,
// once each (CR-4: the next turn reverts or reworks).
func consumeRejections(s *ssot.Snapshot, chg *model.Change) []string {
	var out []string
	for _, id := range model.SortedKeys(s.Changes) {
		c := s.Changes[id]
		for i := range c.StructuralDeltas {
			d := &c.StructuralDeltas[i]
			if d.Decision == nil || d.Decision.Consumed != "" || (d.Status != model.DeltaRejected && d.Status != model.DeltaRevertAsked) {
				continue
			}
			line := fmt.Sprintf("거부됨 %s %s %s: %s → 되돌리거나 다른 안으로 재작업", d.ID, d.Kind, d.Subject, d.Decision.Reason)
			chg.IntentDelta.Additions = append(chg.IntentDelta.Additions, line)
			d.Decision.Consumed = chg.ID
			if chg.ID == "" {
				d.Decision.Consumed = pendingConsumer
			}
			out = append(out, line)
		}
	}
	return out
}

type DecisionFile struct {
	Change    string               `json:"change"`
	Decisions []DeltaDecisionInput `json:"decisions"`
}

// ApplyDecisionFile is the offline channel: decisions downloaded from a review page opened
// without abbd serve (`abbd review --apply FILE`).
func (e *Engine) ApplyDecisionFile(path string) ([]*model.StructuralDelta, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f DecisionFile
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("%s is not a decision file: %w", path, err)
	}
	w, err := e.load()
	if err != nil {
		return nil, err
	}
	var out []*model.StructuralDelta
	for _, in := range f.Decisions {
		in.Channel = "file"
		d, _, err := e.decideIn(w, in)
		if err != nil {
			return out, err
		}
		out = append(out, d)
	}
	return out, e.republish(w, "결정 파일 반영")
}

type ReviewSummary struct {
	Path    string                  `json:"path"`
	Change  string                  `json:"change,omitempty"`
	Open    int                     `json:"open"`
	ByTier  map[model.Tier]int      `json:"by_tier"`
	Deltas  []model.StructuralDelta `json:"deltas"`
	Message string                  `json:"message"`
}

// Review summarizes the cards of one turn (default: the review target) and the page path.
func (e *Engine) Review(chgID string) (*ReviewSummary, error) {
	w, err := e.load()
	if err != nil {
		return nil, err
	}
	target := review.Target(w.s)
	if chgID != "" {
		target = w.s.Changes[chgID]
		if target == nil {
			return nil, fmt.Errorf("%s not found", chgID)
		}
	}
	return summarize(e.Root, target), nil
}

func summarize(root string, c *model.Change) *ReviewSummary {
	r := &ReviewSummary{Path: ssot.Path(root, review.File), ByTier: map[model.Tier]int{}}
	if c == nil {
		r.Message = "구조 변경이 기록된 턴 없음"
		return r
	}
	r.Change, r.Deltas = c.ID, c.StructuralDeltas
	for _, d := range c.StructuralDeltas {
		r.ByTier[d.Tier]++
		if d.Tier != model.TierA && (d.Status == model.DeltaDefaultApplied || d.Status == model.DeltaPending) {
			r.Open++
		}
	}
	r.Message = fmt.Sprintf("%s: 구조 변경 %d건, 결정 대기 %d건", c.ID, len(c.StructuralDeltas), r.Open)
	return r
}

// RenderReview renders a review page in memory for abbd serve, for any turn.
func (e *Engine) RenderReview(chgID, token string) (string, error) {
	w, err := e.load()
	if err != nil {
		return "", err
	}
	target := review.Target(w.s)
	if chgID != "" {
		if target = w.s.Changes[chgID]; target == nil {
			return "", fmt.Errorf("%s not found", chgID)
		}
	}
	l := review.LoadLayout(e.Root)
	page := review.Render(w.s, target, l, w.s.State.Fingerprint, token)
	return page, l.Save(e.Root)
}
