package tabular

import (
	"strconv"
	"strings"
	"unicode"
)

// Fields parses labeled "Key: value" lines from a retrieved table row.
func Fields(content string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		key, val, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		if key == "" || val == "" {
			continue
		}
		out[key] = val
	}
	return out
}

// ParseAmount reads spreadsheet amounts in US or TR grouping.
func ParseAmount(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" || s == "-" {
		return 0, false
	}
	neg := strings.Contains(s, "(") && strings.Contains(s, ")") || strings.HasPrefix(s, "-")
	var b strings.Builder
	for _, r := range s {
		switch {
		case unicode.IsDigit(r), r == '.', r == ',':
			b.WriteRune(r)
		case r == '-' || r == '(' || r == ')' || r == ' ' || r == '\u00a0' || r == '\'':
			continue
		case unicode.IsLetter(r) || r == '%' || r == '$' || r == '€' || r == '₺':
			continue
		default:
			continue
		}
	}
	s = b.String()
	if s == "" {
		return 0, false
	}
	lastComma := strings.LastIndex(s, ",")
	lastDot := strings.LastIndex(s, ".")
	switch {
	case lastComma >= 0 && lastDot >= 0:
		if lastDot > lastComma {
			s = strings.ReplaceAll(s, ",", "")
		} else {
			s = strings.ReplaceAll(s, ".", "")
			s = strings.ReplaceAll(s, ",", ".")
		}
	case lastComma >= 0:
		frac := s[lastComma+1:]
		if len(frac) == 2 {
			s = strings.ReplaceAll(s, ".", "")
			s = strings.ReplaceAll(s, ",", ".")
		} else {
			s = strings.ReplaceAll(s, ",", "")
		}
	case lastDot >= 0:
		if strings.Count(s, ".") > 1 {
			s = strings.ReplaceAll(s, ".", "")
		} else if frac := s[lastDot+1:]; len(frac) == 3 {
			s = strings.ReplaceAll(s, ".", "")
		}
	}
	n, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	if neg {
		n = -n
	}
	return n, true
}
