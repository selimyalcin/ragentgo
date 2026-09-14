package document

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	"github.com/google/uuid"
)

const DefaultNamespace = "default"

// Document is a unit of source material before chunking.
type Document struct {
	ID        string         `json:"id"`
	Content   string         `json:"content"`
	Source    string         `json:"source,omitempty"`
	Namespace string         `json:"namespace,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	Checksum  string         `json:"checksum,omitempty"`
}

// Chunk is a slice of a document that can be embedded and retrieved.
type Chunk struct {
	ID         string         `json:"id"`
	DocumentID string         `json:"document_id"`
	Content    string         `json:"content"`
	Index      int            `json:"index"`
	Namespace  string         `json:"namespace,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

// EmbeddedChunk is a chunk together with its vector.
type EmbeddedChunk struct {
	Chunk
	Embedding []float32 `json:"embedding"`
}

// SearchResult is a retrieved chunk with a similarity score.
// Higher scores are better. Cosine similarity is the default metric.
type SearchResult struct {
	Chunk Chunk   `json:"chunk"`
	Score float32 `json:"score"`
}

// Record is persisted document metadata, without the original full text.
type Record struct {
	ID        string         `json:"id"`
	Namespace string         `json:"namespace"`
	Source    string         `json:"source,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	Checksum  string         `json:"checksum,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

// Normalize fills empty identifiers and namespace so callers do not have to.
func (d Document) Normalize() Document {
	if strings.TrimSpace(d.ID) == "" {
		d.ID = NewID()
	}
	if strings.TrimSpace(d.Namespace) == "" {
		d.Namespace = DefaultNamespace
	}
	if d.Metadata == nil {
		d.Metadata = map[string]any{}
	}
	if d.Checksum == "" {
		d.Checksum = Checksum(d.Content)
	}
	return d
}

// Inherit copies document-level fields onto a chunk.
func (d Document) Inherit(index int, content string) Chunk {
	meta := CloneMetadata(d.Metadata)
	if d.Source != "" {
		if _, ok := meta["source"]; !ok {
			meta["source"] = d.Source
		}
	}
	return Chunk{
		ID:         NewID(),
		DocumentID: d.ID,
		Content:    content,
		Index:      index,
		Namespace:  d.Namespace,
		Metadata:   meta,
	}
}

// NewID returns a time-ordered UUID v7 when the runtime supports it.
func NewID() string {
	id, err := uuid.NewV7()
	if err != nil {
		return uuid.NewString()
	}
	return id.String()
}

// Checksum returns a hex-encoded SHA-256 of the document body.
func Checksum(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

// CloneMetadata copies a metadata map so chunking cannot mutate the parent.
func CloneMetadata(in map[string]any) map[string]any {
	if len(in) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
