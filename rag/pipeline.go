package rag

import (
	"context"
	"encoding/binary"
	"fmt"
	"log/slog"
	"math"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/selimyalcin/ragentgo/cache"
	"github.com/selimyalcin/ragentgo/chunker"
	"github.com/selimyalcin/ragentgo/conversation"
	"github.com/selimyalcin/ragentgo/document"
	"github.com/selimyalcin/ragentgo/embedding"
	"github.com/selimyalcin/ragentgo/internal/hash"
	"github.com/selimyalcin/ragentgo/llm"
	"github.com/selimyalcin/ragentgo/prompt"
	"github.com/selimyalcin/ragentgo/rerank"
	"github.com/selimyalcin/ragentgo/retrieval"
	"github.com/selimyalcin/ragentgo/store"
	"github.com/selimyalcin/ragentgo/tabular"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"
)

// Pipeline is the small orchestration layer that indexes documents and answers questions.
type Pipeline struct {
	embedder       embedding.Embedder
	store          store.VectorStore
	catalog        store.Catalog
	chunker        chunker.Chunker
	retriever      retrieval.Retriever
	generator      llm.Generator
	reranker       rerank.Reranker
	prompts        *prompt.Builder
	cache          cache.Cache
	convos         conversation.Store
	logger         *slog.Logger
	index          IndexOptions
	query          QueryOptions
	maxEmbedTokens int
	llmModel       string
	queryCacheTTL  time.Duration
	queryCacheOff  bool
}

// IndexOptions control ingestion concurrency.
type IndexOptions struct {
	Workers        int
	EmbeddingBatch int
}

// QueryOptions control retrieval and generation at question time.
type QueryOptions struct {
	TopK           int
	MinScore       float32
	Filter         map[string]any
	Namespace      string
	Hybrid         bool
	ConversationID string
	Citations      bool
	Temperature    float32
	MaxTokens      int
}

// IndexResult reports what indexing actually did.
type IndexResult struct {
	DocumentsIndexed int `json:"documents_indexed"`
	DocumentsSkipped int `json:"documents_skipped"`
	ChunksCreated    int `json:"chunks_created"`
}

// QueryResult is a grounded answer plus the passages it came from.
type QueryResult struct {
	Answer  string          `json:"answer"`
	Sources []prompt.Source `json:"sources"`
	Usage   llm.Usage       `json:"usage"`
	Cached  bool            `json:"cached,omitempty"`
}

// New builds a pipeline. Embedder and vector store are required.
func New(opts ...Option) (*Pipeline, error) {
	p := &Pipeline{
		chunker:  chunker.NewRecursive(chunker.DefaultConfig()),
		reranker: rerank.Noop{},
		prompts:  prompt.New("", true),
		logger:   slog.Default(),
		index: IndexOptions{
			Workers:        runtime.NumCPU(),
			EmbeddingBatch: 64,
		},
		query: QueryOptions{
			TopK:        5,
			Citations:   true,
			Temperature: 0.2,
		},
	}
	for _, opt := range opts {
		opt(p)
	}
	if p.embedder == nil {
		return nil, ErrMissingEmbedder
	}
	if p.store == nil {
		return nil, ErrMissingStore
	}
	if p.catalog == nil {
		if c, ok := p.store.(store.Catalog); ok {
			p.catalog = c
		}
	}
	if p.index.Workers <= 0 {
		p.index.Workers = runtime.NumCPU()
	}
	if p.index.EmbeddingBatch <= 0 {
		p.index.EmbeddingBatch = 64
	}
	if p.cache != nil {
		p.embedder = embedding.WithCache(p.embedder, embedding.CacheHooks{
			Get: func(ctx context.Context, key string) ([]float32, bool, error) {
				raw, ok, err := p.cache.Get(ctx, key)
				if err != nil || !ok {
					return nil, ok, err
				}
				return bytesToVec(raw), true, nil
			},
			Set: func(ctx context.Context, key string, vec []float32) error {
				return p.cache.Set(ctx, key, vecToBytes(vec), 24*time.Hour)
			},
			Key: func(model, text string) string {
				return hash.SHA256(model, text)
			},
		})
	}
	if p.retriever == nil {
		p.retriever = retrieval.NewVector(p.embedder, p.store, p.reranker)
	}
	if p.queryCacheTTL == 0 && !p.queryCacheOff {
		p.queryCacheTTL = time.Hour
	}
	return p, nil
}

// IndexDocument chunks, embeds, and upserts a single document.
func (p *Pipeline) IndexDocument(ctx context.Context, doc document.Document) error {
	_, err := p.IndexDocuments(ctx, []document.Document{doc})
	return err
}

// IndexDocuments ingests many documents with bounded parallelism.
func (p *Pipeline) IndexDocuments(ctx context.Context, docs []document.Document) (*IndexResult, error) {
	res := &IndexResult{}
	if len(docs) == 0 {
		return res, nil
	}
	sem := semaphore.NewWeighted(int64(p.index.Workers))
	var mu sync.Mutex
	g, ctx := errgroup.WithContext(ctx)
	for i := range docs {
		doc := docs[i].Normalize()
		if strings.TrimSpace(doc.Content) == "" {
			return res, ErrEmptyDocument
		}
		g.Go(func() error {
			if err := sem.Acquire(ctx, 1); err != nil {
				return err
			}
			defer sem.Release(1)
			indexed, skipped, chunks, err := p.indexOne(ctx, doc)
			if err != nil {
				return err
			}
			mu.Lock()
			res.DocumentsIndexed += indexed
			res.DocumentsSkipped += skipped
			res.ChunksCreated += chunks
			mu.Unlock()
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return res, err
	}
	return res, nil
}

func (p *Pipeline) indexOne(ctx context.Context, doc document.Document) (indexed, skipped, chunks int, err error) {
	if p.catalog != nil {
		prev, ok, err := p.catalog.GetChecksum(ctx, doc.Namespace, doc.ID)
		if err != nil {
			return 0, 0, 0, fmt.Errorf("checksum lookup: %w", err)
		}
		if ok && prev == doc.Checksum {
			return 0, 1, 0, nil
		}
		if err := p.store.DeleteDocument(ctx, doc.ID); err != nil {
			return 0, 0, 0, fmt.Errorf("replace document: %w", err)
		}
		if err := p.catalog.UpsertDocument(ctx, document.Record{
			ID:        doc.ID,
			Namespace: doc.Namespace,
			Source:    doc.Source,
			Metadata:  doc.Metadata,
			Checksum:  doc.Checksum,
		}); err != nil {
			return 0, 0, 0, fmt.Errorf("store document meta: %w", err)
		}
	}
	var parts []document.Chunk
	if isRowUnit(doc) {
		parts = []document.Chunk{doc.Inherit(0, strings.TrimSpace(doc.Content))}
	} else {
		var err error
		parts, err = p.chunker.Chunk(ctx, doc)
		if err != nil {
			return 0, 0, 0, fmt.Errorf("chunk document: %w", err)
		}
	}
	if p.maxEmbedTokens > 0 {
		parts = chunker.Fit(parts, p.maxEmbedTokens)
	}
	if len(parts) == 0 {
		return 1, 0, 0, nil
	}
	embedded := make([]document.EmbeddedChunk, 0, len(parts))
	batch := p.index.EmbeddingBatch
	for i := 0; i < len(parts); i += batch {
		end := i + batch
		if end > len(parts) {
			end = len(parts)
		}
		slice := parts[i:end]
		texts := make([]string, len(slice))
		for j, c := range slice {
			texts[j] = c.Content
		}
		vecs, err := p.embedder.Embed(ctx, texts)
		if err != nil {
			return 0, 0, 0, fmt.Errorf("embed chunks: %w", err)
		}
		if len(vecs) != len(slice) {
			return 0, 0, 0, fmt.Errorf("embed chunks: expected %d vectors, got %d", len(slice), len(vecs))
		}
		for j, c := range slice {
			embedded = append(embedded, document.EmbeddedChunk{Chunk: c, Embedding: vecs[j]})
		}
	}
	if err := p.store.Upsert(ctx, embedded); err != nil {
		return 0, 0, 0, fmt.Errorf("upsert vectors: %w", err)
	}
	return 1, 0, len(embedded), nil
}

// Query retrieves context and asks the generator.
func (p *Pipeline) Query(ctx context.Context, question string, opts QueryOptions) (*QueryResult, error) {
	question = strings.TrimSpace(question)
	if question == "" {
		return nil, ErrEmptyQuery
	}
	if p.generator == nil {
		return nil, ErrMissingGenerator
	}
	opts = p.mergeQuery(opts)
	if cached := p.cachedAnswer(ctx, question, opts); cached != nil {
		return cached, nil
	}
	userPrompt, sources, err := p.ground(ctx, question, opts)
	if err != nil {
		return nil, err
	}
	messages := []llm.Message{{Role: "user", Content: userPrompt}}
	if opts.ConversationID != "" && p.convos != nil {
		hist, err := p.convos.History(ctx, opts.ConversationID, 10)
		if err != nil {
			return nil, err
		}
		prefixed := make([]llm.Message, 0, len(hist)+1)
		for _, m := range hist {
			prefixed = append(prefixed, llm.Message{Role: m.Role, Content: m.Content})
		}
		prefixed = append(prefixed, messages...)
		messages = prefixed
	}
	resp, err := p.generator.Generate(ctx, llm.GenerateRequest{
		Messages:    messages,
		Temperature: opts.Temperature,
		MaxTokens:   opts.MaxTokens,
	})
	if err != nil {
		return nil, fmt.Errorf("generate answer: %w", err)
	}
	if opts.ConversationID != "" && p.convos != nil {
		_ = p.convos.Append(ctx, opts.ConversationID, conversation.Message{Role: "user", Content: question})
		_ = p.convos.Append(ctx, opts.ConversationID, conversation.Message{Role: "assistant", Content: resp.Text})
	}
	out := &QueryResult{Answer: resp.Text, Sources: sources, Usage: resp.Usage}
	p.saveAnswer(ctx, question, opts, out)
	return out, nil
}

// StreamQuery is Query with token streaming. Sources are sent first by the HTTP layer.
func (p *Pipeline) StreamQuery(ctx context.Context, question string, opts QueryOptions) ([]prompt.Source, <-chan llm.StreamChunk, error) {
	question = strings.TrimSpace(question)
	if question == "" {
		return nil, nil, ErrEmptyQuery
	}
	if p.generator == nil {
		return nil, nil, ErrMissingGenerator
	}
	opts = p.mergeQuery(opts)
	if cached := p.cachedAnswer(ctx, question, opts); cached != nil {
		ch := make(chan llm.StreamChunk, 2)
		go func() {
			defer close(ch)
			if cached.Answer != "" {
				ch <- llm.StreamChunk{Text: cached.Answer}
			}
			ch <- llm.StreamChunk{Done: true, Usage: cached.Usage}
		}()
		return cached.Sources, ch, nil
	}
	userPrompt, sources, err := p.ground(ctx, question, opts)
	if err != nil {
		return nil, nil, err
	}
	inner, err := p.generator.Stream(ctx, llm.GenerateRequest{
		Messages:    []llm.Message{{Role: "user", Content: userPrompt}},
		Temperature: opts.Temperature,
		MaxTokens:   opts.MaxTokens,
	})
	if err != nil || !p.shouldCache(opts) {
		return sources, inner, err
	}
	return sources, p.cacheStream(ctx, question, opts, sources, inner), nil
}

const (
	aggregateVectorK = 64
	aggregateTextK   = 512
)

func (p *Pipeline) ground(ctx context.Context, question string, opts QueryOptions) (string, []prompt.Source, error) {
	userK := opts.TopK
	hits, err := p.retrieveQuestion(ctx, question, opts)
	if err != nil {
		return "", nil, err
	}
	if len(hits) == 0 {
		return "", nil, ErrNoDocuments
	}
	if agg := tabular.Try(question, hits); agg != nil {
		userPrompt, _ := p.prompts.BuildComputed(question, agg.Block(), agg.PromptHits())
		_, sources := p.prompts.Build(question, agg.SourceHits())
		return userPrompt, sources, nil
	}
	if userK > 0 && len(hits) > userK {
		hits = hits[:userK]
	}
	userPrompt, sources := p.prompts.Build(question, hits)
	return userPrompt, sources, nil
}

func (p *Pipeline) retrieveQuestion(ctx context.Context, question string, opts QueryOptions) ([]document.SearchResult, error) {
	retrieveOpts := opts
	if _, ok := tabular.Detect(question); ok {
		if retrieveOpts.TopK < aggregateVectorK {
			retrieveOpts.TopK = aggregateVectorK
		}
		retrieveOpts.Hybrid = true
		retrieveOpts.MinScore = 0
	}
	hits, err := p.Retrieve(ctx, question, retrieveOpts)
	if err != nil {
		return nil, err
	}
	if _, ok := tabular.Detect(question); !ok {
		return hits, nil
	}
	ts, ok := p.store.(store.TextStore)
	if !ok {
		return hits, nil
	}
	extra, err := ts.SearchText(ctx, question, store.SearchOptions{
		Namespace: retrieveOpts.Namespace,
		TopK:      aggregateTextK,
		Filter:    retrieveOpts.Filter,
	})
	if err != nil {
		p.logger.Warn("keyword gather failed", "err", err)
		return hits, nil
	}
	return mergeHits(hits, extra), nil
}

func mergeHits(a, b []document.SearchResult) []document.SearchResult {
	seen := map[string]struct{}{}
	out := make([]document.SearchResult, 0, len(a)+len(b))
	add := func(h document.SearchResult) {
		id := h.Chunk.ID
		if id == "" {
			id = h.Chunk.DocumentID
		}
		if id == "" {
			out = append(out, h)
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		out = append(out, h)
	}
	for _, h := range a {
		add(h)
	}
	for _, h := range b {
		add(h)
	}
	return out
}

// Retrieve runs search without calling the LLM.
func (p *Pipeline) Retrieve(ctx context.Context, query string, opts QueryOptions) ([]document.SearchResult, error) {
	opts = p.mergeQuery(opts)
	if p.maxEmbedTokens > 0 {
		query = chunker.TruncateRunes(query, chunker.MaxRunesForTokens(p.maxEmbedTokens))
	}
	return p.retriever.Retrieve(ctx, query, retrieval.RetrieveOptions{
		TopK:      opts.TopK,
		MinScore:  opts.MinScore,
		Filter:    opts.Filter,
		Namespace: opts.Namespace,
		Hybrid:    opts.Hybrid,
	})
}

// DeleteDocument removes a document and its chunks.
func (p *Pipeline) DeleteDocument(ctx context.Context, id string) error {
	return p.store.DeleteDocument(ctx, id)
}

// Catalog returns document metadata when the store supports it.
func (p *Pipeline) Catalog() store.Catalog { return p.catalog }

func (p *Pipeline) mergeQuery(opts QueryOptions) QueryOptions {
	if opts.TopK <= 0 {
		opts.TopK = p.query.TopK
	}
	if opts.MinScore == 0 {
		opts.MinScore = p.query.MinScore
	}
	if opts.Namespace == "" {
		opts.Namespace = p.query.Namespace
		if opts.Namespace == "" {
			opts.Namespace = document.DefaultNamespace
		}
	}
	if opts.Temperature == 0 {
		opts.Temperature = p.query.Temperature
	}
	if opts.MaxTokens <= 0 {
		opts.MaxTokens = p.query.MaxTokens
	}
	if !opts.Citations {
		opts.Citations = p.query.Citations
	}
	if !opts.Hybrid {
		opts.Hybrid = p.query.Hybrid
	}
	return opts
}

func isRowUnit(doc document.Document) bool {
	if doc.Metadata == nil {
		return false
	}
	u, _ := doc.Metadata["unit"].(string)
	return u == "row"
}

func vecToBytes(v []float32) []byte {
	b := make([]byte, 4*len(v))
	for i, f := range v {
		binary.LittleEndian.PutUint32(b[i*4:], math.Float32bits(f))
	}
	return b
}

func bytesToVec(b []byte) []float32 {
	n := len(b) / 4
	out := make([]float32, n)
	for i := 0; i < n; i++ {
		out[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return out
}
