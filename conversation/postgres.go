package conversation

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Postgres stores conversations in the same database as the vector index.
type Postgres struct {
	pool *pgxpool.Pool
}

// NewPostgres prepares the conversations table on the given pool.
func NewPostgres(ctx context.Context, pool *pgxpool.Pool) (*Postgres, error) {
	if pool == nil {
		return nil, errors.New("conversation: pool is required")
	}
	_, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS rag_conversations (
			id BIGSERIAL PRIMARY KEY,
			conversation_id TEXT NOT NULL,
			role TEXT NOT NULL,
			content TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`)
	if err != nil {
		return nil, err
	}
	_, err = pool.Exec(ctx, `CREATE INDEX IF NOT EXISTS rag_conversations_cid_idx ON rag_conversations(conversation_id, created_at)`)
	if err != nil {
		return nil, err
	}
	return &Postgres{pool: pool}, nil
}

// Append implements Store.
func (p *Postgres) Append(ctx context.Context, conversationID string, message Message) error {
	if message.CreatedAt.IsZero() {
		message.CreatedAt = time.Now().UTC()
	}
	_, err := p.pool.Exec(ctx, `
		INSERT INTO rag_conversations (conversation_id, role, content, created_at)
		VALUES ($1, $2, $3, $4)
	`, conversationID, message.Role, message.Content, message.CreatedAt)
	return err
}

// History implements Store.
func (p *Postgres) History(ctx context.Context, conversationID string, limit int) ([]Message, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := p.pool.Query(ctx, `
		SELECT role, content, created_at
		FROM (
			SELECT role, content, created_at
			FROM rag_conversations
			WHERE conversation_id = $1
			ORDER BY created_at DESC
			LIMIT $2
		) t
		ORDER BY created_at ASC
	`, conversationID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Message
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.Role, &m.Content, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
