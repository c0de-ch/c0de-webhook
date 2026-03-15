// webhook-test is a CLI tool for testing the c0de-webhook API.
//
// Usage:
//
//	webhook-test -url http://localhost:8090 -token YOUR_TOKEN -to user@example.com
//	webhook-test -url http://localhost:8090 -token YOUR_TOKEN -to user@example.com -subject "Test" -text "Hello"
//	webhook-test -url http://localhost:8090 -token YOUR_TOKEN -to user@example.com -html "<h1>Hi</h1>"
//	webhook-test -url http://localhost:8090 -token YOUR_TOKEN -to user@example.com -n 5   # send 5 test messages
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

type SendRequest struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	Text    string `json:"text,omitempty"`
	HTML    string `json:"html,omitempty"`
}

type SendResponse struct {
	ID     int64  `json:"id"`
	Status string `json:"status"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

func main() {
	url := flag.String("url", "http://localhost:8090", "c0de-webhook server URL")
	token := flag.String("token", os.Getenv("WEBHOOK_TOKEN"), "API token (or set WEBHOOK_TOKEN env var)")
	to := flag.String("to", "", "recipient email address (required)")
	subject := flag.String("subject", "", "email subject (default: auto-generated)")
	text := flag.String("text", "", "plain text body")
	html := flag.String("html", "", "HTML body")
	count := flag.Int("n", 1, "number of test messages to send")
	health := flag.Bool("health", false, "check server health and exit")
	flag.Parse()

	if *health {
		checkHealth(*url)
		return
	}

	if *token == "" {
		fmt.Fprintln(os.Stderr, "Error: -token is required (or set WEBHOOK_TOKEN env var)")
		flag.Usage()
		os.Exit(1)
	}
	if *to == "" {
		fmt.Fprintln(os.Stderr, "Error: -to is required")
		flag.Usage()
		os.Exit(1)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	endpoint := *url + "/api/v1/send"

	sent, failed := 0, 0
	for i := 1; i <= *count; i++ {
		subj := *subject
		if subj == "" {
			subj = fmt.Sprintf("Test message #%d from webhook-test", i)
		}

		textBody := *text
		htmlBody := *html
		if textBody == "" && htmlBody == "" {
			textBody = fmt.Sprintf("This is test message #%d sent at %s via c0de-webhook.", i, time.Now().Format(time.RFC3339))
		}

		req := SendRequest{
			To:      *to,
			Subject: subj,
			Text:    textBody,
			HTML:    htmlBody,
		}

		body, _ := json.Marshal(req)
		httpReq, err := http.NewRequest("POST", endpoint, bytes.NewReader(body))
		if err != nil {
			fmt.Fprintf(os.Stderr, "[%d/%d] Error creating request: %v\n", i, *count, err)
			failed++
			continue
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Authorization", "Bearer "+*token)

		resp, err := client.Do(httpReq)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[%d/%d] Error: %v\n", i, *count, err)
			failed++
			continue
		}

		respBody, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()

		if resp.StatusCode == http.StatusAccepted {
			var sr SendResponse
			_ = json.Unmarshal(respBody, &sr)
			fmt.Printf("[%d/%d] OK  id=%d status=%s  to=%s subject=%q\n", i, *count, sr.ID, sr.Status, *to, subj)
			sent++
		} else {
			var er ErrorResponse
			_ = json.Unmarshal(respBody, &er)
			fmt.Fprintf(os.Stderr, "[%d/%d] FAIL  http=%d error=%q\n", i, *count, resp.StatusCode, er.Error)
			failed++
		}
	}

	fmt.Printf("\nDone: %d sent, %d failed out of %d\n", sent, failed, *count)
	if failed > 0 {
		os.Exit(1)
	}
}

func checkHealth(url string) {
	resp, err := http.Get(url + "/api/v1/health")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Health check failed: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == 200 {
		fmt.Printf("OK: %s\n", string(body))
	} else {
		fmt.Fprintf(os.Stderr, "Health check failed: HTTP %d\n", resp.StatusCode)
		os.Exit(1)
	}
}
