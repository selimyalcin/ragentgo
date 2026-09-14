.PHONY: test race vet lint tidy run docker-postgres

test:
	go test ./...

race:
	go test -race ./...

vet:
	go vet ./...

tidy:
	go mod tidy

run:
	go run ./cmd/server -env local

cli:
	go run ./cmd/ragentgo -env local status

docker-postgres:
	docker compose up -d postgres

bench:
	go test ./chunker ./store/memory -bench=. -benchmem
