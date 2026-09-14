package pgvector

import (
	"strings"
	"testing"
)

func TestKeywordTSQueryKeepsEntitiesDropsQuestionWords(t *testing.T) {
	q := keywordTSQuery("mb lojistik operasyon müdür kim?")
	if q == "" {
		t.Fatal("empty tsquery")
	}
	if strings.Contains(q, "kim") {
		t.Fatalf("question word leaked: %s", q)
	}
	for _, tok := range []string{"lojistik", "operasyon", "müdür"} {
		if !strings.Contains(q, tok) {
			t.Fatalf("missing %q in %s", tok, q)
		}
	}
	if !strings.Contains(q, "|") {
		t.Fatalf("expected OR query, got %s", q)
	}
}

func TestKeywordTSQueryPayroll(t *testing.T) {
	q := keywordTSQuery("Ahmet Yılmaz nisan ayında ne kadar maaş aldı")
	for _, tok := range []string{"ahmet", "yılmaz", "nisan", "maaş"} {
		if !strings.Contains(q, tok) {
			t.Fatalf("missing %q in %s", tok, q)
		}
	}
	if strings.Contains(q, "kadar") || strings.Contains(q, "'ne':") {
		t.Fatalf("stopwords leaked: %s", q)
	}
}

func TestKeywordTSQueryDropsAggregateWords(t *testing.T) {
	q := keywordTSQuery("Nisan ayında ödenen toplam brüt maaş")
	for _, tok := range []string{"nisan", "maaş"} {
		if !strings.Contains(q, tok) {
			t.Fatalf("missing %q in %s", tok, q)
		}
	}
	if !strings.Contains(q, "brüt") && !strings.Contains(q, "brut") {
		t.Fatalf("missing brüt in %s", q)
	}
	for _, tok := range []string{"toplam", "ödenen", "ayında"} {
		if strings.Contains(q, tok) {
			t.Fatalf("aggregate stopword leaked %q: %s", tok, q)
		}
	}
}
