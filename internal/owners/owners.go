// Package owners reads who owns which code from a repository's CODEOWNERS,
// as GitHub and GitLab do: the last pattern a path matches names its owners.
// It is a fact the repository states, read as it stands, never inferred.
package owners

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Owners is a CODEOWNERS file's rules in the order they appear.
type Owners struct {
	rules []rule
}

type rule struct {
	re     *regexp.Regexp
	owners []string
}

// Read finds CODEOWNERS where GitHub looks for it, under root, and parses
// it; with none, every path has no owner.
func Read(root string) Owners {
	for _, p := range []string{".github/CODEOWNERS", "CODEOWNERS", "docs/CODEOWNERS"} {
		if b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(p))); err == nil {
			return Parse(string(b))
		}
	}
	return Owners{}
}

// Parse reads CODEOWNERS text: a pattern and its owners per line, # for a
// comment.
func Parse(text string) Owners {
	var o Owners
	for _, line := range strings.Split(text, "\n") {
		if i := strings.Index(line, "#"); i >= 0 {
			line = line[:i]
		}
		f := strings.Fields(line)
		if len(f) == 0 {
			continue
		}
		o.rules = append(o.rules, rule{re: compile(f[0]), owners: f[1:]})
	}
	return o
}

// For names the owners of a repository path, slash-separated and relative
// to the root: those of the last rule that matches it.
func (o Owners) For(path string) []string {
	path = strings.TrimPrefix(filepath.ToSlash(path), "./")
	for i := len(o.rules) - 1; i >= 0; i-- {
		if o.rules[i].re.MatchString(path) {
			return o.rules[i].owners
		}
	}
	return nil
}

// compile turns a CODEOWNERS pattern into a regexp over a path: "/" at the
// start anchors it at the root, else it matches at any depth; a trailing "/"
// or a plain name matches everything under it; "*" stays within a segment
// and "**" crosses them.
func compile(p string) *regexp.Regexp {
	anchored := strings.HasPrefix(p, "/") || strings.Contains(strings.TrimSuffix(p, "/"), "/")
	p = strings.TrimPrefix(p, "/")
	var b strings.Builder
	if anchored {
		b.WriteString("^")
	} else {
		b.WriteString("(^|/)")
	}
	for i := 0; i < len(p); i++ {
		switch {
		case strings.HasPrefix(p[i:], "**"):
			b.WriteString(".*")
			i++
		case p[i] == '*':
			b.WriteString("[^/]*")
		case p[i] == '?':
			b.WriteString("[^/]")
		default:
			b.WriteString(regexp.QuoteMeta(string(p[i])))
		}
	}
	if strings.HasSuffix(p, "/") {
		b.WriteString(".*")
	} else {
		b.WriteString("(/.*)?")
	}
	b.WriteString("$")
	return regexp.MustCompile(b.String())
}
