package embedding

import "context"

// Embedder turns text into dense vectors.
type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	Dimension() int
	Model() string
}

// Cached wraps an Embedder and stores vectors by SHA-256(model + text).
type Cached struct {
	inner Embedder
	get   func(ctx context.Context, key string) ([]float32, bool, error)
	set   func(ctx context.Context, key string, vec []float32) error
	key   func(text string) string
}

// CacheHooks are the storage callbacks used by Cached.
type CacheHooks struct {
	Get func(ctx context.Context, key string) ([]float32, bool, error)
	Set func(ctx context.Context, key string, vec []float32) error
	Key func(model, text string) string
}

// WithCache returns an embedder that skips provider calls for known texts.
func WithCache(inner Embedder, hooks CacheHooks) Embedder {
	if inner == nil || hooks.Get == nil || hooks.Set == nil {
		return inner
	}
	keyFn := hooks.Key
	if keyFn == nil {
		keyFn = func(model, text string) string { return model + "\x00" + text }
	}
	return &Cached{
		inner: inner,
		get:   hooks.Get,
		set:   hooks.Set,
		key: func(text string) string {
			return keyFn(inner.Model(), text)
		},
	}
}

// Embed implements Embedder.
func (c *Cached) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	missingIdx := make([]int, 0, len(texts))
	missing := make([]string, 0, len(texts))
	for i, text := range texts {
		vec, ok, err := c.get(ctx, c.key(text))
		if err != nil {
			return nil, err
		}
		if ok {
			out[i] = vec
			continue
		}
		missingIdx = append(missingIdx, i)
		missing = append(missing, text)
	}
	if len(missing) == 0 {
		return out, nil
	}
	vecs, err := c.inner.Embed(ctx, missing)
	if err != nil {
		return nil, err
	}
	for i, vec := range vecs {
		out[missingIdx[i]] = vec
		if err := c.set(ctx, c.key(missing[i]), vec); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// Dimension implements Embedder.
func (c *Cached) Dimension() int { return c.inner.Dimension() }

// Model implements Embedder.
func (c *Cached) Model() string { return c.inner.Model() }
