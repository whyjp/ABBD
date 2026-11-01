// Package group is the last resort that keeps every blueprint view small.
// When a module, or the top level, holds more modules than one view shows
// and no reader grouped them, sync places them in groups by the directory
// their code lives in. A grouping a pstack reader recorded, or a human
// stated, always wins: the fallback only moves modules the script placed or
// left at the top, and it runs again on every sync, so it never outlives the
// layout it read.
package group

import (
	"path"
	"sort"
	"strings"

	"github.com/whyjp/abbd/internal/model"
)

// Budget is how many boxes one blueprint view holds, docs/blueprint-svg.md
// rule 1.
const Budget = 16

// marker is the anchor symbol of a group this package made.
const marker = "(group)"

// Made reports whether the fallback made u.
func Made(u model.Unit) bool {
	return u.Kind == model.Module && len(u.Anchors) == 1 && u.Anchors[0].Symbol == marker
}

// Fallback groups the modules of every container that holds more than
// Budget of them, by the deepest directory that still splits them, and
// returns the units with its groups present and the high-water marks moved
// past any ID it took. Groups it made earlier are reused by directory, and
// left absent when no longer needed.
func Fallback(units []model.Unit, hw map[model.Kind]int) ([]model.Unit, map[model.Kind]int) {
	out := make([]model.Unit, len(units))
	copy(out, units)
	next := map[model.Kind]int{}
	for k, v := range hw {
		next[k] = v
	}
	made := map[string]int{}
	for i, u := range out {
		if Made(u) {
			made[u.Anchors[0].Path] = i
			out[i].Absent = true
		}
	}
	// Start from the layout without the fallback's groups: each unit they
	// held goes back to where the group sat.
	byID := map[model.ID]model.Unit{}
	for _, u := range out {
		byID[u.ID] = u
	}
	for i := range out {
		for p, ok := byID[out[i].Parent]; ok && Made(p) && placedByScript(out[i]); p, ok = byID[out[i].Parent] {
			out[i].Parent = p.Parent
		}
	}
	var split func(container model.ID)
	split = func(container model.ID) {
		var members []int
		for i, u := range out {
			if !u.Absent && u.Parent == container && u.Kind == model.Module && !Made(u) && placedByScript(u) {
				members = append(members, i)
			}
		}
		// The overview draws a top-level contract as a chip on the module
		// that provides it, not as a box.
		boxes := 0
		for _, u := range out {
			if !u.Absent && u.Parent == container && (container != "" || !u.Kind.IsContract()) {
				boxes++
			}
		}
		if boxes <= Budget || len(members) < 2 {
			return
		}
		dirs := map[string][]int{}
		for _, i := range members {
			dirs[home(out[i])] = append(dirs[home(out[i])], i)
		}
		keys := groups(dirs)
		if len(keys) < 2 {
			return
		}
		var names []string
		for k := range keys {
			names = append(names, k)
		}
		sort.Strings(names)
		for _, dir := range names {
			is := keys[dir]
			if len(is) < 2 {
				continue
			}
			g, ok := made[dir]
			if !ok {
				next[model.Module]++
				out = append(out, model.Unit{
					ID:      model.FormatID(model.Module, next[model.Module]),
					Kind:    model.Module,
					Name:    model.Fact{Value: dir + "/", Origin: model.Extracted},
					Anchors: []model.Anchor{{Path: dir, Symbol: marker}},
				})
				g = len(out) - 1
				made[dir] = g
			}
			out[g].Absent, out[g].Parent = false, container
			out[g].ParentOrigin, out[g].ParentSource, out[g].ParentEvidence = "", "", nil
			for _, i := range is {
				out[i].Parent, out[i].ParentOrigin, out[i].ParentSource, out[i].ParentEvidence = out[g].ID, "", "", nil
			}
		}
		for _, dir := range names {
			if g, ok := made[dir]; ok && !out[g].Absent && out[g].Parent == container {
				split(out[g].ID)
			}
		}
	}
	split("")
	var containers []model.ID
	for _, u := range out {
		if !u.Absent && u.Kind == model.Module && !Made(u) {
			containers = append(containers, u.ID)
		}
	}
	sort.Slice(containers, func(i, j int) bool { return containers[i] < containers[j] })
	for _, c := range containers {
		split(c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, next
}

// placedByScript is a unit whose place no reader or human chose.
func placedByScript(u model.Unit) bool {
	return u.ParentOrigin == "" || u.ParentOrigin == model.Extracted
}

// home is the directory a unit's first anchor lives in.
func home(u model.Unit) string {
	if len(u.Anchors) == 0 {
		return "."
	}
	return path.Dir(u.Anchors[0].Path)
}

// groups cuts the directories at the shallowest depth below their common
// prefix that splits them in two or more.
func groups(dirs map[string][]int) map[string][]int {
	var all []string
	for d := range dirs {
		all = append(all, d)
	}
	sort.Strings(all)
	common := strings.Split(all[0], "/")
	for _, d := range all[1:] {
		parts := strings.Split(d, "/")
		n := 0
		for n < len(common) && n < len(parts) && common[n] == parts[n] {
			n++
		}
		common = common[:n]
	}
	for depth := len(common) + 1; ; depth++ {
		out := map[string][]int{}
		deeper := false
		for _, d := range all {
			parts := strings.Split(d, "/")
			if len(parts) > depth {
				deeper = true
				parts = parts[:depth]
			}
			k := strings.Join(parts, "/")
			out[k] = append(out[k], dirs[d]...)
		}
		if len(out) >= 2 || !deeper {
			for k := range out {
				sort.Ints(out[k])
			}
			return out
		}
	}
}
