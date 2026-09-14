package chunker

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/selimyalcin/ragentgo/document"
)

func TestRecursiveRespectsSizeAndOverlap(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 40; i++ {
		b.WriteString("Go is a compiled language used for backends. ")
	}
	chunks, err := NewRecursive(Config{Size: 80, Overlap: 16}).Chunk(context.Background(), document.Document{
		Content:  b.String(),
		Metadata: map[string]any{"lang": "en"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(chunks))
	}
	for i, c := range chunks {
		if c.Index != i {
			t.Fatalf("index = %d want %d", c.Index, i)
		}
		if c.Metadata["lang"] != "en" {
			t.Fatal("metadata must be inherited")
		}
		if strings.TrimSpace(c.Content) == "" {
			t.Fatal("empty chunk")
		}
	}
}

func TestMarkdownKeepsCodeFence(t *testing.T) {
	src := "# Title\n\nIntro paragraph.\n\n```go\nfunc main() {\n  fmt.Println(\"hi\")\n}\n```\n\n## Next\n\nMore text here that should sit in another section.\n"
	chunks, err := NewMarkdown(Config{Size: 80, Overlap: 10}).Chunk(context.Background(), document.Document{Content: src})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range chunks {
		if strings.Contains(c.Content, "func main()") && strings.Contains(c.Content, "```") {
			found = true
			if !strings.Contains(c.Content, "```go") {
				t.Fatal("code fence language tag was dropped")
			}
		}
	}
	if !found {
		t.Fatalf("code block was split apart: %#v", chunks)
	}
}

func TestTokenChunker(t *testing.T) {
	words := strings.Repeat("token ", 50)
	chunks, err := NewToken(Config{Size: 10, Overlap: 2}).Chunk(context.Background(), document.Document{Content: words})
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) < 4 {
		t.Fatalf("got %d chunks", len(chunks))
	}
}

func TestSentenceChunker(t *testing.T) {
	text := "First sentence. Second sentence! Third sentence? Fourth one."
	chunks, err := NewSentence(Config{Size: 40, Overlap: 0}).Chunk(context.Background(), document.Document{Content: text})
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) == 0 {
		t.Fatal("expected chunks")
	}
}

func TestRecursiveEmpty(t *testing.T) {
	chunks, err := NewRecursive(Config{}).Chunk(context.Background(), document.Document{Content: "   "})
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) != 0 {
		t.Fatalf("got %d", len(chunks))
	}
}

func TestMaxRunesForTokensStaysUnderObservedRatio(t *testing.T) {
	max := MaxRunesForTokens(256)
	if max <= 0 || max >= 800 {
		t.Fatalf("max runes = %d, want a budget well under the old 800-char default", max)
	}
	// 800 English chars → 281 tokens. A later labeled row hit 277 tokens inside a
	// 256 window, so the budget must stay well below that density.
	if int(float64(max)*0.55)+24 >= 256 {
		t.Fatalf("budget %d runes is still too large for a 256-token embedder", max)
	}
	if max > 400 {
		t.Fatalf("max runes = %d, want a tight budget after the 277-token failure", max)
	}
}

func TestCapToTokensShrinksDefaultSize(t *testing.T) {
	c := DefaultConfig().CapToTokens(256)
	if c.Size >= 800 {
		t.Fatalf("size = %d, want capped below default", c.Size)
	}
	if c.Overlap >= c.Size {
		t.Fatalf("overlap %d >= size %d", c.Overlap, c.Size)
	}
}

func TestFitKeepsNewlineBoundaries(t *testing.T) {
	block := strings.Repeat("Sheet: Payroll\nAy: Nisan\nAd Soyad: Ahmet Yilmaz\nNet Odeme: 95000\n", 40)
	out := Fit([]document.Chunk{{ID: "c0", Content: block}}, 256)
	if len(out) < 2 {
		t.Fatal("expected split")
	}
	for i, c := range out {
		if utf8.RuneCountInString(c.Content) > MaxRunesForTokens(256) {
			t.Fatalf("chunk %d has %d runes", i, utf8.RuneCountInString(c.Content))
		}
		if strings.Contains(c.Content, "Ad Soyad: Ah") && !strings.Contains(c.Content, "Ahmet") {
			t.Fatalf("split mid-field in chunk %d: %q", i, c.Content)
		}
	}
	if !strings.Contains(out[1].Content, "Sheet: Payroll") {
		t.Fatalf("continuation lost sheet identity: %q", out[1].Content)
	}
}

func TestFitSplitsOversizedChunks(t *testing.T) {
	long := strings.Repeat("x", 2000)
	in := []document.Chunk{{
		ID:         "c0",
		DocumentID: "d0",
		Content:    long,
		Metadata:   map[string]any{"k": "v"},
	}}
	out := Fit(in, 256)
	if len(out) < 2 {
		t.Fatalf("expected split, got %d chunks", len(out))
	}
	max := MaxRunesForTokens(256)
	for i, c := range out {
		if c.Index != i {
			t.Fatalf("index = %d want %d", c.Index, i)
		}
		if utf8.RuneCountInString(c.Content) > max {
			t.Fatalf("chunk %d still has %d runes", i, utf8.RuneCountInString(c.Content))
		}
		if c.Metadata["k"] != "v" {
			t.Fatal("metadata must be copied")
		}
		if i > 0 {
			if _, err := uuid.Parse(c.ID); err != nil {
				t.Fatalf("split chunk id %q must be a UUID: %v", c.ID, err)
			}
		}
	}
	if Fit(in, 0)[0].Content != long {
		t.Fatal("Fit with maxTokens=0 must be a no-op")
	}
}
