// Package whatsapp provides WhatsApp message senders.
//
// Two modes are supported:
//   - Business API: Official Meta WhatsApp Business Platform (cloud-hosted)
//   - Web bridge: Self-hosted via whatsmeow (uses WhatsApp Web protocol)
package whatsapp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// BusinessSender sends messages via the WhatsApp Business API (Meta Cloud API).
type BusinessSender struct {
	PhoneNumberID string
	AccessToken   string
	APIVersion    string // e.g. "v21.0"
	BaseURL       string // override for testing; defaults to "https://graph.facebook.com"
	client        *http.Client
}

func NewBusinessSender(phoneNumberID, accessToken, apiVersion string) *BusinessSender {
	if apiVersion == "" {
		apiVersion = "v21.0"
	}
	return &BusinessSender{
		PhoneNumberID: phoneNumberID,
		AccessToken:   accessToken,
		APIVersion:    apiVersion,
		client:        &http.Client{Timeout: 15 * time.Second},
	}
}

// Send sends a text message to the given phone number.
// Phone should be in international format without + (e.g. "41791234567").
func (s *BusinessSender) Send(phone, text string) error {
	if s.PhoneNumberID == "" || s.AccessToken == "" {
		return fmt.Errorf("whatsapp business API not configured")
	}

	base := s.BaseURL
	if base == "" {
		base = "https://graph.facebook.com"
	}
	url := fmt.Sprintf("%s/%s/%s/messages", base, s.APIVersion, s.PhoneNumberID)

	payload := map[string]interface{}{
		"messaging_product": "whatsapp",
		"to":                phone,
		"type":              "text",
		"text": map[string]string{
			"body": text,
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshaling request: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.AccessToken)

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("whatsapp API error (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// Configured returns true if the sender has the required configuration.
func (s *BusinessSender) Configured() bool {
	return s.PhoneNumberID != "" && s.AccessToken != ""
}
