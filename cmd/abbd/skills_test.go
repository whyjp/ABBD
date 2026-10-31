package main

import (
	"bytes"
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

// Every abbd command a plugin skill tells an agent to run must be one the CLI
// accepts, so a renamed flag cannot leave a skill silently broken.
func TestSkillCommandsAreRealCLICommands(t *testing.T) {
	skills, err := filepath.Glob(filepath.Join("..", "..", "plugins", "*", "skills", "*", "SKILL.md"))
	if err != nil || len(skills) == 0 {
		t.Fatalf("no plugin skills found: %v", err)
	}
	seen := 0
	for _, path := range skills {
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
