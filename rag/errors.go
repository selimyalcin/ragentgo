package rag

import "errors"

var (
	// ErrNoDocuments means retrieval returned nothing the pipeline can use.
	ErrNoDocuments = errors.New("rag: no documents found")

	// ErrEmptyQuery is returned when the caller asks an empty question.
	ErrEmptyQuery = errors.New("rag: query is empty")

	// ErrEmptyDocument is returned when indexing a document without content.
	ErrEmptyDocument = errors.New("rag: document content is empty")

	// ErrDimensionMismatch means a stored vector does not match the embedder.
	ErrDimensionMismatch = errors.New("rag: embedding dimension mismatch")

	// ErrMissingEmbedder means the pipeline was built without an embedder.
	ErrMissingEmbedder = errors.New("rag: embedder is required")

	// ErrMissingStore means the pipeline was built without a vector store.
	ErrMissingStore = errors.New("rag: vector store is required")

	// ErrMissingGenerator means Query was called without an LLM.
	ErrMissingGenerator = errors.New("rag: generator is required")
)
