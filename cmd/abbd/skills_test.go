package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var (
	fence      = regexp.MustCompile("(?s)```[a-z]*\n(.*?)```")
	inline     = regexp.MustCompile("`(abbd [^`]+)`")
	argument   = regexp.MustCompile(`"[^"]*"|'[^']*'|\S+`)
	pstackPath = regexp.MustCompile("`<pstack>/([^`]+)`")
)

func readPlugin(t *testing.T, rel ...string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(append([]string{"..", "..", "plugins", "abbd"}, rel...)...))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// skills lists every skill the plugin ships.
func skills(t *testing.T) []string {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join("..", "..", "plugins", "*", "skills", "*", "SKILL.md"))
	if err != nil || len(paths) < 2 {
		t.Fatalf("expected the abbd-mode and abbd-setup skills, found %v (%v)", paths, err)
	}
	return paths
}

// The plugin ships only its skills: no hooks and no agent of its own.
func TestThePluginShipsOnlySkills(t *testing.T) {
	var manifest struct{ Name string }
	if err := json.Unmarshal([]byte(readPlugin(t, ".claude-plugin", "plugin.json")), &manifest); err != nil || manifest.Name != "abbd" {
		t.Fatalf("plugin.json must parse and name the plugin abbd: %v %+v", err, manifest)
	}
	var market struct{ Plugins []struct{ Name, Source string } }
	b, err := os.ReadFile(filepath.Join("..", "..", ".claude-plugin", "marketplace.json"))
	if err != nil || json.Unmarshal(b, &market) != nil || len(market.Plugins) != 1 || market.Plugins[0].Source != "./plugins/abbd" {
		t.Fatalf("marketplace.json must parse and point at ./plugins/abbd: %v %+v", err, market)
	}
	for _, gone := range []string{"hooks", "agents"} {
		if _, err := os.Stat(filepath.Join("..", "..", "plugins", "abbd", gone)); err == nil {
			t.Errorf("the plugin must not ship %s/", gone)
		}
	}
}

// Every abbd command a skill tells an agent to run must be one the CLI
// accepts, so a renamed flag or command cannot leave a skill silently broken.
func TestSkillCommandsAreRealCLICommands(t *testing.T) {
	seen := 0
	for _, path := range skills(t) {
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
	if seen < 8 {
		t.Fatalf("the skills name only %d abbd commands; the check would prove little", seen)
	}
}

// The skills reference the pinned pstack by path instead of copying it, so
// every path must exist in that version.
func TestSkillsReferenceThePinnedPstack(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	pstack := filepath.Join(home, ".claude", "plugins", "cache", "cursor-plugins", "pstack", "0.15.36")
	if _, err := os.Stat(filepath.Join(pstack, "skills", "poteto-mode", "SKILL.md")); err != nil {
		t.Skip("pstack 0.15.36 is not installed")
	}
	refs := 0
	for _, path := range skills(t) {
		b, _ := os.ReadFile(path)
		for _, m := range pstackPath.FindAllStringSubmatch(string(b), -1) {
			ref := strings.TrimSuffix(m[1], "/")
			if strings.ContainsAny(ref, "<*") {
				continue
			}
			refs++
			if _, err := os.Stat(filepath.Join(pstack, ref)); err != nil {
				t.Errorf("%s references <pstack>/%s, which pstack 0.15.36 does not have", path, m[1])
			}
		}
	}
	if refs < 6 {
		t.Fatalf("found only %d pstack references; the skills should point at pstack rather than inline it", refs)
	}
}

// abbd-mode reads the stored topology before the work, defers every decision
// to poteto-mode and the human, and ends by showing the topology difference.
func TestAbbdModeReadsFirstAndEndsWithTheDiff(t *testing.T) {
	skill := readPlugin(t, "skills", "abbd-mode", "SKILL.md")
	inOrder := func(section string, order ...string) {
		t.Helper()
		_, body, ok := strings.Cut(skill, "## "+section)
		if !ok {
			t.Fatalf("abbd-mode has no %s section", section)
		}
		last := -1
		for _, s := range order {
			i := strings.Index(body, s)
			if i <= last {
				t.Fatalf("abbd-mode's %s must name %v in that order; %q is out of place", section, order, s)
			}
			last = i
		}
	}
	inOrder("Start", "abbd sync", ".abbd/blueprint/index.html", "abbd impact", "`<pstack>/skills/how/SKILL.md`", "abbd record relation")
	inOrder("During", "Autonomy", "abbd record fact")
	inOrder("End", "abbd sync", "abbd diff", "PR description", "Commit `.abbd/`")
	for _, gone := range []string{"turn", "approve", "align", "confirm", "BLOCKED", "grade"} {
		if regexp.MustCompile(`\b` + gone + `\b`).MatchString(skill) {
			t.Errorf("abbd-mode still speaks of %q, which ABBD no longer has", gone)
		}
	}
}

// Setup follows swarm and how unchanged and adds only ABBD's result format,
// which carries what makes a semantic coupling checkable. It has no alignment.
func TestAbbdSetupAsksForCheckableCouplingsAndNoAlignment(t *testing.T) {
	skill := readPlugin(t, "skills", "abbd-setup", "SKILL.md")
	for _, want := range []string{"`<pstack>/skills/swarm/SKILL.md`", "`<pstack>/skills/how/references/explorer-prompt.md`", `"evidence"`, `"via"`, `"parent"`, "verbatim", "abbd understand"} {
		if !strings.Contains(skill, want) {
			t.Errorf("abbd-setup must name %s", want)
		}
	}
	if strings.Contains(skill, "abbd align") || strings.Contains(skill, "abbd confirm") {
		t.Errorf("abbd-setup must not send anyone to an alignment step")
	}
}
