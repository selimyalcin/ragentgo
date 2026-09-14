package app

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/selimyalcin/ragentgo/cache"
	"github.com/selimyalcin/ragentgo/chunker"
	"github.com/selimyalcin/ragentgo/configs"
	"github.com/selimyalcin/ragentgo/conversation"
	"github.com/selimyalcin/ragentgo/embedding"
	embedhf "github.com/selimyalcin/ragentgo/embedding/huggingface"
	embedollama "github.com/selimyalcin/ragentgo/embedding/ollama"
	embedopenai "github.com/selimyalcin/ragentgo/embedding/openai"
	embedcompat "github.com/selimyalcin/ragentgo/embedding/openaicompat"
	"github.com/selimyalcin/ragentgo/internal/openaicompat"
	"github.com/selimyalcin/ragentgo/llm"
	llmollama "github.com/selimyalcin/ragentgo/llm/ollama"
	llmopenai "github.com/selimyalcin/ragentgo/llm/openai"
	llmcompat "github.com/selimyalcin/ragentgo/llm/openaicompat"
	"github.com/selimyalcin/ragentgo/rag"
	"github.com/selimyalcin/ragentgo/store"
	"github.com/selimyalcin/ragentgo/store/memory"
	"github.com/selimyalcin/ragentgo/store/pgvector"
)

// Runtime is the wired HTTP/CLI process: config + pipeline + store.
type Runtime struct {
	Config   *configs.Config
	Pipeline *rag.Pipeline
	Store    store.VectorStore
	Catalog  store.Catalog
	Logger   *slog.Logger
}

// New builds providers from config. Call Close when the process exits.
func New(ctx context.Context, cfg *configs.Config) (*Runtime, error) {
	logger := slog.Default()
	embedder, err := newEmbedder(cfg)
	if err != nil {
		return nil, err
	}
	generator, err := newGenerator(cfg)
	if err != nil {
		return nil, err
	}
	vs, err := newStore(ctx, cfg)
	if err != nil {
		return nil, err
	}
	chunk := newChunker(cfg)
	pipe, err := rag.New(
		rag.WithEmbedder(embedder),
		rag.WithVectorStore(vs),
		rag.WithGenerator(generator),
		rag.WithChunker(chunk),
		rag.WithCache(cache.NewMemory(20_000)),
		rag.WithConversationStore(conversation.NewMemory()),
		rag.WithLogger(logger),
		rag.WithMaxEmbedTokens(cfg.Embedding.MaxInputTokens),
		rag.WithLLMModel(cfg.LLM.Model),
		rag.WithQueryCache(answerCacheTTL(cfg.Cache.AnswerTTLSeconds)),
		rag.WithDefaultQuery(rag.QueryOptions{
			TopK:      cfg.Retrieval.TopK,
			MinScore:  cfg.Retrieval.MinScore,
			Hybrid:    cfg.Retrieval.Hybrid,
			MaxTokens: cfg.LLM.MaxTokens,
		}),
	)
	if err != nil {
		_ = vs.Close()
		return nil, err
	}
	rt := &Runtime{
		Config:   cfg,
		Pipeline: pipe,
		Store:    vs,
		Logger:   logger,
	}
	if c, ok := vs.(store.Catalog); ok {
		rt.Catalog = c
	}
	return rt, nil
}

// Close releases the vector store.
func (r *Runtime) Close() error {
	if r.Store != nil {
		return r.Store.Close()
	}
	return nil
}

func newEmbedder(cfg *configs.Config) (embedding.Embedder, error) {
	opts := openaicompat.Options{
		BaseURL:        cfg.Embedding.BaseURL,
		APIKey:         cfg.Embedding.APIKey,
		Model:          cfg.Embedding.Model,
		Dimension:      cfg.Embedding.Dimension,
		MaxConcurrency: cfg.Embedding.MaxConcurrency,
		Timeout:        configs.ProviderTimeout(cfg.Embedding.TimeoutSeconds, 60*time.Second),
	}
	switch strings.ToLower(cfg.Embedding.Provider) {
	case "openai":
		return embedopenai.NewEmbedder(opts), nil
	case "ollama":
		return embedollama.NewEmbedder(embedollama.Options{
			BaseURL:   cfg.Embedding.BaseURL,
			Model:     cfg.Embedding.Model,
			Dimension: cfg.Embedding.Dimension,
			Timeout:   opts.Timeout,
		}), nil
	case "huggingface", "hf", "huggingfacehub", "huggingface-inference", "hf-inference":
		return embedhf.NewEmbedder(opts), nil
	case "openai-compat", "openai_compat", "compat", "":
		return embedcompat.NewEmbedder(opts), nil
	default:
		return nil, fmt.Errorf("unknown embedding provider %q", cfg.Embedding.Provider)
	}
}

func newGenerator(cfg *configs.Config) (llm.Generator, error) {
	opts := openaicompat.Options{
		BaseURL: cfg.LLM.BaseURL,
		APIKey:  cfg.LLM.APIKey,
		Model:   cfg.LLM.Model,
		Timeout: configs.ProviderTimeout(cfg.LLM.TimeoutSeconds, 120*time.Second),
	}
	switch strings.ToLower(cfg.LLM.Provider) {
	case "openai":
		return llmopenai.NewGenerator(opts), nil
	case "ollama":
		return llmollama.NewGenerator(llmollama.Options{
			BaseURL: cfg.LLM.BaseURL,
			Model:   cfg.LLM.Model,
			Timeout: opts.Timeout,
		}), nil
	case "openai-compat", "openai_compat", "compat", "":
		return llmcompat.NewGenerator(opts), nil
	default:
		return nil, fmt.Errorf("unknown llm provider %q", cfg.LLM.Provider)
	}
}

func newStore(ctx context.Context, cfg *configs.Config) (store.VectorStore, error) {
	switch strings.ToLower(cfg.Store) {
	case "memory":
		return memory.New(), nil
	case "pgvector", "postgres", "postgresql", "":
		return pgvector.New(ctx, pgvector.Options{
			DSN:       cfg.Dsn,
			Dimension: cfg.Database.Dimension,
			Distance:  cfg.Vector.Distance,
		})
	default:
		return nil, fmt.Errorf("unknown store %q", cfg.Store)
	}
}

func answerCacheTTL(seconds int) time.Duration {
	if seconds < 0 {
		return -time.Second
	}
	return time.Duration(seconds) * time.Second
}

func newChunker(cfg *configs.Config) chunker.Chunker {
	c := chunker.Config{Size: cfg.Chunking.Size, Overlap: cfg.Chunking.Overlap}
	strategy := strings.ToLower(cfg.Chunking.Strategy)
	if cfg.Embedding.MaxInputTokens > 0 {
		if strategy == "token" {
			c = c.CapToWordTokens(cfg.Embedding.MaxInputTokens)
		} else {
			c = c.CapToTokens(cfg.Embedding.MaxInputTokens)
		}
	}
	switch strategy {
	case "markdown":
		return chunker.NewMarkdown(c)
	case "sentence":
		return chunker.NewSentence(c)
	case "token":
		return chunker.NewToken(c)
	default:
		return chunker.NewRecursive(c)
	}
}
