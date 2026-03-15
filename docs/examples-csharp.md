# C# / .NET Examples

## .NET 6+ (HttpClient)

```csharp
using System.Net.Http.Json;

const string WebhookUrl = "http://localhost:8090";
const string ApiToken = "YOUR_TOKEN";

using var client = new HttpClient();
client.DefaultRequestHeaders.Add("Authorization", $"Bearer {ApiToken}");

// Send email
var mailResp = await client.PostAsJsonAsync($"{WebhookUrl}/api/v2/mail", new {
    to = "user@example.com",
    subject = "Hello from C#",
    text = "This is a test email.",
    html = "<h1>Hello</h1><p>This is a test email.</p>"
});
var mail = await mailResp.Content.ReadFromJsonAsync<WebhookResponse>();
Console.WriteLine($"Email queued: id={mail.Id}");

// Send WhatsApp
var waResp = await client.PostAsJsonAsync($"{WebhookUrl}/api/v2/whatsapp", new {
    phone = "41791234567",
    text = "Hello from C#!"
});
var wa = await waResp.Content.ReadFromJsonAsync<WebhookResponse>();
Console.WriteLine($"WhatsApp queued: id={wa.Id}");

// Send Telegram
var tgResp = await client.PostAsJsonAsync($"{WebhookUrl}/api/v2/telegram", new {
    chat_id = "123456789",
    text = "Hello from C#!"
});
var tg = await tgResp.Content.ReadFromJsonAsync<WebhookResponse>();
Console.WriteLine($"Telegram queued: id={tg.Id}");

record WebhookResponse(long Id, string Status, string Channel);
```

## Reusable service class

```csharp
using System.Net.Http.Json;

public class WebhookClient : IDisposable
{
    private readonly HttpClient _client;
    private readonly string _baseUrl;

    public WebhookClient(string baseUrl, string token)
    {
        _baseUrl = baseUrl.TrimEnd('/');
        _client = new HttpClient();
        _client.DefaultRequestHeaders.Add("Authorization", $"Bearer {token}");
    }

    public record WebhookResponse(long Id, string Status, string Channel);

    public Task<WebhookResponse?> MailAsync(string to, string subject, string text, string? html = null) =>
        PostAsync("/api/v2/mail", new { to, subject, text, html = html ?? "" });

    public Task<WebhookResponse?> WhatsAppAsync(string phone, string text) =>
        PostAsync("/api/v2/whatsapp", new { phone, text });

    public Task<WebhookResponse?> TelegramAsync(string chatId, string text) =>
        PostAsync("/api/v2/telegram", new { chat_id = chatId, text });

    public async Task<bool> HealthAsync()
    {
        try
        {
            var resp = await _client.GetAsync($"{_baseUrl}/api/v2/health");
            return resp.IsSuccessStatusCode;
        }
        catch { return false; }
    }

    private async Task<WebhookResponse?> PostAsync(string endpoint, object payload)
    {
        var resp = await _client.PostAsJsonAsync($"{_baseUrl}{endpoint}", payload);
        resp.EnsureSuccessStatusCode();
        return await resp.Content.ReadFromJsonAsync<WebhookResponse>();
    }

    public void Dispose() => _client.Dispose();
}
```

**Usage in ASP.NET:**

```csharp
// Program.cs
builder.Services.AddSingleton(new WebhookClient("http://localhost:8090", "YOUR_TOKEN"));

// Controller
[ApiController]
[Route("api/[controller]")]
public class OrdersController : ControllerBase
{
    private readonly WebhookClient _webhook;

    public OrdersController(WebhookClient webhook) => _webhook = webhook;

    [HttpPost("{id}/confirm")]
    public async Task<IActionResult> Confirm(int id)
    {
        var order = await GetOrder(id);
        await _webhook.MailAsync(
            order.CustomerEmail,
            $"Order #{id} Confirmed",
            $"Your order #{id} has been confirmed."
        );
        return Ok();
    }
}
```
