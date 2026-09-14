package openai

import (
	"github.com/selimyalcin/ragentgo/embedding/openaicompat"
)

const defaultBaseURL = "https://api.openai.com/v1"

// Embedder is an OpenAI embeddings client. The wire format is the public
// OpenAI HTTP API, which keeps the core module free of a heavy vendor SDK.
type Embedder = openaicompat.Embedder

// Options configure the OpenAI embedder.
type Options = openaicompat.Options

// NewEmbedder returns an embedder pointed at api.openai.com unless BaseURL is set.
func NewEmbedder(opts Options) *Embedder {
	if opts.BaseURL == "" {
		opts.BaseURL = defaultBaseURL
	}
	if opts.Model == "" {
		opts.Model = "text-embedding-3-small"
	}
	if opts.Dimension == 0 {
		opts.Dimension = 1536
	}
	return openaicompat.NewEmbedder(opts)
}
