package document

import "testing"

func TestNormalizeFillsMissingFields(t *testing.T) {
	doc := Document{Content: "hello world"}.Normalize()
	if doc.ID == "" {
		t.Fatal("expected generated id")
	}
	if doc.Namespace != DefaultNamespace {
		t.Fatalf("namespace = %q", doc.Namespace)
	}
	if doc.Checksum == "" {
		t.Fatal("expected checksum")
	}
	if Checksum("hello world") != doc.Checksum {
		t.Fatal("checksum mismatch")
	}
}

func TestInheritCopiesMetadata(t *testing.T) {
	doc := Document{
		ID:        "doc-1",
		Source:    "readme.md",
		Namespace: "docs",
		Metadata:  map[string]any{"lang": "en"},
	}
	chunk := doc.Inherit(0, "body")
	if chunk.DocumentID != "doc-1" {
		t.Fatalf("document id = %q", chunk.DocumentID)
	}
	if chunk.Namespace != "docs" {
		t.Fatalf("namespace = %q", chunk.Namespace)
	}
	if chunk.Metadata["lang"] != "en" {
		t.Fatalf("metadata not inherited: %#v", chunk.Metadata)
	}
	if chunk.Metadata["source"] != "readme.md" {
		t.Fatalf("source not copied: %#v", chunk.Metadata)
	}
	doc.Metadata["lang"] = "tr"
	if chunk.Metadata["lang"] != "en" {
		t.Fatal("chunk metadata must be a copy")
	}
}

func TestChecksumStable(t *testing.T) {
	if Checksum("a") == Checksum("b") {
		t.Fatal("different content must not share a checksum")
	}
	if Checksum("a") != Checksum("a") {
		t.Fatal("checksum must be stable")
	}
}
