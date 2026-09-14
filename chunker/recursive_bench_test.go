package chunker

import (
	"context"
	"strings"
	"testing"

	"github.com/selimyalcin/ragentgo/document"
)

func BenchmarkRecursiveChunker(b *testing.B) {
	text := strings.Repeat("Go is a compiled programming language used for backend services. ", 400)
	c := NewRecursive(Config{Size: 800, Overlap: 120})
	doc := document.Document{Content: text}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := c.Chunk(context.Background(), doc); err != nil {
			b.Fatal(err)
		}
	}
}
