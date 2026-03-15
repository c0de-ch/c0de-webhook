# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

c0de-webhook is a Go multi-channel messaging gateway. It accepts POST requests and delivers via Email (SMTP), WhatsApp Business API, or Telegram Bot API. Includes a web admin UI for API token management, message queue, per-token statistics, and editable channel settings.

## Build & Run

```bash
# Build
go build -o c0de-webhook .

# Build test client
go build -o webhook-test ./cmd/webhook-test/

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
POST /api/v2/mail     (Bearer token) → SQLite queue → Queue workers → SMTP
POST /api/v2/whatsapp (Bearer token) → SQLite queue → Queue workers → WhatsApp Business API
POST /api/v2/telegram (Bearer token) → SQLite queue → Queue workers → Telegram Bot API
POST /api/v1/send     (Bearer token) → (legacy, routes to mail)
Browser (cookie session)             → Web UI (dashboard, tokens, queue, settings)
```

**Packages:**
- `internal/config` — YAML config + env var overrides
- `internal/store` — SQLite database (tokens, messages, stats, settings)
- `internal/auth` — Bearer token validation (API) + cookie sessions (UI) + password management
- `internal/webhook` — v2 API endpoints (mail, whatsapp, telegram) + v1 legacy
- `internal/mail` — SMTP sender with PLAIN/LOGIN/CRAM-MD5 auth, STARTTLS/TLS
- `internal/whatsapp` — WhatsApp Business API sender (Meta Cloud API)
- `internal/telegram` — Telegram Bot API sender
- `internal/queue` — Background workers that poll DB and dispatch to channel senders
- `internal/ui` — Web UI routes + template rendering
- `web/` — Embedded templates and static CSS (via go:embed)
- `cmd/webhook-test/` — CLI tool for testing the API

**Key interfaces:**
- `mail.Sender` — interface for sending emails, enables mock injection in tests
- `queue.ChannelSender` — struct holding senders for all channels (Mail, WhatsApp, Telegram)

**Database:** SQLite via `modernc.org/sqlite` (pure Go, no CGO). In-memory databases use `cache=shared` for connection pool compatibility. Settings table stores channel config from admin UI.

## Testing

Coverage target: 85%+ on `./internal/...`. Queue tests use polling (2s dispatcher interval) so they take ~30s. Use `-timeout 120s` for the queue package.

## Config

YAML config file + environment variable overrides (WEBHOOK_* prefix) + admin UI settings (stored in SQLite). See `config.example.yaml`. Admin UI settings override config file values; restart required to apply.

## CI/CD

GitHub Actions workflows in `.github/workflows/`:
- `ci.yaml` — lint (golangci-lint) + test (-race) + coverage gate (80%) + build on every push to main
- `release.yaml` — build binaries (linux/amd64, linux/arm64) + Docker multi-arch image to ghcr.io on tag `v*`

Docker image: `ghcr.io/c0de-ch/c0de-webhook`
