package openaicompat

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/selimyalcin/ragentgo/embedding"
	"github.com/selimyalcin/ragentgo/internal/openaicompat"
)

// Embedder implements embedding.Embedder against an OpenAI-compatible API.
type Embedder struct {
	client    *openaicompat.Client
	model     string
	dimension int
}

// Options configure the embedder.
type Options = openaicompat.Options

// NewEmbedder returns an OpenAI-compatible embedder.
func NewEmbedder(opts Options) *Embedder {
	if opts.Model == "" {
		opts.Model = "text-embedding-3-small"
	}
	return &Embedder{
		client:    openaicompat.New(opts),
		model:     opts.Model,
		dimension: opts.Dimension,
	}
}

// Embed implements embedding.Embedder.
func (e *Embedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	if err := e.client.Acquire(ctx); err != nil {
		return nil, err
	}
	defer e.client.Release()

	raw, err := e.client.Post(ctx, "/embeddings", map[string]any{
		"model": e.model,
		"input": texts,
	})
	if err != nil {
		return nil, fmt.Errorf("embed texts: %w", err)
	}
	var resp struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
			Index     int       `json:"index"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("decode embeddings: %w", err)
	}
	out := make([][]float32, len(texts))
	for _, item := range resp.Data {
		if item.Index < 0 || item.Index >= len(out) {
			continue
		}
		out[item.Index] = item.Embedding
		if e.dimension == 0 && len(item.Embedding) > 0 {
			e.dimension = len(item.Embedding)
		}
	}
	for i, vec := range out {
		if vec == nil {
			return nil, fmt.Errorf("embed texts: missing vector for index %d", i)
		}
	}
	return out, nil
}

// Dimension implements embedding.Embedder.
func (e *Embedder) Dimension() int { return e.dimension }

// Model implements embedding.Embedder.
func (e *Embedder) Model() string { return e.model }

var _ embedding.Embedder = (*Embedder)(nil)
