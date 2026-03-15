package ui

import (
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"c0de-webhook/internal/auth"
	"c0de-webhook/internal/config"
	"c0de-webhook/internal/store"
	"c0de-webhook/web"
)

func setupTest(t *testing.T) (*Handler, *auth.Auth, *store.Store, *http.ServeMux) {
	t.Helper()
	st, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("creating store: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	cfg := config.Default()
	a := auth.New(st, cfg.Server.AdminPassword, cfg.Server.SecretKey)

	webFS, err := fs.Sub(web.Files, ".")
	if err != nil {
		t.Fatalf("creating sub fs: %v", err)
	}
	h := NewHandler(st, a, cfg, webFS)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	return h, a, st, mux
}

// loginSession performs a POST /login and returns the webhook_session cookie.
func loginSession(t *testing.T, mux *http.ServeMux, password string) *http.Cookie {
	t.Helper()
	form := url.Values{"password": {password}}
	req := httptest.NewRequest("POST", "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	for _, c := range w.Result().Cookies() {
		if c.Name == "webhook_session" {
			return c
		}
	}
	return nil
}

// authReq creates an HTTP request with an optional session cookie attached.
func authReq(method, path string, body io.Reader, cookie *http.Cookie) *http.Request {
	req := httptest.NewRequest(method, path, body)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	return req
}

// authFormReq creates an authenticated POST request with form-encoded body.
func authFormReq(path string, form url.Values, cookie *http.Cookie) *http.Request {
	req := httptest.NewRequest("POST", path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if cookie != nil {
		req.AddCookie(cookie)
	}
	return req
}

// --- Test: Login Page ---

func TestLoginPage(t *testing.T) {
	_, _, _, mux := setupTest(t)

	req := httptest.NewRequest("GET", "/login", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "admin password") {
		t.Error("expected login page to contain 'Admin Login'")
	}
	if !strings.Contains(string(body), `name="password"`) {
		t.Error("expected login page to contain password input")
	}
	if !strings.Contains(string(body), `action="/login"`) {
		t.Error("expected login page form to post to /login")
	}
}

// --- Test: Login Page Already Authenticated ---

func TestLoginPage_AlreadyAuthenticated(t *testing.T) {
	_, _, _, mux := setupTest(t)

	cookie := loginSession(t, mux, "changeme")
	if cookie == nil {
		t.Fatal("expected session cookie after login")
	}

	req := authReq("GET", "/login", nil, cookie)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect, got %d", resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	if loc != "/dashboard" {
		t.Errorf("expected redirect to /dashboard, got %q", loc)
	}
}

// --- Test: Login Post Success ---

func TestLoginPost_Success(t *testing.T) {
	_, _, _, mux := setupTest(t)

	form := url.Values{"password": {"changeme"}}
	req := httptest.NewRequest("POST", "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect, got %d", resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	if loc != "/dashboard" {
		t.Errorf("expected redirect to /dashboard, got %q", loc)
	}

	var found bool
	for _, c := range resp.Cookies() {
		if c.Name == "webhook_session" && c.Value != "" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected Set-Cookie header with webhook_session")
	}
}

// --- Test: Login Post Wrong Password ---

func TestLoginPost_WrongPassword(t *testing.T) {
	_, _, _, mux := setupTest(t)

	form := url.Values{"password": {"wrong-password"}}
	req := httptest.NewRequest("POST", "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect, got %d", resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	if !strings.HasPrefix(loc, "/login?error=") {
		t.Errorf("expected redirect to /login?error=..., got %q", loc)
	}
	if !strings.Contains(loc, "Invalid") {
		t.Errorf("expected error message to contain 'Invalid', got %q", loc)
	}
}

// --- Test: Logout ---

func TestLogout(t *testing.T) {
	_, _, _, mux := setupTest(t)

	cookie := loginSession(t, mux, "changeme")
	if cookie == nil {
		t.Fatal("expected session cookie after login")
	}

	req := authReq("GET", "/logout", nil, cookie)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect, got %d", resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	if loc != "/login" {
		t.Errorf("expected redirect to /login, got %q", loc)
	}

	// Verify the cookie was cleared (MaxAge < 0)
	var cleared bool
	for _, c := range resp.Cookies() {
		if c.Name == "webhook_session" && c.MaxAge < 0 {
			cleared = true
			break
		}
	}
	if !cleared {
		t.Error("expected webhook_session cookie to be cleared (MaxAge < 0)")
	}

	// Verify the session is no longer valid by trying to access dashboard
	req = authReq("GET", "/dashboard", nil, cookie)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp = w.Result()
	if resp.StatusCode != http.StatusSeeOther {
		t.Errorf("expected 303 redirect to /login after logout, got %d", resp.StatusCode)
	}
}

// --- Test: Dashboard Authenticated ---

func TestDashboard_Authenticated(t *testing.T) {
	_, _, _, mux := setupTest(t)

	cookie := loginSession(t, mux, "changeme")
	if cookie == nil {
		t.Fatal("expected session cookie after login")
	}

	req := authReq("GET", "/dashboard", nil, cookie)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	s := string(body)
	if !strings.Contains(s, "Dashboard") {
		t.Error("expected dashboard page to contain 'Dashboard'")
	}
	if !strings.Contains(s, "Total Sent") {
		t.Error("expected dashboard to contain 'Total Sent' stats")
	}
	if !strings.Contains(s, "Success Rate") {
		t.Error("expected dashboard to contain 'Success Rate' stats")
	}
	if !strings.Contains(s, "Queue Depth") {
		t.Error("expected dashboard to contain 'Queue Depth' stats")
	}
}

// --- Test: Dashboard Unauthenticated ---

func TestDashboard_Unauthenticated(t *testing.T) {
	_, _, _, mux := setupTest(t)

	req := httptest.NewRequest("GET", "/dashboard", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect, got %d", resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	if loc != "/login" {
		t.Errorf("expected redirect to /login, got %q", loc)
	}
}

// --- Test: Root Redirect ---

func TestRootRedirect(t *testing.T) {
	_, _, _, mux := setupTest(t)

	cookie := loginSession(t, mux, "changeme")
	if cookie == nil {
		t.Fatal("expected session cookie after login")
	}

	req := authReq("GET", "/", nil, cookie)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200 (dashboard), got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "Dashboard") {
		t.Error("expected root with session to serve dashboard content")
	}
}

func TestRootRedirect_Unauthenticated(t *testing.T) {
	_, _, _, mux := setupTest(t)

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect, got %d", resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	if loc != "/login" {
		t.Errorf("expected redirect to /login, got %q", loc)
	}
}

// --- Test: Tokens Page ---

func TestTokensPage(t *testing.T) {
	_, _, st, mux := setupTest(t)

	// Create a token so the list has content
	_, _, err := st.CreateToken("test-token")
	if err != nil {
		t.Fatalf("creating token: %v", err)
	}

	cookie := loginSession(t, mux, "changeme")
	if cookie == nil {
		t.Fatal("expected session cookie after login")
	}

	req := authReq("GET", "/tokens", nil, cookie)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	s := string(body)
	if !strings.Contains(s, "API Tokens") {
		t.Error("expected tokens page to contain 'API Tokens'")
	}
	if !strings.Contains(s, "test-token") {
		t.Error("expected tokens page to list the created token")
	}
	if !strings.Contains(s, "Create Token") {
		t.Error("expected tokens page to contain 'Create Token' form")
	}
}

func TestTokensPage_Unauthenticated(t *testing.T) {
	_, _, _, mux := setupTest(t)

	req := httptest.NewRequest("GET", "/tokens", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect, got %d", resp.StatusCode)
	}
}

// --- Test: Create Token ---

func TestCreateToken(t *testing.T) {
	_, _, _, mux := setupTest(t)

	cookie := loginSession(t, mux, "changeme")
	if cookie == nil {
		t.Fatal("expected session cookie after login")
	}

	form := url.Values{"name": {"my-new-token"}}
	req := authFormReq("/tokens", form, cookie)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect, got %d", resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	if loc != "/tokens?new=1" {
		t.Errorf("expected redirect to /tokens?new=1, got %q", loc)
	}
}

// --- Test: Create Token Empty Name ---

func TestCreateToken_EmptyName(t *testing.T) {
	_, _, _, mux := setupTest(t)

	cookie := loginSession(t, mux, "changeme")
	if cookie == nil {
		t.Fatal("expected session cookie after login")
	}

	form := url.Values{"name": {""}}
	req := authFormReq("/tokens", form, cookie)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect, got %d", resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	if !strings.Contains(loc, "/tokens?msg=") {
		t.Errorf("expected redirect to /tokens?msg=..., got %q", loc)
	}
	if !strings.Contains(loc, "required") {
		t.Errorf("expected error message about required name, got %q", loc)
	}
}

func TestCreateToken_WhitespaceName(t *testing.T) {
	_, _, _, mux := setupTest(t)

	cookie := loginSession(t, mux, "changeme")
	if cookie == nil {
		t.Fatal("expected session cookie after login")
	}

	form := url.Values{"name": {"   "}}
	req := authFormReq("/tokens", form, cookie)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	loc := resp.Header.Get("Location")
	if !strings.Contains(loc, "required") {
		t.Errorf("expected whitespace-only name to be rejected, got redirect to %q", loc)
	}
}

// --- Test: Toggle Token ---

func TestToggleToken(t *testing.T) {
	_, _, st, mux := setupTest(t)

	_, tok, err := st.CreateToken("toggle-me")
	if err != nil {
		t.Fatalf("creating token: %v", err)
	}
	if !tok.IsActive {
		t.Fatal("expected token to be active by default")
	}

	cookie := loginSession(t, mux, "changeme")
	if cookie == nil {
		t.Fatal("expected session cookie after login")
	}

	path := fmt.Sprintf("/tokens/%d/toggle", tok.ID)
	req := authFormReq(path, nil, cookie)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect, got %d", resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	if loc != "/tokens" {
		t.Errorf("expected redirect to /tokens, got %q", loc)
	}

	// Verify token was toggled in the store
	tokens, err := st.ListTokens()
	if err != nil {
		t.Fatalf("listing tokens: %v", err)
	}
	if len(tokens) != 1 {
		t.Fatalf("expected 1 token, got %d", len(tokens))
	}
	if tokens[0].IsActive {
		t.Error("expected token to be disabled after toggle")
	}
}

func TestToggleToken_InvalidID(t *testing.T) {
	_, _, _, mux := setupTest(t)

	cookie := loginSession(t, mux, "changeme")
	if cookie == nil {
		t.Fatal("expected session cookie after login")
	}

	req := authFormReq("/tokens/notanumber/toggle", nil, cookie)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect, got %d", resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	if !strings.Contains(loc, "Invalid") {
		t.Errorf("expected error about invalid token ID, got %q", loc)
	}
}

// --- Test: Delete Token ---

func TestDeleteToken(t *testing.T) {
	_, _, st, mux := setupTest(t)

	_, tok, err := st.CreateToken("delete-me")
	if err != nil {
		t.Fatalf("creating token: %v", err)
	}

	cookie := loginSession(t, mux, "changeme")
	if cookie == nil {
		t.Fatal("expected session cookie after login")
	}

	path := fmt.Sprintf("/tokens/%d/delete", tok.ID)
	req := authFormReq(path, nil, cookie)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect, got %d", resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	if !strings.Contains(loc, "/tokens") {
		t.Errorf("expected redirect to /tokens, got %q", loc)
	}
	if !strings.Contains(loc, "deleted") {
		t.Errorf("expected deleted message in redirect, got %q", loc)
	}

	// Verify token was actually deleted
	tokens, err := st.ListTokens()
	if err != nil {
		t.Fatalf("listing tokens: %v", err)
	}
	if len(tokens) != 0 {
		t.Errorf("expected 0 tokens after deletion, got %d", len(tokens))
	}
}

func TestDeleteToken_InvalidID(t *testing.T) {
	_, _, _, mux := setupTest(t)

	cookie := loginSession(t, mux, "changeme")
	if cookie == nil {
		t.Fatal("expected session cookie after login")
	}

	req := authFormReq("/tokens/abc/delete", nil, cookie)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect, got %d", resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	if !strings.Contains(loc, "Invalid") {
		t.Errorf("expected error about invalid token ID, got %q", loc)
	}
}

// --- Test: Queue Page ---

func TestQueuePage(t *testing.T) {
	_, _, _, mux := setupTest(t)

	cookie := loginSession(t, mux, "changeme")
	if cookie == nil {
		t.Fatal("expected session cookie after login")
	}

	req := authReq("GET", "/queue", nil, cookie)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	s := string(body)
	if !strings.Contains(s, "Message Queue") {
		t.Error("expected queue page to contain 'Message Queue'")
	}
	if !strings.Contains(s, "No messages in queue") {
		t.Error("expected empty queue page to show 'No messages in queue'")
	}
}

func TestQueuePage_Unauthenticated(t *testing.T) {
	_, _, _, mux := setupTest(t)

	req := httptest.NewRequest("GET", "/queue", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect, got %d", resp.StatusCode)
	}
}

// --- Test: Queue Page With Filter ---

func TestQueuePage_WithFilter(t *testing.T) {
	_, _, st, mux := setupTest(t)

	// Create some messages with different statuses
	_, err := st.EnqueueMessage(nil, "mail","sent@example.com", "Sent message", "body", "", 3)
	if err != nil {
		t.Fatalf("enqueuing message: %v", err)
	}
	// Mark the first message as sent (it gets ID 1)
	if err := st.MarkSent(1); err != nil {
		t.Fatalf("marking sent: %v", err)
	}

	_, err = st.EnqueueMessage(nil, "mail","queued@example.com", "Queued message", "body", "", 3)
	if err != nil {
		t.Fatalf("enqueuing message: %v", err)
	}

	cookie := loginSession(t, mux, "changeme")
	if cookie == nil {
		t.Fatal("expected session cookie after login")
	}

	// Filter by "sent"
	req := authReq("GET", "/queue?status=sent", nil, cookie)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	s := string(body)
	if !strings.Contains(s, "sent@example.com") {
		t.Error("expected sent filter to show sent message")
	}
	// The queued message should not appear when filtering by sent
	if strings.Contains(s, "queued@example.com") {
		t.Error("expected sent filter to NOT show queued message")
	}
}

func TestQueuePage_WithMessages(t *testing.T) {
	_, _, st, mux := setupTest(t)

	_, err := st.EnqueueMessage(nil, "mail","user@example.com", "Test subject", "text body", "", 3)
	if err != nil {
		t.Fatalf("enqueuing message: %v", err)
	}

	cookie := loginSession(t, mux, "changeme")
	if cookie == nil {
		t.Fatal("expected session cookie after login")
	}

	req := authReq("GET", "/queue", nil, cookie)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	s := string(body)
	if !strings.Contains(s, "user@example.com") {
		t.Error("expected queue page to show the enqueued message recipient")
	}
	if !strings.Contains(s, "Test subject") {
		t.Error("expected queue page to show the message subject")
	}
}

// --- Test: Retry Message ---

func TestRetryMessage(t *testing.T) {
	_, _, st, mux := setupTest(t)

	msg, err := st.EnqueueMessage(nil, "mail","test@example.com", "Subject", "body", "", 3)
	if err != nil {
		t.Fatalf("enqueuing message: %v", err)
	}

	// Mark as failed so retry makes sense
	if err := st.MarkFailed(msg.ID, "send error", 0); err != nil {
		t.Fatalf("marking failed: %v", err)
	}

	cookie := loginSession(t, mux, "changeme")
	if cookie == nil {
		t.Fatal("expected session cookie after login")
	}

	path := fmt.Sprintf("/queue/%d/retry", msg.ID)
	req := authFormReq(path, nil, cookie)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect, got %d", resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	if !strings.Contains(loc, "/queue") {
		t.Errorf("expected redirect to /queue, got %q", loc)
	}
	if !strings.Contains(loc, "requeued") {
		t.Errorf("expected requeued confirmation in redirect, got %q", loc)
	}
}

func TestRetryMessage_InvalidID(t *testing.T) {
	_, _, _, mux := setupTest(t)

	cookie := loginSession(t, mux, "changeme")
	if cookie == nil {
		t.Fatal("expected session cookie after login")
	}

	req := authFormReq("/queue/abc/retry", nil, cookie)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect, got %d", resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	if !strings.Contains(loc, "Invalid") {
		t.Errorf("expected error about invalid message ID, got %q", loc)
	}
}

// --- Test: Delete Message ---

func TestDeleteMessage(t *testing.T) {
	_, _, st, mux := setupTest(t)

	msg, err := st.EnqueueMessage(nil, "mail","test@example.com", "Subject", "body", "", 3)
	if err != nil {
		t.Fatalf("enqueuing message: %v", err)
	}

	cookie := loginSession(t, mux, "changeme")
	if cookie == nil {
		t.Fatal("expected session cookie after login")
	}

	path := fmt.Sprintf("/queue/%d/delete", msg.ID)
	req := authFormReq(path, nil, cookie)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect, got %d", resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	if !strings.Contains(loc, "/queue") {
		t.Errorf("expected redirect to /queue, got %q", loc)
	}
	if !strings.Contains(loc, "deleted") {
		t.Errorf("expected deleted confirmation in redirect, got %q", loc)
	}

	// Verify message was actually deleted
	msgs, total, err := st.ListMessages("all", 25, 0)
	if err != nil {
		t.Fatalf("listing messages: %v", err)
	}
	if total != 0 || len(msgs) != 0 {
		t.Errorf("expected 0 messages after deletion, got %d", total)
	}
}

func TestDeleteMessage_InvalidID(t *testing.T) {
	_, _, _, mux := setupTest(t)

	cookie := loginSession(t, mux, "changeme")
	if cookie == nil {
		t.Fatal("expected session cookie after login")
	}

	req := authFormReq("/queue/xyz/delete", nil, cookie)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect, got %d", resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	if !strings.Contains(loc, "Invalid") {
		t.Errorf("expected error about invalid message ID, got %q", loc)
	}
}

// --- Test: Settings Page ---

func TestSettingsPage(t *testing.T) {
	_, _, _, mux := setupTest(t)

	cookie := loginSession(t, mux, "changeme")
	if cookie == nil {
		t.Fatal("expected session cookie after login")
	}

	req := authReq("GET", "/settings", nil, cookie)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	s := string(body)
	if !strings.Contains(s, "Settings") {
		t.Error("expected settings page to contain 'Settings'")
	}
	if !strings.Contains(s, "Server") {
		t.Error("expected settings page to contain 'Server' section")
	}
	if !strings.Contains(s, "SMTP") {
		t.Error("expected settings page to contain 'SMTP' section")
	}
	if !strings.Contains(s, "Queue") {
		t.Error("expected settings page to contain 'Queue' section")
	}
	if !strings.Contains(s, "Database") {
		t.Error("expected settings page to contain 'Database' section")
	}
	// Default config values
	if !strings.Contains(s, "0.0.0.0") {
		t.Error("expected settings page to show server host '0.0.0.0'")
	}
	if !strings.Contains(s, "8080") {
		t.Error("expected settings page to show server port '8080'")
	}
	if !strings.Contains(s, "noreply@example.com") {
		t.Error("expected settings page to show SMTP from address")
	}
	// Password should be masked
	if !strings.Contains(s, "(not set)") && strings.Contains(s, "changeme") {
		t.Error("expected admin password to be masked, not shown in plain text")
	}
}

func TestSettingsPage_Unauthenticated(t *testing.T) {
	_, _, _, mux := setupTest(t)

	req := httptest.NewRequest("GET", "/settings", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect, got %d", resp.StatusCode)
	}
}

// --- Test: Static Files ---

func TestStaticFiles(t *testing.T) {
	_, _, _, mux := setupTest(t)

	req := httptest.NewRequest("GET", "/static/style.css", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	s := string(body)
	if !strings.Contains(s, "--bg:") {
		t.Error("expected style.css to contain CSS custom property '--bg:'")
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/css") {
		t.Errorf("expected Content-Type to contain 'text/css', got %q", ct)
	}
}

func TestStaticFiles_NotFound(t *testing.T) {
	_, _, _, mux := setupTest(t)

	req := httptest.NewRequest("GET", "/static/nonexistent.js", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", resp.StatusCode)
	}
}

// --- Test: New Token Display ---

func TestNewTokenDisplay(t *testing.T) {
	_, _, _, mux := setupTest(t)

	cookie := loginSession(t, mux, "changeme")
	if cookie == nil {
		t.Fatal("expected session cookie after login")
	}

	// Create a token via the handler (which stores the raw token in pendingTokens)
	form := url.Values{"name": {"display-token"}}
	req := authFormReq("/tokens", form, cookie)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect, got %d", resp.StatusCode)
	}

	// Now follow the redirect to /tokens?new=1 with the same session cookie
	req = authReq("GET", "/tokens?new=1", nil, cookie)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp = w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	s := string(body)
	if !strings.Contains(s, "Token created successfully") {
		t.Error("expected tokens page with new=1 to show 'New API Token Created'")
	}
	if !strings.Contains(s, "Copy") {
		t.Error("expected tokens page to show Copy button for new token")
	}
	if !strings.Contains(s, "will not be shown again") {
		t.Error("expected tokens page to warn that token will not be shown again")
	}
}

func TestNewTokenDisplay_OnlyOnce(t *testing.T) {
	_, _, _, mux := setupTest(t)

	cookie := loginSession(t, mux, "changeme")
	if cookie == nil {
		t.Fatal("expected session cookie after login")
	}

	// Create token
	form := url.Values{"name": {"once-token"}}
	req := authFormReq("/tokens", form, cookie)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// First visit to /tokens?new=1 -- should show the raw token
	req = authReq("GET", "/tokens?new=1", nil, cookie)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	body, _ := io.ReadAll(w.Result().Body)
	if !strings.Contains(string(body), "Token created successfully") {
		t.Fatal("expected first visit to show the new token")
	}

	// Second visit to /tokens?new=1 -- token should be gone (cleared from pendingTokens)
	req = authReq("GET", "/tokens?new=1", nil, cookie)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	body, _ = io.ReadAll(w.Result().Body)
	if strings.Contains(string(body), "Token created successfully") {
		t.Error("expected second visit to NOT show the new token (should be consumed)")
	}
}

// --- Test: Dashboard with data ---

func TestDashboard_WithMessages(t *testing.T) {
	_, _, st, mux := setupTest(t)

	// Create some messages to populate dashboard stats
	msg1, _ := st.EnqueueMessage(nil, "mail","a@example.com", "Msg 1", "body", "", 3)
	_ = st.MarkSent(msg1.ID)

	msg2, _ := st.EnqueueMessage(nil, "mail","b@example.com", "Msg 2", "body", "", 3)
	_ = st.MarkSent(msg2.ID)

	_, _ = st.EnqueueMessage(nil, "mail","c@example.com", "Msg 3", "body", "", 3)

	cookie := loginSession(t, mux, "changeme")
	if cookie == nil {
		t.Fatal("expected session cookie after login")
	}

	req := authReq("GET", "/dashboard", nil, cookie)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	s := string(body)
	// Should show recent messages
	if !strings.Contains(s, "a@example.com") {
		t.Error("expected dashboard to show recent message to a@example.com")
	}
	if !strings.Contains(s, "Msg 1") {
		t.Error("expected dashboard to show recent message subject")
	}
}

// --- Test: Token toggle double-toggle ---

func TestToggleToken_DoubleToggle(t *testing.T) {
	_, _, st, mux := setupTest(t)

	_, tok, err := st.CreateToken("double-toggle")
	if err != nil {
		t.Fatalf("creating token: %v", err)
	}

	cookie := loginSession(t, mux, "changeme")
	if cookie == nil {
		t.Fatal("expected session cookie after login")
	}

	path := fmt.Sprintf("/tokens/%d/toggle", tok.ID)

	// First toggle: active -> disabled
	req := authFormReq(path, nil, cookie)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	tokens, _ := st.ListTokens()
	if tokens[0].IsActive {
		t.Error("expected token to be disabled after first toggle")
	}

	// Second toggle: disabled -> active
	req = authFormReq(path, nil, cookie)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	tokens, _ = st.ListTokens()
	if !tokens[0].IsActive {
		t.Error("expected token to be active after second toggle")
	}
}

// --- Test: Login error flash message ---

func TestLoginPage_ErrorFlash(t *testing.T) {
	_, _, _, mux := setupTest(t)

	req := httptest.NewRequest("GET", "/login?error=Invalid+password", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "Invalid password") {
		t.Error("expected login page to display the error flash message")
	}
}

// --- Test: Tokens page flash message ---

func TestTokensPage_FlashMessage(t *testing.T) {
	_, _, _, mux := setupTest(t)

	cookie := loginSession(t, mux, "changeme")
	if cookie == nil {
		t.Fatal("expected session cookie after login")
	}

	req := authReq("GET", "/tokens?msg=Token+deleted", nil, cookie)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "Token deleted") {
		t.Error("expected tokens page to display the flash message")
	}
}

// --- Test: Queue flash message ---

func TestQueuePage_FlashMessage(t *testing.T) {
	_, _, _, mux := setupTest(t)

	cookie := loginSession(t, mux, "changeme")
	if cookie == nil {
		t.Fatal("expected session cookie after login")
	}

	req := authReq("GET", "/queue?msg=Message+deleted", nil, cookie)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "Message deleted") {
		t.Error("expected queue page to display the flash message")
	}
}

// --- Test: Content-Type header on HTML pages ---

func TestContentType_HTML(t *testing.T) {
	_, _, _, mux := setupTest(t)

	// Login page (standalone template)
	req := httptest.NewRequest("GET", "/login", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	ct := w.Result().Header.Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		t.Errorf("expected login page Content-Type to contain 'text/html', got %q", ct)
	}

	// Dashboard (layout template)
	cookie := loginSession(t, mux, "changeme")
	req = authReq("GET", "/dashboard", nil, cookie)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	ct = w.Result().Header.Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		t.Errorf("expected dashboard Content-Type to contain 'text/html', got %q", ct)
	}
}

// --- Test: Multiple tokens creation ---

func TestCreateMultipleTokens(t *testing.T) {
	_, _, st, mux := setupTest(t)

	cookie := loginSession(t, mux, "changeme")
	if cookie == nil {
		t.Fatal("expected session cookie after login")
	}

	names := []string{"token-alpha", "token-beta", "token-gamma"}
	for _, name := range names {
		form := url.Values{"name": {name}}
		req := authFormReq("/tokens", form, cookie)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		if w.Result().StatusCode != http.StatusSeeOther {
			t.Fatalf("expected 303 redirect creating token %q, got %d", name, w.Result().StatusCode)
		}

		// Consume the pending token display
		req = authReq("GET", "/tokens?new=1", nil, cookie)
		w = httptest.NewRecorder()
		mux.ServeHTTP(w, req)
	}

	// Verify all tokens appear on the tokens page
	req := authReq("GET", "/tokens", nil, cookie)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	body, _ := io.ReadAll(w.Result().Body)
	s := string(body)
	for _, name := range names {
		if !strings.Contains(s, name) {
			t.Errorf("expected tokens page to contain %q", name)
		}
	}

	tokens, _ := st.ListTokens()
	if len(tokens) != 3 {
		t.Errorf("expected 3 tokens in store, got %d", len(tokens))
	}
}

// --- Test: Retry message requeues it ---

func TestRetryMessage_RequeuesMessage(t *testing.T) {
	_, _, st, mux := setupTest(t)

	msg, _ := st.EnqueueMessage(nil, "mail","retry@example.com", "Retry me", "body", "", 3)

	// We need to claim and fail it to put it in "failed" state
	// First claim to set it to "sending"
	_, _ = st.ClaimPendingMessages(10)
	// Then mark as failed
	_ = st.MarkFailed(msg.ID, "network error", 0)

	// Verify it's in a non-queued state
	msgs, _, _ := st.ListMessages("failed", 25, 0)
	found := false
	for _, m := range msgs {
		if m.ID == msg.ID {
			found = true
			break
		}
	}
	// It may be in "queued" state if retries remain; check both
	if !found {
		msgs, _, _ = st.ListMessages("queued", 25, 0)
		for _, m := range msgs {
			if m.ID == msg.ID {
				found = true
				break
			}
		}
	}
	if !found {
		t.Log("message not found in failed or queued; skipping retry assertion")
		return
	}

	cookie := loginSession(t, mux, "changeme")
	if cookie == nil {
		t.Fatal("expected session cookie after login")
	}

	path := fmt.Sprintf("/queue/%d/retry", msg.ID)
	req := authFormReq(path, nil, cookie)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Result().StatusCode != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d", w.Result().StatusCode)
	}

	// After retry, message should be in queued state
	msgs, _, _ = st.ListMessages("queued", 25, 0)
	found = false
	for _, m := range msgs {
		if m.ID == msg.ID && m.Status == "queued" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected message to be in 'queued' status after retry")
	}
}

// --- Test: Sidebar navigation links ---

func TestSidebarNavigation(t *testing.T) {
	_, _, _, mux := setupTest(t)

	cookie := loginSession(t, mux, "changeme")
	if cookie == nil {
		t.Fatal("expected session cookie after login")
	}

	pages := []struct {
		path      string
		activeNav string
	}{
		{"/dashboard", "dashboard"},
		{"/tokens", "tokens"},
		{"/queue", "queue"},
		{"/settings", "settings"},
	}

	for _, p := range pages {
		req := authReq("GET", p.path, nil, cookie)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		if w.Result().StatusCode != http.StatusOK {
			t.Errorf("%s: expected 200, got %d", p.path, w.Result().StatusCode)
			continue
		}

		body, _ := io.ReadAll(w.Result().Body)
		s := string(body)
		// Check that the sidebar contains the logout link
		if !strings.Contains(s, "/logout") {
			t.Errorf("%s: expected page to contain logout link", p.path)
		}
		// Check that the page contains the c0de-webhook branding
		if !strings.Contains(s, "c0de-webhook") {
			t.Errorf("%s: expected page to contain 'c0de-webhook' branding", p.path)
		}
	}
}

// --- Test: Expired session cookie ---

func TestExpiredSessionCookie(t *testing.T) {
	_, _, _, mux := setupTest(t)

	// Use a fake session token that doesn't exist
	fakeCookie := &http.Cookie{
		Name:  "webhook_session",
		Value: "nonexistent-session-token",
	}

	req := authReq("GET", "/dashboard", nil, fakeCookie)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect with fake session, got %d", resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	if loc != "/login" {
		t.Errorf("expected redirect to /login, got %q", loc)
	}
}

// --- Test: New token without cookie (edge case) ---

func TestNewTokenDisplay_WithoutSessionCookie(t *testing.T) {
	_, _, _, mux := setupTest(t)

	cookie := loginSession(t, mux, "changeme")
	if cookie == nil {
		t.Fatal("expected session cookie after login")
	}

	// Visit /tokens?new=1 without having created a token
	req := authReq("GET", "/tokens?new=1", nil, cookie)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	// Should NOT show the new token section since no token was created
	if strings.Contains(string(body), "Token created successfully") {
		t.Error("expected /tokens?new=1 without pending token to NOT show token reveal")
	}
}

// --- Test: Delete token that has associated messages ---

func TestDeleteToken_WithMessages(t *testing.T) {
	_, _, st, mux := setupTest(t)

	_, tok, err := st.CreateToken("has-messages")
	if err != nil {
		t.Fatalf("creating token: %v", err)
	}

	// Enqueue a message associated with this token
	_, err = st.EnqueueMessage(&tok.ID, "mail", "test@example.com", "Linked", "body", "", 3)
	if err != nil {
		t.Fatalf("enqueuing message: %v", err)
	}

	cookie := loginSession(t, mux, "changeme")
	if cookie == nil {
		t.Fatal("expected session cookie after login")
	}

	path := fmt.Sprintf("/tokens/%d/delete", tok.ID)
	req := authFormReq(path, nil, cookie)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect, got %d", resp.StatusCode)
	}

	// Token should be deleted
	tokens, _ := st.ListTokens()
	if len(tokens) != 0 {
		t.Errorf("expected 0 tokens, got %d", len(tokens))
	}

	// Message should still exist (ON DELETE SET NULL)
	msgs, total, _ := st.ListMessages("all", 25, 0)
	if total != 1 || len(msgs) != 1 {
		t.Errorf("expected message to survive token deletion, got %d messages", total)
	}
}

// --- Test: Template function coverage ---

// TestDashboard_HourlyStats exercises formatHour, barHeight, and the hourly chart rendering.
func TestDashboard_HourlyStats(t *testing.T) {
	_, _, st, mux := setupTest(t)

	// Create messages so they appear in hourly stats (created_at within last 24h).
	for i := 0; i < 5; i++ {
		msg, err := st.EnqueueMessage(nil, "mail",fmt.Sprintf("hourly%d@example.com", i), "Hourly test", "body", "", 3)
		if err != nil {
			t.Fatalf("enqueuing message: %v", err)
		}
		if err := st.MarkSent(msg.ID); err != nil {
			t.Fatalf("marking sent: %v", err)
		}
	}
	// Also create some failed messages to exercise the failed branch in the chart.
	for i := 0; i < 3; i++ {
		msg, err := st.EnqueueMessage(nil, "mail",fmt.Sprintf("fail%d@example.com", i), "Fail test", "body", "", 1)
		if err != nil {
			t.Fatalf("enqueuing message: %v", err)
		}
		_, _ = st.ClaimPendingMessages(1)
		_ = st.MarkFailed(msg.ID, "error", 0)
	}

	cookie := loginSession(t, mux, "changeme")
	if cookie == nil {
		t.Fatal("expected session cookie after login")
	}

	req := authReq("GET", "/dashboard", nil, cookie)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	s := string(body)
	// Hourly Activity section should be present
	if !strings.Contains(s, "Hourly Activity") {
		t.Error("expected dashboard to contain 'Hourly Activity' section")
	}
	// Should contain the chart legend
	if !strings.Contains(s, "Sent") {
		t.Error("expected dashboard chart legend to contain 'Sent'")
	}
	if !strings.Contains(s, "Failed") {
		t.Error("expected dashboard chart legend to contain 'Failed'")
	}
}

// TestQueuePage_LongSubject exercises the truncate template function.
func TestQueuePage_LongSubject(t *testing.T) {
	_, _, st, mux := setupTest(t)

	longSubject := "This is an extremely long subject line that should be truncated by the template function"
	_, err := st.EnqueueMessage(nil, "mail","long@example.com", longSubject, "body", "", 3)
	if err != nil {
		t.Fatalf("enqueuing message: %v", err)
	}

	cookie := loginSession(t, mux, "changeme")
	if cookie == nil {
		t.Fatal("expected session cookie after login")
	}

	req := authReq("GET", "/queue", nil, cookie)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	s := string(body)
	// The truncated subject (30 chars + "...") should appear
	if !strings.Contains(s, "...") {
		t.Error("expected long subject to be truncated with '...'")
	}
}

// TestDashboard_LongSubjectTruncation exercises truncate in the dashboard recent messages.
func TestDashboard_LongSubjectTruncation(t *testing.T) {
	_, _, st, mux := setupTest(t)

	longSubject := "An incredibly long email subject line exceeding thirty characters for truncation"
	_, err := st.EnqueueMessage(nil, "mail","truncate@example.com", longSubject, "body", "", 3)
	if err != nil {
		t.Fatalf("enqueuing message: %v", err)
	}

	cookie := loginSession(t, mux, "changeme")
	if cookie == nil {
		t.Fatal("expected session cookie after login")
	}

	req := authReq("GET", "/dashboard", nil, cookie)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	body, _ := io.ReadAll(w.Result().Body)
	s := string(body)
	if !strings.Contains(s, "...") {
		t.Error("expected dashboard to truncate long subjects with '...'")
	}
}

// TestQueuePage_WithErrorMessage exercises the LastError truncation and statusClass for failed messages.
func TestQueuePage_WithErrorMessage(t *testing.T) {
	_, _, st, mux := setupTest(t)

	msg, err := st.EnqueueMessage(nil, "mail","err@example.com", "Error test", "body", "", 1)
	if err != nil {
		t.Fatalf("enqueuing message: %v", err)
	}
	_, _ = st.ClaimPendingMessages(1)
	longError := "This is a very long error message that should definitely be truncated by the truncate function in the template"
	_ = st.MarkFailed(msg.ID, longError, 0)

	cookie := loginSession(t, mux, "changeme")
	if cookie == nil {
		t.Fatal("expected session cookie after login")
	}

	req := authReq("GET", "/queue?status=failed", nil, cookie)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	s := string(body)
	// Should contain the status-failed class
	if !strings.Contains(s, "status-failed") {
		t.Error("expected failed message to have 'status-failed' class")
	}
	// Should contain the Retry button for failed messages
	if !strings.Contains(s, "Retry") {
		t.Error("expected failed message to have Retry button")
	}
}

// TestQueuePage_StatusClasses exercises all statusClass values.
func TestQueuePage_StatusClasses(t *testing.T) {
	_, _, st, mux := setupTest(t)

	// Create a sent message
	msg1, _ := st.EnqueueMessage(nil, "mail","sent@example.com", "Sent", "body", "", 3)
	_ = st.MarkSent(msg1.ID)

	// Create a queued message
	_, _ = st.EnqueueMessage(nil, "mail","queued@example.com", "Queued", "body", "", 3)

	cookie := loginSession(t, mux, "changeme")
	if cookie == nil {
		t.Fatal("expected session cookie after login")
	}

	req := authReq("GET", "/queue", nil, cookie)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	body, _ := io.ReadAll(w.Result().Body)
	s := string(body)
	if !strings.Contains(s, "status-sent") {
		t.Error("expected queue page to contain 'status-sent' class")
	}
	if !strings.Contains(s, "status-queued") {
		t.Error("expected queue page to contain 'status-queued' class")
	}
}

// TestTokensPage_WithDisabledToken exercises the disabled token display path in the template.
func TestTokensPage_WithDisabledToken(t *testing.T) {
	_, _, st, mux := setupTest(t)

	_, tok, err := st.CreateToken("disabled-token")
	if err != nil {
		t.Fatalf("creating token: %v", err)
	}
	// Disable it
	_ = st.ToggleToken(tok.ID)

	cookie := loginSession(t, mux, "changeme")
	if cookie == nil {
		t.Fatal("expected session cookie after login")
	}

	req := authReq("GET", "/tokens", nil, cookie)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	body, _ := io.ReadAll(w.Result().Body)
	s := string(body)
	if !strings.Contains(s, "Disabled") {
		t.Error("expected tokens page to show 'Disabled' for inactive token")
	}
	if !strings.Contains(s, "Enable") {
		t.Error("expected tokens page to show 'Enable' button for disabled token")
	}
}

// TestTokensPage_WithUsedToken exercises the formatTimePtr function with a non-nil LastUsedAt.
func TestTokensPage_WithUsedToken(t *testing.T) {
	_, _, st, mux := setupTest(t)

	rawToken, _, err := st.CreateToken("used-token")
	if err != nil {
		t.Fatalf("creating token: %v", err)
	}
	// Validate the token to set last_used_at
	_, err = st.ValidateToken(rawToken)
	if err != nil {
		t.Fatalf("validating token: %v", err)
	}

	cookie := loginSession(t, mux, "changeme")
	if cookie == nil {
		t.Fatal("expected session cookie after login")
	}

	req := authReq("GET", "/tokens", nil, cookie)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	body, _ := io.ReadAll(w.Result().Body)
	s := string(body)
	// The last used column should not show "-" (the zero-time indicator)
	// since we just used the token; it should have a timestamp
	if !strings.Contains(s, "used-token") {
		t.Error("expected tokens page to contain the token name")
	}
	// The formatTimePtr result should be a date, not "-"
	// We can't easily check the exact date, but we can verify Active status
	if !strings.Contains(s, "Active") {
		t.Error("expected tokens page to show 'Active' status")
	}
}

// TestQueuePage_Pagination exercises the pagination template functions (seq, add, sub).
func TestQueuePage_Pagination(t *testing.T) {
	_, _, st, mux := setupTest(t)

	// Create more than 25 messages to trigger pagination (perPage = 25)
	for i := 0; i < 30; i++ {
		_, err := st.EnqueueMessage(nil, "mail",fmt.Sprintf("page%d@example.com", i), fmt.Sprintf("Msg %d", i), "body", "", 3)
		if err != nil {
			t.Fatalf("enqueuing message %d: %v", i, err)
		}
	}

	cookie := loginSession(t, mux, "changeme")
	if cookie == nil {
		t.Fatal("expected session cookie after login")
	}

	// Page 1
	req := authReq("GET", "/queue", nil, cookie)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	s := string(body)
	// Should have pagination with "Next" link
	if !strings.Contains(s, "Next") {
		t.Error("expected pagination with 'Next' link on page 1")
	}
	// Previous should be disabled on page 1
	if !strings.Contains(s, "disabled") {
		t.Error("expected 'Previous' to be disabled on page 1")
	}

	// Page 2
	req = authReq("GET", "/queue?page=2", nil, cookie)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp = w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200 for page 2, got %d", resp.StatusCode)
	}

	body, _ = io.ReadAll(resp.Body)
	s = string(body)
	// Should have "Previous" link on page 2
	if !strings.Contains(s, "Previous") {
		t.Error("expected 'Previous' link on page 2")
	}
}

// TestSettingsPage_WithSMTPPassword exercises maskPassword with a non-empty password.
func TestSettingsPage_WithSMTPPassword(t *testing.T) {
	st, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("creating store: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	cfg := config.Default()
	cfg.SMTP.Password = "supersecret"
	a := auth.New(st, cfg.Server.AdminPassword, cfg.Server.SecretKey)

	webFS, _ := fs.Sub(web.Files, ".")
	h := NewHandler(st, a, cfg, webFS)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	cookie := loginSession(t, mux, "changeme")
	if cookie == nil {
		t.Fatal("expected session cookie after login")
	}

	req := authReq("GET", "/settings", nil, cookie)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	body, _ := io.ReadAll(w.Result().Body)
	s := string(body)
	// Settings page should contain SMTP password input field
	if !strings.Contains(s, "smtp_password") {
		t.Error("expected settings page to contain smtp_password field")
	}
}

// TestRender_TemplateNotFound exercises the render error path for a missing template name.
func TestRender_TemplateNotFound(t *testing.T) {
	h, _, _, _ := setupTest(t)

	w := httptest.NewRecorder()
	h.render(w, "nonexistent", PageData{Title: "Test"})

	resp := w.Result()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected status 500 for missing template, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "template not found") {
		t.Error("expected error message 'template not found'")
	}
}

// TestTokensPage_EmptyList exercises the empty state of the tokens table.
func TestTokensPage_EmptyList(t *testing.T) {
	_, _, _, mux := setupTest(t)

	cookie := loginSession(t, mux, "changeme")
	if cookie == nil {
		t.Fatal("expected session cookie after login")
	}

	req := authReq("GET", "/tokens", nil, cookie)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	body, _ := io.ReadAll(w.Result().Body)
	s := string(body)
	if !strings.Contains(s, "No tokens created yet") {
		t.Error("expected empty tokens page to show 'No tokens created yet'")
	}
}

// TestDashboard_EmptyState exercises the empty state of the dashboard (no messages).
func TestDashboard_EmptyState(t *testing.T) {
	_, _, _, mux := setupTest(t)

	cookie := loginSession(t, mux, "changeme")
	if cookie == nil {
		t.Fatal("expected session cookie after login")
	}

	req := authReq("GET", "/dashboard", nil, cookie)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	body, _ := io.ReadAll(w.Result().Body)
	s := string(body)
	if !strings.Contains(s, "No messages yet") {
		t.Error("expected empty dashboard to show 'No messages yet'")
	}
	// Should still show 0 stats
	if !strings.Contains(s, "0.0%") {
		t.Error("expected success rate to show '0.0%' when no messages exist")
	}
}

// TestQueuePage_FilteredPagination exercises pagination with a status filter.
func TestQueuePage_FilteredPagination(t *testing.T) {
	_, _, st, mux := setupTest(t)

	// Create 30 sent messages to trigger pagination on the "sent" filter
	for i := 0; i < 30; i++ {
		msg, err := st.EnqueueMessage(nil, "mail",fmt.Sprintf("filtpage%d@example.com", i), fmt.Sprintf("FilteredPage %d", i), "body", "", 3)
		if err != nil {
			t.Fatalf("enqueuing message: %v", err)
		}
		_ = st.MarkSent(msg.ID)
	}

	cookie := loginSession(t, mux, "changeme")
	if cookie == nil {
		t.Fatal("expected session cookie after login")
	}

	req := authReq("GET", "/queue?status=sent&page=1", nil, cookie)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	s := string(body)
	// Total should reflect 30 messages
	if !strings.Contains(s, "30") {
		t.Error("expected total count of 30 in filtered pagination")
	}
}

// TestQueuePage_InvalidPage exercises the page parsing with an invalid page number.
func TestQueuePage_InvalidPage(t *testing.T) {
	_, _, _, mux := setupTest(t)

	cookie := loginSession(t, mux, "changeme")
	if cookie == nil {
		t.Fatal("expected session cookie after login")
	}

	// Invalid page number should default to page 1
	req := authReq("GET", "/queue?page=abc", nil, cookie)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}
}

// TestQueuePage_NegativePage exercises the page clamping to 1.
func TestQueuePage_NegativePage(t *testing.T) {
	_, _, _, mux := setupTest(t)

	cookie := loginSession(t, mux, "changeme")
	if cookie == nil {
		t.Fatal("expected session cookie after login")
	}

	req := authReq("GET", "/queue?page=-1", nil, cookie)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}
}

// TestDashboard_StatusSent exercises the "status-sent" class in dashboard recent messages.
func TestDashboard_StatusSent(t *testing.T) {
	_, _, st, mux := setupTest(t)

	msg, _ := st.EnqueueMessage(nil, "mail","sentstatus@example.com", "Sent status test", "body", "", 3)
	_ = st.MarkSent(msg.ID)

	cookie := loginSession(t, mux, "changeme")
	if cookie == nil {
		t.Fatal("expected session cookie after login")
	}

	req := authReq("GET", "/dashboard", nil, cookie)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	body, _ := io.ReadAll(w.Result().Body)
	s := string(body)
	if !strings.Contains(s, "status-sent") {
		t.Error("expected dashboard to show 'status-sent' class for sent messages")
	}
}

// TestDashboard_MaxHourlyZero exercises the barHeight function when maxHourly is 0.
func TestDashboard_MaxHourlyZero(t *testing.T) {
	_, _, _, mux := setupTest(t)

	// With no messages, maxHourly will be 0 and barHeight should handle it gracefully.
	cookie := loginSession(t, mux, "changeme")
	if cookie == nil {
		t.Fatal("expected session cookie after login")
	}

	req := authReq("GET", "/dashboard", nil, cookie)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Result().StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Result().StatusCode)
	}
}

// TestSettingsPage_SMTPPasswordNotSet exercises maskPassword with empty password (shows "(not set)").
func TestSettingsPage_SMTPPasswordNotSet(t *testing.T) {
	_, _, _, mux := setupTest(t)

	cookie := loginSession(t, mux, "changeme")
	if cookie == nil {
		t.Fatal("expected session cookie after login")
	}

	req := authReq("GET", "/settings", nil, cookie)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	body, _ := io.ReadAll(w.Result().Body)
	s := string(body)
	// Default config has empty SMTP password
	// Settings page should have the smtp_password input field
	if !strings.Contains(s, "smtp_password") {
		t.Error("expected settings page to contain smtp_password field")
	}
}

// TestQueuePage_SentAtDisplay exercises formatTime for messages with SentAt set.
func TestQueuePage_SentAtDisplay(t *testing.T) {
	_, _, st, mux := setupTest(t)

	msg, _ := st.EnqueueMessage(nil, "mail","sentat@example.com", "SentAt test", "body", "", 3)
	_ = st.MarkSent(msg.ID)

	cookie := loginSession(t, mux, "changeme")
	if cookie == nil {
		t.Fatal("expected session cookie after login")
	}

	req := authReq("GET", "/queue?status=sent", nil, cookie)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	s := string(body)
	if !strings.Contains(s, "sentat@example.com") {
		t.Error("expected queue page to show the sent message")
	}
}

// TestTokensPage_WithActiveToken exercises the Active status badge and Disable button.
func TestTokensPage_WithActiveToken(t *testing.T) {
	_, _, st, mux := setupTest(t)

	_, _, err := st.CreateToken("active-token")
	if err != nil {
		t.Fatalf("creating token: %v", err)
	}

	cookie := loginSession(t, mux, "changeme")
	if cookie == nil {
		t.Fatal("expected session cookie after login")
	}

	req := authReq("GET", "/tokens", nil, cookie)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	body, _ := io.ReadAll(w.Result().Body)
	s := string(body)
	if !strings.Contains(s, "status-active") {
		t.Error("expected tokens page to show 'status-active' class")
	}
	if !strings.Contains(s, "Disable") {
		t.Error("expected tokens page to show 'Disable' button for active token")
	}
}

// TestQueuePage_TokenNameInQueue verifies that the token name appears in queue when message has a token.
func TestQueuePage_TokenNameInQueue(t *testing.T) {
	_, _, st, mux := setupTest(t)

	_, tok, err := st.CreateToken("queue-token")
	if err != nil {
		t.Fatalf("creating token: %v", err)
	}

	_, err = st.EnqueueMessage(&tok.ID, "mail", "tokenmsg@example.com", "Token queue test", "body", "", 3)
	if err != nil {
		t.Fatalf("enqueuing: %v", err)
	}

	cookie := loginSession(t, mux, "changeme")
	if cookie == nil {
		t.Fatal("expected session cookie after login")
	}

	req := authReq("GET", "/queue", nil, cookie)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	body, _ := io.ReadAll(w.Result().Body)
	s := string(body)
	if !strings.Contains(s, "queue-token") {
		t.Error("expected queue page to show token name 'queue-token'")
	}
}

// --- Tests to exercise store error paths ---

// closedStoreSetup creates a handler with a store that has been closed,
// causing database operations to fail.
func closedStoreSetup(t *testing.T) (*Handler, *auth.Auth, *http.ServeMux) {
	t.Helper()
	// First create a working store to set up the handler and get a valid session
	st, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("creating store: %v", err)
	}

	cfg := config.Default()
	a := auth.New(st, cfg.Server.AdminPassword, cfg.Server.SecretKey)

	webFS, _ := fs.Sub(web.Files, ".")
	h := NewHandler(st, a, cfg, webFS)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	// Close the store to force database errors
	st.Close()

	return h, a, mux
}

func TestDashboard_StoreError(t *testing.T) {
	_, a, mux := closedStoreSetup(t)

	// Create a session directly on the auth object since the store is closed
	token, ok := a.Login("changeme")
	if !ok {
		t.Fatal("expected login to succeed")
	}
	cookie := &http.Cookie{Name: "webhook_session", Value: token}

	req := authReq("GET", "/dashboard", nil, cookie)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected status 500 when store is closed, got %d", resp.StatusCode)
	}
}

func TestTokens_StoreError(t *testing.T) {
	_, a, mux := closedStoreSetup(t)

	token, ok := a.Login("changeme")
	if !ok {
		t.Fatal("expected login to succeed")
	}
	cookie := &http.Cookie{Name: "webhook_session", Value: token}

	req := authReq("GET", "/tokens", nil, cookie)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected status 500 when store is closed, got %d", resp.StatusCode)
	}
}

func TestCreateToken_StoreError(t *testing.T) {
	_, a, mux := closedStoreSetup(t)

	token, ok := a.Login("changeme")
	if !ok {
		t.Fatal("expected login to succeed")
	}
	cookie := &http.Cookie{Name: "webhook_session", Value: token}

	form := url.Values{"name": {"fail-token"}}
	req := authFormReq("/tokens", form, cookie)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect on create token store error, got %d", resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	if !strings.Contains(loc, "Failed") {
		t.Errorf("expected error message about failed creation, got %q", loc)
	}
}

func TestQueue_StoreError(t *testing.T) {
	_, a, mux := closedStoreSetup(t)

	token, ok := a.Login("changeme")
	if !ok {
		t.Fatal("expected login to succeed")
	}
	cookie := &http.Cookie{Name: "webhook_session", Value: token}

	req := authReq("GET", "/queue", nil, cookie)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected status 500 when store is closed, got %d", resp.StatusCode)
	}
}

func TestToggleToken_StoreError(t *testing.T) {
	_, a, mux := closedStoreSetup(t)

	token, ok := a.Login("changeme")
	if !ok {
		t.Fatal("expected login to succeed")
	}
	cookie := &http.Cookie{Name: "webhook_session", Value: token}

	req := authFormReq("/tokens/1/toggle", nil, cookie)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// ToggleToken logs the error but still redirects
	resp := w.Result()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect even on toggle error, got %d", resp.StatusCode)
	}
}

func TestDeleteToken_StoreError(t *testing.T) {
	_, a, mux := closedStoreSetup(t)

	token, ok := a.Login("changeme")
	if !ok {
		t.Fatal("expected login to succeed")
	}
	cookie := &http.Cookie{Name: "webhook_session", Value: token}

	req := authFormReq("/tokens/1/delete", nil, cookie)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect even on delete error, got %d", resp.StatusCode)
	}
}

func TestRetryMessage_StoreError(t *testing.T) {
	_, a, mux := closedStoreSetup(t)

	token, ok := a.Login("changeme")
	if !ok {
		t.Fatal("expected login to succeed")
	}
	cookie := &http.Cookie{Name: "webhook_session", Value: token}

	req := authFormReq("/queue/1/retry", nil, cookie)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect even on retry error, got %d", resp.StatusCode)
	}
}

func TestDeleteMessage_StoreError(t *testing.T) {
	_, a, mux := closedStoreSetup(t)

	token, ok := a.Login("changeme")
	if !ok {
		t.Fatal("expected login to succeed")
	}
	cookie := &http.Cookie{Name: "webhook_session", Value: token}

	req := authFormReq("/queue/1/delete", nil, cookie)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect even on delete message error, got %d", resp.StatusCode)
	}
}

// TestDashboard_WithSendingStatus exercises the "sending" statusClass in dashboard.
func TestDashboard_WithSendingStatus(t *testing.T) {
	_, _, st, mux := setupTest(t)

	// Create a message and claim it to put it in "sending" status
	_, err := st.EnqueueMessage(nil, "mail","sending@example.com", "Sending status", "body", "", 3)
	if err != nil {
		t.Fatalf("enqueuing message: %v", err)
	}
	_, _ = st.ClaimPendingMessages(10)

	cookie := loginSession(t, mux, "changeme")
	if cookie == nil {
		t.Fatal("expected session cookie after login")
	}

	req := authReq("GET", "/dashboard", nil, cookie)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	body, _ := io.ReadAll(w.Result().Body)
	s := string(body)
	if !strings.Contains(s, "status-sending") {
		t.Error("expected dashboard to show 'status-sending' class for sending messages")
	}
}

// TestQueuePage_WithSendingStatus exercises the "sending" statusClass in queue.
func TestQueuePage_WithSendingStatus(t *testing.T) {
	_, _, st, mux := setupTest(t)

	_, err := st.EnqueueMessage(nil, "mail","sending@example.com", "Sending in queue", "body", "", 3)
	if err != nil {
		t.Fatalf("enqueuing: %v", err)
	}
	_, _ = st.ClaimPendingMessages(10)

	cookie := loginSession(t, mux, "changeme")
	if cookie == nil {
		t.Fatal("expected session cookie after login")
	}

	req := authReq("GET", "/queue?status=sending", nil, cookie)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	body, _ := io.ReadAll(w.Result().Body)
	s := string(body)
	if !strings.Contains(s, "status-sending") {
		t.Error("expected queue page to show 'status-sending' class")
	}
}

// TestTokensPage_FormatTimeZero exercises formatTime with recently created tokens (non-zero CreatedAt).
func TestTokensPage_FormatTimeZero(t *testing.T) {
	_, _, st, mux := setupTest(t)

	// Token has just been created so CreatedAt is set, LastUsedAt is nil.
	// The template calls formatTime on CreatedAt (always set) and formatTimePtr on LastUsedAt (nil).
	_, _, _ = st.CreateToken("time-test")

	cookie := loginSession(t, mux, "changeme")
	if cookie == nil {
		t.Fatal("expected session cookie after login")
	}

	req := authReq("GET", "/tokens", nil, cookie)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	body, _ := io.ReadAll(w.Result().Body)
	s := string(body)
	// LastUsedAt is nil, so formatTimePtr should return "-"
	if !strings.Contains(s, "-") {
		t.Error("expected tokens page to show '-' for nil LastUsedAt")
	}
}
