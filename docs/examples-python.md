# Python Examples

## Simple (requests)

```python
import requests

WEBHOOK_URL = "http://localhost:8090"
API_TOKEN = "YOUR_TOKEN"

headers = {
    "Content-Type": "application/json",
    "Authorization": f"Bearer {API_TOKEN}",
}


def send_mail(to, subject, text, html=""):
    resp = requests.post(f"{WEBHOOK_URL}/api/v2/mail", json={
        "to": to,
        "subject": subject,
        "text": text,
        "html": html,
    }, headers=headers)
    resp.raise_for_status()
    return resp.json()


def send_whatsapp(phone, text):
    resp = requests.post(f"{WEBHOOK_URL}/api/v2/whatsapp", json={
        "phone": phone,
        "text": text,
    }, headers=headers)
    resp.raise_for_status()
    return resp.json()


def send_telegram(chat_id, text):
    resp = requests.post(f"{WEBHOOK_URL}/api/v2/telegram", json={
        "chat_id": chat_id,
        "text": text,
    }, headers=headers)
    resp.raise_for_status()
    return resp.json()


# Usage
result = send_mail("user@example.com", "Hello", "Hello from Python!")
print(f"Email queued: id={result['id']}")

result = send_whatsapp("41791234567", "Hello from Python!")
print(f"WhatsApp queued: id={result['id']}")

result = send_telegram("123456789", "Hello from Python!")
print(f"Telegram queued: id={result['id']}")
```

## Reusable client class

```python
import requests
from dataclasses import dataclass
from typing import Optional


@dataclass
class WebhookResponse:
    id: int
    status: str
    channel: str


class WebhookClient:
    def __init__(self, base_url: str, token: str):
        self.base_url = base_url.rstrip("/")
        self.session = requests.Session()
        self.session.headers.update({
            "Content-Type": "application/json",
            "Authorization": f"Bearer {token}",
        })

    def mail(self, to: str, subject: str, text: str, html: str = "") -> WebhookResponse:
        return self._post("/api/v2/mail", {
            "to": to, "subject": subject, "text": text, "html": html,
        })

    def whatsapp(self, phone: str, text: str) -> WebhookResponse:
        return self._post("/api/v2/whatsapp", {"phone": phone, "text": text})

    def telegram(self, chat_id: str, text: str) -> WebhookResponse:
        return self._post("/api/v2/telegram", {"chat_id": chat_id, "text": text})

    def health(self) -> bool:
        try:
            resp = self.session.get(f"{self.base_url}/api/v2/health")
            return resp.status_code == 200
        except requests.RequestException:
            return False

    def _post(self, endpoint: str, payload: dict) -> WebhookResponse:
        resp = self.session.post(f"{self.base_url}{endpoint}", json=payload)
        resp.raise_for_status()
        data = resp.json()
        return WebhookResponse(id=data["id"], status=data["status"], channel=data["channel"])


# Usage
webhook = WebhookClient("http://localhost:8090", "YOUR_TOKEN")

if webhook.health():
    print("Server is up")

resp = webhook.mail("user@example.com", "Test", "Hello from Python!")
print(f"Queued: {resp}")
```

## Django integration

```python
# notifications/webhook.py
from django.conf import settings
import requests

class WebhookNotifier:
    def __init__(self):
        self.url = settings.WEBHOOK_URL      # "http://localhost:8090"
        self.token = settings.WEBHOOK_TOKEN  # "YOUR_TOKEN"
        self.headers = {
            "Content-Type": "application/json",
            "Authorization": f"Bearer {self.token}",
        }

    def notify_user(self, user, subject, message):
        """Send notification via the user's preferred channel."""
        if user.phone and user.prefers_whatsapp:
            requests.post(f"{self.url}/api/v2/whatsapp", json={
                "phone": user.phone,
                "text": message,
            }, headers=self.headers)
        elif user.telegram_id:
            requests.post(f"{self.url}/api/v2/telegram", json={
                "chat_id": user.telegram_id,
                "text": message,
            }, headers=self.headers)
        else:
            requests.post(f"{self.url}/api/v2/mail", json={
                "to": user.email,
                "subject": subject,
                "text": message,
            }, headers=self.headers)


# views.py
notifier = WebhookNotifier()

def order_confirmed(request, order_id):
    order = Order.objects.get(id=order_id)
    notifier.notify_user(
        order.user,
        f"Order #{order.id} confirmed",
        f"Your order #{order.id} for {order.total} has been confirmed."
    )
```

## Monitoring script

```python
#!/usr/bin/env python3
"""Send alerts when a service is down."""
import requests
import sys

WEBHOOK = "http://localhost:8090"
TOKEN = "YOUR_TOKEN"
ALERT_EMAIL = "ops@example.com"

def check_service(name, url):
    try:
        resp = requests.get(url, timeout=5)
        if resp.status_code != 200:
            return f"{name} returned HTTP {resp.status_code}"
    except requests.RequestException as e:
        return f"{name} unreachable: {e}"
    return None

def send_alert(subject, body):
    requests.post(f"{WEBHOOK}/api/v2/mail", json={
        "to": ALERT_EMAIL,
        "subject": subject,
        "text": body,
    }, headers={
        "Authorization": f"Bearer {TOKEN}",
        "Content-Type": "application/json",
    })

services = {
    "API": "https://api.example.com/health",
    "Website": "https://www.example.com",
    "Database proxy": "http://db-proxy:8080/health",
}

errors = []
for name, url in services.items():
    err = check_service(name, url)
    if err:
        errors.append(err)

if errors:
    send_alert(
        f"ALERT: {len(errors)} service(s) down",
        "\n".join(errors)
    )
    sys.exit(1)
```
