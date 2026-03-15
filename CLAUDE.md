# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

c0de-webhook is a Go multi-channel messaging gateway. It accepts POST requests and delivers via Email (SMTP), WhatsApp (Business API + Web/whatsmeow), or Telegram Bot API. Includes a web admin UI for API token management, message queue, per-token statistics, and editable channel settings.

## Pre-Push Checklist (MANDATORY)

**ALWAYS run these 3 steps before every `git push`. Do NOT push if any fail.**

```bash
# 1. Lint (must show 0 issues)
CGO_ENABLED=1 golangci-lint run

# 2. Build (must succeed)
CGO_ENABLED=1 go build -o c0de-webhook .

# 3. Coverage (must be >= 70%)
CGO_ENABLED=1 go test -coverprofile=coverage.out -covermode=atomic ./internal/...
go tool cover -func=coverage.out | grep total
```

If lint fails, fix all issues before pushing. If coverage drops below 70%, add tests for new code before pushing.

## Build & Run

```bash
# Build (CGO required for whatsmeow/go-sqlite3)
CGO_ENABLED=1 go build -o c0de-webhook .

# Build test client
go build -o webhook-test ./cmd/webhook-test/

# Run (requires config file)
./c0de-webhook -config config.example.yaml

# Run tests
CGO_ENABLED=1 go test ./internal/...

# Run tests with coverage
CGO_ENABLED=1 go test -coverprofile=coverage.out ./internal/...
go tool cover -func=coverage.out

# Run a single package's tests
CGO_ENABLED=1 go test -v ./internal/store/
CGO_ENABLED=1 go test -v -run TestWorkerProcessesMessages ./internal/queue/

# Lint (requires golangci-lint v2)
CGO_ENABLED=1 golangci-lint run

# Docker build
docker build -t c0de-webhook .
```

Note: Go is at `/usr/local/go/bin/go`. CGO_ENABLED=1 is required (whatsmeow uses mattn/go-sqlite3). The SSH key for pushing is `~/.ssh/github-c0de_id_ed25519` via the `github-c0de` SSH host alias.

## Architecture

```
POST /api/v2/mail     (Bearer token) → SQLite queue → Queue workers → SMTP
POST /api/v2/whatsapp (Bearer token) → SQLite queue → Queue workers → WhatsApp Web or Business API
POST /api/v2/telegram (Bearer token) → SQLite queue → Queue workers → Telegram Bot API
POST /api/v1/send     (Bearer token) → (legacy, routes to mail)
Browser (cookie session)             → Web UI (dashboard, tokens, queue, settings)
```

**Packages:**
- `internal/config` — YAML config + env var overrides
- `internal/store` — SQLite database (tokens, messages, stats, settings)
- `internal/auth` — Bearer token validation (API) + cookie sessions (UI) + password management
- `internal/webhook` — v2 API endpoints (mail, whatsapp, telegram) + v1 legacy
- `internal/mail` — SMTP sender with PLAIN/LOGIN/CRAM-MD5 auth, STARTTLS/TLS, skip-verify
- `internal/whatsapp` — WhatsApp Business API sender + Web client (whatsmeow)
- `internal/telegram` — Telegram Bot API sender
- `internal/queue` — Background workers that poll DB and dispatch to channel senders
- `internal/ui` — Web UI routes + template rendering + live JSON endpoints
- `web/` — Embedded templates and static CSS (via go:embed)
- `cmd/webhook-test/` — CLI tool for testing the API

**Key interfaces:**
- `mail.Sender` — interface for sending emails, enables mock injection in tests
- `queue.ChannelSender` — struct holding senders for all channels (Mail, WhatsApp, WhatsAppWeb, Telegram)
- `ui.OnSettingsSaved` — callback for hot-reloading senders when settings change

**Database:** SQLite via `modernc.org/sqlite` (pure Go) for app data + `mattn/go-sqlite3` (CGO) for whatsmeow session. In-memory databases use `cache=shared` for connection pool compatibility.

## Testing

Coverage target: 80%+ on testable packages (CI gate). whatsapp/web.go (whatsmeow) is excluded from coverage measurement since it requires a real WhatsApp connection. Queue tests use polling (2s dispatcher interval) so they take ~30s. Use `-timeout 120s` for the queue package. whatsapp/web.go has low coverage because whatsmeow requires a real WhatsApp connection.

## Lint

Uses golangci-lint v2 with config in `.golangci.yml`. Key settings:
- errcheck: Close() calls excluded (standard Go pattern)
- staticcheck: QF* and ST1000 suppressed
- govet: hostport check disabled

## Config

YAML config file + environment variable overrides (WEBHOOK_* prefix) + admin UI settings (stored in SQLite). See `config.example.yaml`. Admin UI settings override config file values and apply immediately (hot-reload via worker.UpdateSenders).

## CI/CD

GitHub Actions workflows in `.github/workflows/`:
- `ci.yaml` — Go 1.25, golangci-lint v7, CGO_ENABLED=1, test + coverage gate (80%) + build
- `release.yaml` — CGO build for linux/amd64+arm64, Docker multi-arch image to ghcr.io on tag `v*`

Docker image: `ghcr.io/c0de-ch/c0de-webhook`
Git remote uses SSH: `git@github-c0de:c0de-ch/c0de-webhook.git`
