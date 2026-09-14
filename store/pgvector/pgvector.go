package pgvector

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	pgvec "github.com/pgvector/pgvector-go"
	pgxvec "github.com/pgvector/pgvector-go/pgx"
	"github.com/selimyalcin/ragentgo/document"
	"github.com/selimyalcin/ragentgo/store"
)

// Store is a PostgreSQL + pgvector backend.
type Store struct {
	pool      *pgxpool.Pool
	dimension int
	distance  string
}

// Options configure the pgvector store.
type Options struct {
	DSN       string
	Dimension int
	Distance  string // cosine, l2, ip
}

// New connects to PostgreSQL and ensures the schema exists.
func New(ctx context.Context, opts Options) (*Store, error) {
	if opts.DSN == "" {
		return nil, errors.New("pgvector: dsn is required")
	}
	if opts.Dimension <= 0 {
		opts.Dimension = 1536
	}
	if opts.Distance == "" {
		opts.Distance = "cosine"
	}
	cfg, err := pgxpool.ParseConfig(opts.DSN)
	if err != nil {
		return nil, fmt.Errorf("parse dsn: %w", err)
	}
	cfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		if _, err := conn.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS vector"); err != nil {
			return err
		}
		return pgxvec.RegisterTypes(ctx, conn)
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("connect postgres: %w", err)
	}
	s := &Store{pool: pool, dimension: opts.Dimension, distance: opts.Distance}
	if err := s.migrate(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) migrate(ctx context.Context) error {
	stmts := []string{
		`CREATE EXTENSION IF NOT EXISTS vector`,
		`CREATE TABLE IF NOT EXISTS rag_documents (
			id UUID PRIMARY KEY,
			namespace TEXT NOT NULL DEFAULT 'default',
			source TEXT,
			metadata JSONB NOT NULL DEFAULT '{}',
			checksum TEXT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS rag_chunks (
			id UUID PRIMARY KEY,
			document_id UUID NOT NULL REFERENCES rag_documents(id) ON DELETE CASCADE,
			namespace TEXT NOT NULL DEFAULT 'default',
			chunk_index INTEGER NOT NULL,
			content TEXT NOT NULL,
			metadata JSONB NOT NULL DEFAULT '{}',
			embedding VECTOR(%d),
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`, s.dimension),
		`CREATE INDEX IF NOT EXISTS rag_chunks_document_idx ON rag_chunks(document_id)`,
		`CREATE INDEX IF NOT EXISTS rag_chunks_namespace_idx ON rag_chunks(namespace)`,
		`CREATE INDEX IF NOT EXISTS rag_documents_namespace_idx ON rag_documents(namespace)`,
	}
	for _, stmt := range stmts {
		if _, err := s.pool.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
	}
	ops := "vector_cosine_ops"
	switch s.distance {
	case "l2":
		ops = "vector_l2_ops"
	case "ip":
		ops = "vector_ip_ops"
	}
	if _, err := s.pool.Exec(ctx, fmt.Sprintf(
		`CREATE INDEX IF NOT EXISTS rag_chunks_embedding_hnsw ON rag_chunks USING hnsw (embedding %s)`, ops,
	)); err != nil {
		return fmt.Errorf("create hnsw index: %w", err)
	}
	return nil
}

// Upsert implements store.VectorStore.
func (s *Store) Upsert(ctx context.Context, chunks []document.EmbeddedChunk) error {
	if len(chunks) == 0 {
		return nil
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	for _, c := range chunks {
		if c.Namespace == "" {
			c.Namespace = document.DefaultNamespace
		}
		meta, err := json.Marshal(c.Metadata)
		if err != nil {
			return fmt.Errorf("encode metadata: %w", err)
		}
		source, _ := c.Metadata["source"].(string)
		_, err = tx.Exec(ctx, `
			INSERT INTO rag_documents (id, namespace, source, metadata, updated_at)
			VALUES ($1, $2, $3, $4, NOW())
			ON CONFLICT (id) DO UPDATE SET
				namespace = EXCLUDED.namespace,
				source = COALESCE(NULLIF(EXCLUDED.source, ''), rag_documents.source),
				metadata = EXCLUDED.metadata,
				updated_at = NOW()
		`, c.DocumentID, c.Namespace, source, meta)
		if err != nil {
			return fmt.Errorf("upsert document: %w", err)
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO rag_chunks (id, document_id, namespace, chunk_index, content, metadata, embedding)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			ON CONFLICT (id) DO UPDATE SET
				content = EXCLUDED.content,
				metadata = EXCLUDED.metadata,
				embedding = EXCLUDED.embedding,
				chunk_index = EXCLUDED.chunk_index
		`, c.ID, c.DocumentID, c.Namespace, c.Index, c.Content, meta, pgvec.NewVector(c.Embedding))
		if err != nil {
			return fmt.Errorf("upsert chunk: %w", err)
		}
	}
	return tx.Commit(ctx)
}

// Search implements store.VectorStore.
func (s *Store) Search(ctx context.Context, vector []float32, opts store.SearchOptions) ([]document.SearchResult, error) {
	opts = opts.WithDefaults()
	op, scoreExpr := s.distanceSQL()
	q := fmt.Sprintf(`
		SELECT id, document_id, namespace, chunk_index, content, metadata, %s AS score
		FROM rag_chunks
		WHERE namespace = $1
		  AND embedding IS NOT NULL
		  AND ($2::jsonb IS NULL OR metadata @> $2::jsonb)
		ORDER BY embedding %s $3
		LIMIT $4
	`, scoreExpr, op)
	filter, err := filterJSON(opts.Filter)
	if err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, q, opts.Namespace, filter, pgvec.NewVector(vector), opts.TopK)
	if err != nil {
		return nil, fmt.Errorf("vector search: %w", err)
	}
	defer rows.Close()
	return scanResults(rows, opts.MinScore)
}

// SearchHybrid implements store.HybridStore using Reciprocal Rank Fusion.
func (s *Store) SearchHybrid(ctx context.Context, query string, vector []float32, opts store.SearchOptions) ([]document.SearchResult, error) {
	opts = opts.WithDefaults()
	vectorHits, err := s.Search(ctx, vector, opts)
	if err != nil {
		return nil, err
	}
	keywordHits, err := s.searchText(ctx, query, opts)
	if err != nil {
		return nil, err
	}
	if len(keywordHits) == 0 {
		return vectorHits, nil
	}
	return rrf(vectorHits, keywordHits, opts.TopK, opts.MinScore), nil
}

// SearchText implements store.TextStore.
func (s *Store) SearchText(ctx context.Context, query string, opts store.SearchOptions) ([]document.SearchResult, error) {
	opts = opts.WithDefaults()
	return s.searchText(ctx, query, opts)
}

func (s *Store) searchText(ctx context.Context, query string, opts store.SearchOptions) ([]document.SearchResult, error) {
	filter, err := filterJSON(opts.Filter)
	if err != nil {
		return nil, err
	}
	tsQuery := keywordTSQuery(query)
	if tsQuery == "" {
		return nil, nil
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, document_id, namespace, chunk_index, content, metadata,
		       ts_rank_cd(to_tsvector('simple', content), $3::tsquery) AS score
		FROM rag_chunks
		WHERE namespace = $1
		  AND ($2::jsonb IS NULL OR metadata @> $2::jsonb)
		  AND to_tsvector('simple', content) @@ $3::tsquery
		ORDER BY score DESC
		LIMIT $4
	`, opts.Namespace, filter, tsQuery, opts.TopK)
	if err != nil {
		return nil, fmt.Errorf("keyword search: %w", err)
	}
	defer rows.Close()
	return scanResults(rows, 0)
}

// DeleteDocument implements store.VectorStore.
func (s *Store) DeleteDocument(ctx context.Context, documentID string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM rag_documents WHERE id = $1`, documentID)
	if err != nil {
		return fmt.Errorf("delete document: %w", err)
	}
	return nil
}

// Close implements store.VectorStore.
func (s *Store) Close() error {
	s.pool.Close()
	return nil
}

// UpsertDocument implements store.Catalog.
func (s *Store) UpsertDocument(ctx context.Context, rec document.Record) error {
	if rec.Namespace == "" {
		rec.Namespace = document.DefaultNamespace
	}
	meta, err := json.Marshal(rec.Metadata)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO rag_documents (id, namespace, source, metadata, checksum, updated_at)
		VALUES ($1, $2, $3, $4, $5, NOW())
		ON CONFLICT (id) DO UPDATE SET
			namespace = EXCLUDED.namespace,
			source = EXCLUDED.source,
			metadata = EXCLUDED.metadata,
			checksum = EXCLUDED.checksum,
			updated_at = NOW()
	`, rec.ID, rec.Namespace, rec.Source, meta, rec.Checksum)
	return err
}

// DeleteBySource removes every catalog row (and cascaded chunks) for a file name.
func (s *Store) DeleteBySource(ctx context.Context, namespace, source string) error {
	if namespace == "" {
		namespace = document.DefaultNamespace
	}
	if strings.TrimSpace(source) == "" {
		return nil
	}
	_, err := s.pool.Exec(ctx, `DELETE FROM rag_documents WHERE namespace = $1 AND source = $2`, namespace, source)
	return err
}

// GetDocument implements store.Catalog.
func (s *Store) GetDocument(ctx context.Context, documentID string) (*document.Record, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, namespace, source, metadata, checksum, created_at, updated_at
		FROM rag_documents WHERE id = $1
	`, documentID)
	rec, err := scanRecord(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return rec, err
}

// ListDocuments implements store.Catalog.
func (s *Store) ListDocuments(ctx context.Context, namespace string, limit, offset int) ([]document.Record, int, error) {
	if namespace == "" {
		namespace = document.DefaultNamespace
	}
	if limit <= 0 {
		limit = 50
	}
	var total int
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM rag_documents WHERE namespace = $1`, namespace).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, namespace, source, metadata, checksum, created_at, updated_at
		FROM rag_documents
		WHERE namespace = $1
		ORDER BY updated_at DESC
		LIMIT $2 OFFSET $3
	`, namespace, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []document.Record
	for rows.Next() {
		rec, err := scanRecord(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *rec)
	}
	return out, total, rows.Err()
}

// GetChecksum implements store.Catalog.
func (s *Store) GetChecksum(ctx context.Context, namespace, documentID string) (string, bool, error) {
	var checksum string
	err := s.pool.QueryRow(ctx, `
		SELECT checksum FROM rag_documents WHERE id = $1 AND ($2 = '' OR namespace = $2)
	`, documentID, namespace).Scan(&checksum)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return checksum, checksum != "", nil
}

// Fingerprint implements store.Catalog.
func (s *Store) Fingerprint(ctx context.Context, namespace string) (string, error) {
	if namespace == "" {
		namespace = document.DefaultNamespace
	}
	var (
		count  int64
		latest time.Time
		digest string
	)
	err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*)::bigint,
		       COALESCE(MAX(updated_at), TIMESTAMPTZ 'epoch'),
		       md5(COALESCE(string_agg(id::text || ':' || COALESCE(checksum, ''), ',' ORDER BY id), ''))
		FROM rag_documents
		WHERE namespace = $1
	`, namespace).Scan(&count, &latest, &digest)
	if err != nil {
		return "", fmt.Errorf("corpus fingerprint: %w", err)
	}
	return fmt.Sprintf("%d|%s|%s", count, latest.UTC().Format(time.RFC3339Nano), digest), nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanRecord(row rowScanner) (*document.Record, error) {
	var rec document.Record
	var meta []byte
	if err := row.Scan(&rec.ID, &rec.Namespace, &rec.Source, &meta, &rec.Checksum, &rec.CreatedAt, &rec.UpdatedAt); err != nil {
		return nil, err
	}
	if len(meta) > 0 {
		_ = json.Unmarshal(meta, &rec.Metadata)
	}
	if rec.Metadata == nil {
		rec.Metadata = map[string]any{}
	}
	return &rec, nil
}

func scanResults(rows pgx.Rows, minScore float32) ([]document.SearchResult, error) {
	var out []document.SearchResult
	for rows.Next() {
		var (
			c     document.Chunk
			meta  []byte
			score float32
		)
		if err := rows.Scan(&c.ID, &c.DocumentID, &c.Namespace, &c.Index, &c.Content, &meta, &score); err != nil {
			return nil, err
		}
		if len(meta) > 0 {
			_ = json.Unmarshal(meta, &c.Metadata)
		}
		if c.Metadata == nil {
			c.Metadata = map[string]any{}
		}
		if minScore > 0 && score < minScore {
			continue
		}
		out = append(out, document.SearchResult{Chunk: c, Score: score})
	}
	return out, rows.Err()
}

func (s *Store) distanceSQL() (op, score string) {
	switch s.distance {
	case "l2":
		return "<->", "1.0 / (1.0 + (embedding <-> $3))"
	case "ip":
		return "<#>", "(embedding <#> $3) * -1"
	default:
		return "<=>", "1.0 - (embedding <=> $3)"
	}
}

func filterJSON(filter map[string]any) (any, error) {
	if len(filter) == 0 {
		return nil, nil
	}
	b, err := json.Marshal(filter)
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

func rrf(a, b []document.SearchResult, topK int, minScore float32) []document.SearchResult {
	const k = 60.0
	scores := map[string]float32{}
	items := map[string]document.SearchResult{}
	add := func(list []document.SearchResult) {
		for rank, item := range list {
			id := item.Chunk.ID
			items[id] = item
			scores[id] += float32(1.0 / (k + float64(rank+1)))
		}
	}
	add(a)
	add(b)
	out := make([]document.SearchResult, 0, len(items))
	for id, item := range items {
		item.Score = scores[id]
		if minScore > 0 && item.Score < minScore {
			continue
		}
		out = append(out, item)
	}
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].Score > out[i].Score {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	if topK > 0 && len(out) > topK {
		out = out[:topK]
	}
	return out
}
