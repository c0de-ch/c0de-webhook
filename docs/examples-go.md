# Go Examples

## Simple client

```go
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	webhookURL = "http://localhost:8090"
	apiToken   = "YOUR_TOKEN"
)

var client = &http.Client{Timeout: 10 * time.Second}

// SendMail sends an email via c0de-webhook.
func SendMail(to, subject, text, html string) (int64, error) {
	return send("/api/v2/mail", map[string]string{
		"to":      to,
		"subject": subject,
		"text":    text,
		"html":    html,
	})
}

// SendWhatsApp sends a WhatsApp message via c0de-webhook.
func SendWhatsApp(phone, text string) (int64, error) {
	return send("/api/v2/whatsapp", map[string]string{
		"phone": phone,
		"text":  text,
	})
}

// SendTelegram sends a Telegram message via c0de-webhook.
func SendTelegram(chatID, text string) (int64, error) {
	return send("/api/v2/telegram", map[string]string{
		"chat_id": chatID,
		"text":    text,
	})
}

func send(endpoint string, payload map[string]string) (int64, error) {
	body, _ := json.Marshal(payload)
	req, err := http.NewRequest("POST", webhookURL+endpoint, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiToken)

	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusAccepted {
		return 0, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		ID     int64  `json:"id"`
		Status string `json:"status"`
	}
	json.Unmarshal(respBody, &result)
	return result.ID, nil
}

func main() {
	// Send email
	id, err := SendMail("user@example.com", "Hello", "Hello from Go!", "")
	if err != nil {
		fmt.Printf("Email error: %v\n", err)
	} else {
		fmt.Printf("Email queued: id=%d\n", id)
	}

	// Send WhatsApp
	id, err = SendWhatsApp("41791234567", "Hello from Go!")
	if err != nil {
		fmt.Printf("WhatsApp error: %v\n", err)
	} else {
		fmt.Printf("WhatsApp queued: id=%d\n", id)
	}

	// Send Telegram
	id, err = SendTelegram("123456789", "Hello from Go!")
	if err != nil {
		fmt.Printf("Telegram error: %v\n", err)
	} else {
		fmt.Printf("Telegram queued: id=%d\n", id)
	}
}
```

## Reusable package

```go
package webhook

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

func New(baseURL, token string) *Client {
	return &Client{
		BaseURL: baseURL,
		Token:   token,
		HTTP:    &http.Client{Timeout: 10 * time.Second},
	}
}

type Response struct {
	ID      int64  `json:"id"`
	Status  string `json:"status"`
	Channel string `json:"channel"`
}

func (c *Client) Mail(to, subject, text, html string) (*Response, error) {
	return c.post("/api/v2/mail", map[string]string{
		"to": to, "subject": subject, "text": text, "html": html,
	})
}

func (c *Client) WhatsApp(phone, text string) (*Response, error) {
	return c.post("/api/v2/whatsapp", map[string]string{
		"phone": phone, "text": text,
	})
}

func (c *Client) Telegram(chatID, text string) (*Response, error) {
	return c.post("/api/v2/telegram", map[string]string{
		"chat_id": chatID, "text": text,
	})
}

func (c *Client) Health() error {
	resp, err := c.HTTP.Get(c.BaseURL + "/api/v2/health")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("health check failed: HTTP %d", resp.StatusCode)
	}
	return nil
}

func (c *Client) post(path string, payload interface{}) (*Response, error) {
	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", c.BaseURL+path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.Token)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusAccepted {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, data)
	}

	var r Response
	json.Unmarshal(data, &r)
	return &r, nil
}
```

**Usage:**

```go
wh := webhook.New("http://localhost:8090", "YOUR_TOKEN")
resp, err := wh.Mail("user@example.com", "Subject", "Body", "")
fmt.Println(resp.ID, resp.Status)
```
