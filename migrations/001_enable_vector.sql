-- Enable pgvector. The Go store also runs this on startup and creates
-- rag_documents / rag_chunks with the configured embedding dimension.
CREATE EXTENSION IF NOT EXISTS vector;
