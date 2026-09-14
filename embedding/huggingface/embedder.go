package huggingface

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/selimyalcin/ragentgo/embedding"
	"github.com/selimyalcin/ragentgo/internal/openaicompat"
)

const (
	// DefaultInferenceURL is the Hugging Face feature-extraction host.
	// OpenAI-style /v1/embeddings is not served for most embedding models.
	DefaultInferenceURL = "https://router.huggingface.co/hf-inference"
	DefaultEmbedModel   = "intfloat/multilingual-e5-small"
)

// Embedder calls Hugging Face Inference feature-extraction.
type Embedder struct {
	client    *openaicompat.Client
	model     string
	dimension int
}

// Options configure the Hugging Face embedder.
type Options = openaicompat.Options

// NewEmbedder returns a feature-extraction embedder.
func NewEmbedder(opts Options) *Embedder {
	opts.BaseURL = inferenceBase(opts.BaseURL)
	if opts.Model == "" {
		opts.Model = DefaultEmbedModel
	}
	if opts.Dimension == 0 {
		opts.Dimension = 384
	}
	return &Embedder{
		client:    openaicompat.New(opts),
		model:     strings.Trim(opts.Model, "/"),
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

	payload := map[string]any{
		"inputs": texts,
		"options": map[string]any{
			"wait_for_model": true,
			"use_cache":      true,
		},
	}
	raw, err := e.client.Post(ctx, featurePath(e.model), payload)
	if err != nil {
		if strings.Contains(err.Error(), "status 404") {
			raw, err = e.client.Post(ctx, "/models/"+e.model, payload)
		}
		if err != nil {
			return nil, fmt.Errorf("huggingface embed: %w", err)
		}
	}
	vecs, err := parseFeatureVectors(raw, len(texts))
	if err != nil {
		return nil, err
	}
	if e.dimension == 0 && len(vecs) > 0 {
		e.dimension = len(vecs[0])
	}
	return vecs, nil
}

// Dimension implements embedding.Embedder.
func (e *Embedder) Dimension() int { return e.dimension }

// Model implements embedding.Embedder.
func (e *Embedder) Model() string { return e.model }

func inferenceBase(base string) string {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	switch {
	case base == "", strings.Contains(base, "router.huggingface.co/v1"), strings.HasSuffix(base, "/v1"):
		return DefaultInferenceURL
	default:
		return base
	}
}

func featurePath(model string) string {
	return "/models/" + model + "/pipeline/feature-extraction"
}

func parseFeatureVectors(raw []byte, n int) ([][]float32, error) {
	var tok3 [][][]float32
	if err := json.Unmarshal(raw, &tok3); err == nil && len(tok3) > 0 && len(tok3[0]) > 0 && len(tok3[0][0]) > 0 {
		if len(tok3) == n {
			out := make([][]float32, n)
			for i, seq := range tok3 {
				out[i] = meanPool(seq)
			}
			return out, nil
		}
	}
	var batch [][]float32
	if err := json.Unmarshal(raw, &batch); err == nil && len(batch) > 0 && len(batch[0]) > 0 {
		if n == 1 && len(batch) != 1 {
			return [][]float32{meanPool(batch)}, nil
		}
		if len(batch) == n {
			return batch, nil
		}
	}
	var single []float32
	if err := json.Unmarshal(raw, &single); err == nil && len(single) > 0 && n == 1 {
		return [][]float32{single}, nil
	}
	return nil, fmt.Errorf("huggingface embed decode: unexpected payload (%d bytes)", len(raw))
}

func meanPool(tokens [][]float32) []float32 {
	if len(tokens) == 0 {
		return nil
	}
	dim := len(tokens[0])
	out := make([]float32, dim)
	for _, tok := range tokens {
		if len(tok) != dim {
			continue
		}
		for i, v := range tok {
			out[i] += v
		}
	}
	scale := 1 / float32(len(tokens))
	for i := range out {
		out[i] *= scale
	}
	return out
}

var _ embedding.Embedder = (*Embedder)(nil)
