package rerank

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/selimyalcin/ragentgo/document"
	"github.com/selimyalcin/ragentgo/llm"
)

// Reranker reorders retrieved chunks.
type Reranker interface {
	Rerank(ctx context.Context, query string, docs []document.SearchResult) ([]document.SearchResult, error)
}

// Noop leaves results in the order the retriever produced.
type Noop struct{}

// Rerank implements Reranker.
func (Noop) Rerank(_ context.Context, _ string, docs []document.SearchResult) ([]document.SearchResult, error) {
	return docs, nil
}

// LLM asks a generator to score each chunk. Keep the candidate list small.
type LLM struct {
	generator llm.Generator
}

// NewLLM returns an LLM reranker.
func NewLLM(g llm.Generator) *LLM {
	return &LLM{generator: g}
}

// Rerank implements Reranker.
func (l *LLM) Rerank(ctx context.Context, query string, docs []document.SearchResult) ([]document.SearchResult, error) {
	if l == nil || l.generator == nil || len(docs) < 2 {
		return docs, nil
	}
	var b strings.Builder
	b.WriteString("Score each passage from 0 to 1 for how well it answers the question. Reply with one score per line as `index:score`.\n\nQuestion:\n")
	b.WriteString(query)
	b.WriteString("\n\nPassages:\n")
	for i, d := range docs {
		fmt.Fprintf(&b, "[%d] %s\n", i, truncate(d.Chunk.Content, 500))
	}
	resp, err := l.generator.Generate(ctx, llm.GenerateRequest{
		Messages: []llm.Message{{Role: "user", Content: b.String()}},
	})
	if err != nil {
		return nil, fmt.Errorf("rerank: %w", err)
	}
	scores := parseScores(resp.Text, len(docs))
	out := append([]document.SearchResult(nil), docs...)
	for i := range out {
		if s, ok := scores[i]; ok {
			out[i].Score = s
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return out, nil
}

func parseScores(text string, n int) map[int]float32 {
	out := map[int]float32{}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, ":")
		if len(parts) < 2 {
			continue
		}
		idx, err := strconv.Atoi(strings.TrimSpace(strings.Trim(parts[0], "[]")))
		if err != nil || idx < 0 || idx >= n {
			continue
		}
		score, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 32)
		if err != nil {
			continue
		}
		out[idx] = float32(score)
	}
	return out
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
