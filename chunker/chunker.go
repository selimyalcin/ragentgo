package chunker

import (
	"context"

	"github.com/selimyalcin/ragentgo/document"
)

// Chunker splits a document into overlapping pieces small enough to embed.
type Chunker interface {
	Chunk(ctx context.Context, doc document.Document) ([]document.Chunk, error)
}

// Config is shared by the built-in splitters.
type Config struct {
	Size    int
	Overlap int
}

// DefaultConfig is a reasonable starting point for prose and Markdown.
func DefaultConfig() Config {
	return Config{Size: 800, Overlap: 120}
}

func (c Config) withDefaults() Config {
	if c.Size <= 0 {
		c.Size = 800
	}
	if c.Overlap < 0 {
		c.Overlap = 0
	}
	if c.Overlap >= c.Size {
		c.Overlap = c.Size / 8
	}
	return c
}
