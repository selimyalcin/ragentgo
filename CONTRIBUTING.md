# Contributing to RAGentGo

Thanks for taking the time. This repository is a Go library first and an HTTP service second. Changes that keep the public packages small and composable are easier to review than new framework layers.

## What to work on

Good first contributions:

- tests around chunking, retrieval ranking, and namespace isolation
- loader coverage for awkward real-world files
- clearer errors from providers
- examples that compile

Please open an issue before a large redesign.

## Setup

You need Go 1.25+, Docker (for Postgres), and optionally an OpenAI-compatible API on `http://localhost:8000/v1`.

```bash
docker compose up -d postgres
go test ./...
go test -race ./...
go vet ./...
```

Config is loaded from `configs/config.<environment>.yaml`. Pass `-env local` when you run the server.

## Style

- `gofmt` is required
- exported types and functions need a full sentence of godoc
- wrap errors with `%w`
- every network or database method takes `context.Context`
- do not log API keys
- do not add a dependency unless an existing package cannot do the job

Public interfaces live at package boundaries (`embedding.Embedder`, `store.VectorStore`, `llm.Generator`). Keep structs concrete inside a package when an interface would only have one implementation.

## Pull requests

Use the template in `.github/PULL_REQUEST_TEMPLATE.md`. Include the test commands you ran. If you change HTTP behavior, add or update the Postman collection under `postman/`.

## License

By contributing you agree that your work is licensed under GPL-3.0, the same license as the rest of the project.
