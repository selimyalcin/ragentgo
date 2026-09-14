package memory

import (
	"context"
	"testing"

	"github.com/selimyalcin/ragentgo/document"
	"github.com/selimyalcin/ragentgo/store"
)

func TestNamespaceIsolation(t *testing.T) {
	s := New()
	ctx := context.Background()
	err := s.Upsert(ctx, []document.EmbeddedChunk{
		{
			Chunk:     document.Chunk{ID: "a", DocumentID: "d1", Content: "alpha", Namespace: "one"},
			Embedding: []float32{1, 0},
		},
		{
			Chunk:     document.Chunk{ID: "b", DocumentID: "d2", Content: "beta", Namespace: "two"},
			Embedding: []float32{1, 0},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.Search(ctx, []float32{1, 0}, store.SearchOptions{Namespace: "one", TopK: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Chunk.ID != "a" {
		t.Fatalf("leaked namespaces: %#v", got)
	}
}

func TestMetadataFilterAndMinScore(t *testing.T) {
	s := New()
	ctx := context.Background()
	_ = s.Upsert(ctx, []document.EmbeddedChunk{
		{
			Chunk: document.Chunk{
				ID: "a", DocumentID: "d1", Content: "go", Namespace: "default",
				Metadata: map[string]any{"lang": "en"},
			},
			Embedding: []float32{1, 0},
		},
		{
			Chunk: document.Chunk{
				ID: "b", DocumentID: "d2", Content: "rust", Namespace: "default",
				Metadata: map[string]any{"lang": "tr"},
			},
			Embedding: []float32{0.2, 0.98},
		},
	})
	got, err := s.Search(ctx, []float32{1, 0}, store.SearchOptions{
		TopK:     10,
		MinScore: 0.5,
		Filter:   map[string]any{"lang": "en"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Chunk.ID != "a" {
		t.Fatalf("filter failed: %#v", got)
	}
}

func TestDeleteDocument(t *testing.T) {
	s := New()
	ctx := context.Background()
	_ = s.Upsert(ctx, []document.EmbeddedChunk{
		{Chunk: document.Chunk{ID: "a", DocumentID: "d1", Namespace: "default"}, Embedding: []float32{1, 0}},
	})
	if err := s.DeleteDocument(ctx, "d1"); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Search(ctx, []float32{1, 0}, store.SearchOptions{TopK: 5})
	if len(got) != 0 {
		t.Fatalf("expected empty, got %#v", got)
	}
}

func TestCosineRanking(t *testing.T) {
	s := New()
	ctx := context.Background()
	_ = s.Upsert(ctx, []document.EmbeddedChunk{
		{Chunk: document.Chunk{ID: "near", DocumentID: "d1", Namespace: "default"}, Embedding: []float32{0.9, 0.1}},
		{Chunk: document.Chunk{ID: "far", DocumentID: "d2", Namespace: "default"}, Embedding: []float32{0.1, 0.9}},
	})
	got, err := s.Search(ctx, []float32{1, 0}, store.SearchOptions{TopK: 2})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Chunk.ID != "near" {
		t.Fatalf("ranking: %#v", got)
	}
}

func TestFingerprintChangesWithCorpus(t *testing.T) {
	s := New()
	ctx := context.Background()
	first, err := s.Fingerprint(ctx, "default")
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Upsert(ctx, []document.EmbeddedChunk{
		{Chunk: document.Chunk{ID: "a", DocumentID: "d1", Namespace: "default", Content: "one"}, Embedding: []float32{1, 0}},
	})
	second, err := s.Fingerprint(ctx, "default")
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("expected fingerprint to change after upsert")
	}
	other, err := s.Fingerprint(ctx, "other")
	if err != nil {
		t.Fatal(err)
	}
	if other == second {
		t.Fatal("namespaces must not share fingerprints")
	}
}
