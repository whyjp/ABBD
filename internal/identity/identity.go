// Package identity implements anchor-bundle stable IDs (PRD ch.06, FR-27, AC-BOOT-03).
package identity

import (
	"path"

	"github.com/whyjp/cccv/internal/model"
)

type Verdict int

const (
	New Verdict = iota
	Candidate
	Same
)

// Policy is the match_policy of PRD ch.06.
const (
	SameIfAtLeast = 2
	CandidateIf   = 1
)

type Existing struct {
	ID      string
	Anchors []model.Anchor
}

type Match struct {
	Verdict Verdict
	ID      string
	Matched []model.Anchor
}

// Resolve finds the existing object that best matches the given anchors.
// renames maps old path -> new path (git rename detection or content-hash moves) and is
// applied to existing path anchors before counting.
func Resolve(anchors []model.Anchor, existing []Existing, renames map[string]string, taken map[string]bool) Match {
	best := Match{Verdict: New}
	for _, e := range existing {
		if taken[e.ID] {
			continue
		}
		matched := intersect(anchors, Rewrite(e.Anchors, renames))
		if len(matched) > len(best.Matched) {
			best = Match{ID: e.ID, Matched: matched}
		}
	}
	switch {
	case len(best.Matched) >= SameIfAtLeast:
		best.Verdict = Same
	case len(best.Matched) == CandidateIf:
		best.Verdict = Candidate
	default:
		best = Match{Verdict: New}
	}
	return best
}

// Rewrite applies path renames to path anchors and to path-shaped prefixes of other anchors.
func Rewrite(anchors []model.Anchor, renames map[string]string) []model.Anchor {
	if len(renames) == 0 {
		return anchors
	}
	out := make([]model.Anchor, len(anchors))
	for i, a := range anchors {
		if a.Kind == model.AnchorPath {
			if to, ok := renames[a.Ref]; ok {
				a.Ref = to
			}
		}
		out[i] = a
	}
	return out
}

func intersect(a, b []model.Anchor) []model.Anchor {
	set := map[model.Anchor]bool{}
	for _, x := range b {
		set[x] = true
	}
	var out []model.Anchor
	seen := map[model.Anchor]bool{}
	for _, x := range a {
		if set[x] && !seen[x] {
			out = append(out, x)
			seen[x] = true
		}
	}
	return out
}

// ContentRenames detects moved files by identical content hash: a path that disappeared and
// a new path with the same hash. Works without git.
func ContentRenames(base, next map[string]string) map[string]string {
	gone := map[string]string{}
	for p, h := range base {
		if _, ok := next[p]; !ok {
			gone[h] = p
		}
	}
	out := map[string]string{}
	for p, h := range next {
		if _, existed := base[p]; existed {
			continue
		}
		if old, ok := gone[h]; ok {
			out[old] = p
			// A module anchor is a directory; a moved file also moves its directory anchor.
			if od, nd := path.Dir(old), path.Dir(p); od != nd {
				out[od] = nd
			}
		}
	}
	return out
}
