package store

// Owners is who owns which code by a repository's CODEOWNERS, as GitHub
// and GitLab read it: the last pattern a path matches names its owners. It
// is a fact the repository states, read as it stands, never inferred.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type Owners struct {
	rules []ownerRule
}

type ownerRule struct {
	re     *regexp.Regexp
	owners []string
}

// ReadOwners finds CODEOWNERS where GitHub looks for it, under root, and parses
// it; with none, every path has no owner.
func ReadOwners(root string) Owners {
	for _, p := range []string{".github/CODEOWNERS", "CODEOWNERS", "docs/CODEOWNERS"} {
		if b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(p))); err == nil {
			return ParseOwners(string(b))
		}
	}
	return Owners{}
}

// ParseOwners reads CODEOWNERS text: a pattern and its owners per line, # for a
// comment.
func ParseOwners(text string) Owners {
	var o Owners
	for _, line := range strings.Split(text, "\n") {
		if i := strings.Index(line, "#"); i >= 0 {
			line = line[:i]
		}
		f := strings.Fields(line)
		if len(f) == 0 {
			continue
		}
		o.rules = append(o.rules, ownerRule{re: ownerPattern(f[0]), owners: f[1:]})
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

// ownerPattern turns a CODEOWNERS pattern into a regexp over a path: "/" at the
// start anchors it at the root, else it matches at any depth; a trailing "/"
// or a plain name matches everything under it; "*" stays within a segment
// and "**" crosses them.
func ownerPattern(p string) *regexp.Regexp {
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
