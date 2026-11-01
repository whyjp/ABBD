package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const (
	blockBegin = "<!-- abbd:begin -->"
	blockEnd   = "<!-- abbd:end -->"
)

// block is the always-on instruction an assistant reads in every session, so
// it asks the blueprint before it greps and leaves the topology current.
const block = blockBegin + `
## ABBD blueprint

This repository keeps its architecture topology in ` + "`.abbd/`" + `, drawn at ` + "`.abbd/blueprint/index.html`" + `.

- Ask architecture questions of the blueprint first, before grepping: ` + "`abbd explain <ID|NAME>`" + `, ` + "`abbd path <A> <B>`" + `, ` + "`abbd impact <IDS>`" + `.
- After changing code, run ` + "`abbd sync`" + `, put ` + "`abbd diff`" + ` in the PR description, and commit ` + "`.abbd/`" + ` with the code.
- The /abbd skill says how to read, query and record the topology.
` + blockEnd + "\n"

// install writes the instruction block into CLAUDE.md at the project root,
// replacing the block an earlier install wrote and keeping everything else.
func (e env) install() error {
	path := filepath.Join(e.root, "CLAUDE.md")
	old, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	text := string(old)
	nl := "\n"
	if strings.Contains(text, "\r\n") {
		nl = "\r\n"
	}
	want := strings.ReplaceAll(block, "\n", nl)
	begin, end := markers(text)
	var next string
	switch {
	case begin < 0 && end < 0:
		if text != "" && !strings.HasSuffix(text, nl) {
			text += nl
		}
		if text != "" {
			text += nl
		}
		next = text + want
	case begin >= 0 && end > begin:
		next = text[:begin] + want + text[end:]
	default:
		return fmt.Errorf("%s has an unmatched %s or %s marker; fix it by hand, then rerun abbd install", path, blockBegin, blockEnd)
	}
	if next == string(old) {
		fmt.Fprintln(e.stdout, "CLAUDE.md: the abbd block is current")
		return nil
	}
	if err := os.WriteFile(path, []byte(next), 0o644); err != nil {
		return err
	}
	if begin >= 0 {
		fmt.Fprintln(e.stdout, "CLAUDE.md: replaced the abbd block")
	} else {
		fmt.Fprintln(e.stdout, "CLAUDE.md: added the abbd block")
	}
	return nil
}

// markers returns where the block's begin line starts and where its end line
// ends, or -1 for a marker not found. A marker counts only alone on its line
// and outside fenced code, so a CLAUDE.md that shows the markers in an
// example keeps the example.
func markers(text string) (begin, end int) {
	begin, end = -1, -1
	fence := ""
	for off := 0; off < len(text); {
		next := len(text)
		if n := strings.IndexByte(text[off:], '\n'); n >= 0 {
			next = off + n + 1
		}
		line := strings.TrimSpace(text[off:next])
		switch {
		case fence != "":
			if strings.HasPrefix(line, fence) {
				fence = ""
			}
		case strings.HasPrefix(line, "```") || strings.HasPrefix(line, "~~~"):
			fence = line[:3]
		case line == blockBegin && begin < 0:
			begin = off
		case line == blockEnd && end < 0:
			end = next
		}
		off = next
	}
	return begin, end
}
