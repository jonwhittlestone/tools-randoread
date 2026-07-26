.PHONY: run build test test-cover fmt lint clean docker-build docker-up docker-down deploy health auth daily rando clipped dropbox-status build-send-to-remarkable send-to-remarkable

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

daily:
	curl -s -H "X-Auth-Token: $(TOKEN)" "http://localhost:$(PORT)/api/daily" | jq .

rando:
	curl -s -H "X-Auth-Token: $(TOKEN)" "http://localhost:$(PORT)/api/rando" | jq .

clipped:
	curl -s -H "X-Auth-Token: $(TOKEN)" "http://localhost:$(PORT)/api/clipped" | jq .

dropbox-status:
	curl -s -H "X-Auth-Token: $(TOKEN)" "http://localhost:$(PORT)/api/dropbox/status" | jq .

## ── send-to-remarkable CLI (not part of the web server) ────────────────────

build-send-to-remarkable:
	@mkdir -p bin
	go build -o bin/send-to-remarkable ./cmd/send-to-remarkable
	@echo "Binary written to bin/send-to-remarkable"

# usage: make send-to-remarkable EPUB=path/to/book.epub REMARKABLE_PASSWORD=...
send-to-remarkable: build-send-to-remarkable
	REMARKABLE_PASSWORD=$(REMARKABLE_PASSWORD) ./bin/send-to-remarkable $(EPUB)

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
