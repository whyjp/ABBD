package analysis

import (
	"path"
	"regexp"
	"strings"

	"github.com/whyjp/abbd/internal/model"
)

var (
	createTable = regexp.MustCompile(`(?i)\bcreate\s+(?:or\s+replace\s+)?(?:temp(?:orary)?\s+)?(table|view|materialized\s+view)\s+(?:if\s+not\s+exists\s+)?([\w."]+)`)
	sqlKeyword  = regexp.MustCompile(`(?i)\b(select|insert|update|delete|from|into|join|table|upsert|merge)\b`)
	sqlWrite    = regexp.MustCompile(`(?i)\b(insert|update|delete|create|alter|upsert|merge)\b`)
)

// sqlContracts turns CREATE TABLE/VIEW statements in *.sql files into db-table "owns" contracts.
// The owner is the module whose code writes the table in a SQL string; failing that, any module
// that mentions it; failing that, the module (or directory) that holds the migration.
func sqlContracts(rp *repo, g *goResult, modules []ModuleFacts) []ContractFacts {
	var out []ContractFacts
	for _, f := range rp.files {
		if !strings.EqualFold(path.Ext(f), ".sql") {
			continue
		}
		data, err := rp.read(f)
		if err != nil {
			g.unknowns = append(g.unknowns, "cannot read "+f+": "+err.Error())
			continue
		}
		src := string(data)
		for _, m := range createTable.FindAllStringSubmatchIndex(src, -1) {
			kindWord := strings.ToLower(src[m[2]:m[3]])
			name := strings.ReplaceAll(src[m[4]:m[5]], `"`, "")
			table := name[strings.LastIndex(name, ".")+1:]
			c := ContractFacts{
				Kind: KindDBTable, Direction: DirOwns, Selector: name,
				Location: loc(f, strings.Count(src[:m[0]], "\n")+1), Origin: model.Extracted,
			}
			owner, refs := tableOwner(table, g.literals)
			switch {
			case owner != "":
				c.OwnerPath = owner
				if len(refs) > 1 {
					c.Note = "also referenced by " + strings.Join(without(refs, owner), ", ")
				}
			default:
				if mod, ok := moduleFor(path.Dir(f), modules); ok {
					c.OwnerPath = mod
				} else {
					c.OwnerPath = path.Dir(f)
				}
				c.Note = "no code references the table in a SQL string; owner is where the migration lives"
			}
			if kindWord != "table" {
				c.Note = strings.TrimPrefix(c.Note+"; "+kindWord, "; ")
			}
			out = append(out, c)
		}
	}
	return out
}

// tableOwner returns the owning module and every module that references the table in SQL.
// Writers win over readers; ties go to the first module Path so the choice is stable.
func tableOwner(table string, literals map[string][]stringLit) (string, []string) {
	ref := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(table) + `\b`)
	var refs, writers []string
	for _, mod := range model.SortedKeys(literals) {
		referenced, writes := false, false
		for _, lit := range literals[mod] {
			if ref.MatchString(lit.value) && sqlKeyword.MatchString(lit.value) {
				referenced = true
				writes = writes || sqlWrite.MatchString(lit.value)
			}
		}
		if referenced {
			refs = append(refs, mod)
		}
		if writes {
			writers = append(writers, mod)
		}
	}
	switch {
	case len(writers) > 0:
		return writers[0], refs
	case len(refs) > 0:
		return refs[0], refs
	}
	return "", nil
}

func without(s []string, drop string) []string {
	var out []string
	for _, v := range s {
		if v != drop {
			out = append(out, v)
		}
	}
	return out
}
