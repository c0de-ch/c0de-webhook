# Java Examples

## Java 11+ (HttpClient)

```java
import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.time.Duration;

public class WebhookExample {

    private static final String WEBHOOK_URL = "http://localhost:8090";
    private static final String API_TOKEN = "YOUR_TOKEN";
    private static final HttpClient client = HttpClient.newBuilder()
            .connectTimeout(Duration.ofSeconds(10))
            .build();

    public static void main(String[] args) throws Exception {
        // Send email
        String mailResponse = sendMail(
            "user@example.com",
            "Hello from Java",
            "This is a test email sent from Java.",
            ""
        );
        System.out.println("Mail: " + mailResponse);

        // Send WhatsApp
        String waResponse = sendWhatsApp("41791234567", "Hello from Java!");
        System.out.println("WhatsApp: " + waResponse);

        // Send Telegram
        String tgResponse = sendTelegram("123456789", "Hello from Java!");
        System.out.println("Telegram: " + tgResponse);
    }

    static String sendMail(String to, String subject, String text, String html) throws Exception {
        String json = String.format(
            "{\"to\":\"%s\",\"subject\":\"%s\",\"text\":\"%s\",\"html\":\"%s\"}",
            to, subject, text, html
        );
        return post("/api/v2/mail", json);
    }

    static String sendWhatsApp(String phone, String text) throws Exception {
        String json = String.format("{\"phone\":\"%s\",\"text\":\"%s\"}", phone, text);
        return post("/api/v2/whatsapp", json);
    }

    static String sendTelegram(String chatId, String text) throws Exception {
        String json = String.format("{\"chat_id\":\"%s\",\"text\":\"%s\"}", chatId, text);
        return post("/api/v2/telegram", json);
    }

    private static String post(String endpoint, String json) throws Exception {
        HttpRequest request = HttpRequest.newBuilder()
                .uri(URI.create(WEBHOOK_URL + endpoint))
                .header("Content-Type", "application/json")
                .header("Authorization", "Bearer " + API_TOKEN)
                .POST(HttpRequest.BodyPublishers.ofString(json))
                .build();

        HttpResponse<String> response = client.send(request, HttpResponse.BodyHandlers.ofString());

        if (response.statusCode() != 202) {
            throw new RuntimeException("HTTP " + response.statusCode() + ": " + response.body());
        }
        return response.body();
    }
}
```

## Reusable client class

```java
import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.time.Duration;
import com.fasterxml.jackson.databind.ObjectMapper;
import java.util.Map;

public class WebhookClient {

    private final String baseUrl;
    private final String token;
    private final HttpClient client;
    private final ObjectMapper mapper = new ObjectMapper();

    public WebhookClient(String baseUrl, String token) {
        this.baseUrl = baseUrl;
        this.token = token;
        this.client = HttpClient.newBuilder()
                .connectTimeout(Duration.ofSeconds(10))
                .build();
    }

    public record WebhookResponse(long id, String status, String channel) {}

    public WebhookResponse mail(String to, String subject, String text, String html) throws Exception {
        return post("/api/v2/mail", Map.of(
            "to", to, "subject", subject, "text", text, "html", html != null ? html : ""
        ));
    }

    public WebhookResponse whatsapp(String phone, String text) throws Exception {
        return post("/api/v2/whatsapp", Map.of("phone", phone, "text", text));
    }

    public WebhookResponse telegram(String chatId, String text) throws Exception {
        return post("/api/v2/telegram", Map.of("chat_id", chatId, "text", text));
    }

    public boolean health() {
        try {
            HttpRequest req = HttpRequest.newBuilder()
                    .uri(URI.create(baseUrl + "/api/v2/health"))
                    .GET().build();
            HttpResponse<String> resp = client.send(req, HttpResponse.BodyHandlers.ofString());
            return resp.statusCode() == 200;
        } catch (Exception e) {
            return false;
        }
    }

    private WebhookResponse post(String endpoint, Map<String, String> body) throws Exception {
        String json = mapper.writeValueAsString(body);
        HttpRequest request = HttpRequest.newBuilder()
                .uri(URI.create(baseUrl + endpoint))
                .header("Content-Type", "application/json")
                .header("Authorization", "Bearer " + token)
                .POST(HttpRequest.BodyPublishers.ofString(json))
                .build();

        HttpResponse<String> response = client.send(request, HttpResponse.BodyHandlers.ofString());
        if (response.statusCode() != 202) {
            throw new RuntimeException("HTTP " + response.statusCode() + ": " + response.body());
        }
        return mapper.readValue(response.body(), WebhookResponse.class);
    }
}
```

**Usage:**

```java
WebhookClient webhook = new WebhookClient("http://localhost:8090", "YOUR_TOKEN");

// Send email
WebhookResponse resp = webhook.mail("user@example.com", "Alert", "CPU > 90%", null);
System.out.println("Queued with ID: " + resp.id());

// Send WhatsApp
webhook.whatsapp("41791234567", "Your order is ready!");

// Send Telegram
webhook.telegram("123456789", "Deployment complete");

// Health check
if (webhook.health()) {
    System.out.println("Webhook server is up");
}
```

## Spring Boot integration

```java
import org.springframework.stereotype.Service;
import org.springframework.web.client.RestTemplate;
import org.springframework.http.*;

@Service
public class NotificationService {

    private final RestTemplate rest = new RestTemplate();
    private final String webhookUrl = "http://localhost:8090";
    private final String token = "YOUR_TOKEN";

    public void sendEmail(String to, String subject, String body) {
        send("/api/v2/mail",
            Map.of("to", to, "subject", subject, "text", body));
    }

    public void sendWhatsApp(String phone, String message) {
        send("/api/v2/whatsapp",
            Map.of("phone", phone, "text", message));
    }

    public void sendTelegram(String chatId, String message) {
        send("/api/v2/telegram",
            Map.of("chat_id", chatId, "text", message));
    }

    private void send(String endpoint, Map<String, String> payload) {
        HttpHeaders headers = new HttpHeaders();
        headers.setContentType(MediaType.APPLICATION_JSON);
        headers.setBearerAuth(token);

        HttpEntity<Map<String, String>> request = new HttpEntity<>(payload, headers);
        ResponseEntity<String> response = rest.postForEntity(
            webhookUrl + endpoint, request, String.class);

        if (!response.getStatusCode().is2xxSuccessful()) {
            throw new RuntimeException("Webhook failed: " + response.getBody());
        }
    }
}
```
