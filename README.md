# RAGentGo

Go library (and a small HTTP server) that indexes documents and answers questions from them.

You plug in an embedder, a vector store, and a chat model. RAGentGo chunks the files, stores the vectors, retrieves the matching rows or passages, and asks the model to answer with citations. It is meant to live inside a Go service — not to be another Python RAG framework.

Postgres + [pgvector](https://github.com/pgvector/pgvector) is the default store so embeddings and metadata sit in the same database. There is no LangChain dependency.

```mermaid
flowchart LR
    D[Files] --> L[Loader]
    L --> C[Chunker]
    C --> E[Embedder]
    E --> V[pgvector]

    Q[Question] --> R[Hybrid search]
    V --> R
    R --> T[Optional Go totals]
    T --> P[Prompt]
    P --> LLM[Chat model]
    LLM --> A[Answer]
```

## What it does well

- Spreadsheets: `.xlsx` sheets become one document per data row (`Header: value`), which is what you want for payroll, invoices, and ledgers.
- Totals in Go: questions like “Nisan ayında ödenen toplam brüt maaş?” gather matching rows and sum them in code. The chat model is told the result so it does not add a random subset of `top_k` hits.
- Hybrid search: cosine similarity plus Postgres full-text, fused with RRF. Keyword search uses `simple` FTS and drops question words (`kim`, `ne`, `toplam`, …).
- Small embedding windows: MiniLM-style models often cap at 256 tokens. Set `embedding.max_input_tokens`; the chunker splits labeled rows before `/embeddings` instead of dying with `exceed_context_size_error`.
- Answer cache: identical questions reuse the last answer until the corpus fingerprint changes (ingest, replace, delete). Conversations are not cached. Responses include `"cached": true`.
- Providers are separate: local MiniLM, Hugging Face embeddings, OpenAI, Ollama, or any OpenAI-compatible chat endpoint can be mixed. Embedding and LLM do not have to share a base URL.

## Install

```bash
go get github.com/selimyalcin/ragentgo@v0.1.0
```

Go **1.26** or later. Module path: `github.com/selimyalcin/ragentgo`.

## Five-minute start

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/selimyalcin/ragentgo/document"
	embedcompat "github.com/selimyalcin/ragentgo/embedding/openaicompat"
	llmcompat "github.com/selimyalcin/ragentgo/llm/openaicompat"
	"github.com/selimyalcin/ragentgo/rag"
	"github.com/selimyalcin/ragentgo/store/memory"
)

func main() {
	ctx := context.Background()

	pipe, err := rag.New(
		rag.WithEmbedder(embedcompat.NewEmbedder(embedcompat.Options{
			BaseURL: "http://localhost:8000/v1",
			Model:   "text-embedding-3-small",
		})),
		rag.WithVectorStore(memory.New()),
		rag.WithGenerator(llmcompat.NewGenerator(llmcompat.Options{
			BaseURL: "http://localhost:8000/v1",
			Model:   "gpt-4.1-mini",
		})),
	)
	if err != nil {
		log.Fatal(err)
	}

	err = pipe.IndexDocument(ctx, document.Document{
		Content: "RAGentGo indexes documents and answers questions with citations.",
		Source:  "readme.md",
	})
	if err != nil {
		log.Fatal(err)
	}

	result, err := pipe.Query(ctx, "What does this project do?", rag.QueryOptions{})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(result.Answer)
}
```

`examples/ollama` talks to a local Ollama daemon. Hugging Face embeddings use `embedding/huggingface` (see below) — not `/v1/embeddings`.

## HTTP API

The server is Fiber v2, under `/v1`. Listen address comes from config (`localhost:8080` in the sample files).

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/health` | liveness |
| GET | `/ready` | store is up |
| POST | `/v1/documents` | index JSON text |
| POST | `/v1/documents/batch` | index many documents |
| POST | `/v1/documents/file` | multipart upload |
| POST | `/v1/documents/url` | render one page in Chrome |
| POST | `/v1/crawl` | walk a public site in Chrome |
| GET | `/v1/documents` | list by namespace |
| GET | `/v1/documents/:id` | metadata |
| DELETE | `/v1/documents/:id` | delete document and chunks |
| POST | `/v1/search` | retrieve chunks only |
| POST | `/v1/query` | question answering |
| POST | `/v1/query/stream` | SSE tokens |

```bash
curl -s http://localhost:8080/v1/documents \
  -H 'content-type: application/json' \
  -d '{"content":"pgvector keeps embeddings next to metadata.","source":"db.md","namespace":"docs"}'

curl -s http://localhost:8080/v1/query \
  -H 'content-type: application/json' \
  -d '{"question":"Where are embeddings stored?","namespace":"docs","top_k":5}'
```

`POST /v1/documents/file` (and `ragentgo ingest`) reads `.xlsx`, `.xls`, `.docx`, `.doc`, `.pptx`, `.ppt`, `.pdf`, `.odt`, `.ods`, `.rtf`, `.csv`, `.json`, HTML, and plain text. Scanned PDFs have no OCR. Old binary `.xls` / `.doc` are best-effort; prefer `.xlsx` / `.docx`. Re-uploading the same filename replaces the previous rows for that source.

Site ingest uses a real Chrome session (chromedp), not a bare HTTP GET. Private networks are blocked unless you turn that off. The API Docker image already includes Chromium.

A Postman collection lives in [`postman/RAGentGo.postman_collection.json`](postman/RAGentGo.postman_collection.json).

## CLI

```bash
go run ./cmd/ragentgo -env local ingest ./docs
go run ./cmd/ragentgo -env local query "How does authentication work?"
go run ./cmd/ragentgo -env local search "authentication middleware"
go run ./cmd/ragentgo -env local status
go run ./cmd/ragentgo serve -env local
```

## Config

YAML under `configs/`, selected by `-env`:

| Flag | File |
|------|------|
| `-env local` | `configs/config.local.yaml` |
| `-env development` | `configs/config.development.yaml` |
| `-env main` | `configs/config.main.yaml` |

Environment variables override YAML. Nested fields are not stomped by the shared `ai_api_url` once `embedding.base_url` or `llm.base_url` is set, so chat and embeddings can point at different hosts.

```text
RAGENTGO_DATABASE_URL=postgres://ragentgo:ragentgo@localhost:5433/ragentgo?sslmode=disable
RAGENTGO_EMBEDDING_PROVIDER=huggingface
RAGENTGO_EMBEDDING_BASE_URL=https://router.huggingface.co/hf-inference
RAGENTGO_EMBEDDING_MODEL=intfloat/multilingual-e5-small
RAGENTGO_EMBEDDING_DIMENSION=384
RAGENTGO_EMBEDDING_MAX_INPUT_TOKENS=512
RAGENTGO_LLM_BASE_URL=https://router.huggingface.co/v1
RAGENTGO_LLM_MODEL=deepseek-ai/DeepSeek-V4.1-Flash:novita
RAGENTGO_LLM_API_KEY=hf_...
RAGENTGO_RETRIEVAL_HYBRID=true
RAGENTGO_CACHE_ANSWER_TTL_SECONDS=3600
```

`cache.answer_ttl_seconds: -1` turns the answer cache off. Corpus changes still bust entries even before TTL.

Put tokens in env vars, not in committed YAML.

### Hugging Face

Chat uses the OpenAI-compatible router (`/v1/chat/completions`). **Feature extraction does not.** `POST https://router.huggingface.co/v1/embeddings` returns 404 for typical sentence-transformer models. Set:

```yaml
embedding:
  provider: huggingface
  model: intfloat/multilingual-e5-small
  base_url: https://router.huggingface.co/hf-inference
  dimension: 384
```

That hits `/models/<id>/pipeline/feature-extraction`. Keep `dimension` in sync with the model **before** the first Postgres start; the `VECTOR(n)` column is created then. Changing the embedding model later means re-ingesting the files — old vectors stay in the previous space even if the size matches.

Do not send a chat model (DeepSeek, Llama, …) to the embedder.

### OpenAI, Ollama, local gateways

OpenAI is the same `/v1` wire format with `https://api.openai.com/v1`. Ollama uses `/api/embed` and `/api/chat` (`examples/ollama`). Local llama.cpp / Jan / vLLM servers work as `openai-compat`.

HTTPS providers speak HTTP/2. Plain `http://localhost:8000` stays on HTTP/1.1 without keep-alives — those gateways often EOF on reused HTTP/2 sockets.

## Spreadsheets and totals

Excel is not dumped as one blob. After the header row, each data row becomes a labeled record (`Sheet`, `Row`, `Ay: Nisan`, `Brüt Maaş (TRY): 151000`, …). Metadata `unit: row` skips extra recursive chunking; `Fit` only splits if the row still overflows the embedder window.

When the question looks like a total, count, average, min, or max, retrieval pulls many more keyword hits, filters on labels such as month, and reduces the numeric column in Go. The prompt gets a `COMPUTED RESULT` block. That is why “32 April salaries → 3,217,000” does not collapse to a three-row hallucination.

Lookup questions (“Ahmet Yılmaz nisan ayında ne kadar maaş aldı?”) skip that path.

## Hybrid search

```go
result, err := pipe.Query(ctx, "How does authentication work?", rag.QueryOptions{
	TopK:      10,
	Hybrid:    true,
	Namespace: "docs",
	Filter: map[string]any{
		"language": "en",
		"category": "security",
	},
})
```

Filters are exact metadata matches (`metadata @> $filter::jsonb` on Postgres).

## Docker

Postgres is published on **host port 5433**.

```bash
docker compose up -d postgres
go run ./cmd/server -env local
```

API container (rebuild after YAML or Go changes — configs are copied into the image, not mounted):

```bash
docker compose up -d --build api
```

Compose `environment:` entries override YAML. If embeddings still hit `host.docker.internal:8000` after you edited the YAML, the compose file is winning.

## PostgreSQL

On first connect the store creates `rag_documents` and `rag_chunks` with an HNSW index (cosine by default).

```yaml
embedding:
  dimension: 384
vector:
  index: hnsw
  distance: cosine
```

## Packages

Import only what you need. Pulling in `rag` does not pull Fiber unless you import the server.

| Package | Role |
|---------|------|
| `rag` | index + query pipeline |
| `document` | shared types |
| `loader` | files, directories, URLs |
| `chunker` | splitters + token-window Fit |
| `tabular` | labeled-row totals |
| `embedding` | embedder interface + cache |
| `embedding/huggingface` | HF feature-extraction |
| `embedding/openaicompat` | `/v1/embeddings` |
| `llm/openaicompat` | `/v1/chat/completions` |
| `store/memory` | in-process vectors |
| `store/pgvector` | Postgres |
| `retrieval` | vector / hybrid search |
| `prompt` | grounded prompt + citations |
| `configs` | Viper YAML loader |

## Benchmarks

Run them locally. This README does not invent numbers.

```bash
go test ./chunker ./store/memory -bench=. -benchmem
```

## Status

Shipped: core pipeline, pgvector, HTTP + CLI, office loaders, hybrid search, spreadsheet row indexing, Go-side aggregates, answer cache, Hugging Face embeddings.

Still open: Qdrant as a first-class store, OpenTelemetry, multi-tenant hardening.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). `gofmt`, `go test ./...`, and `go vet ./...` before a PR.

## License

GPL-3.0. See [LICENSE](LICENSE).
