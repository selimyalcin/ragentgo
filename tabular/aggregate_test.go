package tabular

import (
	"fmt"
	"strings"
	"testing"

	"github.com/selimyalcin/ragentgo/document"
)

var aprilGross = []int{
	151000, 86000, 111000, 88000, 95000, 141000, 123000, 67000,
	111000, 58000, 90000, 156000, 104000, 113000, 103000, 43000,
	110000, 137000, 106000, 46000, 74000, 117000, 120000, 59000,
	148000, 95000, 97000, 108000, 67000, 95000, 112000, 86000,
}

func TestParseAmount(t *testing.T) {
	cases := map[string]float64{
		"151000":        151000,
		"151,000.00":    151000,
		"151.000":       151000,
		"151.000,50":    151000.50,
		"3.217.000":     3217000,
		"(1,234.56)":    -1234.56,
		"86.000 TRY":    86000,
		"43,000":        43000,
		"106000.00 TL":  106000,
	}
	for in, want := range cases {
		got, ok := ParseAmount(in)
		if !ok {
			t.Fatalf("ParseAmount(%q) failed", in)
		}
		if got != want {
			t.Fatalf("ParseAmount(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestDetect(t *testing.T) {
	op, ok := Detect("Nisan ayında ödenen toplam brüt maaş")
	if !ok || op != OpSum {
		t.Fatalf("got %s ok=%v", op, ok)
	}
	if _, ok := Detect("Ahmet Yılmaz nisan ayında ne kadar maaş aldı"); ok {
		t.Fatal("lookup should not aggregate")
	}
	op, ok = Detect("Nisan ayında kaç kişi maaş aldı")
	if !ok || op != OpCount {
		t.Fatalf("count: %s ok=%v", op, ok)
	}
}

func TestTrySumsAprilGross(t *testing.T) {
	hits := payrollHits()
	res := Try("Nisan ayında ödenen toplam brüt maaş nedir?", hits)
	if res == nil {
		t.Fatal("expected aggregate")
	}
	want := 0
	for _, n := range aprilGross {
		want += n
	}
	if res.RowCount != len(aprilGross) {
		t.Fatalf("rows_used = %d, want %d", res.RowCount, len(aprilGross))
	}
	if int(res.Value) != want {
		t.Fatalf("sum = %.0f, want %d", res.Value, want)
	}
	if res.Value == 348000 {
		t.Fatal("subset sum leaked")
	}
	if !strings.Contains(res.Block(), "3217000") {
		t.Fatalf("block missing value:\n%s", res.Block())
	}
}

func TestTryIgnoresLookup(t *testing.T) {
	if Try("Ahmet Yılmaz nisan ayında ne kadar maaş aldı", payrollHits()) != nil {
		t.Fatal("lookup should return nil")
	}
}

func payrollHits() []document.SearchResult {
	var hits []document.SearchResult
	for i, n := range aprilGross {
		hits = append(hits, rowHit(i+1, "Nisan", n))
	}
	for i := 0; i < 5; i++ {
		hits = append(hits, rowHit(100+i, "Mart", 200000+i*1000))
	}
	hits = append(hits, document.SearchResult{Chunk: document.Chunk{
		ID:         "inv-1",
		DocumentID: "inv-1",
		Content:    "Sheet: Faturalar\nRow: 4\nAy: Nisan\nTutar: 999999",
	}})
	return hits
}

func rowHit(row int, month string, gross int) document.SearchResult {
	id := fmt.Sprintf("pay-%d", row)
	content := fmt.Sprintf(
		"Sheet: Maaş Ödemeleri\nRow: %d\nRecord: PRS-%03d | %s | %d\nPersonel: PRS-%03d\nAy: %s\nBrüt Maaş (TRY): %d\nNet Maaş (TRY): %d",
		row, row, month, gross, row, month, gross, gross-20000,
	)
	return document.SearchResult{Chunk: document.Chunk{
		ID:         id,
		DocumentID: id,
		Content:    content,
		Metadata:   map[string]any{"sheet": "Maaş Ödemeleri", "row": row, "unit": "row"},
	}}
}

