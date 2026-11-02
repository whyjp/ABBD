// Package group places a crowded level in the directories the code has.
// When a module, or the top level, holds more boxes than one view shows,
// sync places the units the script placed, or nobody did, in the directory
// their code lives in. A directory is a level the project itself shows.
// It is a writer of its own: each placement is a group parent claim, which
// ranks below a human's and above the script's. It only moves units whose
// winning claim is the script's, or that have none, and it runs again on
// every sync from scratch, so it never outlives the layout it read.
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

// Fallback groups the units of every container that holds more boxes than
// Budget (at the top level, its modules), by the shallowest directory that
// splits them, and returns the units with its groups present and the
// high-water marks moved past any ID it took. Groups it made earlier are
// reused by container and directory, and left absent as unneeded otherwise.
func Fallback(units []model.Unit, hw map[model.Kind]int) ([]model.Unit, map[model.Kind]int) {
	out := make([]model.Unit, len(units))
	copy(out, units)
	next := map[model.Kind]int{}
	for k, v := range hw {
		next[k] = v
	}
	// A group is the fallback's by the container it sits in and its directory:
	// two containers can each have a group of the same directory.
	type place struct {
		in  model.ID
		dir string
	}
	made := map[place]int{}
	for i, u := range out {
		if Made(u) {
			in, _ := u.Claim(model.Group)
			made[place{in.To, u.Anchors[0].Path}] = i
			out[i].SetAbsent(model.Unneeded)
		}
	}
	// Start from the layout without the fallback's claims: each unit they
	// held falls back to its next claim.
	for i := range out {
		out[i].DropParents(model.Group)
	}
	model.Resolve(out)
	scripted := make([]bool, len(out))
	for i, u := range out {
		scripted[i] = u.Parent == "" || u.ParentOrigin == model.Extracted
	}
	placedByScript := func(i int) bool { return scripted[i] }
	claim := func(i int, to model.ID) {
		out[i].Parent = to
		if to != "" {
			out[i].PutParent(model.ParentClaim{To: to, Writer: model.Group})
		}
	}
	at := map[model.ID]int{}
	for i, u := range out {
		at[u.ID] = i
	}
	// A path is a directory when an anchor lies under it. The
	// store says so on every machine alike, and a file never has one.
	dirs := map[string]bool{}
	for _, u := range out {
		if u.Absent || Made(u) {
			continue
		}
		for _, a := range u.Anchors {
			for d := path.Dir(a.Path); d != "." && d != "/" && !dirs[d]; d = path.Dir(d) {
				dirs[d] = true
			}
		}
	}
	var split func(container model.ID)
	split = func(container model.ID) {
		// Only units below the container's own directory group: a group of
		// that directory or one above it repeats the container, and a unit
		// outside it has no directory of the container's to go in.
		own := ""
		if container != "" {
			own = span(out[at[container]], dirs)
		}
		var members []int
		for i, u := range out {
			if !u.Absent && u.Parent == container && (u.Kind == model.Module || container != "") && !Made(u) && placedByScript(i) && below(home(u), own) {
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
			g, ok := made[place{container, dir}]
			if !ok {
				next[model.Module]++
				out = append(out, model.Unit{
					ID:      model.FormatID(model.Module, next[model.Module]),
					Kind:    model.Module,
					Name:    model.Said(model.Group, dir+"/"),
					Anchors: []model.Anchor{{Path: dir, Symbol: marker}},
				})
				scripted = append(scripted, true)
				g = len(out) - 1
				at[out[g].ID] = g
				made[place{container, dir}] = g
			}
			out[g].SetAbsent("")
			claim(g, container)
			for _, i := range is {
				claim(i, out[g].ID)
			}
		}
		for _, dir := range names {
			if g, ok := made[place{container, dir}]; ok && !out[g].Absent && out[g].Parent == container {
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
	model.Resolve(out)
	return out, next
}

// home is the directory a unit's first anchor lives in.
func home(u model.Unit) string {
	if len(u.Anchors) == 0 {
		return "."
	}
	return path.Dir(u.Anchors[0].Path)
}

// span is the directory a container spans: a group's own, or the deepest
// directory that holds every anchor of a module, "" for the root. An anchor
// in dirs is a directory and holds itself.
func span(u model.Unit, dirs map[string]bool) string {
	if Made(u) {
		return u.Anchors[0].Path
	}
	var common []string
	for i, a := range u.Anchors {
		d := a.Path
		if !dirs[d] {
			d = path.Dir(d)
		}
		parts := strings.Split(d, "/")
		if parts[0] == "." {
			return ""
		}
		if i == 0 {
			common = parts
			continue
		}
		n := 0
		for n < len(common) && n < len(parts) && common[n] == parts[n] {
			n++
		}
		common = common[:n]
	}
	return strings.Join(common, "/")
}

// below reports whether directory d lies under directory own, "" for the
// root.
func below(d, own string) bool {
	return own == "" || strings.HasPrefix(d, own+"/")
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
