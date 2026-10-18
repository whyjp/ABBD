package analysis

import (
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/whyjp/for-my-harness/internal/model"
)

type servingIndex struct {
	rp      *repo
	res     *Result
	modules map[string]ModuleFacts
	docker  map[string]ServingFacts // by Dockerfile path
}

func servingFacts(rp *repo, res *Result) []ServingFacts {
	s := &servingIndex{rp: rp, res: res, modules: map[string]ModuleFacts{}, docker: map[string]ServingFacts{}}
	for _, m := range res.Modules {
		if _, ok := s.modules[m.Path]; !ok || m.Language == "go" {
			s.modules[m.Path] = m
		}
	}
	var out []ServingFacts
	for _, f := range rp.files {
		if unit, ok := dockerfileUnit(path.Base(f)); ok {
			sf := s.dockerfile(f, unit)
			s.docker[f] = sf
			out = append(out, sf)
		}
	}
	for _, f := range rp.files {
		ext := path.Ext(f)
		if ext != ".yml" && ext != ".yaml" {
			continue
		}
		data, err := rp.read(f)
		if err != nil {
			continue
		}
		if isComposeFile(path.Base(f)) {
			out = append(out, s.compose(f, string(data))...)
		} else {
			out = append(out, s.k8s(f, string(data))...)
		}
	}
	return out
}

// dockerfileUnit reports whether base is a Dockerfile and returns the unit its name implies, if any.
func dockerfileUnit(base string) (string, bool) {
	switch {
	case base == "Dockerfile":
		return "", true
	case strings.HasPrefix(base, "Dockerfile."):
		return strings.TrimPrefix(base, "Dockerfile."), true
	case strings.HasSuffix(base, ".Dockerfile"):
		return strings.TrimSuffix(base, ".Dockerfile"), true
	}
	return "", false
}

func isComposeFile(base string) bool {
	switch base {
	case "docker-compose.yml", "docker-compose.yaml", "compose.yml", "compose.yaml":
		return true
	}
	return false
}

func (s *servingIndex) dockerfile(f, unit string) ServingFacts {
	sf := ServingFacts{Source: f, Origin: model.Extracted}
	data, err := s.rp.read(f)
	if err != nil {
		s.res.Unknowns = append(s.res.Unknowns, "cannot read "+f+": "+err.Error())
		return sf
	}
	dir := path.Dir(f)
	direct := map[string]bool{}
	for _, line := range dockerInstructions(string(data)) {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		args := fields[1:]
		switch strings.ToUpper(fields[0]) {
		case "RUN":
			for _, t := range goBuildTargets(args) {
				s.mapToken(dir, t, direct)
			}
		case "ENTRYPOINT", "CMD":
			for _, t := range args {
				t = strings.Trim(t, `[]",'`)
				s.mapBinary(path.Base(t), direct)
				s.mapToken(dir, t, direct)
			}
		case "COPY", "ADD":
			if slices.ContainsFunc(args, func(a string) bool { return strings.HasPrefix(a, "--from") }) {
				continue
			}
			for _, t := range args[:len(args)-1] {
				if !strings.HasPrefix(t, "--") {
					s.mapToken(dir, t, direct)
				}
			}
		}
	}
	entries := model.SortedKeys(direct)
	switch {
	case unit != "":
		sf.DeployUnit = unit
	case len(entries) == 1 && s.modules[entries[0]].IsMain:
		sf.DeployUnit = s.rp.dirName(entries[0])
	default:
		sf.DeployUnit = s.rp.dirName(dir)
	}
	sf.ModulePaths = s.closure(entries)
	if len(sf.ModulePaths) == 0 {
		s.res.Unknowns = append(s.res.Unknowns, f+": no build, COPY or ENTRYPOINT hint maps it to a module")
	}
	return sf
}

// dockerInstructions joins backslash continuations and drops comments.
func dockerInstructions(src string) []string {
	var out []string
	cur := ""
	for _, line := range strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "#") {
			continue
		}
		if strings.HasSuffix(t, "\\") {
			cur += strings.TrimSuffix(t, "\\") + " "
			continue
		}
		if cur += t; strings.TrimSpace(cur) != "" {
			out = append(out, cur)
		}
		cur = ""
	}
	return out
}

func goBuildTargets(args []string) []string {
	var out []string
	for i := 0; i+1 < len(args); i++ {
		if args[i] != "go" || (args[i+1] != "build" && args[i+1] != "install") {
			continue
		}
		for j := i + 2; j < len(args); j++ {
			a := args[j]
			if a == "&&" || a == ";" || a == "||" {
				break
			}
			if a == "-o" || a == "-ldflags" || a == "-tags" {
				j++
				continue
			}
			if strings.HasPrefix(a, ".") {
				out = append(out, a)
			}
		}
	}
	return out
}

func (s *servingIndex) mapToken(dir, tok string, direct map[string]bool) {
	tok = strings.TrimSuffix(strings.TrimSuffix(tok, "/..."), "/")
	if tok == "" || strings.HasPrefix(tok, "/") || strings.Contains(tok, "$") {
		return
	}
	p := path.Clean(path.Join(dir, tok))
	if _, ok := s.modules[p]; ok {
		direct[p] = true
	} else if s.rp.set[p] {
		if mod, ok := moduleFor(path.Dir(p), s.res.Modules); ok {
			direct[mod] = true
		}
	}
}

func (s *servingIndex) mapBinary(name string, direct map[string]bool) {
	for p, m := range s.modules {
		if m.IsMain && path.Base(p) == name {
			direct[p] = true
		}
	}
}

// closure adds every in-repo module the entry modules import, since all of them ship in the unit.
func (s *servingIndex) closure(entries []string) []string {
	seen := map[string]bool{}
	queue := slices.Clone(entries)
	for len(queue) > 0 {
		p := queue[0]
		queue = queue[1:]
		if seen[p] {
			continue
		}
		seen[p] = true
		queue = append(queue, s.modules[p].Imports...)
	}
	return model.SortedKeys(seen)
}

type composeService struct{ name, build, dockerfile, image string }

var yamlKV = regexp.MustCompile(`^([\w.-]+):\s*(.*)$`)

func indentOf(line string) int { return len(line) - len(strings.TrimLeft(line, " ")) }

// composeServices reads the top-level services map of a compose file without a YAML parser.
func composeServices(src string) []composeService {
	var out []composeService
	in, svcIndent, buildIndent := false, -1, -1
	for _, line := range strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n") {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		ind := indentOf(line)
		if ind == 0 {
			in = t == "services:"
			svcIndent = -1
			continue
		}
		if !in {
			continue
		}
		if svcIndent < 0 {
			svcIndent = ind
		}
		m := yamlKV.FindStringSubmatch(t)
		if m == nil {
			continue
		}
		key, val := m[1], strings.Trim(m[2], `"'`)
		switch {
		case ind == svcIndent:
			out = append(out, composeService{name: strings.Trim(key, `"'`)})
			buildIndent = -1
		case len(out) == 0:
		case key == "build" && ind > svcIndent:
			buildIndent = ind
			out[len(out)-1].build = val
		case key == "context" && buildIndent >= 0 && ind > buildIndent:
			out[len(out)-1].build = val
		case key == "dockerfile" && buildIndent >= 0 && ind > buildIndent:
			out[len(out)-1].dockerfile = val
		case key == "image":
			out[len(out)-1].image = val
		}
	}
	return out
}

func (s *servingIndex) compose(f, src string) []ServingFacts {
	var out []ServingFacts
	for _, svc := range composeServices(src) {
		sf := ServingFacts{DeployUnit: svc.name, Source: f, Origin: model.Extracted}
		if svc.build != "" {
			df := svc.dockerfile
			if df == "" {
				df = "Dockerfile"
			}
			sf.ModulePaths = s.docker[path.Clean(path.Join(path.Dir(f), svc.build, df))].ModulePaths
		} else if svc.image != "" {
			sf.ModulePaths = s.byImage(svc.image)
		}
		if len(sf.ModulePaths) == 0 {
			s.res.Unknowns = append(s.res.Unknowns, f+": service "+svc.name+" is not mapped to a module")
		}
		out = append(out, sf)
	}
	return out
}

var (
	k8sWorkload = regexp.MustCompile(`(?m)^kind:\s*["']?(Deployment|StatefulSet|DaemonSet|Job|CronJob)["']?\s*$`)
	k8sImage    = regexp.MustCompile(`(?m)^\s*-?\s*image:\s*["']?([^"'\s]+)`)
	k8sName     = regexp.MustCompile(`^name:\s*["']?([^"'\s]+)`)
)

func (s *servingIndex) k8s(f, src string) []ServingFacts {
	var out []ServingFacts
	for _, doc := range regexp.MustCompile(`(?m)^---\s*$`).Split(src, -1) {
		if !k8sWorkload.MatchString(doc) {
			continue
		}
		name := k8sMetadataName(doc)
		if name == "" {
			s.res.Unknowns = append(s.res.Unknowns, f+": workload without metadata.name")
			continue
		}
		sf := ServingFacts{DeployUnit: name, Source: f, Origin: model.Extracted}
		for _, m := range k8sImage.FindAllStringSubmatch(doc, -1) {
			sf.ModulePaths = append(sf.ModulePaths, s.byImage(m[1])...)
		}
		sf.ModulePaths = sortedUnique(sf.ModulePaths)
		if len(sf.ModulePaths) == 0 {
			s.res.Unknowns = append(s.res.Unknowns, f+": workload "+name+" image is not mapped to a module")
		}
		out = append(out, sf)
	}
	return out
}

func k8sMetadataName(doc string) string {
	inMeta, childIndent := false, -1
	for _, line := range strings.Split(strings.ReplaceAll(doc, "\r\n", "\n"), "\n") {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		ind := indentOf(line)
		if ind == 0 {
			inMeta = t == "metadata:"
			continue
		}
		if !inMeta {
			continue
		}
		if childIndent < 0 {
			childIndent = ind
		}
		if ind == childIndent {
			if m := k8sName.FindStringSubmatch(t); m != nil {
				return m[1]
			}
		}
	}
	return ""
}

// byImage maps an image reference to modules via a Dockerfile unit or main module of the same name.
func (s *servingIndex) byImage(image string) []string {
	base := path.Base(image)
	if i := strings.IndexAny(base, ":@"); i >= 0 {
		base = base[:i]
	}
	for _, f := range model.SortedKeys(s.docker) {
		if s.docker[f].DeployUnit == base {
			return s.docker[f].ModulePaths
		}
	}
	direct := map[string]bool{}
	s.mapBinary(base, direct)
	return s.closure(model.SortedKeys(direct))
}
