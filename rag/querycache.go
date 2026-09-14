package rag

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/selimyalcin/ragentgo/document"
	"github.com/selimyalcin/ragentgo/internal/hash"
	"github.com/selimyalcin/ragentgo/llm"
	"github.com/selimyalcin/ragentgo/prompt"
)

func (p *Pipeline) shouldCache(opts QueryOptions) bool {
	return p.cache != nil && !p.queryCacheOff && opts.ConversationID == ""
}

func (p *Pipeline) cachedAnswer(ctx context.Context, question string, opts QueryOptions) *QueryResult {
	if !p.shouldCache(opts) {
		return nil
	}
	key, ok := p.answerCacheKey(ctx, question, opts)
	if !ok {
		return nil
	}
	raw, hit, err := p.cache.Get(ctx, key)
	if err != nil || !hit {
		return nil
	}
	var out QueryResult
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	out.Cached = true
	return &out
}

func (p *Pipeline) saveAnswer(ctx context.Context, question string, opts QueryOptions, res *QueryResult) {
	if res == nil || !p.shouldCache(opts) {
		return
	}
	key, ok := p.answerCacheKey(ctx, question, opts)
	if !ok {
		return
	}
	stored := *res
	stored.Cached = false
	raw, err := json.Marshal(stored)
	if err != nil {
		return
	}
	if err := p.cache.Set(ctx, key, raw, p.queryCacheTTL); err != nil && p.logger != nil {
		p.logger.Warn("answer cache store failed", "err", err)
	}
}

func (p *Pipeline) cacheStream(ctx context.Context, question string, opts QueryOptions, sources []prompt.Source, inner <-chan llm.StreamChunk) <-chan llm.StreamChunk {
	out := make(chan llm.StreamChunk)
	go func() {
		defer close(out)
		var (
			b     strings.Builder
			usage llm.Usage
			fail  bool
		)
		for chunk := range inner {
			if chunk.Text != "" {
				b.WriteString(chunk.Text)
			}
			if chunk.Err != nil {
				fail = true
			}
			if chunk.Done {
				usage = chunk.Usage
			}
			out <- chunk
		}
		if fail {
			return
		}
		p.saveAnswer(ctx, question, opts, &QueryResult{
			Answer:  b.String(),
			Sources: sources,
			Usage:   usage,
		})
	}()
	return out
}

func (p *Pipeline) answerCacheKey(ctx context.Context, question string, opts QueryOptions) (string, bool) {
	fp, err := p.corpusFingerprint(ctx, opts.Namespace)
	if err != nil {
		if p.logger != nil {
			p.logger.Warn("corpus fingerprint failed", "err", err)
		}
		return "", false
	}
	filter := ""
	if len(opts.Filter) > 0 {
		raw, err := json.Marshal(opts.Filter)
		if err != nil {
			return "", false
		}
		filter = string(raw)
	}
	hybrid := "0"
	if opts.Hybrid {
		hybrid = "1"
	}
	citations := "0"
	if opts.Citations {
		citations = "1"
	}
	model := p.llmModel
	if model == "" && p.generator != nil {
		model = fmt.Sprintf("%T", p.generator)
	}
	embedModel := ""
	if p.embedder != nil {
		embedModel = p.embedder.Model()
	}
	return hash.SHA256(
		"answer",
		question,
		opts.Namespace,
		model,
		embedModel,
		strconv.Itoa(opts.TopK),
		fmt.Sprintf("%g", opts.MinScore),
		hybrid,
		fmt.Sprintf("%g", opts.Temperature),
		strconv.Itoa(opts.MaxTokens),
		citations,
		filter,
		fp,
	), true
}

func (p *Pipeline) corpusFingerprint(ctx context.Context, namespace string) (string, error) {
	if p.catalog == nil {
		return "", fmt.Errorf("catalog required for answer cache")
	}
	if namespace == "" {
		namespace = document.DefaultNamespace
	}
	return p.catalog.Fingerprint(ctx, namespace)
}
