package memory

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/selimyalcin/ragentgo/document"
	"github.com/selimyalcin/ragentgo/store"
)

// Store is an in-process vector store. Useful in tests and small CLIs.
type Store struct {
	mu     sync.RWMutex
	chunks map[string]document.EmbeddedChunk
	docs   map[string]document.Record
}

// New returns an empty memory store.
func New() *Store {
	return &Store{
		chunks: map[string]document.EmbeddedChunk{},
		docs:   map[string]document.Record{},
	}
}

// Upsert implements store.VectorStore.
func (s *Store) Upsert(ctx context.Context, chunks []document.EmbeddedChunk) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	for _, c := range chunks {
		if c.Namespace == "" {
			c.Namespace = document.DefaultNamespace
		}
		s.chunks[c.ID] = c
		rec, ok := s.docs[c.DocumentID]
		if !ok {
			rec = document.Record{
				ID:        c.DocumentID,
				Namespace: c.Namespace,
				CreatedAt: now,
			}
		}
		if src, _ := c.Metadata["source"].(string); src != "" {
			rec.Source = src
		}
		rec.Metadata = document.CloneMetadata(c.Metadata)
		rec.UpdatedAt = now
		s.docs[c.DocumentID] = rec
	}
	return nil
}

// Search implements store.VectorStore.
func (s *Store) Search(ctx context.Context, vector []float32, opts store.SearchOptions) ([]document.SearchResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	opts = opts.WithDefaults()
	s.mu.RLock()
	defer s.mu.RUnlock()
	results := make([]document.SearchResult, 0, len(s.chunks))
	for _, c := range s.chunks {
		if c.Namespace != opts.Namespace {
			continue
		}
		if !matchFilter(c.Metadata, opts.Filter) {
			continue
		}
		score := cosine(vector, c.Embedding)
		if opts.MinScore > 0 && score < opts.MinScore {
			continue
		}
		results = append(results, document.SearchResult{Chunk: c.Chunk, Score: score})
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Score > results[j].Score })
	if len(results) > opts.TopK {
		results = results[:opts.TopK]
	}
	return results, nil
}

// SearchText implements store.TextStore with token overlap over chunk text.
func (s *Store) SearchText(ctx context.Context, query string, opts store.SearchOptions) ([]document.SearchResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	opts = opts.WithDefaults()
	tokens := textTokens(query)
	if len(tokens) == 0 {
		return nil, nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	results := make([]document.SearchResult, 0)
	for _, c := range s.chunks {
		if c.Namespace != opts.Namespace {
			continue
		}
		if !matchFilter(c.Metadata, opts.Filter) {
			continue
		}
		content := strings.ToLowerSpecial(unicode.TurkishCase, c.Content)
		var score float32
		for _, tok := range tokens {
			if strings.Contains(content, tok) {
				score++
			}
		}
		if score == 0 {
			continue
		}
		results = append(results, document.SearchResult{Chunk: c.Chunk, Score: score})
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Score > results[j].Score })
	if len(results) > opts.TopK {
		results = results[:opts.TopK]
	}
	return results, nil
}

func textTokens(q string) []string {
	q = strings.ToLowerSpecial(unicode.TurkishCase, strings.TrimSpace(q))
	var (
		out  []string
		cur  strings.Builder
		emit = func() {
			t := strings.TrimSpace(cur.String())
			cur.Reset()
			if utf8.RuneCountInString(t) < 3 {
				return
			}
			out = append(out, t)
		}
	)
	for _, r := range q {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			cur.WriteRune(r)
			continue
		}
		emit()
	}
	emit()
	return out
}

// DeleteDocument implements store.VectorStore.
func (s *Store) DeleteDocument(ctx context.Context, documentID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, c := range s.chunks {
		if c.DocumentID == documentID {
			delete(s.chunks, id)
		}
	}
	delete(s.docs, documentID)
	return nil
}

// DeleteBySource implements store.Catalog.
func (s *Store) DeleteBySource(ctx context.Context, namespace, source string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if namespace == "" {
		namespace = document.DefaultNamespace
	}
	if source == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, rec := range s.docs {
		if rec.Namespace == namespace && rec.Source == source {
			for cid, c := range s.chunks {
				if c.DocumentID == id {
					delete(s.chunks, cid)
				}
			}
			delete(s.docs, id)
		}
	}
	return nil
}

// Close implements store.VectorStore.
func (s *Store) Close() error { return nil }

// UpsertDocument implements store.Catalog.
func (s *Store) UpsertDocument(ctx context.Context, rec document.Record) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if rec.Namespace == "" {
		rec.Namespace = document.DefaultNamespace
	}
	now := time.Now().UTC()
	if existing, ok := s.docs[rec.ID]; ok {
		rec.CreatedAt = existing.CreatedAt
	} else if rec.CreatedAt.IsZero() {
		rec.CreatedAt = now
	}
	rec.UpdatedAt = now
	s.docs[rec.ID] = rec
	return nil
}

// GetDocument implements store.Catalog.
func (s *Store) GetDocument(ctx context.Context, documentID string) (*document.Record, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	rec, ok := s.docs[documentID]
	if !ok {
		return nil, nil
	}
	cp := rec
	return &cp, nil
}

// ListDocuments implements store.Catalog.
func (s *Store) ListDocuments(ctx context.Context, namespace string, limit, offset int) ([]document.Record, int, error) {
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}
	if namespace == "" {
		namespace = document.DefaultNamespace
	}
	if limit <= 0 {
		limit = 50
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	all := make([]document.Record, 0)
	for _, rec := range s.docs {
		if rec.Namespace == namespace {
			all = append(all, rec)
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].UpdatedAt.After(all[j].UpdatedAt) })
	total := len(all)
	if offset > len(all) {
		return []document.Record{}, total, nil
	}
	all = all[offset:]
	if len(all) > limit {
		all = all[:limit]
	}
	return all, total, nil
}

// GetChecksum implements store.Catalog.
func (s *Store) GetChecksum(ctx context.Context, namespace, documentID string) (string, bool, error) {
	rec, err := s.GetDocument(ctx, documentID)
	if err != nil || rec == nil {
		return "", false, err
	}
	if namespace != "" && rec.Namespace != namespace {
		return "", false, nil
	}
	return rec.Checksum, rec.Checksum != "", nil
}

// Fingerprint implements store.Catalog.
func (s *Store) Fingerprint(ctx context.Context, namespace string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if namespace == "" {
		namespace = document.DefaultNamespace
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := make([]string, 0)
	var latest time.Time
	for _, rec := range s.docs {
		if rec.Namespace != namespace {
			continue
		}
		if rec.UpdatedAt.After(latest) {
			latest = rec.UpdatedAt
		}
		ids = append(ids, rec.ID+":"+rec.Checksum)
	}
	sort.Strings(ids)
	return fmt.Sprintf("%d|%s|%s", len(ids), latest.UTC().Format(time.RFC3339Nano), strings.Join(ids, ",")), nil
}

func cosine(a, b []float32) float32 {
	if len(a) == 0 || len(b) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return float32(dot / (math.Sqrt(na) * math.Sqrt(nb)))
}

func matchFilter(meta map[string]any, filter map[string]any) bool {
	if len(filter) == 0 {
		return true
	}
	for k, want := range filter {
		got, ok := meta[k]
		if !ok {
			return false
		}
		if fmt.Sprint(got) != fmt.Sprint(want) {
			return false
		}
	}
	return true
}
