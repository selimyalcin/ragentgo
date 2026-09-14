package loader

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestJSONAndHTML(t *testing.T) {
	dir := t.TempDir()
	jsonPath := filepath.Join(dir, "docs.json")
	if err := os.WriteFile(jsonPath, []byte(`[{"content":"hello json"},{"text":"second"}]`), 0o644); err != nil {
		t.Fatal(err)
	}
	docs, err := JSON(Options{}).Load(context.Background(), jsonPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 2 || docs[0].Content != "hello json" {
		t.Fatalf("%#v", docs)
	}

	htmlPath := filepath.Join(dir, "page.html")
	if err := os.WriteFile(htmlPath, []byte(`<html><head><script>nope()</script></head><body><p>Visible</p></body></html>`), 0o644); err != nil {
		t.Fatal(err)
	}
	docs, err = HTML(Options{}).Load(context.Background(), htmlPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 1 || !strings.Contains(docs[0].Content, "Visible") || strings.Contains(docs[0].Content, "nope") {
		t.Fatalf("%#v", docs)
	}
}

func TestWebRejectsLoopback(t *testing.T) {
	_, err := Web(Options{}).Load(context.Background(), "http://127.0.0.1/secret")
	if err == nil {
		t.Fatal("expected private address rejection")
	}
}
