package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var (
	fence    = regexp.MustCompile("(?s)```[a-z]*\n(.*?)```")
	inline   = regexp.MustCompile("`(abbd [^`]+)`")
	argument = regexp.MustCompile(`"[^"]*"|'[^']*'|\S+`)
)

// pluginDocs lists every skill and agent file the plugin ships.
func pluginDocs(t *testing.T) []string {
	t.Helper()
	skills, err := filepath.Glob(filepath.Join("..", "..", "plugins", "*", "skills", "*", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	agents, err := filepath.Glob(filepath.Join("..", "..", "plugins", "*", "agents", "*.md"))
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) < 2 || len(agents) == 0 {
		t.Fatalf("expected the abbd-setup and abbd-mode skills and the abbd agent, found %v %v", skills, agents)
	}
	return append(skills, agents...)
}

// Every abbd command a plugin skill tells an agent to run must be one the CLI
// accepts, so a renamed flag cannot leave a skill silently broken.
func TestSkillCommandsAreRealCLICommands(t *testing.T) {
	seen := 0
	for _, path := range pluginDocs(t) {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var lines []string
		for _, m := range fence.FindAllStringSubmatch(string(b), -1) {
			lines = append(lines, strings.Split(m[1], "\n")...)
		}
		for _, m := range inline.FindAllStringSubmatch(string(b), -1) {
			lines = append(lines, m[1])
		}
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "abbd ") {
				continue
			}
			seen++
			args := argument.FindAllString(strings.TrimPrefix(line, "abbd "), -1)
			for i, a := range args {
				if strings.HasPrefix(a, "<") || strings.HasPrefix(a, `"`) || strings.HasPrefix(a, "'") {
					args[i] = "X"
				}
			}
			if _, err := parse(args, &bytes.Buffer{}); err != nil {
				t.Errorf("%s: %q is not a valid abbd command: %v", path, line, err)
			}
		}
	}
	if seen == 0 {
		t.Fatalf("the skills name no abbd commands; the check would prove nothing")
	}
}

func readPlugin(t *testing.T, rel ...string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(append([]string{"..", "..", "plugins", "abbd"}, rel...)...))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// Subagents of an abbd-mode run must carry the ABBD context, so the skill
// names the plugin's own agent and that agent reads the turn before it works.
func TestAbbdModeSubagentsRunAsTheAbbdAgent(t *testing.T) {
	var manifest struct{ Name string }
	if err := json.Unmarshal([]byte(readPlugin(t, ".claude-plugin", "plugin.json")), &manifest); err != nil {
		t.Fatal(err)
	}
	agent := readPlugin(t, "agents", "abbd-agent.md")
	name := regexp.MustCompile(`(?m)^name: (\S+)$`).FindStringSubmatch(agent)
	if name == nil {
		t.Fatalf("the agent file has no name")
	}
	skill := readPlugin(t, "skills", "abbd-mode", "SKILL.md")
	want := fmt.Sprintf(`subagent_type: "%s:%s"`, manifest.Name, name[1])
	if !strings.Contains(skill, want) {
		t.Fatalf("abbd-mode must spawn subagents with %s", want)
	}
	if strings.Contains(skill, `subagent_type: "pstack:poteto-agent"`) {
		t.Fatalf("abbd-mode must not spawn poteto-agent, which works without the ABBD context")
	}
	order := []string{"abbd turn show", ".abbd/intended.yaml", "agents/poteto-agent.md"}
	last := -1
	for _, s := range order {
		i := strings.Index(agent, s)
		if i <= last {
			t.Fatalf("the agent must read %v in that order before poteto-agent's instructions; %q is out of place", order, s)
		}
		last = i
	}
}

var pstackPath = regexp.MustCompile("`<pstack>/([^`]+)`")

// The skills reference the pinned pstack by path instead of copying it. The
// paths must exist in that version, and every playbook poteto-mode routes to
// must have a row in abbd-mode, so a pstack upgrade that adds one fails here.
func TestAbbdModeCoversThePinnedPstack(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	pstack := filepath.Join(home, ".claude", "plugins", "cache", "cursor-plugins", "pstack", "0.15.36")
	if _, err := os.Stat(filepath.Join(pstack, "skills", "poteto-mode", "SKILL.md")); err != nil {
		t.Skip("pstack 0.15.36 is not installed")
	}
	for _, path := range pluginDocs(t) {
		b, _ := os.ReadFile(path)
		for _, m := range pstackPath.FindAllStringSubmatch(string(b), -1) {
			ref := strings.TrimSuffix(m[1], "/")
			if strings.ContainsAny(ref, "<*") {
				continue
			}
			if _, err := os.Stat(filepath.Join(pstack, ref)); err != nil {
				t.Errorf("%s references <pstack>/%s, which pstack 0.15.36 does not have", path, m[1])
			}
		}
	}
	mode, err := os.ReadFile(filepath.Join(pstack, "skills", "poteto-mode", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	_, list, _ := strings.Cut(string(mode), "## Playbooks")
	skill := readPlugin(t, "skills", "abbd-mode", "SKILL.md")
	_, table, _ := strings.Cut(skill, "## Route")
	table, _, _ = strings.Cut(table, "## Start")
	playbooks := regexp.MustCompile(`(?m)^- \*\*(.+?)\.\*\*`).FindAllStringSubmatch(list, -1)
	if len(playbooks) < 20 {
		t.Fatalf("found only %d playbooks in poteto-mode; the list format changed", len(playbooks))
	}
	for _, m := range playbooks {
		if !strings.Contains(table, m[1]) {
			t.Errorf("abbd-mode's route table has no row for the %s playbook", m[1])
		}
	}
}

// pstack code playbooks end in Opening a PR, so abbd-mode must hold that step
// until the turn has ended, or a DRAFT or BLOCKED turn ships without its record.
func TestAbbdModeEndsTheTurnBeforeAnyPR(t *testing.T) {
	skill := readPlugin(t, "skills", "abbd-mode", "SKILL.md")
	_, end, _ := strings.Cut(skill, "## End")
	turnEnd := strings.Index(end, "abbd turn end")
	pr := strings.Index(end, "**Opening a PR**")
	if turnEnd < 0 || pr < turnEnd || !strings.Contains(end[pr:], "SYNCED") || !strings.Contains(end[pr:], ".abbd/changes/") {
		t.Fatalf("abbd-mode's End must run abbd turn end, then Opening a PR only on SYNCED or CLOSED with the CHG record committed")
	}
}

// ABBD has no code-reading procedure of its own: abbd-mode sends each direct
// design unit through pstack's how (or blast-radius) after the impact, and
// only then registers the result and records the scan, before any edit.
func TestAbbdModeScansDirectUnitsWithPstackBeforeDeclaring(t *testing.T) {
	skill := readPlugin(t, "skills", "abbd-mode", "SKILL.md")
	_, start, _ := strings.Cut(skill, "## Start")
	order := []string{"abbd turn start", "`<pstack>/skills/how/SKILL.md`", "abbd propose relation", "abbd turn scan", "abbd turn declare"}
	last := -1
	for _, s := range order {
		i := strings.Index(start, s)
		if i <= last {
			t.Fatalf("abbd-mode's Start must run %v in that order; %q is out of place", order, s)
		}
		last = i
	}
	if !strings.Contains(start, "`<pstack>/skills/blast-radius/SKILL.md` when the change spans") || !strings.Contains(start, "unchanged") {
		t.Fatalf("the scan must follow how or blast-radius unchanged")
	}
}

// Setup follows swarm and how unchanged and adds only ABBD's result format,
// which must carry what makes a semantic coupling checkable.
func TestAbbdSetupAsksForCheckableSemanticCouplings(t *testing.T) {
	skill := readPlugin(t, "skills", "abbd-setup", "SKILL.md")
	for _, want := range []string{"`<pstack>/skills/swarm/SKILL.md`", "`<pstack>/skills/how/references/explorer-prompt.md`", `"evidence"`, `"via"`, `"parent"`, "verbatim"} {
		if !strings.Contains(skill, want) {
			t.Errorf("abbd-setup must name %s", want)
		}
	}
	agent := readPlugin(t, "agents", "abbd-agent.md")
	if !strings.Contains(agent, "FROM.KIND.TO") || !strings.Contains(agent, "PATH:LINES:QUOTE") {
		t.Errorf("the abbd agent must report the couplings it met by key with their evidence")
	}
}
