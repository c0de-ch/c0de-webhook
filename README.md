# c0de-webhook

A self-hosted webhook-to-email gateway. Accepts POST requests with `{to, subject, text, html}` and forwards them via SMTP to any mail server (M365, Postfix, Gmail, etc.).

Features:
- **Webhook API** — simple JSON endpoint with Bearer token auth
- **Web UI** — manage API tokens, view message queue, statistics dashboard
- **Queue with retry** — automatic retries with exponential backoff
- **SMTP support** — PLAIN, LOGIN (M365), CRAM-MD5 auth; STARTTLS, implicit TLS, plain
- **Config file + env vars** — YAML config with environment variable overrides
- **Single binary** — templates and static assets embedded, no external files needed

## Quick Start

### Option 1: Binary

```bash
# Download the latest release (or build from source)
go build -o c0de-webhook .

# Copy and edit the config
cp config.example.yaml config.yaml
# Edit config.yaml — at minimum set smtp and admin_password

# Run
./c0de-webhook -config config.yaml
```

Open `http://localhost:8080`, log in with your admin password, create an API token, and start sending:

```bash
curl -X POST http://localhost:8080/api/v1/send \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"to":"user@example.com","subject":"Hello","text":"Hello from c0de-webhook!"}'
```

### Option 2: Docker

```bash
# Create your config
cp config.example.yaml config.yaml
# Edit config.yaml

docker run -d \
  --name c0de-webhook \
  -p 8080:8080 \
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
      - "8080:8080"
    volumes:
      - ./config.yaml:/etc/c0de-webhook/config.yaml:ro
      - webhook-data:/data
    restart: unless-stopped

volumes:
  webhook-data:
```

## Configuration

Configuration is loaded in order (later overrides earlier):
1. Built-in defaults
2. YAML config file (`-config` flag or `WEBHOOK_CONFIG` env var)
3. Environment variables

### Config File

See [config.example.yaml](config.example.yaml) for all options.

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `WEBHOOK_CONFIG` | Path to config file | _(none)_ |
| `WEBHOOK_SERVER_HOST` | Listen address | `0.0.0.0` |
| `WEBHOOK_SERVER_PORT` | Listen port | `8080` |
| `WEBHOOK_ADMIN_PASSWORD` | Admin UI password | `changeme` |
| `WEBHOOK_SECRET_KEY` | Session signing key | _(random)_ |
| `WEBHOOK_SMTP_HOST` | SMTP server host | `localhost` |
| `WEBHOOK_SMTP_PORT` | SMTP server port | `25` |
| `WEBHOOK_SMTP_USERNAME` | SMTP username | _(empty)_ |
| `WEBHOOK_SMTP_PASSWORD` | SMTP password | _(empty)_ |
| `WEBHOOK_SMTP_FROM` | From address | `noreply@example.com` |
| `WEBHOOK_SMTP_TLS` | Enable STARTTLS | `false` |
| `WEBHOOK_SMTP_AUTH_METHOD` | Auth method: `plain`, `login`, `crammd5`, `none` | `none` |
| `WEBHOOK_DB_PATH` | SQLite database path | `./data/webhook.db` |
| `WEBHOOK_QUEUE_WORKERS` | Number of queue workers | `2` |
| `WEBHOOK_QUEUE_MAX_RETRIES` | Max send retries | `3` |
| `WEBHOOK_QUEUE_RETRY_DELAY` | Base retry delay | `30s` |

### SMTP Examples

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

## API

### Send Email

```
POST /api/v1/send
Authorization: Bearer <token>
Content-Type: application/json
```

**Request body:**
```json
{
  "to": "user@example.com",
  "subject": "Hello",
  "text": "Plain text body",
  "html": "<h1>HTML body</h1>"
}
```

- `to` — recipient address (comma-separated for multiple)
- `subject` — email subject (required)
- `text` / `html` — at least one required; both sends multipart/alternative

**Response (202):**
```json
{
  "id": 42,
  "status": "queued"
}
```

### Health Check

```
GET /api/v1/health
```

Returns `{"status":"ok"}` — no auth required. Use for K8s liveness/readiness probes.

## Production Deployment

### Linux (systemd)

```bash
# Create user and directories
sudo useradd -r -s /bin/false webhook
sudo mkdir -p /etc/c0de-webhook /var/lib/c0de-webhook
sudo cp config.yaml /etc/c0de-webhook/
sudo chown -R webhook:webhook /var/lib/c0de-webhook

# Install binary
sudo cp c0de-webhook /usr/local/bin/

# Create systemd service
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
# Edit secrets first
vim k8s/secret.yaml

# Edit config
vim k8s/configmap.yaml

# Deploy
kubectl apply -f k8s/
```

Manifests included:
- `k8s/deployment.yaml` — Deployment with health probes and resource limits
- `k8s/service.yaml` — ClusterIP Service
- `k8s/configmap.yaml` — Non-sensitive config
- `k8s/secret.yaml` — Passwords and keys
- `k8s/pvc.yaml` — Persistent volume for SQLite

### Security Checklist

- [ ] Change `admin_password` and `secret_key` from defaults
- [ ] Use HTTPS via reverse proxy (nginx, Caddy, Traefik)
- [ ] Restrict network access to the admin UI
- [ ] Store SMTP credentials in K8s Secrets or a secrets manager
- [ ] Set resource limits in production

## Development

```bash
# Build
go build -o c0de-webhook .

# Run tests
go test -race ./...

# Run tests with coverage
go test -race -coverprofile=coverage.out ./...
go tool cover -html=coverage.out -o coverage.html

# Lint
golangci-lint run

# Run locally
./c0de-webhook -config config.example.yaml
```

## License

MIT
