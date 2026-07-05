.PHONY: run build test test-cover fmt lint clean docker-build docker-up docker-down deploy health

BINARY=bin/randoread
PORT?=8080

## ── Local development ──────────────────────────────────────────────────────

run:
	@echo "Starting randoread on :$(PORT)..."
	@AUTH_TOKEN=$${AUTH_TOKEN:-devtoken} AUTH_TOKEN_ISSUED_AT=$${AUTH_TOKEN_ISSUED_AT:-2026-01-01T00:00:00Z} PORT=$(PORT) go run .

# NOTE: this repo lives in a Dropbox-synced folder on Jon's machines. Avoid
# running `make build` from a Dropbox-synced checkout — the compiled binary
# would get synced across machines. Fine to run from a plain clone (CI, or
# the rsynced copy on doylestonex).
build:
	@mkdir -p bin
	go build -o $(BINARY) .
	@echo "Binary written to $(BINARY)"

test:
	go test ./... -v

test-cover:
	go test ./... -cover

fmt:
	gofmt -w .

lint:
	staticcheck ./...

clean:
	rm -rf bin/

## ── Smoke tests (server must be running) ───────────────────────────────────

TOKEN?=devtoken

health:
	curl -s http://localhost:$(PORT)/health | jq .

auth:
	curl -s "http://localhost:$(PORT)/api/auth?token=$(TOKEN)" | jq .

## ── Docker ─────────────────────────────────────────────────────────────────

docker-build:
	docker-compose build

docker-up:
	docker-compose up -d
	@sleep 3
	@curl -s http://localhost:8085/health | jq .

docker-down:
	docker-compose down

## ── Deploy to doylestonex ──────────────────────────────────────────────────

deploy:
	@bash deploy/deploy.sh
