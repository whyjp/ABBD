package main

import (
	"path/filepath"
	"testing"

	"github.com/whyjp/abbd/internal/store"
)

func (p *project) wantDenied(t *testing.T, deny bool, calls ...map[string]any) {
	t.Helper()
	for _, c := range calls {
		o, out := p.fire("PreToolUse", c)
		if got := denied(o) != ""; got != deny {
			t.Errorf("PreToolUse deny=%v for %q, want %v (%s)", got, c["tool_input"], deny, out)
		}
	}
}

func pwsh(cmd string) map[string]any { return tool("PowerShell", map[string]any{"command": cmd}) }

// Each command here reaches a human's command, the turn record or the mode
// through a spelling the first guard missed.
func TestPreToolUseDeniesEverySpellingOfAHumansWrite(t *testing.T) {
	p := activeTurnProject(t, topoV1).project
	sessions := store.Open(p.root).SessionsDir()
	var calls []map[string]any
	for _, c := range []string{
		`abbd approve K`,
		`./abbd approve K`,
		`C:/bin/abbd.exe approve K`,
		`FOO=1 abbd approve K`,
		`sh -c "abbd approve K"`,
		`abbd "approve" K`,
		`abbd 'mode' "off" --session s1`,
		`abbd --root . approve K`,
		`abbd mode off --session s1`,
		"abbd turn start --session s1 --seeds MOD-0001 --request - <<EOF\n$(abbd approve K)\nEOF",
		"cat <<'EOF' | bash\nabbd approve K\nEOF",
		`bash <<< "abbd approve K"`,
		"echo \"`abbd approve K`\"",
		`cd .abbd && rm intended.yaml`,
		`cd .abbd; echo x > intended.yaml`,
		`cp /tmp/intended.yaml .abbd/`,
		`mv x .abbd/intended.yaml`,
		`rm .abbd/sessions/*`,
		`rm -rf .abbd`,
		`rm -rf .ab*`,
		`rm -rf .git`,
		`rm -f .git/abbd/sessions/s1.on`,
		`git checkout -- .abbd`,
		`git checkout -- .`,
		`git restore .abbd/intended.yaml`,
		`git clean -fdx`,
		`git stash -u`,
		`git reset --hard`,
		`tee .abbd/project.yaml < x`,
		`sed -i s/a/b/ .abbd/intended.yaml`,
		`python -c "open('.abbd/intended.yaml','w')"`,
		`echo x > .abbd/extractor/extract.py`,
	} {
		calls = append(calls, p.bash(c))
	}
	for _, c := range []string{
		`& 'C:\bin\abbd.exe' approve K`,
		`Remove-Item .abbd\sessions -Recurse`,
		`Set-Content -Path .abbd\units.yaml -Value x`,
		"@'\nabbd approve K\n'@ | Invoke-Expression",
	} {
		calls = append(calls, pwsh(c))
	}
	for _, f := range []string{
		filepath.Join(p.root, ".abbd", "intended.yaml"),
		".abbd/intended.yaml",
		`.abbd\INTENDED.yaml`,
		filepath.Join(p.root, ".abbd", ".", "intended.yaml"),
		filepath.Join(p.root, "x", "..", ".abbd", "decisions", "ADR-1.md"),
		filepath.Join(p.root, ".abbd", "units.yaml"),
		filepath.Join(p.root, ".abbd", "state.yaml"),
		filepath.Join(p.root, ".abbd", ".gitignore"),
		filepath.Join(p.root, ".abbd", "extractor", "extract.py"),
		filepath.Join(sessions, "s1.on"),
	} {
		calls = append(calls, tool("Write", map[string]any{"file_path": f, "content": "x"}))
	}
	p.wantDenied(t, true, calls...)
}

// A command that only reads, or carries text that merely mentions a protected
// name, is not the guard's to deny.
func TestPreToolUseAllowsReadsAndRequestText(t *testing.T) {
	p := activeTurnProject(t, topoV1).project
	var calls []map[string]any
	for _, c := range []string{
		"cd sub && abbd turn start --session s1 --seeds MOD-0001 --request - <<'EOF'\nfix .abbd/intended.yaml parsing, then abbd approve it\nEOF",
		"git commit -q -F - <<'EOF'\nTell abbd-mode that abbd approve is the human's\nEOF",
		`grep -rn "abbd approve" docs`,
		`cat .abbd/intended.yaml`,
		`git diff -- .abbd && git log --oneline -3`,
		`abbd mode on --session s9`,
		`abbd ingest fake.json`,
		`rm -rf build/*`,
		`cp ../notes.txt .`,
		`git checkout -b charge-in-cents`,
		`echo "abbd approve K" > notes.txt`,
	} {
		calls = append(calls, p.bash(c))
	}
	for _, c := range []string{
		"@'\nmake abbd approve take many keys in .abbd/intended.yaml\n'@ | abbd turn start --session s1 --seeds MOD-0001 --request -",
		`Get-Content .abbd\intended.yaml`,
	} {
		calls = append(calls, pwsh(c))
	}
	p.wantDenied(t, false, calls...)
}

// The greenfield window opens abbd setup -- ARGV and .abbd/extractor/ only,
// and only for a command that does nothing else.
func TestGreenfieldSetupIsAllowedOnlyInItsExactForm(t *testing.T) {
	p := activeTurnProject(t, topoV1).project
	write(t, filepath.Join(p.root, ".abbd", "project.yaml"), "extract: []\n")
	var deny []map[string]any
	for _, c := range []string{
		"abbd setup -- python .abbd/extractor/extract.py && abbd approve new-relation:MOD-0001.depends_on.MOD-0002",
		"abbd setup -- python x.py; echo [] > .abbd/intended.yaml",
		"abbd setup --adopt doc.md -- python x.py",
		"abbd setup -- python x.py > .abbd/intended.yaml",
		"cd sub && abbd setup -- python x.py",
		"abbd setup -- python x.py && echo registered",
		"abbd setup -- python $(abbd approve K)",
		"abbd setup --greenfield units.json",
	} {
		deny = append(deny, p.bash(c))
	}
	deny = append(deny, tool("Write", map[string]any{"file_path": filepath.Join(p.root, ".abbd", "intended.yaml"), "content": "x"}))
	p.wantDenied(t, true, deny...)
	p.wantDenied(t, false,
		p.bash("abbd setup -- python .abbd/extractor/extract.py"),
		tool("Write", map[string]any{"file_path": filepath.Join(p.root, ".abbd", "extractor", "extract.py"), "content": "print('{}')"}),
	)
}

// Once registered, a script outside .abbd/ is as fixed as one inside it.
func TestRegisteredScriptOutsideAbbdIsProtected(t *testing.T) {
	p := activeTurnProject(t, topoV1).project
	write(t, filepath.Join(p.root, "tools", "extract.py"), "print('{}')\n")
	write(t, filepath.Join(p.root, ".abbd", "project.yaml"), "extract: [python, tools/extract.py]\n")
	p.wantDenied(t, true,
		tool("Edit", map[string]any{"file_path": filepath.Join(p.root, "tools", "extract.py"), "old_string": "{}", "new_string": "x"}),
		p.bash("echo x >> tools/extract.py"),
	)
	p.wantDenied(t, false,
		p.bash("cat tools/extract.py"),
		tool("Edit", map[string]any{"file_path": filepath.Join(p.root, "tools", "other.py"), "old_string": "a", "new_string": "b"}),
	)
}
