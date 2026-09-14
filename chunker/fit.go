package chunker

import (
	"strings"
	"unicode/utf8"

	"github.com/selimyalcin/ragentgo/document"
)

// Conservative tokenizer estimate for MiniLM-style models.
// 800 English chars measured 281 tokens (≈0.35/rune). A labeled Turkish
// payroll row later measured 277 tokens under 256 n_ctx, so the budget uses
// 0.65 tokens/rune plus a special-token reserve.
const (
	tokensPerRune = 0.65
	tokenReserve  = 24
)

// MaxRunesForTokens is a conservative character budget for a token window.
func MaxRunesForTokens(maxTokens int) int {
	if maxTokens <= 0 {
		return 0
	}
	if maxTokens <= tokenReserve+8 {
		return 24
	}
	n := int(float64(maxTokens-tokenReserve) / tokensPerRune)
	if n < 32 {
		n = 32
	}
	return n
}

// TruncateRunes cuts s to at most maxRunes runes.
func TruncateRunes(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return s
	}
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes])
}

// CapToTokens shrinks Size so character-based chunkers stay inside a token window.
func (c Config) CapToTokens(maxTokens int) Config {
	c = c.withDefaults()
	if maxTokens <= 0 {
		return c
	}
	maxRunes := MaxRunesForTokens(maxTokens)
	if c.Size > maxRunes {
		c.Size = maxRunes
	}
	if c.Overlap >= c.Size {
		c.Overlap = c.Size / 8
	}
	return c
}

// CapToWordTokens shrinks Size for the whitespace token chunker.
func (c Config) CapToWordTokens(maxTokens int) Config {
	c = c.withDefaults()
	if maxTokens <= 0 {
		return c
	}
	budget := maxTokens - tokenReserve
	if budget < 16 {
		budget = 16
	}
	if c.Size > budget {
		c.Size = budget
	}
	if c.Overlap >= c.Size {
		c.Overlap = c.Size / 8
	}
	return c
}

// Fit re-splits chunks that still exceed a token window, including unsplittable
// Markdown fences or long sentences the primary chunker left intact.
func Fit(chunks []document.Chunk, maxTokens int) []document.Chunk {
	if maxTokens <= 0 || len(chunks) == 0 {
		return chunks
	}
	maxRunes := MaxRunesForTokens(maxTokens)
	out := make([]document.Chunk, 0, len(chunks))
	for _, ch := range chunks {
		text := strings.TrimSpace(ch.Content)
		if text == "" {
			continue
		}
		if utf8.RuneCountInString(text) <= maxRunes {
			if text != ch.Content {
				ch.Content = text
			}
			out = append(out, ch)
			continue
		}
		prefix := rowIdentityPrefix(text)
		for i, part := range splitForFit(text, maxRunes) {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			if i > 0 {
				part = withRowPrefix(prefix, part, maxRunes)
			}
			next := ch
			next.Content = part
			next.Metadata = document.CloneMetadata(ch.Metadata)
			if i > 0 {
				next.ID = document.NewID()
			}
			out = append(out, next)
		}
	}
	for i := range out {
		out[i].Index = i
	}
	return out
}

func splitForFit(text string, maxRunes int) []string {
	if !strings.Contains(text, "\n") {
		return splitRunes(text, maxRunes)
	}
	lines := strings.Split(text, "\n")
	var (
		out []string
		b   strings.Builder
		n   int
	)
	flush := func() {
		s := strings.TrimSpace(b.String())
		if s != "" {
			out = append(out, s)
		}
		b.Reset()
		n = 0
	}
	for _, line := range lines {
		need := utf8.RuneCountInString(line)
		if need > maxRunes {
			flush()
			out = append(out, splitRunes(line, maxRunes)...)
			continue
		}
		if n > 0 && n+1+need > maxRunes {
			flush()
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
			n++
		}
		b.WriteString(line)
		n += need
	}
	flush()
	if len(out) == 0 {
		return splitRunes(text, maxRunes)
	}
	return out
}

func rowIdentityPrefix(text string) string {
	var lines []string
	for _, line := range strings.Split(text, "\n") {
		trim := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trim, "Sheet:"), strings.HasPrefix(trim, "Row:"):
			lines = append(lines, trim)
		default:
			return strings.Join(lines, "\n")
		}
	}
	return strings.Join(lines, "\n")
}

func withRowPrefix(prefix, part string, maxRunes int) string {
	if prefix == "" || strings.Contains(part, prefix) {
		return part
	}
	joined := prefix + "\n" + part
	if utf8.RuneCountInString(joined) <= maxRunes {
		return joined
	}
	return part
}
