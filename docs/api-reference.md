# API Reference

All endpoints require an API token passed via the `Authorization: Bearer <token>` header, except for health checks.

Base URL: `http://your-server:8090`

---

## Authentication

Create API tokens in the admin UI at `/tokens`. Each token is shown once at creation — copy it immediately.

```
Authorization: Bearer 998c728a87cdc60f5b5969c3d07150cf17963cbd2cbd23e3a09f5bd1873a77b3
```

---

## v2 Endpoints

### POST /api/v2/mail

Send an email via SMTP.

**Request:**
```json
{
  "to": "user@example.com",
  "subject": "Order Confirmation",
  "text": "Your order #1234 has been confirmed.",
  "html": "<h1>Order Confirmed</h1><p>Your order #1234 has been confirmed.</p>"
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `to` | string | yes | Recipient email. Comma-separated for multiple: `"a@x.com, b@x.com"` |
| `subject` | string | yes | Email subject line |
| `text` | string | one of text/html | Plain text body |
| `html` | string | one of text/html | HTML body. If both provided, sends multipart/alternative |

**Response (202):**
```json
{
  "id": 42,
  "status": "queued",
  "channel": "mail"
}
```

### POST /api/v2/whatsapp

Send a WhatsApp message via the Meta Business API.

**Request:**
```json
{
  "phone": "41791234567",
  "text": "Your appointment is confirmed for tomorrow at 10:00."
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `phone` | string | yes | Phone number in international format without `+` (e.g. `41791234567`) |
| `text` | string | yes | Message text (max 4096 chars) |

**Response (202):**
```json
{
  "id": 43,
  "status": "queued",
  "channel": "whatsapp"
}
```

### POST /api/v2/telegram

Send a Telegram message via the Bot API.

**Request:**
```json
{
  "chat_id": "123456789",
  "text": "Build #567 passed successfully."
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `chat_id` | string | yes | Telegram chat ID (get from `/getUpdates` on your bot) |
| `text` | string | yes | Message text |

**Response (202):**
```json
{
  "id": 44,
  "status": "queued",
  "channel": "telegram"
}
```

### GET /api/v2/health

Health check (no auth required).

**Response (200):**
```json
{
  "status": "ok"
}
```

---

## v1 Endpoints (Legacy)

| Endpoint | Description |
|----------|-------------|
| `POST /api/v1/send` | Same as `POST /api/v2/mail` |
| `GET /api/v1/health` | Same as `GET /api/v2/health` |

---

## Error Responses

All errors return JSON:

```json
{
  "error": "description of what went wrong"
}
```

| HTTP Status | Meaning |
|-------------|---------|
| 400 | Bad request — missing required fields or invalid JSON |
| 401 | Unauthorized — missing or invalid Bearer token |
| 500 | Server error — failed to enqueue message |

---

## Message Lifecycle

1. **queued** — message accepted, waiting in queue
2. **sending** — worker picked it up, delivery in progress
3. **sent** — delivered successfully
4. **failed** — all retry attempts exhausted

Failed messages can be retried from the admin UI queue page.
