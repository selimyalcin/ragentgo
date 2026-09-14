package rag

import (
	"log/slog"
	"strings"
	"time"

	"github.com/selimyalcin/ragentgo/cache"
	"github.com/selimyalcin/ragentgo/chunker"
	"github.com/selimyalcin/ragentgo/conversation"
	"github.com/selimyalcin/ragentgo/embedding"
	"github.com/selimyalcin/ragentgo/llm"
	"github.com/selimyalcin/ragentgo/prompt"
	"github.com/selimyalcin/ragentgo/rerank"
	"github.com/selimyalcin/ragentgo/retrieval"
	"github.com/selimyalcin/ragentgo/store"
)

// Option configures a Pipeline.
type Option func(*Pipeline)

// WithEmbedder sets the embedding provider.
func WithEmbedder(e embedding.Embedder) Option {
	return func(p *Pipeline) { p.embedder = e }
}

// WithVectorStore sets the vector store.
func WithVectorStore(s store.VectorStore) Option {
	return func(p *Pipeline) { p.store = s }
}

// WithCatalog sets document metadata storage. Optional when the store already implements it.
func WithCatalog(c store.Catalog) Option {
	return func(p *Pipeline) { p.catalog = c }
}

// WithChunker overrides the default recursive chunker.
func WithChunker(c chunker.Chunker) Option {
	return func(p *Pipeline) { p.chunker = c }
}

// WithRetriever overrides the default vector retriever.
func WithRetriever(r retrieval.Retriever) Option {
	return func(p *Pipeline) { p.retriever = r }
}

// WithGenerator sets the LLM used by Query.
func WithGenerator(g llm.Generator) Option {
	return func(p *Pipeline) { p.generator = g }
}

// WithReranker sets an optional reranker.
func WithReranker(r rerank.Reranker) Option {
	return func(p *Pipeline) { p.reranker = r }
}

// WithPrompt sets the prompt builder.
func WithPrompt(b *prompt.Builder) Option {
	return func(p *Pipeline) { p.prompts = b }
}

// WithCache sets an embedding cache.
func WithCache(c cache.Cache) Option {
	return func(p *Pipeline) { p.cache = c }
}

// WithConversationStore keeps chat history next to answers.
func WithConversationStore(s conversation.Store) Option {
	return func(p *Pipeline) { p.convos = s }
}

// WithLogger sets structured logging.
func WithLogger(l *slog.Logger) Option {
	return func(p *Pipeline) {
		if l != nil {
			p.logger = l
		}
	}
}

// WithIndexOptions sets ingestion worker counts.
func WithIndexOptions(o IndexOptions) Option {
	return func(p *Pipeline) { p.index = o }
}

// WithDefaultQuery sets default retrieval parameters.
func WithDefaultQuery(o QueryOptions) Option {
	return func(p *Pipeline) { p.query = o }
}

// WithMaxEmbedTokens caps chunk and query text for embedding models with a
// small context window. Zero disables the cap.
func WithMaxEmbedTokens(n int) Option {
	return func(p *Pipeline) { p.maxEmbedTokens = n }
}

// WithLLMModel records the generator model name for answer-cache keys.
func WithLLMModel(model string) Option {
	return func(p *Pipeline) { p.llmModel = strings.TrimSpace(model) }
}

// WithQueryCache sets the answer-cache TTL. A negative duration disables it.
// Zero means keep entries until the corpus fingerprint changes or eviction.
func WithQueryCache(ttl time.Duration) Option {
	return func(p *Pipeline) {
		if ttl < 0 {
			p.queryCacheOff = true
			return
		}
		p.queryCacheTTL = ttl
		p.queryCacheOff = false
	}
}
