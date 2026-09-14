package prompt

import (
	"fmt"
	"strings"

	"github.com/selimyalcin/ragentgo/document"
)

const defaultTemplate = `You answer questions using the retrieved records.

The context is made of table rows and document passages. Treat each record as structured facts (name, role, company, month, amount, date).

Rules:
- Prefer exact values from matching rows. Quote names, titles, months, and amounts as written.
- If a COMPUTED RESULT block is present, use its value for totals, counts, averages, min, and max. Do not add a subset of the listed rows.
- If several rows match, mention each distinct match.
- Answer in the same language as the question.
- If the context does not contain the fact, say the information was not found.
- Cite supporting records as [n].

CONTEXT:

{{context}}

QUESTION:

{{question}}`

// Builder renders the grounded RAG prompt.
type Builder struct {
	template  string
	citations bool
}

// New returns a prompt builder. An empty template uses the built-in default.
func New(template string, citations bool) *Builder {
	if strings.TrimSpace(template) == "" {
		template = defaultTemplate
	}
	return &Builder{template: template, citations: citations}
}

// Build returns the user prompt and a source list aligned with citation numbers.
func (b *Builder) Build(question string, hits []document.SearchResult) (string, []Source) {
	return b.BuildComputed(question, "", hits)
}

// BuildComputed is Build with an optional authoritative numeric result.
func (b *Builder) BuildComputed(question, computed string, hits []document.SearchResult) (string, []Source) {
	sources := make([]Source, 0, len(hits))
	var ctx strings.Builder
	if strings.TrimSpace(computed) != "" {
		ctx.WriteString(strings.TrimSpace(computed))
		ctx.WriteString("\n\n")
	}
	for i, hit := range hits {
		n := i + 1
		src := Source{
			Index:      n,
			DocumentID: hit.Chunk.DocumentID,
			ChunkID:    hit.Chunk.ID,
			Content:    hit.Chunk.Content,
			Score:      hit.Score,
			Metadata:   hit.Chunk.Metadata,
		}
		if name, _ := hit.Chunk.Metadata["source"].(string); name != "" {
			src.Source = name
		} else if name, _ := hit.Chunk.Metadata["filename"].(string); name != "" {
			src.Source = name
		}
		sources = append(sources, src)
		fmt.Fprintf(&ctx, "[%d] %s\n\n", n, hit.Chunk.Content)
	}
	prompt := strings.ReplaceAll(b.template, "{{context}}", strings.TrimSpace(ctx.String()))
	prompt = strings.ReplaceAll(prompt, "{{question}}", question)
	return prompt, sources
}

// Source is a cited retrieved chunk.
type Source struct {
	Index      int            `json:"index"`
	DocumentID string         `json:"document_id"`
	ChunkID    string         `json:"chunk_id"`
	Source     string         `json:"source,omitempty"`
	Content    string         `json:"content"`
	Score      float32        `json:"score"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

// Citations reports whether citation markers are requested.
func (b *Builder) Citations() bool { return b.citations }
