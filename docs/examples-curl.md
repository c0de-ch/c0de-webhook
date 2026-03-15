# cURL Examples

Replace `YOUR_TOKEN` with an API token created in the admin UI.

## Email

### Simple text email

```bash
curl -X POST http://localhost:8090/api/v2/mail \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "to": "user@example.com",
    "subject": "Hello",
    "text": "Hello from c0de-webhook!"
  }'
```

### HTML email

```bash
curl -X POST http://localhost:8090/api/v2/mail \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "to": "user@example.com",
    "subject": "Weekly Report",
    "html": "<h1>Weekly Report</h1><p>All systems operational.</p>",
    "text": "Weekly Report\n\nAll systems operational."
  }'
```

### Multiple recipients

```bash
curl -X POST http://localhost:8090/api/v2/mail \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "to": "alice@example.com, bob@example.com",
    "subject": "Team Update",
    "text": "Sprint 42 completed."
  }'
```

## WhatsApp

```bash
curl -X POST http://localhost:8090/api/v2/whatsapp \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "phone": "41791234567",
    "text": "Your order has been shipped!"
  }'
```

## Telegram

```bash
curl -X POST http://localhost:8090/api/v2/telegram \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "chat_id": "123456789",
    "text": "Deployment to production completed."
  }'
```

## Health Check

```bash
curl http://localhost:8090/api/v2/health
# {"status":"ok"}
```

## Using the legacy v1 endpoint

```bash
curl -X POST http://localhost:8090/api/v1/send \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"to":"user@example.com","subject":"Test","text":"Hello"}'
```

## Shell script: send alert on error

```bash
#!/bin/bash
WEBHOOK_URL="http://localhost:8090"
TOKEN="YOUR_TOKEN"

send_alert() {
  local subject="$1"
  local body="$2"
  curl -s -X POST "$WEBHOOK_URL/api/v2/mail" \
    -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: application/json" \
    -d "{\"to\":\"ops@example.com\",\"subject\":\"$subject\",\"text\":\"$body\"}"
}

# Usage
if ! systemctl is-active --quiet nginx; then
  send_alert "ALERT: nginx is down" "nginx service is not running on $(hostname) at $(date)"
fi
```
