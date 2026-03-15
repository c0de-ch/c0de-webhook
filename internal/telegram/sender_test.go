package telegram

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewSender(t *testing.T) {
	s := NewSender("bot123:ABC", "HTML")
	if s.BotToken != "bot123:ABC" {
		t.Errorf("BotToken = %q, want %q", s.BotToken, "bot123:ABC")
	}
	if s.ParseMode != "HTML" {
		t.Errorf("ParseMode = %q, want %q", s.ParseMode, "HTML")
	}
	if s.client == nil {
		t.Error("client should not be nil")
	}
}

func TestSender_Configured(t *testing.T) {
	tests := []struct {
		name     string
		token    string
		expected bool
	}{
		{"with token", "bot123:ABC", true},
		{"empty token", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewSender(tt.token, "")
			if got := s.Configured(); got != tt.expected {
				t.Errorf("Configured() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestSender_NotConfigured(t *testing.T) {
	s := NewSender("", "")
	err := s.Send("12345", "hello")
	if err == nil {
		t.Fatal("expected error when not configured")
	}
	if !strings.Contains(err.Error(), "not configured") {
		t.Errorf("error = %q, want it to contain 'not configured'", err.Error())
	}
}

func TestSender_Send_Success(t *testing.T) {
	var receivedBody map[string]interface{}
	var receivedPath string
	var receivedContentType string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		receivedContentType = r.Header.Get("Content-Type")

		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &receivedBody)

		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	s := NewSender("myBotToken123", "")
	s.BaseURL = srv.URL

	err := s.Send("chat_999", "Hello from test")
	if err != nil {
		t.Fatalf("Send() error: %v", err)
	}

	// Verify URL path contains bot token.
	expectedPath := "/botmyBotToken123/sendMessage"
	if receivedPath != expectedPath {
		t.Errorf("path = %q, want %q", receivedPath, expectedPath)
	}

	// Verify content type.
	if receivedContentType != "application/json" {
		t.Errorf("Content-Type = %q, want %q", receivedContentType, "application/json")
	}

	// Verify body has chat_id and text.
	if receivedBody["chat_id"] != "chat_999" {
		t.Errorf("chat_id = %v, want %q", receivedBody["chat_id"], "chat_999")
	}
	if receivedBody["text"] != "Hello from test" {
		t.Errorf("text = %v, want %q", receivedBody["text"], "Hello from test")
	}

	// parse_mode should NOT be present when empty.
	if _, exists := receivedBody["parse_mode"]; exists {
		t.Errorf("parse_mode should not be set when ParseMode is empty, got %v", receivedBody["parse_mode"])
	}
}

func TestSender_Send_WithParseMode(t *testing.T) {
	var receivedBody map[string]interface{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &receivedBody)

		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	s := NewSender("myBotToken123", "HTML")
	s.BaseURL = srv.URL

	err := s.Send("chat_999", "<b>Bold</b> message")
	if err != nil {
		t.Fatalf("Send() error: %v", err)
	}

	if receivedBody["parse_mode"] != "HTML" {
		t.Errorf("parse_mode = %v, want %q", receivedBody["parse_mode"], "HTML")
	}
	if receivedBody["chat_id"] != "chat_999" {
		t.Errorf("chat_id = %v, want %q", receivedBody["chat_id"], "chat_999")
	}
	if receivedBody["text"] != "<b>Bold</b> message" {
		t.Errorf("text = %v, want %q", receivedBody["text"], "<b>Bold</b> message")
	}
}

func TestSender_Send_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		_, _ = w.Write([]byte(`{"ok":false,"description":"Bad Request: chat not found"}`))
	}))
	defer srv.Close()

	s := NewSender("myBotToken123", "")
	s.BaseURL = srv.URL

	err := s.Send("invalid_chat", "Hello")
	if err == nil {
		t.Fatal("expected error for 400 response")
	}
	if !strings.Contains(err.Error(), "400") {
		t.Errorf("error = %q, want it to contain '400'", err.Error())
	}
	if !strings.Contains(err.Error(), "chat not found") {
		t.Errorf("error = %q, want it to contain response body", err.Error())
	}
}

func TestSender_Send_ConnectionError(t *testing.T) {
	s := NewSender("myBotToken123", "")
	s.BaseURL = "http://127.0.0.1:1" // port 1 should be unreachable

	err := s.Send("chat_999", "Hello")
	if err == nil {
		t.Fatal("expected error for connection failure")
	}
	if !strings.Contains(err.Error(), "sending request") {
		t.Errorf("error = %q, want it to contain 'sending request'", err.Error())
	}
}
