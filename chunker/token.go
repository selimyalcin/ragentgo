package chunker

import (
	"context"
	"strings"
	"unicode"

	"github.com/selimyalcin/ragentgo/document"
)

// Token approximates token boundaries with whitespace. It is useful when you
// want chunk sizes that roughly match embedding model windows without pulling
// in a tokenizer.
type Token struct {
	cfg Config
}

// NewToken returns a whitespace-based token chunker.
func NewToken(cfg Config) *Token {
	return &Token{cfg: cfg.withDefaults()}
}

// Chunk implements Chunker.
func (t *Token) Chunk(ctx context.Context, doc document.Document) ([]document.Chunk, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	doc = doc.Normalize()
	tokens := strings.Fields(doc.Content)
	if len(tokens) == 0 {
		return nil, nil
	}
	size := t.cfg.Size
	if size <= 0 {
		size = 200
	}
	overlap := t.cfg.Overlap
	var chunks []document.Chunk
	for start := 0; start < len(tokens); {
		end := start + size
		if end > len(tokens) {
			end = len(tokens)
		}
		content := strings.Join(tokens[start:end], " ")
		chunks = append(chunks, doc.Inherit(len(chunks), content))
		if end == len(tokens) {
			break
		}
		start = end - overlap
		if start < 0 || start >= end {
			start = end
		}
	}
	return chunks, nil
}

// Sentence splits on sentence-ending punctuation.
type Sentence struct {
	cfg Config
}

// NewSentence returns a sentence-aware chunker that packs sentences up to Size.
func NewSentence(cfg Config) *Sentence {
	return &Sentence{cfg: cfg.withDefaults()}
}

// Chunk implements Chunker.
func (s *Sentence) Chunk(ctx context.Context, doc document.Document) ([]document.Chunk, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	doc = doc.Normalize()
	sentences := splitSentences(doc.Content)
	if len(sentences) == 0 {
		return nil, nil
	}
	merged := merge(sentences, s.cfg.Size, s.cfg.Overlap)
	chunks := make([]document.Chunk, 0, len(merged))
	for _, part := range merged {
		chunks = append(chunks, doc.Inherit(len(chunks), part))
	}
	return chunks, nil
}

func splitSentences(text string) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	var (
		out  []string
		buf  strings.Builder
		prev rune
	)
	for _, r := range text {
		buf.WriteRune(r)
		if (r == '.' || r == '!' || r == '?') && (prev != '.' && prev != '!') {
			// keep going if the next run would start mid-abbreviation; we keep it simple
			out = append(out, strings.TrimSpace(buf.String()))
			buf.Reset()
		}
		if !unicode.IsSpace(r) {
			prev = r
		}
	}
	if rest := strings.TrimSpace(buf.String()); rest != "" {
		out = append(out, rest)
	}
	return out
}
