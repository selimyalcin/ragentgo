package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/selimyalcin/ragentgo/embedding"
	"github.com/selimyalcin/ragentgo/internal/retry"
)

// Embedder calls Ollama's /api/embed endpoint.
type Embedder struct {
	baseURL   string
	model     string
	dimension int
	http      *http.Client
	retry     retry.Policy
}

// Options configure the Ollama embedder.
type Options struct {
	BaseURL   string
	Model     string
	Dimension int
	Timeout   time.Duration
}

// NewEmbedder returns an Ollama embedder.
func NewEmbedder(opts Options) *Embedder {
	if opts.BaseURL == "" {
		opts.BaseURL = "http://localhost:11434"
	}
	if opts.Model == "" {
		opts.Model = "nomic-embed-text"
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 60 * time.Second
	}
	return &Embedder{
		baseURL:   strings.TrimRight(opts.BaseURL, "/"),
		model:     opts.Model,
		dimension: opts.Dimension,
		http:      &http.Client{Timeout: opts.Timeout},
		retry:     retry.DefaultPolicy(),
	}
}

// Embed implements embedding.Embedder.
func (e *Embedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	payload, err := json.Marshal(map[string]any{
		"model": e.model,
		"input": texts,
	})
	if err != nil {
		return nil, err
	}
	var raw []byte
	err = retry.Do(ctx, e.retry, func(ctx context.Context) error {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.baseURL+"/api/embed", bytes.NewReader(payload))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := e.http.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		if resp.StatusCode >= 400 {
			return &retry.HTTPError{StatusCode: resp.StatusCode, Message: string(body)}
		}
		raw = body
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("ollama embed: %w", err)
	}
	var parsed struct {
		Embeddings [][]float32 `json:"embeddings"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("ollama embed decode: %w", err)
	}
	if len(parsed.Embeddings) != len(texts) {
		return nil, fmt.Errorf("ollama embed: expected %d vectors, got %d", len(texts), len(parsed.Embeddings))
	}
	if e.dimension == 0 && len(parsed.Embeddings[0]) > 0 {
		e.dimension = len(parsed.Embeddings[0])
	}
	return parsed.Embeddings, nil
}

// Dimension implements embedding.Embedder.
func (e *Embedder) Dimension() int { return e.dimension }

// Model implements embedding.Embedder.
func (e *Embedder) Model() string { return e.model }

var _ embedding.Embedder = (*Embedder)(nil)
