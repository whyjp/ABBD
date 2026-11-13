package runscan

import (
	"path"
	"regexp"
	"slices"
	"strings"
)

var (
	cppClass  = regexp.MustCompile(`\b(?:class|struct)\s+([A-Za-z_]\w*)\s*(?:final\s*)?:([^;{]*)\{`)
	cppAccess = regexp.MustCompile(`\b(?:public|protected|private|virtual)\b`)
)

// receivers holds, per class a catalog entry names in Recv, the variables
// declared with that class or a subclass of it, keyed by the stem of the
// declaring file, so foo.h and foo.cpp share their members.
type receivers map[string]map[string]map[string]bool

// stem is a file's path without its extension.
func stem(p string) string { return strings.TrimSuffix(p, path.Ext(p)) }

// newReceivers reads the C and C++ files for each Recv class's family,
// the class and every class that derives from it however deep, and then
// for the variables, members and parameters declared with one.
func newReceivers(fs []file) receivers {
	roots := map[string]bool{}
	for _, e := range Catalog {
		if e.Recv != "" {
			roots[e.Recv] = true
		}
	}
	bases := map[string][]string{}
	for _, f := range fs {
		if f.lang != C {
			continue
		}
		for _, m := range cppClass.FindAllSubmatch(f.code, -1) {
			for _, b := range strings.Split(cppAccess.ReplaceAllString(string(m[2]), ""), ",") {
				if b = strings.TrimSpace(b); b != "" {
					bases[string(m[1])] = append(bases[string(m[1])], b)
				}
			}
		}
	}
	rs := receivers{}
	for root := range roots {
		family := map[string]bool{root: true}
		for grew := true; grew; {
			grew = false
			for class, bs := range bases {
				if !family[class] && slices.ContainsFunc(bs, func(b string) bool { return family[b[strings.LastIndex(b, ":")+1:]] }) {
					family[class], grew = true, true
				}
			}
		}
		names := make([]string, 0, len(family))
		for c := range family {
			names = append(names, regexp.QuoteMeta(c))
		}
		slices.Sort(names)
		t := `(?:` + strings.Join(names, "|") + `)`
		decl := regexp.MustCompile(
			`\b` + t + `\s*(?:[*&]\s*|const\s+)*([A-Za-z_]\w*)\s*[;=,)({\[]` +
				`|<\s*` + t + `\s*\*?\s*>\s*(?:[*&]\s*)*([A-Za-z_]\w*)` +
				`|\bauto\s*[*&]?\s*([A-Za-z_]\w*)\s*=\s*(?:new\s+|std::make_(?:unique|shared)\s*<\s*)?` + t + `\b`)
		rs[root] = map[string]map[string]bool{}
		for _, f := range fs {
			if f.lang != C {
				continue
			}
			for _, m := range decl.FindAllSubmatch(f.code, -1) {
				for _, g := range m[1:] {
					if len(g) > 0 {
						k := stem(f.path)
						if rs[root][k] == nil {
							rs[root][k] = map[string]bool{}
						}
						rs[root][k][string(g)] = true
					}
				}
			}
		}
	}
	return rs
}

// declared reports whether the file, or a file of its stem, declares name
// with class or a subclass of it.
func (rs receivers) declared(f file, class, name string) bool {
	return rs[class][stem(f.path)][name]
}
