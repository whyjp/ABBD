// Package verify is the Verification Engine: it runs commands, keeps Evidence and separates
// "ran and failed" from "could not run" (PRD ch.04 tiers, FR-07).
package verify

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/whyjp/cccv/internal/model"
	"github.com/whyjp/cccv/internal/ssot"
)

const runsDir = "verification/runs"

// Run executes one command in root and records Evidence. A missing tool is UNVERIFIABLE, not FAILED.
func Run(root, command, revision, fingerprint, change string) model.Evidence {
	ev := model.Evidence{
		ID: nextRunID(root), Command: command, Dir: ".", Revision: revision, Fingerprint: fingerprint,
		Change: change, StartedAt: time.Now().UTC().Format(time.RFC3339),
	}
	start := time.Now()
	fields := strings.Fields(command)
	if len(fields) == 0 {
		ev.Verification, ev.Reason = model.Unverifiable, "empty command"
		return ev
	}
	prefix, tool := splitCd(command)
	tf := strings.Fields(tool)
	if _, err := exec.LookPath(tf[0]); err != nil {
		ev.Verification, ev.Reason = model.Unverifiable, fmt.Sprintf("%s not available in this environment", tf[0])
		return ev
	}
	goTest := len(tf) >= 2 && tf[0] == "go" && tf[1] == "test"
	run := command
	if goTest && !strings.Contains(command, "-json") {
		run = prefix + "go test -json " + strings.Join(tf[2:], " ")
	}
	cmd := shell(run)
	cmd.Dir = root
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	ev.DurationMs = time.Since(start).Milliseconds()
	ev.ExitCode = cmd.ProcessState.ExitCode()
	text := out.String()
	if goTest {
		ev.Tests, text = parseGoTestJSON(text)
	}
	ev.OutputTail = tail(text, 40)
	switch {
	case err == nil:
		ev.Verification = model.Passed
	case ev.ExitCode < 0:
		ev.Verification, ev.Reason = model.Unverifiable, err.Error()
	default:
		ev.Verification = model.Failed
	}
	if goTest && ev.Verification == model.Passed && len(ev.Tests) == 0 && strings.Contains(text, "no test files") {
		ev.Reason = "no test files"
	}
	return ev
}

// splitCd separates a leading "cd DIR && " (the analyzer's form for nested modules) so the
// tool after it is the one checked for availability.
func splitCd(command string) (prefix, tool string) {
	if strings.HasPrefix(command, "cd ") {
		if i := strings.Index(command, "&&"); i > 0 {
			return command[:i+2] + " ", strings.TrimSpace(command[i+2:])
		}
	}
	return "", command
}

func shell(command string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		if sh, err := exec.LookPath("sh"); err == nil {
			return exec.Command(sh, "-c", command)
		}
		return exec.Command("cmd", "/C", command)
	}
	return exec.Command("sh", "-c", command)
}

func parseGoTestJSON(s string) ([]model.TestResult, string) {
	var results []model.TestResult
	var text strings.Builder
	sc := bufio.NewScanner(strings.NewReader(s))
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		var e struct{ Action, Package, Test, Output string }
		if json.Unmarshal([]byte(line), &e) != nil {
			text.WriteString(line + "\n")
			continue
		}
		if e.Output != "" {
			text.WriteString(e.Output)
		}
		if e.Test == "" || strings.Contains(e.Test, "/") {
			continue
		}
		switch e.Action {
		case "pass":
			results = append(results, model.TestResult{Package: e.Package, Name: e.Test, Result: model.Passed})
		case "fail":
			results = append(results, model.TestResult{Package: e.Package, Name: e.Test, Result: model.Failed})
		case "skip":
			results = append(results, model.TestResult{Package: e.Package, Name: e.Test, Result: model.Unverifiable})
		}
	}
	return results, text.String()
}

func tail(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return Redact(strings.Join(lines, "\n"))
}

// Redact strips values that look like secrets before evidence is stored (NFR-06).
func Redact(s string) string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		low := strings.ToLower(line)
		for _, k := range []string{"password=", "secret=", "token=", "api_key=", "apikey=", "authorization:"} {
			if i := strings.Index(low, k); i >= 0 {
				line = line[:i+len(k)] + "[REDACTED]"
				break
			}
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

func nextRunID(root string) string {
	day := time.Now().UTC().Format("2006-01-02")
	files, _ := filepath.Glob(ssot.Path(root, runsDir, "run-"+day+"-*.yaml"))
	return fmt.Sprintf("run-%s-%03d", day, len(files)+1)
}

// Record assigns the next run ID to an evidence produced without running a command (a check,
// or a check that cannot run here) and saves it.
func Record(root string, ev *model.Evidence) error {
	ev.ID = nextRunID(root)
	return Save(root, *ev)
}

func Save(root string, ev model.Evidence) error {
	return ssot.WriteYAMLFile(root, filepath.Join(runsDir, ev.ID+".yaml"), ev)
}

func Load(root, id string) (model.Evidence, error) {
	var ev model.Evidence
	err := ssot.ReadYAMLFile(root, filepath.Join(runsDir, id+".yaml"), &ev)
	return ev, err
}

func Exists(root, id string) bool {
	_, err := os.Stat(ssot.Path(root, runsDir, id+".yaml"))
	return err == nil
}

// ApplyResults writes per-test outcomes onto TEST records and the features that reference them.
// Only verification changes: a PASSED test never changes an origin or a purpose (PRD ch.06).
func ApplyResults(s *ssot.Snapshot, ev model.Evidence) {
	importPath := map[string]string{}
	for _, m := range s.Modules {
		for _, a := range m.Anchors {
			if a.Kind == model.AnchorSymbol {
				if i := strings.Index(a.Ref, ":"); i >= 0 {
					importPath[a.Ref[i+1:]] = m.ID
				}
			}
		}
	}
	byKey := map[string]model.Verification{}
	for _, r := range ev.Tests {
		byKey[importPath[r.Package]+"/"+r.Name] = r.Result
	}
	for _, t := range s.Tests {
		if v, ok := byKey[t.Module+"/"+t.Name]; ok {
			t.Verification, t.Evidence = v, ev.ID
		}
	}
	for _, f := range s.Features {
		for i := range f.Verification {
			if t := s.Tests[f.Verification[i].TestID]; t != nil && t.Evidence == ev.ID {
				f.Verification[i].Verification, f.Verification[i].Evidence = t.Verification, ev.ID
			}
		}
		allPassed := len(f.Verification) > 0
		for _, ref := range f.Verification {
			if ref.Verification != model.Passed {
				allPassed = false
			}
		}
		for i := range f.Observations {
			if allPassed && f.Observations[i].Verification != model.Failed {
				f.Observations[i].Verification = model.Passed
				f.Observations[i].Evidence = []string{ev.ID}
			}
		}
	}
	applyContractVerification(s, ev)
}

// runtimeKinds need a broker, database or deployed environment; a local test run cannot
// prove them, so they are UNVERIFIABLE with the reason written down (PRD 08A).
var runtimeKinds = map[string]string{
	"kafka-topic": "실서비스 브로커가 필요한 토픽: 로컬 실행으로 확인 불가, 정적 추출만 수행",
	"queue":       "실서비스 큐: 로컬 실행으로 확인 불가",
	"db-table":    "실 DB 스키마: 마이그레이션 적용을 실행하지 않음",
	"env-var":     "배포 환경의 설정값: 로컬에서 확인 불가",
}

// applyContractVerification judges a route only by the tests that name it, and only by
// their outcome in this run: a sibling test passing proves nothing about this contract.
func applyContractVerification(s *ssot.Snapshot, ev model.Evidence) {
	if len(ev.Tests) == 0 {
		return
	}
	for _, m := range s.Modules {
		for i := range m.ExternalContracts {
			c := &m.ExternalContracts[i]
			if reason, ok := runtimeKinds[c.Kind]; ok {
				c.Verification, c.VerificationNote = model.Unverifiable, reason
				continue
			}
			ran, failed := 0, 0
			for _, t := range testsMentioning(s, m, c.Selector) {
				if t.Evidence != ev.ID {
					continue
				}
				ran++
				if t.Verification == model.Failed {
					failed++
				}
			}
			switch {
			case failed > 0:
				c.Verification, c.VerificationNote = model.Failed, ev.ID
			case ran > 0 && ev.Verification == model.Passed:
				c.Verification, c.VerificationNote = model.Passed, ev.ID
			}
		}
	}
}

// testsMentioning is the deterministic link between a route and its tests: the test's file
// names the selector's path literally.
func testsMentioning(s *ssot.Snapshot, m *model.Module, selector string) []*model.Test {
	path := strings.TrimSpace(selector)
	if i := strings.Index(path, " "); i >= 0 {
		path = path[i+1:]
	}
	var out []*model.Test
	for _, tid := range m.Tests {
		t := s.Tests[tid]
		if t == nil {
			continue
		}
		file := t.Location
		if i := strings.LastIndex(file, ":"); i > 0 {
			file = file[:i]
		}
		b, err := os.ReadFile(filepath.Join(s.Root, filepath.FromSlash(file)))
		if err == nil && strings.Contains(string(b), path) {
			out = append(out, t)
		}
	}
	return out
}
