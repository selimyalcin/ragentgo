package cache

import (
	"context"
	"sync"
	"time"
)

// Cache is a small byte cache used for embeddings.
type Cache interface {
	Get(ctx context.Context, key string) ([]byte, bool, error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
}

type entry struct {
	value     []byte
	expiresAt time.Time
}

// Memory is a process-local TTL cache.
type Memory struct {
	mu      sync.Mutex
	items   map[string]entry
	maxSize int
}

// NewMemory returns an in-memory cache. maxSize of 0 means 10_000 entries.
func NewMemory(maxSize int) *Memory {
	if maxSize <= 0 {
		maxSize = 10_000
	}
	return &Memory{items: map[string]entry{}, maxSize: maxSize}
}

// Get implements Cache.
func (m *Memory) Get(ctx context.Context, key string) ([]byte, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.items[key]
	if !ok {
		return nil, false, nil
	}
	if !e.expiresAt.IsZero() && time.Now().After(e.expiresAt) {
		delete(m.items, key)
		return nil, false, nil
	}
	cp := append([]byte(nil), e.value...)
	return cp, true, nil
}

// Set implements Cache.
func (m *Memory) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.items) >= m.maxSize {
		for k := range m.items {
			delete(m.items, k)
			break
		}
	}
	var exp time.Time
	if ttl > 0 {
		exp = time.Now().Add(ttl)
	}
	m.items[key] = entry{value: append([]byte(nil), value...), expiresAt: exp}
	return nil
}
