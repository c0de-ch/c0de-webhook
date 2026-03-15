# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

c0de-webhook is a Go webhook-to-email gateway. It accepts POST requests with `{to, subject, text, html}` and forwards them via SMTP. Includes a web admin UI for API token management, message queue, and statistics.

## Build & Run

```bash
# Build
go build -o c0de-webhook .

# Run (requires config file)
./c0de-webhook -config config.example.yaml

# Run tests
go test ./internal/...

# Run tests with coverage
go test -coverprofile=coverage.out ./internal/...
go tool cover -func=coverage.out

# Run a single package's tests
go test -v ./internal/store/
go test -v -run TestWorkerProcessesMessages ./internal/queue/

# Lint (requires golangci-lint)
golangci-lint run

# Docker build
docker build -t c0de-webhook .
```

Note: Go is at `/usr/local/go/bin/go`. Race detector (`-race`) requires CGO_ENABLED=1.

## Architecture

```
POST /api/v1/send (Bearer token) → SQLite queue → Queue workers → SMTP
Browser (cookie session)         → Web UI (dashboard, tokens, queue, settings)
```

**Packages:**
- `internal/config` — YAML config + env var overrides
- `internal/store` — SQLite database (tokens, messages, stats)
- `internal/auth` — Bearer token validation (API) + cookie sessions (UI)
- `internal/webhook` — POST /api/v1/send endpoint
- `internal/mail` — SMTP sender with PLAIN/LOGIN/CRAM-MD5 auth, STARTTLS/TLS
- `internal/queue` — Background workers that poll DB and send via SMTP
- `internal/ui` — Web UI routes + template rendering
- `web/` — Embedded templates and static CSS (via go:embed)

**Key interfaces:**
- `mail.Sender` — interface for sending emails, enables mock injection in tests

**Database:** SQLite via `modernc.org/sqlite` (pure Go, no CGO). In-memory databases use `cache=shared` for connection pool compatibility.

## Testing

Coverage target: 90%+ on `./internal/...`. Queue tests use polling (2s dispatcher interval) so they take ~30s. Use `-timeout 120s` for the queue package.

## Config

YAML config file + environment variable overrides (WEBHOOK_* prefix). See `config.example.yaml`.
