package conversation

import (
	"context"
	"sync"
	"time"
)

// Message is one turn in a conversation.
type Message struct {
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

// Store keeps chat history separate from document retrieval.
type Store interface {
	Append(ctx context.Context, conversationID string, message Message) error
	History(ctx context.Context, conversationID string, limit int) ([]Message, error)
}

// Memory is an in-process conversation store.
type Memory struct {
	mu    sync.Mutex
	items map[string][]Message
}

// NewMemory returns an empty conversation store.
func NewMemory() *Memory {
	return &Memory{items: map[string][]Message{}}
}

// Append implements Store.
func (m *Memory) Append(ctx context.Context, conversationID string, message Message) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if message.CreatedAt.IsZero() {
		message.CreatedAt = time.Now().UTC()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.items[conversationID] = append(m.items[conversationID], message)
	return nil
}

// History implements Store. Newest messages are last.
func (m *Memory) History(ctx context.Context, conversationID string, limit int) ([]Message, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	all := m.items[conversationID]
	if limit <= 0 || limit >= len(all) {
		out := make([]Message, len(all))
		copy(out, all)
		return out, nil
	}
	out := make([]Message, limit)
	copy(out, all[len(all)-limit:])
	return out, nil
}
