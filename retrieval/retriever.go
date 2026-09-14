package retrieval

import (
	"context"

	"github.com/selimyalcin/ragentgo/document"
	"github.com/selimyalcin/ragentgo/embedding"
	"github.com/selimyalcin/ragentgo/rerank"
	"github.com/selimyalcin/ragentgo/store"
)

// RetrieveOptions control a query-time search.
type RetrieveOptions struct {
	TopK      int
	MinScore  float32
	Filter    map[string]any
	Namespace string
	Hybrid    bool
}

// Retriever turns a text query into ranked chunks.
type Retriever interface {
	Retrieve(ctx context.Context, query string, opts RetrieveOptions) ([]document.SearchResult, error)
}

// HybridRetriever is an optional keyword + vector path.
type HybridRetriever interface {
	RetrieveHybrid(ctx context.Context, query string, opts RetrieveOptions) ([]document.SearchResult, error)
}

// Vector is the default retriever: embed the query, then search the store.
type Vector struct {
	embedder embedding.Embedder
	store    store.VectorStore
	reranker rerank.Reranker
}

// NewVector returns a vector retriever.
func NewVector(embedder embedding.Embedder, vs store.VectorStore, reranker rerank.Reranker) *Vector {
	return &Vector{embedder: embedder, store: vs, reranker: reranker}
}

// Retrieve implements Retriever.
func (v *Vector) Retrieve(ctx context.Context, query string, opts RetrieveOptions) ([]document.SearchResult, error) {
	vecs, err := v.embedder.Embed(ctx, []string{query})
	if err != nil {
		return nil, err
	}
	if len(vecs) == 0 {
		return nil, nil
	}
	searchOpts := store.SearchOptions{
		Namespace: opts.Namespace,
		TopK:      opts.TopK,
		MinScore:  opts.MinScore,
		Filter:    opts.Filter,
		Hybrid:    opts.Hybrid,
	}
	var results []document.SearchResult
	if opts.Hybrid {
		if hs, ok := v.store.(store.HybridStore); ok {
			results, err = hs.SearchHybrid(ctx, query, vecs[0], searchOpts)
		} else {
			results, err = v.store.Search(ctx, vecs[0], searchOpts)
		}
	} else {
		results, err = v.store.Search(ctx, vecs[0], searchOpts)
	}
	if err != nil {
		return nil, err
	}
	if v.reranker != nil {
		results, err = v.reranker.Rerank(ctx, query, results)
		if err != nil {
			return nil, err
		}
	}
	if opts.TopK > 0 && len(results) > opts.TopK {
		results = results[:opts.TopK]
	}
	return results, nil
}

// RetrieveHybrid implements HybridRetriever.
func (v *Vector) RetrieveHybrid(ctx context.Context, query string, opts RetrieveOptions) ([]document.SearchResult, error) {
	opts.Hybrid = true
	return v.Retrieve(ctx, query, opts)
}
