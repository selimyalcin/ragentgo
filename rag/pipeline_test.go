package rag_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/selimyalcin/ragentgo/cache"
	"github.com/selimyalcin/ragentgo/chunker"
	"github.com/selimyalcin/ragentgo/document"
	"github.com/selimyalcin/ragentgo/embedding"
	"github.com/selimyalcin/ragentgo/llm"
	"github.com/selimyalcin/ragentgo/rag"
	"github.com/selimyalcin/ragentgo/store/memory"
)

type hashEmbedder struct{}

func (hashEmbedder) Model() string  { return "hash-8" }
func (hashEmbedder) Dimension() int { return 8 }

func (hashEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i, text := range texts {
		vec := make([]float32, 8)
		lower := strings.ToLower(text)
		for _, r := range lower {
			vec[int(r)%8] += 1
		}
		out[i] = vec
	}
	return out, nil
}

type echoGen struct{}

func (echoGen) Generate(_ context.Context, req llm.GenerateRequest) (*llm.GenerateResponse, error) {
	var last string
	if len(req.Messages) > 0 {
		last = req.Messages[len(req.Messages)-1].Content
	}
	return &llm.GenerateResponse{Text: "Grounded answer.\n\n" + last[:min(80, len(last))]}, nil
}

func (echoGen) Stream(ctx context.Context, req llm.GenerateRequest) (<-chan llm.StreamChunk, error) {
	ch := make(chan llm.StreamChunk, 2)
	go func() {
		defer close(ch)
		resp, _ := echoGen{}.Generate(ctx, req)
		ch <- llm.StreamChunk{Text: resp.Text}
		ch <- llm.StreamChunk{Done: true}
	}()
	return ch, nil
}

var _ embedding.Embedder = hashEmbedder{}
var _ llm.Generator = echoGen{}

func TestPipelineIndexAndQuery(t *testing.T) {
	pipe, err := rag.New(
		rag.WithEmbedder(hashEmbedder{}),
		rag.WithVectorStore(memory.New()),
		rag.WithGenerator(echoGen{}),
	)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	doc := document.Document{
		ID:      "auth",
		Content: "Authentication uses JWT bearer tokens. Middleware rejects missing Authorization headers.",
		Source:  "auth.md",
	}
	if err := pipe.IndexDocument(ctx, doc); err != nil {
		t.Fatal(err)
	}
	res, err := pipe.Query(ctx, "How does authentication work?", rag.QueryOptions{TopK: 3})
	if err != nil {
		t.Fatal(err)
	}
	if res.Answer == "" {
		t.Fatal("empty answer")
	}
	if len(res.Sources) == 0 {
		t.Fatal("expected sources")
	}
}

func TestIncrementalSkip(t *testing.T) {
	store := memory.New()
	pipe, err := rag.New(
		rag.WithEmbedder(hashEmbedder{}),
		rag.WithVectorStore(store),
		rag.WithGenerator(echoGen{}),
	)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	doc := document.Document{ID: "same", Content: "unchanged body"}
	first, err := pipe.IndexDocuments(ctx, []document.Document{doc})
	if err != nil {
		t.Fatal(err)
	}
	second, err := pipe.IndexDocuments(ctx, []document.Document{doc})
	if err != nil {
		t.Fatal(err)
	}
	if first.DocumentsIndexed != 1 || first.ChunksCreated == 0 {
		t.Fatalf("first: %+v", first)
	}
	if second.DocumentsSkipped != 1 || second.DocumentsIndexed != 0 {
		t.Fatalf("second should skip: %+v", second)
	}
}

func TestEmptyQuery(t *testing.T) {
	pipe, _ := rag.New(
		rag.WithEmbedder(hashEmbedder{}),
		rag.WithVectorStore(memory.New()),
		rag.WithGenerator(echoGen{}),
	)
	_, err := pipe.Query(context.Background(), "  ", rag.QueryOptions{})
	if err != rag.ErrEmptyQuery {
		t.Fatalf("err = %v", err)
	}
}

type limitedEmbedder struct {
	inner    embedding.Embedder
	maxRunes int
}

func (l limitedEmbedder) Model() string  { return l.inner.Model() }
func (l limitedEmbedder) Dimension() int { return l.inner.Dimension() }

func (l limitedEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	for _, text := range texts {
		if n := utf8.RuneCountInString(text); n > l.maxRunes {
			return nil, fmt.Errorf("input (%d runes) is larger than max (%d)", n, l.maxRunes)
		}
	}
	return l.inner.Embed(ctx, texts)
}

func TestPipelineFitsOversizedChunks(t *testing.T) {
	maxTokens := 256
	maxRunes := chunker.MaxRunesForTokens(maxTokens)
	pipe, err := rag.New(
		rag.WithEmbedder(limitedEmbedder{inner: hashEmbedder{}, maxRunes: maxRunes}),
		rag.WithVectorStore(memory.New()),
		rag.WithGenerator(echoGen{}),
		rag.WithMaxEmbedTokens(maxTokens),
	)
	if err != nil {
		t.Fatal(err)
	}
	body := strings.Repeat("Go is used for backends and CLIs. ", 80)
	if utf8.RuneCountInString(body) <= maxRunes {
		t.Fatal("fixture must exceed the embedder window")
	}
	if err := pipe.IndexDocument(context.Background(), document.Document{
		ID:      "long",
		Content: body,
	}); err != nil {
		t.Fatal(err)
	}
}

func TestPipelineAggregatesLabeledRows(t *testing.T) {
	april := []int{
		151000, 86000, 111000, 88000, 95000, 141000, 123000, 67000,
		111000, 58000, 90000, 156000, 104000, 113000, 103000, 43000,
		110000, 137000, 106000, 46000, 74000, 117000, 120000, 59000,
		148000, 95000, 97000, 108000, 67000, 95000, 112000, 86000,
	}
	gen := &captureGen{}
	pipe, err := rag.New(
		rag.WithEmbedder(hashEmbedder{}),
		rag.WithVectorStore(memory.New()),
		rag.WithGenerator(gen),
	)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	docs := make([]document.Document, 0, len(april)+5)
	for i, n := range april {
		docs = append(docs, payrollDoc(i+1, "Nisan", n))
	}
	for i := 0; i < 5; i++ {
		docs = append(docs, payrollDoc(200+i, "Mart", 200000))
	}
	if _, err := pipe.IndexDocuments(ctx, docs); err != nil {
		t.Fatal(err)
	}
	res, err := pipe.Query(ctx, "Nisan ayında ödenen toplam brüt maaş nedir?", rag.QueryOptions{TopK: 5})
	if err != nil {
		t.Fatal(err)
	}
	if res.Answer == "" {
		t.Fatal("empty answer")
	}
	if !strings.Contains(gen.last, "COMPUTED RESULT") {
		t.Fatalf("prompt missing computed result:\n%s", gen.last)
	}
	if !strings.Contains(gen.last, "3217000") {
		t.Fatalf("prompt missing full April total:\n%s", gen.last)
	}
	if strings.Contains(gen.last, "348000") {
		t.Fatal("subset total leaked into prompt")
	}
}

func payrollDoc(row int, month string, gross int) document.Document {
	return document.Document{
		ID: fmt.Sprintf("pay-%d", row),
		Content: fmt.Sprintf(
			"Sheet: Maaş Ödemeleri\nRow: %d\nPersonel: PRS-%03d\nAy: %s\nBrüt Maaş (TRY): %d\nNet Maaş (TRY): %d",
			row, row, month, gross, gross-20000,
		),
		Source:   "payroll.xlsx",
		Metadata: map[string]any{"unit": "row", "sheet": "Maaş Ödemeleri", "row": row},
	}
}

type captureGen struct {
	last string
}

func (g *captureGen) Generate(_ context.Context, req llm.GenerateRequest) (*llm.GenerateResponse, error) {
	if len(req.Messages) > 0 {
		g.last = req.Messages[len(req.Messages)-1].Content
	}
	return &llm.GenerateResponse{Text: "Grounded total."}, nil
}

func (g *captureGen) Stream(ctx context.Context, req llm.GenerateRequest) (<-chan llm.StreamChunk, error) {
	ch := make(chan llm.StreamChunk, 2)
	go func() {
		defer close(ch)
		resp, _ := g.Generate(ctx, req)
		ch <- llm.StreamChunk{Text: resp.Text}
		ch <- llm.StreamChunk{Done: true}
	}()
	return ch, nil
}

func TestQueryAnswerCache(t *testing.T) {
	gen := &countingGen{}
	pipe, err := rag.New(
		rag.WithEmbedder(hashEmbedder{}),
		rag.WithVectorStore(memory.New()),
		rag.WithGenerator(gen),
		rag.WithCache(cache.NewMemory(64)),
		rag.WithLLMModel("echo"),
	)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := pipe.IndexDocument(ctx, document.Document{
		ID:      "auth",
		Content: "Authentication uses JWT bearer tokens.",
		Source:  "auth.md",
	}); err != nil {
		t.Fatal(err)
	}
	first, err := pipe.Query(ctx, "How does authentication work?", rag.QueryOptions{TopK: 3})
	if err != nil {
		t.Fatal(err)
	}
	second, err := pipe.Query(ctx, "How does authentication work?", rag.QueryOptions{TopK: 3})
	if err != nil {
		t.Fatal(err)
	}
	if gen.n != 1 {
		t.Fatalf("generator calls = %d, want 1", gen.n)
	}
	if first.Cached {
		t.Fatal("first answer should miss cache")
	}
	if !second.Cached {
		t.Fatal("second answer should hit cache")
	}
	if first.Answer != second.Answer {
		t.Fatalf("cached answer mismatch: %q vs %q", first.Answer, second.Answer)
	}
	if err := pipe.IndexDocument(ctx, document.Document{
		ID:      "auth-2",
		Content: "Passkeys replaced passwords in 2026.",
		Source:  "auth-2.md",
	}); err != nil {
		t.Fatal(err)
	}
	third, err := pipe.Query(ctx, "How does authentication work?", rag.QueryOptions{TopK: 3})
	if err != nil {
		t.Fatal(err)
	}
	if third.Cached {
		t.Fatal("corpus change must bust the answer cache")
	}
	if gen.n != 2 {
		t.Fatalf("generator calls after ingest = %d, want 2", gen.n)
	}
}

func TestQueryAnswerCacheSkipsConversation(t *testing.T) {
	gen := &countingGen{}
	pipe, err := rag.New(
		rag.WithEmbedder(hashEmbedder{}),
		rag.WithVectorStore(memory.New()),
		rag.WithGenerator(gen),
		rag.WithCache(cache.NewMemory(64)),
	)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := pipe.IndexDocument(ctx, document.Document{
		ID:      "auth",
		Content: "Authentication uses JWT bearer tokens.",
	}); err != nil {
		t.Fatal(err)
	}
	q := rag.QueryOptions{TopK: 3, ConversationID: "chat-1"}
	if _, err := pipe.Query(ctx, "How does authentication work?", q); err != nil {
		t.Fatal(err)
	}
	if _, err := pipe.Query(ctx, "How does authentication work?", q); err != nil {
		t.Fatal(err)
	}
	if gen.n != 2 {
		t.Fatalf("conversation queries must not share cache, calls=%d", gen.n)
	}
}

func TestStreamQueryAnswerCache(t *testing.T) {
	gen := &countingGen{}
	pipe, err := rag.New(
		rag.WithEmbedder(hashEmbedder{}),
		rag.WithVectorStore(memory.New()),
		rag.WithGenerator(gen),
		rag.WithCache(cache.NewMemory(64)),
	)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := pipe.IndexDocument(ctx, document.Document{
		ID:      "auth",
		Content: "Authentication uses JWT bearer tokens.",
	}); err != nil {
		t.Fatal(err)
	}
	_, ch, err := pipe.StreamQuery(ctx, "How does authentication work?", rag.QueryOptions{TopK: 3})
	if err != nil {
		t.Fatal(err)
	}
	drainStream(t, ch)
	_, ch, err = pipe.StreamQuery(ctx, "How does authentication work?", rag.QueryOptions{TopK: 3})
	if err != nil {
		t.Fatal(err)
	}
	got := drainStream(t, ch)
	if gen.n != 1 {
		t.Fatalf("stream generator calls = %d, want 1", gen.n)
	}
	if !strings.Contains(got, "Grounded") {
		t.Fatalf("cached stream text = %q", got)
	}
}

type countingGen struct {
	n int
}

func (g *countingGen) Generate(_ context.Context, req llm.GenerateRequest) (*llm.GenerateResponse, error) {
	g.n++
	return &llm.GenerateResponse{Text: fmt.Sprintf("Grounded answer %d", g.n)}, nil
}

func (g *countingGen) Stream(ctx context.Context, req llm.GenerateRequest) (<-chan llm.StreamChunk, error) {
	ch := make(chan llm.StreamChunk, 2)
	go func() {
		defer close(ch)
		resp, _ := g.Generate(ctx, req)
		ch <- llm.StreamChunk{Text: resp.Text}
		ch <- llm.StreamChunk{Done: true}
	}()
	return ch, nil
}

func drainStream(t *testing.T, ch <-chan llm.StreamChunk) string {
	t.Helper()
	var b strings.Builder
	for chunk := range ch {
		if chunk.Err != nil {
			t.Fatal(chunk.Err)
		}
		b.WriteString(chunk.Text)
	}
	return b.String()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
