package pgvector

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Question words that would AND-kill keyword search if left in a tsquery.
var questionWords = map[string]struct{}{
	"kim": {}, "ne": {}, "nedir": {}, "neler": {}, "hangi": {}, "nasil": {}, "nasıl": {},
	"kadar": {}, "icin": {}, "için": {}, "ile": {}, "bir": {}, "bu": {}, "su": {}, "şu": {},
	"mi": {}, "mı": {}, "mu": {}, "mü": {}, "midir": {}, "mıdır": {},
	"who": {}, "what": {}, "when": {}, "where": {}, "why": {}, "how": {},
	"the": {}, "and": {}, "for": {}, "was": {}, "did": {}, "does": {}, "are": {},
	"much": {}, "many": {}, "which": {},
	"toplam": {}, "toplamı": {}, "toplami": {}, "sum": {}, "total": {},
	"ortalama": {}, "average": {}, "count": {}, "adet": {},
	"ödenen": {}, "odenen": {}, "ayında": {}, "ayinda": {},
}

func keywordTSQuery(q string) string {
	var parts []string
	seen := map[string]struct{}{}
	for _, tok := range tokenizeQuery(q) {
		if utf8.RuneCountInString(tok) < 3 {
			continue
		}
		if _, skip := questionWords[tok]; skip {
			continue
		}
		if _, ok := seen[tok]; ok {
			continue
		}
		seen[tok] = struct{}{}
		parts = append(parts, tsqueryTerm(tok)+":*")
	}
	return strings.Join(parts, " | ")
}

func tokenizeQuery(q string) []string {
	q = strings.ToLower(strings.TrimSpace(q))
	var (
		out  []string
		cur  strings.Builder
		emit = func() {
			s := strings.TrimSpace(cur.String())
			if s != "" {
				out = append(out, s)
			}
			cur.Reset()
		}
	)
	for _, r := range q {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			cur.WriteRune(r)
			continue
		}
		emit()
	}
	emit()
	return out
}

func tsqueryTerm(tok string) string {
	tok = strings.ReplaceAll(tok, "'", "")
	tok = strings.ReplaceAll(tok, `\`, "")
	tok = strings.ReplaceAll(tok, ":", "")
	tok = strings.ReplaceAll(tok, "&", "")
	tok = strings.ReplaceAll(tok, "|", "")
	tok = strings.ReplaceAll(tok, "!", "")
	tok = strings.ReplaceAll(tok, "(", "")
	tok = strings.ReplaceAll(tok, ")", "")
	return tok
}
