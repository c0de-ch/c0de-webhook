# c0de-webhook

A self-hosted multi-channel messaging gateway. Accepts webhook POST requests and delivers via **Email (SMTP)**, **WhatsApp**, or **Telegram**.

Features:
- **Multi-channel** — Email, WhatsApp Business API, Telegram Bot API
- **v2 REST API** — `/api/v2/mail`, `/api/v2/whatsapp`, `/api/v2/telegram`
- **Web Admin UI** — manage API tokens, view queue, per-token statistics, editable channel settings, password management
- **Queue with retry** — automatic retries with exponential backoff
- **SMTP support** — PLAIN, LOGIN (M365), CRAM-MD5 auth; STARTTLS, implicit TLS, plain
- **Config file + env vars + UI** — YAML config, environment overrides, admin UI for channel settings
- **Single binary** — templates and static assets embedded, no external files needed

## Quick Start

### Option 1: Binary

```bash
# Download from GitHub Releases or build from source
go build -o c0de-webhook .

# Copy and edit the config
cp config.example.yaml config.yaml
# Edit config.yaml — at minimum set smtp and admin_password

# Run
./c0de-webhook -config config.yaml
```

Open `http://localhost:8090`, log in with your admin password, create an API token, and start sending.

### Option 2: Docker

```bash
docker run -d \
  --name c0de-webhook \
  -p 8090:8090 \
  -v $(pwd)/config.yaml:/etc/c0de-webhook/config.yaml:ro \
  -v webhook-data:/data \
  ghcr.io/c0de-ch/c0de-webhook:latest
```

### Option 3: Docker Compose

```yaml
services:
  webhook:
    image: ghcr.io/c0de-ch/c0de-webhook:latest
    ports:
      - "8090:8090"
    volumes:
      - ./config.yaml:/etc/c0de-webhook/config.yaml:ro
      - webhook-data:/data
    restart: unless-stopped

volumes:
  webhook-data:
```

Docker images are published to `ghcr.io/c0de-ch/c0de-webhook` on every tagged release (multi-arch: linux/amd64, linux/arm64).

## API

### v2 Endpoints

All endpoints require `Authorization: Bearer <token>` and `Content-Type: application/json`.

#### Send Email

```
POST /api/v2/mail
```
```json
{"to": "user@example.com", "subject": "Hello", "text": "Plain text", "html": "<h1>HTML</h1>"}
```
- `to` — recipient (comma-separated for multiple)
- `subject` — required
- `text` / `html` — at least one required

#### Send WhatsApp Message

```
POST /api/v2/whatsapp
```
```json
{"phone": "41791234567", "text": "Hello from webhook!"}
```
- `phone` — international format without `+`
- Requires WhatsApp Business API credentials (configure in Settings)

#### Send Telegram Message

```
POST /api/v2/telegram
```
```json
{"chat_id": "123456789", "text": "Hello from webhook!"}
```
- Requires Telegram Bot Token (configure in Settings)

#### Response (all channels)

```json
{"id": 42, "status": "queued", "channel": "mail"}
```

#### Health Check

```
GET /api/v2/health
```
Returns `{"status":"ok"}` — no auth required. Use for K8s probes.

### v1 Endpoints (legacy)

| Endpoint | Description |
|----------|-------------|
| `POST /api/v1/send` | Send email (same as `/api/v2/mail`) |
| `GET /api/v1/health` | Health check |

### Test Client

A CLI tool is included for testing the API:

```bash
# Build
go build -o webhook-test ./cmd/webhook-test/

# Health check
./webhook-test -url http://localhost:8090 -health

# Send email
./webhook-test -url http://localhost:8090 -token YOUR_TOKEN -to user@example.com

# Send multiple test messages
./webhook-test -url http://localhost:8090 -token YOUR_TOKEN -to user@example.com -n 5

# Custom subject and body
./webhook-test -url http://localhost:8090 -token YOUR_TOKEN -to user@example.com \
  -subject "Test" -text "Hello world"
```

## Admin UI

The web admin interface is available at the server's root URL (default `http://localhost:8090`).

- **Dashboard** — message stats, hourly activity chart, per-token usage breakdown, recent messages
- **API Tokens** — create/disable/delete tokens, click token name for per-token detail with message history
- **Queue** — browse all messages with status filter (queued/sent/failed), retry or delete
- **Settings** — editable SMTP, WhatsApp Business, and Telegram config; admin password change; API endpoint reference

Channel settings configured in the UI are stored in the database and override the config file. Restart the server after changing channel settings.

## Configuration

Configuration is loaded in order (later overrides earlier):
1. Built-in defaults
2. YAML config file (`-config` flag or `WEBHOOK_CONFIG` env var)
3. Environment variables
4. Admin UI settings (stored in database)

### Config File

See [config.example.yaml](config.example.yaml) for all options.

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `WEBHOOK_CONFIG` | Path to config file | _(none)_ |
| `WEBHOOK_SERVER_HOST` | Listen address | `0.0.0.0` |
| `WEBHOOK_SERVER_PORT` | Listen port | `8090` |
| `WEBHOOK_ADMIN_PASSWORD` | Admin UI password | `changeme` |
| `WEBHOOK_SECRET_KEY` | Session signing key | _(random)_ |
| `WEBHOOK_SMTP_HOST` | SMTP server host | `localhost` |
| `WEBHOOK_SMTP_PORT` | SMTP server port | `25` |
| `WEBHOOK_SMTP_USERNAME` | SMTP username | _(empty)_ |
| `WEBHOOK_SMTP_PASSWORD` | SMTP password | _(empty)_ |
| `WEBHOOK_SMTP_FROM` | From address | `noreply@example.com` |
| `WEBHOOK_SMTP_TLS` | Enable STARTTLS | `false` |
| `WEBHOOK_SMTP_AUTH_METHOD` | Auth: `plain`, `login`, `crammd5`, `none` | `none` |
| `WEBHOOK_DB_PATH` | SQLite database path | `./data/webhook.db` |
| `WEBHOOK_QUEUE_WORKERS` | Number of queue workers | `2` |
| `WEBHOOK_QUEUE_MAX_RETRIES` | Max send retries | `3` |
| `WEBHOOK_QUEUE_RETRY_DELAY` | Base retry delay | `30s` |

### Channel Setup

#### Email (SMTP)

Configure via config file, environment variables, or the admin UI Settings page.

**Microsoft 365:**
```yaml
smtp:
  host: "smtp.office365.com"
  port: 587
  username: "user@yourdomain.com"
  password: "your-password"
  from: "noreply@yourdomain.com"
  tls: true
  auth_method: "login"
```

**Postfix (local):**
```yaml
smtp:
  host: "localhost"
  port: 25
  from: "noreply@yourdomain.com"
  auth_method: "none"
```

**Gmail (App Password):**
```yaml
smtp:
  host: "smtp.gmail.com"
  port: 587
  username: "you@gmail.com"
  password: "your-app-password"
  from: "you@gmail.com"
  tls: true
  auth_method: "plain"
```

#### WhatsApp Business API

1. Create a Meta Business account at [developers.facebook.com](https://developers.facebook.com)
2. Set up a WhatsApp Business app and get a **Phone Number ID** and **Access Token**
3. Enter credentials in the admin UI under **Settings > WhatsApp Business API**

#### Telegram Bot API

1. Create a bot via [@BotFather](https://t.me/BotFather) on Telegram
2. Copy the **Bot Token**
3. Enter it in the admin UI under **Settings > Telegram Bot API**
4. Get the `chat_id` by messaging your bot and checking `https://api.telegram.org/bot<TOKEN>/getUpdates`

## Production Deployment

### Linux (systemd)

```bash
sudo useradd -r -s /bin/false webhook
sudo mkdir -p /etc/c0de-webhook /var/lib/c0de-webhook
sudo cp config.yaml /etc/c0de-webhook/
sudo chown -R webhook:webhook /var/lib/c0de-webhook
sudo cp c0de-webhook /usr/local/bin/

sudo tee /etc/systemd/system/c0de-webhook.service <<EOF
[Unit]
Description=c0de-webhook
After=network.target

[Service]
Type=simple
User=webhook
ExecStart=/usr/local/bin/c0de-webhook -config /etc/c0de-webhook/config.yaml
Restart=always
RestartSec=5
Environment=WEBHOOK_DB_PATH=/var/lib/c0de-webhook/webhook.db

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl daemon-reload
sudo systemctl enable --now c0de-webhook
```

### Kubernetes

```bash
vim k8s/secret.yaml    # set passwords
vim k8s/configmap.yaml # set config
kubectl apply -f k8s/
```

Manifests included:
- `k8s/deployment.yaml` — Deployment with health probes and resource limits
- `k8s/service.yaml` — ClusterIP Service
- `k8s/configmap.yaml` — Non-sensitive config
- `k8s/secret.yaml` — Passwords and keys
- `k8s/pvc.yaml` — Persistent volume for SQLite

### Docker Image

Published to GitHub Container Registry on every tagged release:

```bash
docker pull ghcr.io/c0de-ch/c0de-webhook:latest
docker pull ghcr.io/c0de-ch/c0de-webhook:v1.0.0
```

Multi-arch: `linux/amd64` and `linux/arm64`.

### Security Checklist

- [ ] Change `admin_password` and `secret_key` from defaults
- [ ] Use HTTPS via reverse proxy (nginx, Caddy, Traefik)
- [ ] Restrict network access to the admin UI
- [ ] Store credentials in K8s Secrets or a secrets manager
- [ ] Set resource limits in production

## Development

```bash
# Build
go build -o c0de-webhook .

# Build test client
go build -o webhook-test ./cmd/webhook-test/

# Run tests
go test ./internal/...

# Run tests with coverage
go test -coverprofile=coverage.out ./internal/...
go tool cover -html=coverage.out -o coverage.html

# Lint (requires golangci-lint)
golangci-lint run

# Run locally
./c0de-webhook -config config.example.yaml
```

## License

MIT
