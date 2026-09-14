package store

import (
	"context"

	"github.com/selimyalcin/ragentgo/document"
)

// SearchOptions control a vector (or hybrid) lookup.
type SearchOptions struct {
	Namespace string
	TopK      int
	MinScore  float32
	Filter    map[string]any
	Hybrid    bool
}

// VectorStore persists embeddings and runs similarity search.
type VectorStore interface {
	Upsert(ctx context.Context, chunks []document.EmbeddedChunk) error
	Search(ctx context.Context, vector []float32, opts SearchOptions) ([]document.SearchResult, error)
	DeleteDocument(ctx context.Context, documentID string) error
	Close() error
}

// Catalog stores document-level metadata used by the HTTP API and incremental indexing.
type Catalog interface {
	UpsertDocument(ctx context.Context, rec document.Record) error
	GetDocument(ctx context.Context, documentID string) (*document.Record, error)
	ListDocuments(ctx context.Context, namespace string, limit, offset int) ([]document.Record, int, error)
	GetChecksum(ctx context.Context, namespace, documentID string) (string, bool, error)
	DeleteBySource(ctx context.Context, namespace, source string) error
	Fingerprint(ctx context.Context, namespace string) (string, error)
}

// HybridStore is an optional extension for keyword + vector fusion.
type HybridStore interface {
	SearchHybrid(ctx context.Context, query string, vector []float32, opts SearchOptions) ([]document.SearchResult, error)
}

// TextStore is an optional keyword search used to gather many labeled rows
// for totals that would not fit in a small vector TopK.
type TextStore interface {
	SearchText(ctx context.Context, query string, opts SearchOptions) ([]document.SearchResult, error)
}

func (o SearchOptions) WithDefaults() SearchOptions {
	if stringsEmpty(o.Namespace) {
		o.Namespace = document.DefaultNamespace
	}
	if o.TopK <= 0 {
		o.TopK = 5
	}
	return o
}

func stringsEmpty(s string) bool { return s == "" }
