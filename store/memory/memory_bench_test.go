package memory

import (
	"context"
	"testing"

	"github.com/selimyalcin/ragentgo/document"
	"github.com/selimyalcin/ragentgo/store"
)

func BenchmarkMemoryVectorSearch(b *testing.B) {
	s := New()
	ctx := context.Background()
	chunks := make([]document.EmbeddedChunk, 1000)
	for i := range chunks {
		vec := make([]float32, 32)
		vec[i%32] = 1
		chunks[i] = document.EmbeddedChunk{
			Chunk:     document.Chunk{ID: document.NewID(), DocumentID: "d", Namespace: "default", Content: "x"},
			Embedding: vec,
		}
	}
	if err := s.Upsert(ctx, chunks); err != nil {
		b.Fatal(err)
	}
	q := make([]float32, 32)
	q[3] = 1
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := s.Search(ctx, q, store.SearchOptions{TopK: 5}); err != nil {
			b.Fatal(err)
		}
	}
}
