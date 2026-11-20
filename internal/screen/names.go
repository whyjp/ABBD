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
	"utilities": true, "helper": true, "helpers": true, "misc": true, "lib": true, "libs": true, "general": true, "module": true,
	"modules": true, "공통": true, "공용": true, "유틸": true, "유틸리티": true,
	"기반": true, "기타": true, "헬퍼": true, "모듈": true,
}

// nameAdvice reads the names of the overview's parts, which a reader
// takes in before anything else. It asks for a name by role where a name
// is empty or a list of dot-joined items, and for one language where the names
// mix a project's own with bare code identifiers. Like the other advice it
// never fails lint.
func nameAdvice(boxes []Box, part map[string]bool) []string {
	var empty, plain, spoken, listed []string
	for _, b := range boxes {
		if !part[b.ID] {
			continue
		}
		if isEmptyName(b.Name) {
			empty = append(empty, b.Name)
		}
		if joinedItems(b.Name) >= listedFrom {
			listed = append(listed, b.Name)
		}
		switch {
		case hasNonLatinLetter(b.Name):
			spoken = append(spoken, b.Name)
		case isCodeName(b.Name):
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
	if len(listed) > 0 {
		out = append(out, fmt.Sprintf("the overview names %s as a list of %d or more items joined by a middle dot (·). Name each by the one role its parts share, and keep the items for the module's own page", strings.Join(listed, ", "), listedFrom))
	}
	return out
}

// listedFrom is how many dot-joined items make a name a list: a part is
// named by the one role its pieces share, as a hand-drawn box is.
const listedFrom = 3

// joinedItems counts the items of a name joined by a middle dot.
func joinedItems(name string) int {
	items := strings.FieldsFunc(name, func(r rune) bool { return strings.ContainsRune("·ㆍ・∙•‧⋅･", r) })
	n := 0
	for _, it := range items {
		if strings.TrimSpace(it) != "" {
			n++
		}
	}
	return n
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

// hasNonLatinLetter is true for a name written in a script of its own,
// such as Hangul, which an accented Latin letter is not.
func hasNonLatinLetter(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) && !unicode.Is(unicode.Latin, r) {
			return true
		}
	}
	return false
}

// isCodeName is true for a Latin-letter name that is no acronym: a type,
// a package or a crate such as ginS or codec/json. An all-capitals name
// such as API or CLI is spoken in any language.
func isCodeName(s string) bool {
	lower := 0
	for _, r := range s {
		if unicode.IsLetter(r) {
			if unicode.IsLower(r) {
				lower++
			}
		}
	}
	return lower > 0
}
