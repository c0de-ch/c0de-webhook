# JavaScript / TypeScript Examples

## Node.js (fetch)

```javascript
const WEBHOOK_URL = 'http://localhost:8090';
const API_TOKEN = 'YOUR_TOKEN';

async function sendMail(to, subject, text, html = '') {
  const resp = await fetch(`${WEBHOOK_URL}/api/v2/mail`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${API_TOKEN}`,
    },
    body: JSON.stringify({ to, subject, text, html }),
  });

  if (!resp.ok) {
    const err = await resp.json();
    throw new Error(`HTTP ${resp.status}: ${err.error}`);
  }

  return resp.json(); // { id: 42, status: "queued", channel: "mail" }
}

async function sendWhatsApp(phone, text) {
  const resp = await fetch(`${WEBHOOK_URL}/api/v2/whatsapp`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${API_TOKEN}`,
    },
    body: JSON.stringify({ phone, text }),
  });

  if (!resp.ok) {
    const err = await resp.json();
    throw new Error(`HTTP ${resp.status}: ${err.error}`);
  }

  return resp.json();
}

async function sendTelegram(chatId, text) {
  const resp = await fetch(`${WEBHOOK_URL}/api/v2/telegram`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${API_TOKEN}`,
    },
    body: JSON.stringify({ chat_id: chatId, text }),
  });

  if (!resp.ok) {
    const err = await resp.json();
    throw new Error(`HTTP ${resp.status}: ${err.error}`);
  }

  return resp.json();
}

// Usage
(async () => {
  try {
    const mail = await sendMail('user@example.com', 'Hello', 'Hello from Node.js!');
    console.log('Email queued:', mail);

    const wa = await sendWhatsApp('41791234567', 'Hello from Node.js!');
    console.log('WhatsApp queued:', wa);

    const tg = await sendTelegram('123456789', 'Hello from Node.js!');
    console.log('Telegram queued:', tg);
  } catch (err) {
    console.error('Error:', err.message);
  }
})();
```

## Reusable client class

```typescript
class WebhookClient {
  constructor(
    private baseUrl: string,
    private token: string,
  ) {}

  private async post(endpoint: string, body: Record<string, string>) {
    const resp = await fetch(`${this.baseUrl}${endpoint}`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'Authorization': `Bearer ${this.token}`,
      },
      body: JSON.stringify(body),
    });

    const data = await resp.json();
    if (!resp.ok) throw new Error(data.error || `HTTP ${resp.status}`);
    return data as { id: number; status: string; channel: string };
  }

  mail(to: string, subject: string, text: string, html?: string) {
    return this.post('/api/v2/mail', {
      to, subject, text, ...(html ? { html } : {}),
    });
  }

  whatsapp(phone: string, text: string) {
    return this.post('/api/v2/whatsapp', { phone, text });
  }

  telegram(chatId: string, text: string) {
    return this.post('/api/v2/telegram', { chat_id: chatId, text });
  }

  async health(): Promise<boolean> {
    const resp = await fetch(`${this.baseUrl}/api/v2/health`);
    return resp.ok;
  }
}

// Usage
const webhook = new WebhookClient('http://localhost:8090', 'YOUR_TOKEN');

await webhook.mail('user@example.com', 'Alert', 'Server CPU > 90%');
await webhook.whatsapp('41791234567', 'Order shipped!');
await webhook.telegram('123456789', 'Build passed');
```

## Browser (vanilla JS)

```html
<script>
async function sendNotification() {
  const resp = await fetch('http://localhost:8090/api/v2/mail', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Authorization': 'Bearer YOUR_TOKEN',
    },
    body: JSON.stringify({
      to: document.getElementById('email').value,
      subject: 'Contact Form',
      text: document.getElementById('message').value,
    }),
  });

  const data = await resp.json();
  if (resp.ok) {
    alert('Message sent! ID: ' + data.id);
  } else {
    alert('Error: ' + data.error);
  }
}
</script>
```

## Express.js middleware

```javascript
const express = require('express');
const app = express();
app.use(express.json());

const WEBHOOK_URL = 'http://localhost:8090';
const WEBHOOK_TOKEN = 'YOUR_TOKEN';

// Forward contact form to email
app.post('/contact', async (req, res) => {
  const { name, email, message } = req.body;

  const resp = await fetch(`${WEBHOOK_URL}/api/v2/mail`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${WEBHOOK_TOKEN}`,
    },
    body: JSON.stringify({
      to: 'support@yourcompany.com',
      subject: `Contact from ${name}`,
      text: `From: ${name} <${email}>\n\n${message}`,
      html: `<p><strong>From:</strong> ${name} &lt;${email}&gt;</p><p>${message}</p>`,
    }),
  });

  if (resp.ok) {
    res.json({ success: true });
  } else {
    const err = await resp.json();
    res.status(500).json({ error: err.error });
  }
});

app.listen(3000);
```
