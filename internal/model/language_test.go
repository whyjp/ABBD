package model

import "testing"

func TestParseLanguageCanonicalisesATagAndRefusesWhatIsNone(t *testing.T) {
	for in, want := range map[string]Language{"ko": "ko", "KO-kr": "ko-KR", "zh_hant_TW": "zh-Hant-TW", "es-419": "es-419", "de-DE-1996": "de-DE-1996"} {
		if got, err := ParseLanguage(in); err != nil || got != want {
			t.Errorf("ParseLanguage(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "korean", "k", "ko KR", "ko-", "en-US-x", "日本"} {
		if got, err := ParseLanguage(bad); err == nil {
			t.Errorf("ParseLanguage(%q) = %q, want an error", bad, got)
		}
	}
}

func TestDefaultLanguageReadsTheLocaleThePOSIXEnvironmentRanksFirst(t *testing.T) {
	for _, c := range []struct {
		env    map[string]string
		system string
		want   Language
	}{
		{map[string]string{"LANG": "ko_KR.UTF-8"}, "", "ko-KR"},
		{map[string]string{"LANG": "en_US.UTF-8", "LC_MESSAGES": "ja_JP", "LC_ALL": "de_DE@euro"}, "", "de-DE"},
		{map[string]string{"LANG": "en_US.UTF-8", "LC_MESSAGES": "ja_JP"}, "", "ja-JP"},
		{map[string]string{"LANG": "C.UTF-8"}, "ko-KR", "ko-KR"},
		{map[string]string{"LC_ALL": "POSIX"}, "", FallbackLanguage},
		{nil, "", FallbackLanguage},
	} {
		got := languageFrom(func(k string) string { return c.env[k] }, func() string { return c.system })
		if got != c.want {
			t.Errorf("env %v, system %q: %q, want %q", c.env, c.system, got, c.want)
		}
	}
}
