package model

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"unicode"
)

// Language is a validated BCP-47 tag in canonical case, such as ko, en-US
// or zh-Hant: the language a person reads the stored prose in. The zero
// Language says none was set.
type Language string

// FallbackLanguage is the language when neither the project nor the OS
// names one.
const FallbackLanguage Language = "en"

var languageShape = regexp.MustCompile(`^([a-z]{2,3})(-[a-z]{4})?(-(?:[a-z]{2}|[0-9]{3}))?((?:-(?:[a-z0-9]{5,8}|[0-9][a-z0-9]{3}))*)$`)

// ParseLanguage validates s as a BCP-47 tag of a language, an optional
// script, an optional region and variants, and returns it in canonical case.
func ParseLanguage(s string) (Language, error) {
	m := languageShape.FindStringSubmatch(strings.ToLower(strings.ReplaceAll(strings.TrimSpace(s), "_", "-")))
	if m == nil {
		return "", fmt.Errorf("language %q is no BCP-47 tag such as ko, en or zh-Hant", s)
	}
	t := m[1]
	if m[2] != "" {
		t += "-" + strings.ToUpper(m[2][1:2]) + m[2][2:]
	}
	if m[3] != "" {
		t += strings.ToUpper(m[3])
	}
	return Language(t + m[4]), nil
}

// UnmarshalText refuses a tag ParseLanguage refuses, so a bad tag in
// project.yaml stops the load that reads it.
func (l *Language) UnmarshalText(b []byte) error {
	p, err := ParseLanguage(string(b))
	*l = p
	return err
}

// DefaultLanguage is the language the OS names for its user: on Windows the
// user's default locale, which outranks the LANG a POSIX shell such as Git
// for Windows exports; else LC_ALL, then LC_MESSAGES, then LANG, as POSIX
// ranks them; and FallbackLanguage when none names a language.
func DefaultLanguage() Language { return languageFrom(os.Getenv, osLocale) }

func languageFrom(getenv func(string) string, system func() string) Language {
	if l, err := ParseLanguage(system()); err == nil {
		return l
	}
	for _, k := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if l, ok := posixLocale(getenv(k)); ok {
			return l
		}
	}
	return FallbackLanguage
}

// posixLocale reads a POSIX locale such as ko_KR.UTF-8 or de_DE@euro. C
// and POSIX name no language.
func posixLocale(v string) (Language, bool) {
	v, _, _ = strings.Cut(v, ".")
	v, _, _ = strings.Cut(v, "@")
	if v == "" || v == "C" || v == "POSIX" {
		return "", false
	}
	l, err := ParseLanguage(v)
	return l, err == nil
}

// The writing systems a language is written in, by language subtag and by
// script subtag. A language neither lists has no script to judge by.
var (
	hangul   = []*unicode.RangeTable{unicode.Hangul}
	japanese = []*unicode.RangeTable{unicode.Hiragana, unicode.Katakana, unicode.Han}
	han      = []*unicode.RangeTable{unicode.Han}
	cyrillic = []*unicode.RangeTable{unicode.Cyrillic}
	arabic   = []*unicode.RangeTable{unicode.Arabic}
	hebrew   = []*unicode.RangeTable{unicode.Hebrew}
	deva     = []*unicode.RangeTable{unicode.Devanagari}
	latin    = []*unicode.RangeTable{unicode.Latin}

	scriptOfLanguage = map[string][]*unicode.RangeTable{
		"ko": hangul, "ja": japanese, "zh": han,
		"ru": cyrillic, "uk": cyrillic, "be": cyrillic, "bg": cyrillic, "mk": cyrillic, "kk": cyrillic, "mn": cyrillic,
		"el": {unicode.Greek}, "ar": arabic, "fa": arabic, "ur": arabic, "he": hebrew, "yi": hebrew,
		"th": {unicode.Thai}, "hi": deva, "mr": deva, "ne": deva,
		"en": latin, "de": latin, "fr": latin, "es": latin, "it": latin, "pt": latin, "nl": latin, "sv": latin,
		"da": latin, "no": latin, "nb": latin, "nn": latin, "fi": latin, "is": latin, "pl": latin, "cs": latin,
		"sk": latin, "sl": latin, "hr": latin, "bs": latin, "ro": latin, "hu": latin, "tr": latin, "et": latin,
		"lv": latin, "lt": latin, "sq": latin, "ca": latin, "eu": latin, "gl": latin, "ga": latin, "cy": latin,
		"mt": latin, "af": latin, "sw": latin, "id": latin, "ms": latin, "vi": latin, "tl": latin, "fil": latin,
		"az": latin, "uz": latin, "eo": latin, "la": latin,
	}
	scriptOfSubtag = map[string][]*unicode.RangeTable{
		"Hang": hangul, "Kore": hangul, "Jpan": japanese, "Hani": han, "Hans": han, "Hant": han,
		"Cyrl": cyrillic, "Grek": {unicode.Greek}, "Arab": arabic, "Hebr": hebrew, "Thai": {unicode.Thai}, "Deva": deva, "Latn": latin,
	}
)

// Script is the letters the language is written in: those its script
// subtag names, else those of its language subtag, else none, and prose in
// a language with none is not judged.
func (l Language) Script() []*unicode.RangeTable {
	parts := strings.Split(string(l), "-")
	if len(parts) > 1 {
		if s, ok := scriptOfSubtag[parts[1]]; ok {
			return s
		}
	}
	return scriptOfLanguage[parts[0]]
}
