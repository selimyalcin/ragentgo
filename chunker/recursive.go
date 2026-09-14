package chunker

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/selimyalcin/ragentgo/document"
)

// Recursive splits text by a cascade of separators, keeping pieces under Size.
// It is the default chunker because it works well on mixed prose without
// needing a tokenizer.
type Recursive struct {
	cfg        Config
	separators []string
}

// NewRecursive returns a recursive character splitter.
func NewRecursive(cfg Config) *Recursive {
	cfg = cfg.withDefaults()
	return &Recursive{
		cfg:        cfg,
		separators: []string{"\n\n", "\n", ". ", " ", ""},
	}
}

// Chunk implements Chunker.
func (r *Recursive) Chunk(ctx context.Context, doc document.Document) ([]document.Chunk, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	doc = doc.Normalize()
	text := strings.TrimSpace(doc.Content)
	if text == "" {
		return nil, nil
	}
	parts := r.split(text, r.separators)
	merged := merge(parts, r.cfg.Size, r.cfg.Overlap)
	chunks := make([]document.Chunk, 0, len(merged))
	for i, part := range merged {
		if strings.TrimSpace(part) == "" {
			continue
		}
		chunks = append(chunks, doc.Inherit(len(chunks)+i-i, strings.TrimSpace(part)))
		chunks[len(chunks)-1].Index = len(chunks) - 1
	}
	return chunks, nil
}

func (r *Recursive) split(text string, seps []string) []string {
	if utf8.RuneCountInString(text) <= r.cfg.Size || len(seps) == 0 {
		return []string{text}
	}
	sep := seps[0]
	rest := seps[1:]
	var pieces []string
	if sep == "" {
		return splitRunes(text, r.cfg.Size)
	}
	for _, p := range strings.Split(text, sep) {
		if strings.TrimSpace(p) == "" {
			continue
		}
		joined := p
		if sep != " " && sep != "" && !strings.HasSuffix(p, sep) && sep != "\n" && sep != "\n\n" {
			// keep sentence punctuation attached when we split on ". "
			if sep == ". " {
				joined = p + "."
			}
		}
		if utf8.RuneCountInString(joined) <= r.cfg.Size {
			pieces = append(pieces, strings.TrimSpace(joined))
			continue
		}
		pieces = append(pieces, r.split(joined, rest)...)
	}
	return pieces
}

func splitRunes(text string, size int) []string {
	runes := []rune(text)
	var out []string
	for len(runes) > 0 {
		n := size
		if n > len(runes) {
			n = len(runes)
		}
		out = append(out, string(runes[:n]))
		runes = runes[n:]
	}
	return out
}

func merge(parts []string, size, overlap int) []string {
	if len(parts) == 0 {
		return nil
	}
	var (
		out     []string
		current strings.Builder
		count   int
	)
	flush := func() {
		s := strings.TrimSpace(current.String())
		if s != "" {
			out = append(out, s)
		}
		current.Reset()
		count = 0
	}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		n := utf8.RuneCountInString(part)
		if n == 0 {
			continue
		}
		if count > 0 && count+1+n > size {
			prev := current.String()
			flush()
			if overlap > 0 {
				tail := tailRunes(prev, overlap)
				if tail != "" {
					current.WriteString(tail)
					count = utf8.RuneCountInString(tail)
				}
			}
		}
		if current.Len() > 0 {
			current.WriteByte(' ')
			count++
		}
		current.WriteString(part)
		count += n
	}
	flush()
	return out
}

func tailRunes(s string, n int) string {
	runes := []rune(strings.TrimSpace(s))
	if n >= len(runes) {
		return string(runes)
	}
	return string(runes[len(runes)-n:])
}
