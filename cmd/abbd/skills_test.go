package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/whyjp/abbd/internal/screen"
)

var (
	fence    = regexp.MustCompile("(?s)```[a-z]*\n(.*?)```")
	inline   = regexp.MustCompile("`(abbd [^`]+)`")
	argument = regexp.MustCompile(`"[^"]*"|'[^']*'|\S+`)

	readerPath   = regexp.MustCompile("`((?:\\.\\./abbd/)?references/reader\\.md)`")
	needsPstack  = regexp.MustCompile(`<pstack>|(?i)install pstack`)
	pstackHowHop = regexp.MustCompile(`\b(?:how(?:'s| reads?)|run how)\b`)
)

func readRepo(t *testing.T, rel ...string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(append([]string{"..", ".."}, rel...)...))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func readPlugin(t *testing.T, rel ...string) string {
	t.Helper()
	return readRepo(t, append([]string{"plugins", "abbd"}, rel...)...)
}

// skills lists every skill the plugin ships.
func skills(t *testing.T) []string {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join("..", "..", "plugins", "*", "skills", "*", "SKILL.md"))
	if err != nil || len(paths) < 2 {
		t.Fatalf("expected the abbd and abbd-setup skills, found %v (%v)", paths, err)
	}
	return paths
}

// The plugin ships only its skills: no hooks, no agent of its own, and no
// mode, since the active mode decides how work runs and ABBD is a capability.
func TestThePluginShipsOnlySkills(t *testing.T) {
	var manifest struct{ Name string }
	if err := json.Unmarshal([]byte(readPlugin(t, ".claude-plugin", "plugin.json")), &manifest); err != nil || manifest.Name != "abbd" {
		t.Fatalf("plugin.json must parse and name the plugin abbd: %v %+v", err, manifest)
	}
	var market struct {
		Plugins []struct{ Name, Source string }
	}
	b, err := os.ReadFile(filepath.Join("..", "..", ".claude-plugin", "marketplace.json"))
	if err != nil || json.Unmarshal(b, &market) != nil || len(market.Plugins) != 1 || market.Plugins[0].Source != "./plugins/abbd" {
		t.Fatalf("marketplace.json must parse and point at ./plugins/abbd: %v %+v", err, market)
	}
	for _, gone := range []string{"hooks", "agents", filepath.Join("skills", "abbd-mode")} {
		if _, err := os.Stat(filepath.Join("..", "..", "plugins", "abbd", gone)); err == nil {
			t.Errorf("the plugin must not ship %s/", gone)
		}
	}
	for _, f := range [][]string{{"README.md"}, {".claude-plugin", "marketplace.json"}, {"plugins", "abbd", ".claude-plugin", "plugin.json"}} {
		if strings.Contains(readRepo(t, f...), "abbd-mode") {
			t.Errorf("%s still names abbd-mode, which no longer exists", filepath.Join(f...))
		}
	}
}

// commandLines lists the abbd command lines a text quotes in code.
func commandLines(text string) []string {
	var lines []string
	for _, m := range fence.FindAllStringSubmatch(text, -1) {
		lines = append(lines, strings.Split(m[1], "\n")...)
	}
	for _, m := range inline.FindAllStringSubmatch(text, -1) {
		lines = append(lines, m[1])
	}
	var out []string
	for _, line := range lines {
		if line = strings.TrimSpace(line); strings.HasPrefix(line, "abbd ") {
			out = append(out, line)
		}
	}
	return out
}

// Every abbd command a skill or the CLAUDE.md block tells an agent to run
// must be one the CLI accepts, so a renamed flag or command cannot leave an
// instruction silently broken.
func TestSkillCommandsAreRealCLICommands(t *testing.T) {
	texts := map[string]string{"the CLAUDE.md block": block}
	for _, path := range skills(t) {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		texts[path] = string(b)
	}
	texts["the reader"] = readPlugin(t, "skills", "abbd", "references", "reader.md")
	seen := 0
	for where, text := range texts {
		for _, line := range commandLines(text) {
			seen++
			args := argument.FindAllString(strings.TrimPrefix(line, "abbd "), -1)
			for i, a := range args {
				if strings.HasPrefix(a, "<") || strings.HasPrefix(a, `"`) || strings.HasPrefix(a, "'") {
					args[i] = "X"
				}
			}
			if _, err := parse(args, &bytes.Buffer{}); err != nil {
				t.Errorf("%s: %q is not a valid abbd command: %v", where, line, err)
			}
		}
	}
	if seen < 12 {
		t.Fatalf("the instructions name only %d abbd commands; the check would prove little", seen)
	}
}

// ABBD works without pstack: both skills read code with the bundled reader,
// and pstack appears only in the reader's paragraph that checks whether it is
// installed before using it as an optional engine.
func TestSkillsReferenceTheBundledReader(t *testing.T) {
	for _, path := range skills(t) {
		b, _ := os.ReadFile(path)
		skill := string(b)
		m := readerPath.FindStringSubmatch(skill)
		if m == nil {
			t.Errorf("%s must name the bundled reader, references/reader.md", path)
			continue
		}
		if _, err := os.Stat(filepath.Join(filepath.Dir(path), m[1])); err != nil {
			t.Errorf("%s names %s, which the plugin does not ship", path, m[1])
		}
		if needsPstack.MatchString(skill) {
			t.Errorf("%s still needs pstack; it must work with the bundled reader alone", path)
		}
		if loc := pstackHowHop.FindStringIndex(skill); loc != nil {
			t.Errorf("%s still sends reading to pstack how: %q", path, skill[loc[0]:loc[1]])
		}
	}
	reader := readPlugin(t, "skills", "abbd", "references", "reader.md")
	var mentions []string
	for _, para := range strings.Split(reader, "\n\n") {
		if strings.Contains(para, "pstack") {
			mentions = append(mentions, para)
		}
	}
	if len(mentions) != 1 || !strings.Contains(mentions[0], "exists") || !strings.Contains(mentions[0], "Glob") {
		t.Fatalf("the reader must name pstack in one paragraph that checks with Glob whether it exists, found %d: %q", len(mentions), mentions)
	}
}

// /abbd is a capability any mode can use: it reads and queries the blueprint
// before the work, records what readers and the human establish, and ends
// with the topology difference. It routes nothing and never says when to ask
// the human, because the active mode owns both.
func TestAbbdQueriesFirstEndsWithTheDiffAndRoutesNothing(t *testing.T) {
	skill := readPlugin(t, "skills", "abbd", "SKILL.md")
	front, _, _ := strings.Cut(strings.TrimPrefix(skill, "---\n"), "\n---\n")
	for _, trigger := range []string{"name: abbd\n", "architecture", "topology", "blueprint", "/abbd"} {
		if !strings.Contains(front, trigger) {
			t.Errorf("/abbd's frontmatter must carry %q so it triggers on architecture questions", trigger)
		}
	}
	if strings.Contains(front, "disable-model-invocation: true") {
		t.Errorf("/abbd must be invocable by the model on an architecture question")
	}
	inOrder := func(section string, order ...string) {
		t.Helper()
		_, body, ok := strings.Cut(skill, "## "+section)
		if !ok {
			t.Fatalf("/abbd has no %s section", section)
		}
		body, _, _ = strings.Cut(body, "\n## ")
		last := -1
		for _, s := range order {
			i := strings.Index(body, s)
			if i <= last {
				t.Fatalf("/abbd's %s must name %v in that order; %q is out of place", section, order, s)
			}
			last = i
		}
	}
	inOrder("Before work", ".abbd/blueprint/index.html", "abbd explain", "abbd path", "abbd impact", "`references/reader.md`", "abbd record relation", "abbd record unit")
	inOrder("What the human states", "abbd record fact", "--source", "abbd record relation")
	inOrder("After work", "abbd sync", "abbd diff", "PR description", "Commit `.abbd/`")
	for _, routing := range []string{`(?i)\bplaybooks?\b`, `(?i)\bautonomy\b`, `AskQuestion`, `(?i)ask the (human|user)`, `(?i)when to ask`, `(?i)\broute\b`, `-mode/SKILL\.md`} {
		if loc := regexp.MustCompile(routing).FindStringIndex(skill); loc != nil {
			t.Errorf("/abbd must not route work or decide when to ask; it says %q", skill[loc[0]:loc[1]])
		}
	}
	for _, gone := range []string{"turn", "approve", "align", "confirm", "BLOCKED", "grade"} {
		if regexp.MustCompile(`\b` + gone + `\b`).MatchString(skill) {
			t.Errorf("/abbd still speaks of %q, which ABBD no longer has", gone)
		}
	}
}

// Setup reads with the bundled reader and adds ABBD's result format,
// which carries what makes a semantic coupling checkable. It has no
// alignment, and it ends by installing the always-on block.
func TestAbbdSetupAsksForCheckableCouplingsAndNoAlignment(t *testing.T) {
	skill := readPlugin(t, "skills", "abbd-setup", "SKILL.md")
	for _, want := range []string{"`../abbd/references/reader.md`", `"evidence"`, `"via"`, `"parent"`, "verbatim", "abbd understand", "`abbd install`"} {
		if !strings.Contains(skill, want) {
			t.Errorf("abbd-setup must name %s", want)
		}
	}
	if strings.Contains(skill, "abbd align") || strings.Contains(skill, "abbd confirm") {
		t.Errorf("abbd-setup must not send anyone to an alignment step")
	}
	_, steps, _ := strings.Cut(skill, "## Steps")
	steps, _, _ = strings.Cut(steps, "\n## ")
	if i := strings.Index(steps, "`abbd install`"); i < 0 || i < strings.LastIndex(steps, "abbd understand") {
		t.Errorf("abbd-setup's steps must install the CLAUDE.md block after storing how's text")
	}
}

// Setup lints the drawings, records the debt that stays, then installs the
// block, and names the way out of the first record's refusal. The bounds it
// gives the agent choosing the units are the ones abbd lint holds.
func TestAbbdSetupLintsRecordsThenInstallsAtTheBoundsLintHolds(t *testing.T) {
	skill := readPlugin(t, "skills", "abbd-setup", "SKILL.md")
	_, steps, _ := strings.Cut(skill, "## Steps")
	steps, _, _ = strings.Cut(steps, "\n## ")
	last := -1
	for _, s := range []string{"run `abbd lint`", "`abbd lint --record`", "`abbd install`"} {
		i := strings.Index(steps, s)
		if i <= last {
			t.Fatalf("abbd-setup's steps must run lint, then lint --record, then install; %q is out of place", s)
		}
		last = i
	}
	if !strings.Contains(steps, "`abbd lint --record --keep-level <VIEW>=<REASON>`") {
		t.Errorf("abbd-setup must name --keep-level, the way out of the first record's refusal for a level it lets stay")
	}
	b := screen.LevelBounds()
	for _, want := range []string{
		fmt.Sprintf("%d to %d top-level modules", b.OverviewFewest, b.OverviewMost),
		fmt.Sprintf("draws %d to %d parts", b.PageFewest, b.PageMost),
		fmt.Sprintf("about %d a box at most", b.LinesPerBox),
	} {
		if !strings.Contains(steps, want) {
			t.Errorf("abbd-setup must state the bound lint holds, %q", want)
		}
	}
}

func claudeMD(t *testing.T, root string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, "CLAUDE.md"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// install writes one marked block into the project's CLAUDE.md: it creates
// the file, replaces an older block where it stood, keeps what the project
// wrote around it, and a second run changes nothing.
func TestInstallKeepsOneBlockInClaudeMD(t *testing.T) {
	p, _ := bare(t)
	p.want(0, "added the abbd block", "install")
	if got := claudeMD(t, p.root); got != block {
		t.Fatalf("a fresh CLAUDE.md holds the block alone:\n%s", got)
	}

	mine := "# Project\n\nUse tabs.\n\n" + blockBegin + "\nold advice\n" + blockEnd + "\n\n## Tests\nRun make test.\n"
	write(t, filepath.Join(p.root, "CLAUDE.md"), mine)
	p.want(0, "replaced the abbd block", "install")
	want := "# Project\n\nUse tabs.\n\n" + block + "\n## Tests\nRun make test.\n"
	if got := claudeMD(t, p.root); got != want {
		t.Fatalf("the old block is replaced where it stood and the rest kept:\n%s", got)
	}
	p.want(0, "the abbd block is current", "install")
	if got := claudeMD(t, p.root); got != want || strings.Count(got, blockBegin) != 1 {
		t.Fatalf("a second install must leave one block and change nothing:\n%s", got)
	}

	write(t, filepath.Join(p.root, "CLAUDE.md"), "# Project\r\nUse tabs.")
	p.want(0, "added the abbd block", "install")
	p.want(0, "the abbd block is current", "install")
	if got := claudeMD(t, p.root); got != "# Project\r\nUse tabs.\r\n\r\n"+strings.ReplaceAll(block, "\n", "\r\n") {
		t.Fatalf("the block follows the file's line endings, after a blank line:\n%q", got)
	}

	for _, broken := range []string{"# Project\n" + blockBegin + "\nhalf a block\n", "# Project\nhalf a block\n" + blockEnd + "\n"} {
		write(t, filepath.Join(p.root, "CLAUDE.md"), broken)
		p.want(1, "unmatched", "install")
		if got := claudeMD(t, p.root); got != broken {
			t.Fatalf("an unmatched marker leaves the file untouched:\n%s", got)
		}
	}
}

// The steps the skill runs after setup, install and understand, leave a
// current state current: neither touches the code the state was synced from.
// A state the code already moved past stays STALE.
func TestInstallAndUnderstandKeepTheStateCurrent(t *testing.T) {
	p := semanticProject(t)
	p.want(0, "state: CURRENT", "status")
	p.want(0, "added the abbd block", "install")
	p.want(0, "state: CURRENT", "status")
	p.want(0, "understood", "understand", "MOD-0001", p.file(t, "how.md", "### Overview\nbilling charges.\n"))
	p.want(0, "state: CURRENT", "status")

	p.code("billing/extra.go", "package billing\n")
	write(t, filepath.Join(p.root, "CLAUDE.md"), "# Project\n")
	p.want(0, "added the abbd block", "install")
	p.want(0, "state: STALE", "status")
}

// A CLAUDE.md that shows the markers in a fenced example keeps the example;
// install appends its own block after it.
func TestInstallLeavesMarkersInsideACodeFenceAlone(t *testing.T) {
	p, _ := bare(t)
	doc := "# Project\n\nThe abbd block looks like:\n\n```\n" + blockBegin + "\nmy own example\n" + blockEnd + "\n```\n"
	write(t, filepath.Join(p.root, "CLAUDE.md"), doc)
	p.want(0, "added the abbd block", "install")
	if got := claudeMD(t, p.root); got != doc+"\n"+block {
		t.Errorf("install changed content outside its block:\n%s", got)
	}
}

// status --check gates a hook or CI on what an agent can bring current in the
// same change: the state, the blueprint, the module texts and drifted evidence.
// Dead evidence is history the views already dropped, so it never fails.
func TestStatusCheckFailsOnlyOnWhatTheChangeCanBringCurrent(t *testing.T) {
	p := semanticProject(t)
	p.want(0, "state: CURRENT", "status", "--check")
	p.want(0, "understood", "understand", "MOD-0001", p.file(t, "how.md", "### Overview\nbilling inserts charges.\n"))
	p.want(0, "understanding: 1 module text(s), 0 STALE", "status", "--check")

	p.code("billing/charge.go", "package billing\n\nfunc Charge() {}\n")
	p.want(1, "the state is STALE", "status", "--check")
	p.resync()
	out := p.want(1, "1 module text(s) STALE", "status", "--check")
	if !strings.Contains(out, "evidence item(s) drifted") || strings.Contains(out, "the state is STALE") {
		t.Fatalf("after sync only the text and the drifted quotes are out of date:\n%s", out)
	}
	p.want(0, "state: CURRENT", "status")

	p.laterRevision()
	p.resync()
	p.want(0, "understood", "understand", "MOD-0001", p.file(t, "how2.md", "### Overview\nbilling charges.\n"))
	out = p.want(0, "0 drifted", "status", "--check")
	if strings.Contains(out, " 0 dead") {
		t.Fatalf("the quote that left the code should be dead now, and dead must not fail the check:\n%s", out)
	}

	write(t, filepath.Join(p.root, ".abbd", "project.yaml"), p.read("project.yaml")+"# edited\n")
	p.want(1, "the blueprint is STALE", "status", "--check")
}

// A Windows checkout with core.autocrlf writes .abbd's text files with CRLF
// endings. The same state must still read current there.
func TestStatusCheckPassesOnACRLFCheckout(t *testing.T) {
	p := semanticProject(t)
	p.want(0, "understood", "understand", "MOD-0001", p.file(t, "how.md", "### Overview\nbilling inserts charges.\n"))
	filepath.WalkDir(filepath.Join(p.root, ".abbd"), func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && filepath.Ext(path) != ".html" {
			b, _ := os.ReadFile(path)
			os.WriteFile(path, bytes.ReplaceAll(b, []byte("\n"), []byte("\r\n")), 0o644)
		}
		return nil
	})
	p.want(0, "blueprint: CURRENT", "status", "--check")
}

// A text whose module left the state, or is no longer one abbd understand
// accepts, shows beside no module and no command can restamp it. The check
// names it and passes, so it cannot block CI for good.
func TestStatusCheckReportsAnOrphanTextAndPasses(t *testing.T) {
	p := semanticProject(t)
	p.want(0, "understood", "understand", "MOD-0001", p.file(t, "how.md", "### Overview\nbilling inserts charges.\n"))
	text := strings.Replace(p.read("understanding/MOD-0001.md"), "unit: MOD-0001", "unit: MOD-0099", 1)
	write(t, filepath.Join(p.root, ".abbd", "understanding", "MOD-0099.md"), text)
	p.resync()
	out := p.want(0, "MOD-0099", "status", "--check")
	if !strings.Contains(out, "1 orphaned") || !strings.Contains(out, "understanding/MOD-0099.md") {
		t.Fatalf("status must name the orphan and how to drop it:\n%s", out)
	}
}

// Without git abbd cannot fingerprint the code, so the check cannot vouch for
// the state. It fails and says why, rather than sending the agent to a sync
// that changes nothing.
func TestStatusCheckFailsOnAnUnknownStateWithTheRealCause(t *testing.T) {
	p := semanticProject(t)
	os.WriteFile(filepath.Join(p.root, ".git", "index"), []byte("not an index"), 0o644)
	out := p.want(1, "git could not fingerprint the working tree", "status", "--check")
	if strings.Contains(out, "the state is UNKNOWN, run abbd sync") {
		t.Fatalf("a sync cannot fix a broken git index:\n%s", out)
	}
	os.RemoveAll(filepath.Join(p.root, ".git"))
	out = p.want(1, "is not a git work tree", "status", "--check")
	if strings.Contains(out, "run abbd sync") {
		t.Fatalf("a sync cannot fingerprint without git:\n%s", out)
	}
}
