# Multi-stage build: compile to a single static Go binary, then run it in a
# minimal image. No runtime dependencies beyond the binary itself.

FROM golang:1.22-alpine AS builder

WORKDIR /app

COPY go.mod go.sum* ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o randoread .

FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends curl ca-certificates && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY --from=builder /app/randoread .

ENV PORT=8080

EXPOSE 8080

CMD ["./randoread"]
