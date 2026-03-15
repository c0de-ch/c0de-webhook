package webhook

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"c0de-webhook/internal/auth"
	"c0de-webhook/internal/store"
)

func setupTest(t *testing.T) (*store.Store, *auth.Auth, *Handler, string) {
	t.Helper()
	st, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	a := auth.New(st, "admin", "secret")
	h := NewHandler(st, a, 3)

	rawToken, _, err := st.CreateToken("test")
	if err != nil {
		t.Fatalf("failed to create token: %v", err)
	}

	return st, a, h, rawToken
}

func sendRequest(t *testing.T, handler http.Handler, method, path, body, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	return rr
}

func TestHandleSend_Success(t *testing.T) {
	_, _, h, rawToken := setupTest(t)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	body := `{"to":"user@example.com","subject":"Hello","text":"Hello World"}`
	rr := sendRequest(t, mux, "POST", "/api/v1/send", body, rawToken)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected status 202, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp SendResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.ID == 0 {
		t.Error("expected non-zero message ID")
	}
	if resp.Status != "queued" {
		t.Errorf("expected status 'queued', got %q", resp.Status)
	}

	ct := rr.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected Content-Type 'application/json', got %q", ct)
	}
}

func TestHandleSend_WithHTML(t *testing.T) {
	_, _, h, rawToken := setupTest(t)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	body := `{"to":"user@example.com","subject":"Hello","text":"plain text","html":"<h1>Hello</h1>"}`
	rr := sendRequest(t, mux, "POST", "/api/v1/send", body, rawToken)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected status 202, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp SendResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Status != "queued" {
		t.Errorf("expected status 'queued', got %q", resp.Status)
	}
}

func TestHandleSend_HTMLOnly(t *testing.T) {
	_, _, h, rawToken := setupTest(t)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	body := `{"to":"user@example.com","subject":"Hello","html":"<h1>Hello</h1>"}`
	rr := sendRequest(t, mux, "POST", "/api/v1/send", body, rawToken)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected status 202, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp SendResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Status != "queued" {
		t.Errorf("expected status 'queued', got %q", resp.Status)
	}
}

func TestHandleSend_NoAuth(t *testing.T) {
	_, _, h, _ := setupTest(t)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	body := `{"to":"user@example.com","subject":"Hello","text":"Hello World"}`
	rr := sendRequest(t, mux, "POST", "/api/v1/send", body, "")

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp ErrorResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Error == "" {
		t.Error("expected non-empty error message")
	}
}

func TestHandleSend_InvalidToken(t *testing.T) {
	_, _, h, _ := setupTest(t)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	body := `{"to":"user@example.com","subject":"Hello","text":"Hello World"}`
	rr := sendRequest(t, mux, "POST", "/api/v1/send", body, "invalidtoken")

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp ErrorResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Error == "" {
		t.Error("expected non-empty error message")
	}
}

func TestHandleSend_InvalidJSON(t *testing.T) {
	_, _, h, rawToken := setupTest(t)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	body := `{this is not valid json}`
	rr := sendRequest(t, mux, "POST", "/api/v1/send", body, rawToken)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp ErrorResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !strings.Contains(resp.Error, "invalid JSON") {
		t.Errorf("expected error to contain 'invalid JSON', got %q", resp.Error)
	}
}

func TestHandleSend_MissingTo(t *testing.T) {
	_, _, h, rawToken := setupTest(t)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	body := `{"subject":"Hello","text":"Hello World"}`
	rr := sendRequest(t, mux, "POST", "/api/v1/send", body, rawToken)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp ErrorResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !strings.Contains(resp.Error, "\"to\" is required") {
		t.Errorf("expected error to mention '\"to\" is required', got %q", resp.Error)
	}
}

func TestHandleSend_MissingSubject(t *testing.T) {
	_, _, h, rawToken := setupTest(t)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	body := `{"to":"user@example.com","text":"Hello World"}`
	rr := sendRequest(t, mux, "POST", "/api/v1/send", body, rawToken)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp ErrorResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !strings.Contains(resp.Error, "\"subject\" is required") {
		t.Errorf("expected error to mention '\"subject\" is required', got %q", resp.Error)
	}
}

func TestHandleSend_MissingBody(t *testing.T) {
	_, _, h, rawToken := setupTest(t)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	body := `{"to":"user@example.com","subject":"Hello"}`
	rr := sendRequest(t, mux, "POST", "/api/v1/send", body, rawToken)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp ErrorResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !strings.Contains(resp.Error, "\"text\" or \"html\" is required") {
		t.Errorf("expected error to mention '\"text\" or \"html\" is required', got %q", resp.Error)
	}
}

func TestHandleSend_MessageEnqueued(t *testing.T) {
	st, _, h, rawToken := setupTest(t)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	body := `{"to":"enqueue@example.com","subject":"Enqueue Test","text":"Check storage"}`
	rr := sendRequest(t, mux, "POST", "/api/v1/send", body, rawToken)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected status 202, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp SendResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Verify the message was actually persisted in the store
	msgs, total, err := st.ListMessages("queued", 100, 0)
	if err != nil {
		t.Fatalf("failed to list messages: %v", err)
	}
	if total == 0 {
		t.Fatal("expected at least one queued message, got zero")
	}

	var found bool
	for _, m := range msgs {
		if m.ID == resp.ID {
			found = true
			if m.To != "enqueue@example.com" {
				t.Errorf("expected To='enqueue@example.com', got %q", m.To)
			}
			if m.Subject != "Enqueue Test" {
				t.Errorf("expected Subject='Enqueue Test', got %q", m.Subject)
			}
			if m.Status != "queued" {
				t.Errorf("expected Status='queued', got %q", m.Status)
			}
			break
		}
	}
	if !found {
		t.Errorf("message with ID %d not found in store", resp.ID)
	}
}

func TestHandleHealth(t *testing.T) {
	_, _, h, _ := setupTest(t)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest("GET", "/api/v1/health", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["status"] != "ok" {
		t.Errorf("expected status 'ok', got %q", resp["status"])
	}

	ct := rr.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected Content-Type 'application/json', got %q", ct)
	}
}

func TestRouteRegistration(t *testing.T) {
	_, _, h, rawToken := setupTest(t)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	// Verify POST /api/v1/send is handled (not 404/405)
	body := `{"to":"user@example.com","subject":"Test","text":"body"}`
	rr := sendRequest(t, mux, "POST", "/api/v1/send", body, rawToken)
	if rr.Code == http.StatusNotFound || rr.Code == http.StatusMethodNotAllowed {
		t.Errorf("POST /api/v1/send should be registered, got status %d", rr.Code)
	}

	// Verify GET /api/v1/health is handled (not 404/405)
	req := httptest.NewRequest("GET", "/api/v1/health", nil)
	rr2 := httptest.NewRecorder()
	mux.ServeHTTP(rr2, req)
	if rr2.Code == http.StatusNotFound || rr2.Code == http.StatusMethodNotAllowed {
		t.Errorf("GET /api/v1/health should be registered, got status %d", rr2.Code)
	}

	// Verify wrong method on /api/v1/send is rejected
	reqGet := httptest.NewRequest("GET", "/api/v1/send", nil)
	rr3 := httptest.NewRecorder()
	mux.ServeHTTP(rr3, reqGet)
	if rr3.Code == http.StatusOK || rr3.Code == http.StatusAccepted {
		t.Error("GET /api/v1/send should not return a success status")
	}
}

// --- WhatsApp v2 endpoint tests ---

func TestHandleWhatsAppSend_Success(t *testing.T) {
	_, _, h, rawToken := setupTest(t)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	body := `{"phone":"+1234567890","text":"Hello via WhatsApp"}`
	rr := sendRequest(t, mux, "POST", "/api/v2/whatsapp", body, rawToken)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected status 202, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp SendResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.ID == 0 {
		t.Error("expected non-zero message ID")
	}
	if resp.Status != "queued" {
		t.Errorf("expected status 'queued', got %q", resp.Status)
	}
	if resp.Channel != "whatsapp" {
		t.Errorf("expected channel 'whatsapp', got %q", resp.Channel)
	}
}

func TestHandleWhatsAppSend_MissingPhone(t *testing.T) {
	_, _, h, rawToken := setupTest(t)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	body := `{"text":"Hello via WhatsApp"}`
	rr := sendRequest(t, mux, "POST", "/api/v2/whatsapp", body, rawToken)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp ErrorResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !strings.Contains(resp.Error, "\"phone\" is required") {
		t.Errorf("expected error to mention '\"phone\" is required', got %q", resp.Error)
	}
}

func TestHandleWhatsAppSend_MissingText(t *testing.T) {
	_, _, h, rawToken := setupTest(t)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	body := `{"phone":"+1234567890"}`
	rr := sendRequest(t, mux, "POST", "/api/v2/whatsapp", body, rawToken)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp ErrorResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !strings.Contains(resp.Error, "\"text\" is required") {
		t.Errorf("expected error to mention '\"text\" is required', got %q", resp.Error)
	}
}

func TestHandleWhatsAppSend_NoAuth(t *testing.T) {
	_, _, h, _ := setupTest(t)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	body := `{"phone":"+1234567890","text":"Hello via WhatsApp"}`
	rr := sendRequest(t, mux, "POST", "/api/v2/whatsapp", body, "")

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp ErrorResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Error == "" {
		t.Error("expected non-empty error message")
	}
}

// --- Telegram v2 endpoint tests ---

func TestHandleTelegramSend_Success(t *testing.T) {
	_, _, h, rawToken := setupTest(t)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	body := `{"chat_id":"123456789","text":"Hello via Telegram"}`
	rr := sendRequest(t, mux, "POST", "/api/v2/telegram", body, rawToken)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected status 202, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp SendResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.ID == 0 {
		t.Error("expected non-zero message ID")
	}
	if resp.Status != "queued" {
		t.Errorf("expected status 'queued', got %q", resp.Status)
	}
	if resp.Channel != "telegram" {
		t.Errorf("expected channel 'telegram', got %q", resp.Channel)
	}
}

func TestHandleTelegramSend_MissingChatID(t *testing.T) {
	_, _, h, rawToken := setupTest(t)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	body := `{"text":"Hello via Telegram"}`
	rr := sendRequest(t, mux, "POST", "/api/v2/telegram", body, rawToken)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp ErrorResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !strings.Contains(resp.Error, "\"chat_id\" is required") {
		t.Errorf("expected error to mention '\"chat_id\" is required', got %q", resp.Error)
	}
}

func TestHandleTelegramSend_MissingText(t *testing.T) {
	_, _, h, rawToken := setupTest(t)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	body := `{"chat_id":"123456789"}`
	rr := sendRequest(t, mux, "POST", "/api/v2/telegram", body, rawToken)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp ErrorResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !strings.Contains(resp.Error, "\"text\" is required") {
		t.Errorf("expected error to mention '\"text\" is required', got %q", resp.Error)
	}
}

// --- v2 mail endpoint test ---

func TestHandleV2MailSend_Success(t *testing.T) {
	_, _, h, rawToken := setupTest(t)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	body := `{"to":"user@example.com","subject":"V2 Mail","text":"Hello from v2"}`
	rr := sendRequest(t, mux, "POST", "/api/v2/mail", body, rawToken)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected status 202, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp SendResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.ID == 0 {
		t.Error("expected non-zero message ID")
	}
	if resp.Status != "queued" {
		t.Errorf("expected status 'queued', got %q", resp.Status)
	}
	if resp.Channel != "mail" {
		t.Errorf("expected channel 'mail', got %q", resp.Channel)
	}
}

// --- v2 route registration test ---

func TestV2RouteRegistration(t *testing.T) {
	_, _, h, rawToken := setupTest(t)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	// Verify POST /api/v2/mail is registered
	mailBody := `{"to":"user@example.com","subject":"Test","text":"body"}`
	rr := sendRequest(t, mux, "POST", "/api/v2/mail", mailBody, rawToken)
	if rr.Code == http.StatusNotFound || rr.Code == http.StatusMethodNotAllowed {
		t.Errorf("POST /api/v2/mail should be registered, got status %d", rr.Code)
	}

	// Verify POST /api/v2/whatsapp is registered
	waBody := `{"phone":"+1234567890","text":"test"}`
	rr2 := sendRequest(t, mux, "POST", "/api/v2/whatsapp", waBody, rawToken)
	if rr2.Code == http.StatusNotFound || rr2.Code == http.StatusMethodNotAllowed {
		t.Errorf("POST /api/v2/whatsapp should be registered, got status %d", rr2.Code)
	}

	// Verify POST /api/v2/telegram is registered
	tgBody := `{"chat_id":"123","text":"test"}`
	rr3 := sendRequest(t, mux, "POST", "/api/v2/telegram", tgBody, rawToken)
	if rr3.Code == http.StatusNotFound || rr3.Code == http.StatusMethodNotAllowed {
		t.Errorf("POST /api/v2/telegram should be registered, got status %d", rr3.Code)
	}

	// Verify GET /api/v2/health is registered
	req := httptest.NewRequest("GET", "/api/v2/health", nil)
	rr4 := httptest.NewRecorder()
	mux.ServeHTTP(rr4, req)
	if rr4.Code == http.StatusNotFound || rr4.Code == http.StatusMethodNotAllowed {
		t.Errorf("GET /api/v2/health should be registered, got status %d", rr4.Code)
	}
}

func TestHandleWhatsAppSend_InvalidJSON(t *testing.T) {
	_, _, h, rawToken := setupTest(t)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	rr := sendRequest(t, mux, "POST", "/api/v2/whatsapp", `not json`, rawToken)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestHandleTelegramSend_InvalidJSON(t *testing.T) {
	_, _, h, rawToken := setupTest(t)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	rr := sendRequest(t, mux, "POST", "/api/v2/telegram", `not json`, rawToken)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestHandleTelegramSend_NoAuth(t *testing.T) {
	_, _, h, _ := setupTest(t)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	rr := sendRequest(t, mux, "POST", "/api/v2/telegram", `{"chat_id":"123","text":"hi"}`, "")
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestHandleV2MailSend_InvalidJSON(t *testing.T) {
	_, _, h, rawToken := setupTest(t)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	rr := sendRequest(t, mux, "POST", "/api/v2/mail", `{broken`, rawToken)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}
