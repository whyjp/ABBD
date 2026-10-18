package analysis

import (
	"encoding/json"
	"path"
	"regexp"
	"strings"

	"github.com/whyjp/abbd/internal/model"
)

type symbolPattern struct {
	re   *regexp.Regexp
	kind string
}

// regexLang is a deliberately shallow analyzer: symbols, routes and env reads, no imports or calls.
type regexLang struct {
	name     string
	matches  func(file string) bool
	isTest   func(file string) bool
	symbols  []symbolPattern
	routes   func(src string) []routeMatch
	env      []*regexp.Regexp
	testFunc *regexp.Regexp
}

type routeMatch struct {
	selector string
	offset   int
}

var adrFile = regexp.MustCompile(`(?i)^adr[-_ ]?\d+`)

var (
	pyRoute   = regexp.MustCompile(`(?m)^[ \t]*@\w+\.(get|post|put|delete|patch|route)\(\s*["']([^"']+)["']([^\n]*)`)
	pyMethods = regexp.MustCompile(`methods\s*=\s*[\[(]([^\])]*)[\])]`)
	jsRoute   = regexp.MustCompile("\\b\\w+\\.(get|post|put|delete|patch)\\(\\s*[\"'`](/[^\"'`]*)[\"'`]")
)

var languages = []regexLang{
	{
		name:    "python",
		matches: func(f string) bool { return strings.HasSuffix(f, ".py") },
		isTest: func(f string) bool {
			b := path.Base(f)
			return strings.HasPrefix(b, "test_") || strings.HasSuffix(b, "_test.py") || b == "conftest.py"
		},
		symbols: []symbolPattern{
			{regexp.MustCompile(`(?m)^(?:async\s+)?def\s+([A-Za-z]\w*)`), "func"},
			{regexp.MustCompile(`(?m)^class\s+([A-Za-z]\w*)`), "type"},
		},
		routes: pythonRoutes,
		env: []*regexp.Regexp{
			regexp.MustCompile(`os\.(?:getenv|environ\.get)\(\s*["']([^"']+)["']`),
			regexp.MustCompile(`os\.environ\[\s*["']([^"']+)["']\s*\]`),
		},
		testFunc: regexp.MustCompile(`(?m)^(?:async\s+)?def\s+(test_\w+)`),
	},
	jsLang("javascript", ".js", ".jsx", ".mjs", ".cjs"),
	jsLang("typescript", ".ts", ".tsx"),
}

func jsLang(name string, exts ...string) regexLang {
	return regexLang{
		name: name,
		matches: func(f string) bool {
			if strings.HasSuffix(f, ".d.ts") {
				return false
			}
			for _, e := range exts {
				if strings.HasSuffix(f, e) {
					return true
				}
			}
			return false
		},
		isTest: func(f string) bool {
			b := path.Base(f)
			return strings.Contains(b, ".test.") || strings.Contains(b, ".spec.") || strings.Contains(f, "__tests__/")
		},
		symbols: []symbolPattern{
			{regexp.MustCompile(`(?m)^(?:export\s+(?:default\s+)?)?(?:async\s+)?function\s*\*?\s*([A-Za-z_$][\w$]*)`), "func"},
			{regexp.MustCompile(`(?m)^(?:export\s+(?:default\s+)?)?(?:abstract\s+)?class\s+([A-Za-z_$][\w$]*)`), "type"},
			{regexp.MustCompile(`(?m)^export\s+(?:interface|type|enum)\s+([A-Za-z_$][\w$]*)`), "type"},
			{regexp.MustCompile(`(?m)^export\s+(?:const|let|var)\s+([A-Za-z_$][\w$]*)`), "var"},
		},
		routes: func(src string) []routeMatch {
			var out []routeMatch
			for _, m := range jsRoute.FindAllStringSubmatchIndex(src, -1) {
				out = append(out, routeMatch{strings.ToUpper(src[m[2]:m[3]]) + " " + src[m[4]:m[5]], m[0]})
			}
			return out
		},
		env: []*regexp.Regexp{
			regexp.MustCompile(`process\.env\.([A-Za-z_]\w*)`),
			regexp.MustCompile(`process\.env\[\s*["']([^"']+)["']\s*\]`),
		},
	}
}

func pythonRoutes(src string) []routeMatch {
	var out []routeMatch
	for _, m := range pyRoute.FindAllStringSubmatchIndex(src, -1) {
		verb, route, rest := strings.ToUpper(src[m[2]:m[3]]), src[m[4]:m[5]], src[m[6]:m[7]]
		if verb != "ROUTE" {
			out = append(out, routeMatch{verb + " " + route, m[0]})
			continue
		}
		methods := []string{"GET"}
		if mm := pyMethods.FindStringSubmatch(rest); mm != nil {
			methods = nil
			for _, v := range strings.Split(mm[1], ",") {
				if v = strings.ToUpper(strings.Trim(strings.TrimSpace(v), `"'`)); v != "" {
					methods = append(methods, v)
				}
			}
		}
		for _, v := range methods {
			out = append(out, routeMatch{v + " " + route, m[0]})
		}
	}
	return out
}

func lineAt(src string, offset int) int { return strings.Count(src[:offset], "\n") + 1 }

func analyzeRegexLanguages(rp *repo, res *Result) {
	for _, lang := range languages {
		byDir := map[string][]string{}
		for _, f := range rp.files {
			if lang.matches(f) {
				byDir[path.Dir(f)] = append(byDir[path.Dir(f)], f)
			}
		}
		for _, dir := range model.SortedKeys(byDir) {
			m := ModuleFacts{Path: dir, Name: rp.dirName(dir), Language: lang.name, Analyzer: "regex"}
			hasSource := false
			for _, f := range byDir[dir] {
				m.Files = append(m.Files, f)
				data, err := rp.read(f)
				if err != nil {
					res.Unknowns = append(res.Unknowns, "cannot read "+f+": "+err.Error())
					continue
				}
				src := string(data)
				if lang.isTest(f) {
					res.Tests = append(res.Tests, lang.tests(f, dir, src)...)
					continue
				}
				hasSource = true
				m.Symbols = append(m.Symbols, lang.symbolsIn(f, src)...)
				res.Contracts = append(res.Contracts, lang.contractsIn(f, dir, src)...)
			}
			if hasSource {
				res.Modules = append(res.Modules, m)
			}
		}
		if len(byDir) > 0 {
			res.Unknowns = append(res.Unknowns, lang.name+" analyzed by regex: imports and calls are not resolved")
		}
	}
}

func (l regexLang) symbolsIn(f, src string) []model.Symbol {
	var out []model.Symbol
	for _, sp := range l.symbols {
		for _, m := range sp.re.FindAllStringSubmatchIndex(src, -1) {
			out = append(out, model.Symbol{Name: src[m[2]:m[3]], Kind: sp.kind, Location: loc(f, lineAt(src, m[0]))})
		}
	}
	return out
}

func (l regexLang) contractsIn(f, dir, src string) []ContractFacts {
	var out []ContractFacts
	for _, r := range l.routes(src) {
		out = append(out, ContractFacts{Kind: KindHTTPRoute, Direction: DirExposes, Selector: r.selector,
			OwnerPath: dir, Location: loc(f, lineAt(src, r.offset)), Origin: model.Extracted})
	}
	for _, re := range l.env {
		for _, m := range re.FindAllStringSubmatchIndex(src, -1) {
			out = append(out, ContractFacts{Kind: KindEnvVar, Direction: DirReads, Selector: src[m[2]:m[3]],
				OwnerPath: dir, Location: loc(f, lineAt(src, m[0])), Origin: model.Extracted})
		}
	}
	return out
}

func (l regexLang) tests(f, dir, src string) []TestFacts {
	if l.testFunc == nil {
		return nil
	}
	var out []TestFacts
	for _, m := range l.testFunc.FindAllStringSubmatchIndex(src, -1) {
		name := src[m[2]:m[3]]
		out = append(out, TestFacts{Name: name, ModulePath: dir, Location: loc(f, lineAt(src, m[0])),
			Command: "pytest " + f + "::" + name})
	}
	return out
}

func packageScripts(rp *repo, f string) map[string]bool {
	data, err := rp.read(f)
	if err != nil {
		return nil
	}
	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	if json.Unmarshal(data, &pkg) != nil {
		return nil
	}
	out := map[string]bool{}
	for k := range pkg.Scripts {
		out[k] = true
	}
	return out
}
