package tabular

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

func fold(s string) string {
	s = strings.ToLowerSpecial(unicode.TurkishCase, strings.TrimSpace(s))
	replacer := strings.NewReplacer(
		"ı", "i",
		"ğ", "g",
		"ü", "u",
		"ş", "s",
		"ö", "o",
		"ç", "c",
	)
	return replacer.Replace(s)
}

func tokenize(s string) []string {
	var (
		out []string
		cur strings.Builder
		emit = func() {
			t := fold(cur.String())
			cur.Reset()
			if t == "" || utf8.RuneCountInString(t) < 3 {
				return
			}
			out = append(out, t)
		}
	)
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			cur.WriteRune(r)
			continue
		}
		emit()
	}
	emit()
	return out
}

func unique(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, t := range in {
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	return out
}
