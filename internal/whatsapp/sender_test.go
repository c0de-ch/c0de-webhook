package whatsapp

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewBusinessSender(t *testing.T) {
	s := NewBusinessSender("12345", "tok_abc", "v19.0")
	if s.PhoneNumberID != "12345" {
		t.Errorf("PhoneNumberID = %q, want %q", s.PhoneNumberID, "12345")
	}
	if s.AccessToken != "tok_abc" {
		t.Errorf("AccessToken = %q, want %q", s.AccessToken, "tok_abc")
	}
	if s.APIVersion != "v19.0" {
		t.Errorf("APIVersion = %q, want %q", s.APIVersion, "v19.0")
	}
	if s.client == nil {
		t.Error("client should not be nil")
	}

	// Default API version when empty string is passed.
	s2 := NewBusinessSender("12345", "tok_abc", "")
	if s2.APIVersion != "v21.0" {
		t.Errorf("default APIVersion = %q, want %q", s2.APIVersion, "v21.0")
	}
}

func TestBusinessSender_Configured(t *testing.T) {
	tests := []struct {
		name     string
		phoneID  string
		token    string
		expected bool
	}{
		{"both set", "123", "tok", true},
		{"empty phone", "", "tok", false},
		{"empty token", "123", "", false},
		{"both empty", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewBusinessSender(tt.phoneID, tt.token, "")
			if got := s.Configured(); got != tt.expected {
				t.Errorf("Configured() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestBusinessSender_NotConfigured(t *testing.T) {
	s := NewBusinessSender("", "", "")
	err := s.Send("41791234567", "hello")
	if err == nil {
		t.Fatal("expected error when not configured")
	}
	if !strings.Contains(err.Error(), "not configured") {
		t.Errorf("error = %q, want it to contain 'not configured'", err.Error())
	}
}

func TestBusinessSender_Send_Success(t *testing.T) {
	var receivedBody map[string]interface{}
	var receivedAuth string
	var receivedContentType string
	var receivedPath string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		receivedAuth = r.Header.Get("Authorization")
		receivedContentType = r.Header.Get("Content-Type")

		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &receivedBody)

		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"messages":[{"id":"wamid.abc"}]}`))
	}))
	defer srv.Close()

	s := NewBusinessSender("phone123", "my_token", "v21.0")
	s.BaseURL = srv.URL

	err := s.Send("41791234567", "Hello, World!")
	if err != nil {
		t.Fatalf("Send() error: %v", err)
	}

	// Verify URL path.
	expectedPath := "/v21.0/phone123/messages"
	if receivedPath != expectedPath {
		t.Errorf("path = %q, want %q", receivedPath, expectedPath)
	}

	// Verify headers.
	if receivedAuth != "Bearer my_token" {
		t.Errorf("Authorization = %q, want %q", receivedAuth, "Bearer my_token")
	}
	if receivedContentType != "application/json" {
		t.Errorf("Content-Type = %q, want %q", receivedContentType, "application/json")
	}

	// Verify body fields.
	if receivedBody["messaging_product"] != "whatsapp" {
		t.Errorf("messaging_product = %v, want %q", receivedBody["messaging_product"], "whatsapp")
	}
	if receivedBody["to"] != "41791234567" {
		t.Errorf("to = %v, want %q", receivedBody["to"], "41791234567")
	}
	if receivedBody["type"] != "text" {
		t.Errorf("type = %v, want %q", receivedBody["type"], "text")
	}
	textMap, ok := receivedBody["text"].(map[string]interface{})
	if !ok {
		t.Fatal("text field is not a map")
	}
	if textMap["body"] != "Hello, World!" {
		t.Errorf("text.body = %v, want %q", textMap["body"], "Hello, World!")
	}
}

func TestBusinessSender_Send_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		_, _ = w.Write([]byte(`{"error":{"message":"invalid phone"}}`))
	}))
	defer srv.Close()

	s := NewBusinessSender("phone123", "my_token", "v21.0")
	s.BaseURL = srv.URL

	err := s.Send("bad_phone", "Hello")
	if err == nil {
		t.Fatal("expected error for 400 response")
	}
	if !strings.Contains(err.Error(), "400") {
		t.Errorf("error = %q, want it to contain '400'", err.Error())
	}
	if !strings.Contains(err.Error(), "invalid phone") {
		t.Errorf("error = %q, want it to contain response body", err.Error())
	}
}

func TestBusinessSender_Send_ConnectionError(t *testing.T) {
	s := NewBusinessSender("phone123", "my_token", "v21.0")
	s.BaseURL = "http://127.0.0.1:1" // port 1 should be unreachable

	err := s.Send("41791234567", "Hello")
	if err == nil {
		t.Fatal("expected error for connection failure")
	}
	if !strings.Contains(err.Error(), "sending request") {
		t.Errorf("error = %q, want it to contain 'sending request'", err.Error())
	}
}
