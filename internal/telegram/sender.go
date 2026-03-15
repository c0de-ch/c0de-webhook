// Package telegram provides a Telegram Bot API message sender.
package telegram

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Sender sends messages via the Telegram Bot API.
type Sender struct {
	BotToken  string
	ParseMode string // "HTML", "Markdown", or "" for plain text
	BaseURL   string // override for testing; defaults to "https://api.telegram.org"
	client    *http.Client
}

func NewSender(botToken, parseMode string) *Sender {
	return &Sender{
		BotToken:  botToken,
		ParseMode: parseMode,
		client:    &http.Client{Timeout: 15 * time.Second},
	}
}

// Send sends a text message to the given chat ID.
func (s *Sender) Send(chatID, text string) error {
	if s.BotToken == "" {
		return fmt.Errorf("telegram bot token not configured")
	}

	base := s.BaseURL
	if base == "" {
		base = "https://api.telegram.org"
	}
	url := fmt.Sprintf("%s/bot%s/sendMessage", base, s.BotToken)

	payload := map[string]interface{}{
		"chat_id": chatID,
		"text":    text,
	}
	if s.ParseMode != "" {
		payload["parse_mode"] = s.ParseMode
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshaling request: %w", err)
	}

	resp, err := s.client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		return fmt.Errorf("telegram API error (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// Configured returns true if the sender has the required configuration.
func (s *Sender) Configured() bool {
	return s.BotToken != ""
}
