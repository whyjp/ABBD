package workflow

import (
	"fmt"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/ssot"
	"github.com/whyjp/abbd/internal/verify"
)

// SetDriver registers a user-level check for a feature or contract (PRD 02: a verification
// recipe per entry point). It lives in project.yaml, so it outlives any one turn.
func (e *Engine) SetDriver(id, run, expect string) (map[string]ssot.Driver, error) {
	w, err := e.load()
	if err != nil {
		return nil, err
	}
	if _, err := inspectIn(w, id); err != nil {
		return nil, fmt.Errorf("driver target: %w", err)
	}
	if strings.TrimSpace(run) == "" || strings.TrimSpace(expect) == "" {
		return nil, fmt.Errorf("a driver needs --run and --expect: a user-level run without an expected outcome proves nothing")
	}
	if w.s.Project.Drivers == nil {
		w.s.Project.Drivers = map[string]ssot.Driver{}
	}
	w.s.Project.Drivers[id] = ssot.Driver{Run: run, Expect: expect}
	return w.s.Project.Drivers, w.s.Save()
}

func (e *Engine) Drivers() (map[string]ssot.Driver, error) {
	w, err := e.load()
	if err != nil {
		return nil, err
	}
	return w.s.Project.Drivers, nil
}

// drive runs the drivers of what this turn touches (all drivers when no turn is active) and
// records each as user-level evidence: PASSED only when the command succeeds and its output
// contains the expected text.
func (e *Engine) drive(w *world, chg *model.Change, rep *EvidenceReport) (*EvidenceReport, error) {
	ids := model.SortedKeys(w.s.Project.Drivers)
	if chg != nil {
		scope := map[string]bool{}
		for _, id := range append(append(append([]string{}, chg.Impacted...), chg.Delivers...), chg.Plan.Targets...) {
			scope[id] = true
			if f := w.s.Features[id]; f != nil {
				for _, c := range f.ExternalContracts {
					scope[c] = true
				}
			}
		}
		var in []string
		for _, id := range ids {
			if scope[id] {
				in = append(in, id)
			}
		}
		ids = in
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("no driver covers this turn; register one: abbd driver set FEAT-x|IF-x --run CMD --expect TEXT")
	}
	sort.Strings(ids)
	changeID := ""
	if chg != nil {
		changeID = chg.ID
	}
	for _, id := range ids {
		d := w.s.Project.Drivers[id]
		ev := model.Evidence{Command: d.Run, Dir: ".", Revision: w.ws.Revision, Fingerprint: w.ws.Fingerprint, Change: changeID, StartedAt: now(), Level: model.LevelUser}
		start := time.Now()
		cmd := shellCmd(d.Run)
		cmd.Dir = e.Root
		out, err := cmd.CombinedOutput()
		ev.DurationMs = time.Since(start).Milliseconds()
		ev.ExitCode = cmd.ProcessState.ExitCode()
		ev.OutputTail = verify.Redact(lastLines(string(out), 30))
		switch {
		case err == nil && strings.Contains(string(out), d.Expect):
			ev.Verification = model.Passed
		case err == nil:
			ev.Verification, ev.Reason = model.Failed, fmt.Sprintf("%s: output lacks the expected %q", id, d.Expect)
		default:
			ev.Verification, ev.Reason = model.Failed, fmt.Sprintf("%s: %v", id, err)
		}
		if err := verify.Record(e.Root, &ev); err != nil {
			return nil, err
		}
		rep.Runs = append(rep.Runs, ev)
		rep.Ran = append(rep.Ran, fmt.Sprintf("driver %s → %s (%s)", id, ev.Verification, ev.ID))
		if chg != nil {
			chg.VerificationEvidence = append(chg.VerificationEvidence, ev.ID)
		}
	}
	rep.Overall = aggregate(rep.Runs)
	if err := w.s.Save(); err != nil {
		return nil, err
	}
	if rep.Overall == model.Failed {
		return rep, ErrFailed
	}
	return rep, nil
}

func shellCmd(command string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		if sh, err := exec.LookPath("sh"); err == nil {
			return exec.Command(sh, "-c", command)
		}
		return exec.Command("cmd", "/C", command)
	}
	return exec.Command("sh", "-c", command)
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

// Candidate is one repeated problem found in the receipts (learn --from-receipts).
type LearnCandidate struct {
	Signal  string   `json:"signal"`
	Count   int      `json:"count"`
	Changes []string `json:"changes"`
	Suggest string   `json:"suggest"`
}

// FromReceipts reads what ABBD already recorded (failing tests, rejected cards, blocked turns,
// sync refusals) and returns only what happened in two or more turns: a correction is earned
// by repetition, not by one bad turn.
func (e *Engine) FromReceipts() ([]LearnCandidate, error) {
	w, err := e.load()
	if err != nil {
		return nil, err
	}
	seen := map[string]map[string]bool{}
	add := func(signal, chg string) {
		if seen[signal] == nil {
			seen[signal] = map[string]bool{}
		}
		seen[signal][chg] = true
	}
	for _, cid := range model.SortedKeys(w.s.Changes) {
		c := w.s.Changes[cid]
		for _, id := range c.VerificationEvidence {
			if ev, err := verify.Load(e.Root, id); err == nil {
				for _, t := range ev.Tests {
					if t.Result == model.Failed {
						add("failing test "+t.Package+"."+t.Name, cid)
					}
				}
			}
		}
		for _, d := range c.StructuralDeltas {
			if d.Status == model.DeltaRejected || d.Status == model.DeltaRevertAsked {
				add(fmt.Sprintf("rejected %s", d.Kind), cid)
			}
		}
		for _, b := range c.Blocked {
			if i := strings.Index(b, ":"); i > 0 {
				b = b[:i]
			}
			add("blocked "+b, cid)
		}
	}
	var out []LearnCandidate
	for _, sig := range model.SortedKeys(seen) {
		if len(seen[sig]) < 2 {
			continue
		}
		suggest := "test: a check that fails on this mistake, proven by a FAILED verify on the old code"
		switch {
		case strings.HasPrefix(sig, "rejected forbidden_dependency"), strings.HasPrefix(sig, "rejected dependency"):
			suggest = "architecture: abbd intent module --forbid, so the engine flags it before review"
		case strings.HasPrefix(sig, "rejected contract"):
			suggest = "architecture: abbd intent contract (compatibility / external consumers) so the tier is right up front"
		case strings.HasPrefix(sig, "blocked"):
			suggest = "skill or project.yaml policy: the same stop recurs"
		}
		out = append(out, LearnCandidate{Signal: sig, Count: len(seen[sig]), Changes: model.SortedKeys(seen[sig]), Suggest: suggest})
	}
	return out, nil
}
