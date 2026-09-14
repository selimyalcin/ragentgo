package chunker

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/selimyalcin/ragentgo/document"
)

// Markdown keeps fenced code blocks intact and prefers heading boundaries.
type Markdown struct {
	cfg Config
}

// NewMarkdown returns a Markdown-aware chunker.
func NewMarkdown(cfg Config) *Markdown {
	return &Markdown{cfg: cfg.withDefaults()}
}

// Chunk implements Chunker.
func (m *Markdown) Chunk(ctx context.Context, doc document.Document) ([]document.Chunk, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	doc = doc.Normalize()
	sections := splitMarkdown(doc.Content)
	var packed []string
	var buf strings.Builder
	flush := func() {
		s := strings.TrimSpace(buf.String())
		if s != "" {
			packed = append(packed, s)
		}
		buf.Reset()
	}
	for _, section := range sections {
		if utf8.RuneCountInString(section) > m.cfg.Size && !isFence(section) {
			flush()
			rec := NewRecursive(m.cfg)
			sub, err := rec.Chunk(ctx, document.Document{Content: section})
			if err != nil {
				return nil, err
			}
			for _, c := range sub {
				packed = append(packed, c.Content)
			}
			continue
		}
		if buf.Len() > 0 && utf8.RuneCountInString(buf.String())+utf8.RuneCountInString(section)+2 > m.cfg.Size {
			flush()
		}
		if buf.Len() > 0 {
			buf.WriteString("\n\n")
		}
		buf.WriteString(section)
	}
	flush()
	chunks := make([]document.Chunk, 0, len(packed))
	for _, part := range packed {
		chunks = append(chunks, doc.Inherit(len(chunks), part))
	}
	return chunks, nil
}

func splitMarkdown(content string) []string {
	lines := strings.Split(content, "\n")
	var (
		sections []string
		buf      strings.Builder
		inFence  bool
	)
	flush := func() {
		s := strings.TrimSpace(buf.String())
		if s != "" {
			sections = append(sections, s)
		}
		buf.Reset()
	}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			if inFence {
				buf.WriteString(line)
				buf.WriteByte('\n')
				inFence = false
				flush()
				continue
			}
			flush()
			inFence = true
			buf.WriteString(line)
			buf.WriteByte('\n')
			continue
		}
		if inFence {
			buf.WriteString(line)
			buf.WriteByte('\n')
			continue
		}
		if strings.HasPrefix(trimmed, "#") && buf.Len() > 0 {
			flush()
		}
		buf.WriteString(line)
		buf.WriteByte('\n')
	}
	flush()
	return sections
}

func isFence(s string) bool {
	t := strings.TrimSpace(s)
	return strings.HasPrefix(t, "```") && strings.HasSuffix(t, "```")
}
