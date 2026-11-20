package screen

import (
	"fmt"
	"strings"
	"unicode"
)

// A name that says only that a box is shared, basic or a module tells the
// reader nothing about what it does, the way a hand-drawn diagram never
// labels a box "common". These are such words, English and Korean.
var emptyWords = map[string]bool{
	"common": true, "shared": true, "util": true, "utils": true, "utility": true,
	"utilities": true, "helper": true, "helpers": true, "base": true, "core": true,
	"misc": true, "lib": true, "libs": true, "general": true, "module": true,
	"modules": true, "공통": true, "공용": true, "유틸": true, "유틸리티": true,
	"기반": true, "코어": true, "기타": true, "헬퍼": true, "모듈": true,
}

// nameAdvice reads the names of the overview's parts, which a reader
// takes in before anything else. It asks for a name by role where a name
// is empty, and for one language where the names mix a project's own with
// bare code identifiers. Like the other advice it never fails lint.
func nameAdvice(boxes []Box, part map[string]bool) []string {
	var empty, plain, spoken []string
	for _, b := range boxes {
		if !part[b.ID] {
			continue
		}
		if isEmptyName(b.Name) {
			empty = append(empty, b.Name)
		}
		if hasNonASCIILetter(b.Name) {
			spoken = append(spoken, b.Name)
		} else if b.Name != "" {
			plain = append(plain, b.Name)
		}
	}
	var out []string
	if len(empty) > 0 {
		out = append(out, fmt.Sprintf("the overview names %s by how it is used, not by what it does. Name each by the role its parts play, or place its parts inside the modules that use them", strings.Join(empty, ", ")))
	}
	if len(spoken) >= 2 && len(plain) >= 2 {
		out = append(out, fmt.Sprintf("the overview mixes names in the project's language with bare code names (%s). Name every module in one language, by its role, and keep identifiers such as a type or package name for the module's own page", strings.Join(plain, ", ")))
	}
	return out
}

func isEmptyName(name string) bool {
	words := strings.FieldsFunc(strings.ToLower(name), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	for _, w := range words {
		if !emptyWords[w] {
			return false
		}
	}
	return len(words) > 0
}

func hasNonASCIILetter(s string) bool {
	for _, r := range s {
		if r > unicode.MaxASCII && unicode.IsLetter(r) {
			return true
		}
	}
	return false
}
