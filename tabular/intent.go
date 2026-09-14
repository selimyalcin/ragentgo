package tabular

import "strings"

// Op is a spreadsheet-style aggregation.
type Op string

const (
	OpSum   Op = "sum"
	OpCount Op = "count"
	OpAvg   Op = "avg"
	OpMin   Op = "min"
	OpMax   Op = "max"
)

var (
	stopTokens = map[string]struct{}{
		"ayinda": {}, "ayin": {}, "ayinin": {},
		"odenen": {}, "odeme": {}, "odemeler": {}, "odemeleri": {},
		"kadar": {}, "icin": {}, "ile": {}, "bir": {}, "bu": {}, "su": {},
		"olan": {}, "olarak": {}, "belirlenir": {}, "nedir": {}, "neler": {},
		"hangi": {}, "nasil": {}, "the": {}, "and": {}, "for": {}, "was": {},
		"did": {}, "does": {}, "are": {}, "much": {}, "many": {}, "which": {},
		"paid": {}, "listed": {}, "month": {}, "ay": {},
	}
	opTokens = map[string]struct{}{
		"toplam": {}, "toplami": {}, "sum": {}, "total": {}, "totals": {},
		"ortalama": {}, "average": {}, "avg": {}, "mean": {},
		"kac": {}, "count": {}, "adet": {}, "sayisi": {},
		"max": {}, "min": {}, "maksimum": {}, "minimum": {},
		"yuksek": {}, "dusuk": {}, "buyuk": {}, "kucuk": {},
	}
	measureTokens = map[string]struct{}{
		"maas": {}, "brut": {}, "net": {}, "tutar": {}, "tutari": {},
		"amount": {}, "salary": {}, "wage": {}, "gross": {}, "pay": {},
		"ucret": {}, "gelir": {}, "gider": {}, "fiyat": {}, "price": {},
		"cost": {}, "odeme": {}, "payment": {}, "bonus": {}, "kesinti": {},
		"sgk": {}, "vergi": {}, "tax": {}, "try": {}, "usd": {}, "eur": {},
		"tl": {}, "miktar": {}, "quantity": {},
	}
	monthTokens = map[string]struct{}{
		"ocak": {}, "january": {},
		"subat": {}, "february": {},
		"mart": {}, "march": {},
		"nisan": {}, "april": {},
		"mayis": {}, "may": {},
		"haziran": {}, "june": {},
		"temmuz": {}, "july": {},
		"agustos": {}, "august": {},
		"eylul": {}, "september": {},
		"ekim": {}, "october": {},
		"kasim": {}, "november": {},
		"aralik": {}, "december": {},
	}
)

// Detect reports whether the question asks for an aggregate over many rows.
func Detect(question string) (Op, bool) {
	f := fold(question)
	switch {
	case strings.Contains(f, "en yuksek"), strings.Contains(f, "en buyuk"),
		hasWord(f, "maksimum"), hasWord(f, "maximum"), hasWord(f, "max"):
		return OpMax, true
	case strings.Contains(f, "en dusuk"), strings.Contains(f, "en kucuk"),
		hasWord(f, "minimum"), hasWord(f, "min"):
		return OpMin, true
	case hasWord(f, "ortalama"), hasWord(f, "average"), hasWord(f, "avg"), hasWord(f, "mean"):
		return OpAvg, true
	case isCount(f):
		return OpCount, true
	case hasWord(f, "toplam"), hasWord(f, "toplami"), hasWord(f, "sum"),
		hasWord(f, "total"), hasWord(f, "totals"):
		return OpSum, true
	default:
		return "", false
	}
}

func isCount(f string) bool {
	if strings.Contains(f, "ne kadar") || strings.Contains(f, "how much") {
		return false
	}
	if strings.Contains(f, "how many") || strings.Contains(f, "number of") {
		return true
	}
	if hasWord(f, "kac") && (hasWord(f, "kisi") || hasWord(f, "adet") ||
		hasWord(f, "kayit") || hasWord(f, "satir") || hasWord(f, "personel") ||
		hasWord(f, "calisan")) {
		return true
	}
	return hasWord(f, "sayisi") || hasWord(f, "count")
}

func hasWord(folded, word string) bool {
	for _, t := range tokenize(folded) {
		if t == word {
			return true
		}
	}
	return false
}
