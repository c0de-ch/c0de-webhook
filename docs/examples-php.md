# PHP Examples

## Simple (cURL)

```php
<?php

define('WEBHOOK_URL', 'http://localhost:8090');
define('API_TOKEN', 'YOUR_TOKEN');

function sendMail(string $to, string $subject, string $text, string $html = ''): array {
    return webhookPost('/api/v2/mail', [
        'to' => $to,
        'subject' => $subject,
        'text' => $text,
        'html' => $html,
    ]);
}

function sendWhatsApp(string $phone, string $text): array {
    return webhookPost('/api/v2/whatsapp', [
        'phone' => $phone,
        'text' => $text,
    ]);
}

function sendTelegram(string $chatId, string $text): array {
    return webhookPost('/api/v2/telegram', [
        'chat_id' => $chatId,
        'text' => $text,
    ]);
}

function webhookPost(string $endpoint, array $payload): array {
    $ch = curl_init(WEBHOOK_URL . $endpoint);
    curl_setopt_array($ch, [
        CURLOPT_POST => true,
        CURLOPT_POSTFIELDS => json_encode($payload),
        CURLOPT_HTTPHEADER => [
            'Content-Type: application/json',
            'Authorization: Bearer ' . API_TOKEN,
        ],
        CURLOPT_RETURNTRANSFER => true,
        CURLOPT_TIMEOUT => 10,
    ]);

    $response = curl_exec($ch);
    $httpCode = curl_getinfo($ch, CURLINFO_HTTP_CODE);
    curl_close($ch);

    if ($httpCode !== 202) {
        throw new RuntimeException("Webhook failed (HTTP $httpCode): $response");
    }

    return json_decode($response, true);
}

// Usage
$result = sendMail('user@example.com', 'Hello', 'Hello from PHP!');
echo "Email queued: id={$result['id']}\n";

$result = sendWhatsApp('41791234567', 'Hello from PHP!');
echo "WhatsApp queued: id={$result['id']}\n";

$result = sendTelegram('123456789', 'Hello from PHP!');
echo "Telegram queued: id={$result['id']}\n";
```

## Laravel integration

```php
<?php
// app/Services/WebhookNotifier.php

namespace App\Services;

use Illuminate\Support\Facades\Http;

class WebhookNotifier
{
    private string $baseUrl;
    private string $token;

    public function __construct()
    {
        $this->baseUrl = config('services.webhook.url');   // http://localhost:8090
        $this->token = config('services.webhook.token');   // YOUR_TOKEN
    }

    public function mail(string $to, string $subject, string $text, string $html = ''): array
    {
        return $this->post('/api/v2/mail', compact('to', 'subject', 'text', 'html'));
    }

    public function whatsapp(string $phone, string $text): array
    {
        return $this->post('/api/v2/whatsapp', compact('phone', 'text'));
    }

    public function telegram(string $chatId, string $text): array
    {
        return $this->post('/api/v2/telegram', ['chat_id' => $chatId, 'text' => $text]);
    }

    private function post(string $endpoint, array $payload): array
    {
        $response = Http::withToken($this->token)
            ->post($this->baseUrl . $endpoint, $payload);

        if (!$response->successful()) {
            throw new \RuntimeException("Webhook failed: " . $response->body());
        }

        return $response->json();
    }
}
```

**Usage in a controller:**

```php
use App\Services\WebhookNotifier;

class OrderController extends Controller
{
    public function confirm(Order $order, WebhookNotifier $webhook)
    {
        $webhook->mail(
            $order->customer->email,
            "Order #{$order->id} Confirmed",
            "Your order has been confirmed.",
        );

        return response()->json(['status' => 'confirmed']);
    }
}
```
