package prompt

import (
	"strings"
	"testing"

	"github.com/selimyalcin/ragentgo/document"
)

func TestBuildComputedPrefersAuthoritativeTotal(t *testing.T) {
	b := New("", true)
	hits := []document.SearchResult{{
		Chunk: document.Chunk{ID: "1", DocumentID: "1", Content: "Ay: Nisan\nBrüt Maaş (TRY): 151000"},
	}}
	prompt, sources := b.BuildComputed("toplam", "COMPUTED RESULT\n- value: 3217000", hits)
	if len(sources) != 1 {
		t.Fatalf("sources = %d", len(sources))
	}
	if !strings.Contains(prompt, "COMPUTED RESULT") || !strings.Contains(prompt, "3217000") {
		t.Fatalf("prompt missing computed total:\n%s", prompt)
	}
	if !strings.Contains(strings.ToLower(prompt), "do not add a subset") {
		t.Fatal("template missing subset rule")
	}
}
