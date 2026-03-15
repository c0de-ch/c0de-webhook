package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"c0de-webhook/internal/store"
)

// newTestStore creates an in-memory SQLite store for testing.
func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("failed to create test store: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// newTestAuth creates an Auth instance backed by an in-memory store.
func newTestAuth(t *testing.T) (*Auth, *store.Store) {
	t.Helper()
	s := newTestStore(t)
	a := New(s, "test123", "supersecretkey")
	return a, s
}

func TestValidateAPIToken_Valid(t *testing.T) {
	a, s := newTestAuth(t)

	rawToken, created, err := s.CreateToken("test-token")
	if err != nil {
		t.Fatalf("CreateToken failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+rawToken)

	tok, err := a.ValidateAPIToken(req)
	if err != nil {
		t.Fatalf("ValidateAPIToken returned error: %v", err)
	}
	if tok == nil {
		t.Fatal("expected non-nil token")
	}
	if tok.ID != created.ID {
		t.Errorf("expected token ID %d, got %d", created.ID, tok.ID)
	}
	if tok.Name != "test-token" {
		t.Errorf("expected token name %q, got %q", "test-token", tok.Name)
	}
	if !tok.IsActive {
		t.Error("expected token to be active")
	}
}

func TestValidateAPIToken_NoHeader(t *testing.T) {
	a, _ := newTestAuth(t)

	req := httptest.NewRequest(http.MethodGet, "/", nil)

	_, err := a.ValidateAPIToken(req)
	if err != ErrNoToken {
		t.Errorf("expected ErrNoToken, got %v", err)
	}
}

func TestValidateAPIToken_InvalidFormat(t *testing.T) {
	a, _ := newTestAuth(t)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")

	_, err := a.ValidateAPIToken(req)
	if err != ErrInvalidAuth {
		t.Errorf("expected ErrInvalidAuth, got %v", err)
	}
}

func TestValidateAPIToken_BadToken(t *testing.T) {
	a, _ := newTestAuth(t)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer invalidtoken123")

	_, err := a.ValidateAPIToken(req)
	if err == nil {
		t.Error("expected error for bad token, got nil")
	}
}

func TestValidateAPIToken_DisabledToken(t *testing.T) {
	a, s := newTestAuth(t)

	rawToken, created, err := s.CreateToken("disabled-token")
	if err != nil {
		t.Fatalf("CreateToken failed: %v", err)
	}

	// Disable the token
	if err := s.ToggleToken(created.ID); err != nil {
		t.Fatalf("ToggleToken failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+rawToken)

	_, err = a.ValidateAPIToken(req)
	if err == nil {
		t.Error("expected error for disabled token, got nil")
	}
}

func TestLogin_Success(t *testing.T) {
	a, _ := newTestAuth(t)

	token, ok := a.Login("test123")
	if !ok {
		t.Fatal("expected login to succeed")
	}
	if token == "" {
		t.Error("expected non-empty session token")
	}
}

func TestLogin_Wrong(t *testing.T) {
	a, _ := newTestAuth(t)

	token, ok := a.Login("wrongpassword")
	if ok {
		t.Error("expected login to fail")
	}
	if token != "" {
		t.Errorf("expected empty token, got %q", token)
	}
}

func TestValidateSession(t *testing.T) {
	a, _ := newTestAuth(t)

	sessionToken, ok := a.Login("test123")
	if !ok {
		t.Fatal("login failed")
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{
		Name:  sessionCookieName,
		Value: sessionToken,
	})

	if !a.ValidateSession(req) {
		t.Error("expected session to be valid")
	}
}

func TestValidateSession_NoCookie(t *testing.T) {
	a, _ := newTestAuth(t)

	req := httptest.NewRequest(http.MethodGet, "/", nil)

	if a.ValidateSession(req) {
		t.Error("expected session validation to fail without cookie")
	}
}

func TestValidateSession_InvalidCookie(t *testing.T) {
	a, _ := newTestAuth(t)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{
		Name:  sessionCookieName,
		Value: "nonexistentsessiontoken",
	})

	if a.ValidateSession(req) {
		t.Error("expected session validation to fail with invalid cookie")
	}
}

func TestValidateSession_Expired(t *testing.T) {
	a, _ := newTestAuth(t)

	sessionToken, ok := a.Login("test123")
	if !ok {
		t.Fatal("login failed")
	}

	// Manually set session expiry to the past
	a.mu.Lock()
	a.sessions[sessionToken] = time.Now().Add(-1 * time.Hour)
	a.mu.Unlock()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{
		Name:  sessionCookieName,
		Value: sessionToken,
	})

	if a.ValidateSession(req) {
		t.Error("expected expired session to be invalid")
	}

	// Verify the expired session was cleaned up
	a.mu.RLock()
	_, exists := a.sessions[sessionToken]
	a.mu.RUnlock()
	if exists {
		t.Error("expected expired session to be deleted from sessions map")
	}
}

func TestSetSessionCookie(t *testing.T) {
	a, _ := newTestAuth(t)

	recorder := httptest.NewRecorder()
	a.SetSessionCookie(recorder, "testsessionvalue")

	resp := recorder.Result()
	cookies := resp.Cookies()

	if len(cookies) == 0 {
		t.Fatal("expected Set-Cookie header")
	}

	var found *http.Cookie
	for _, c := range cookies {
		if c.Name == sessionCookieName {
			found = c
			break
		}
	}
	if found == nil {
		t.Fatalf("expected cookie with name %q", sessionCookieName)
	}
	if found.Value != "testsessionvalue" {
		t.Errorf("expected cookie value %q, got %q", "testsessionvalue", found.Value)
	}
	if !found.HttpOnly {
		t.Error("expected HttpOnly to be true")
	}
	if found.Path != "/" {
		t.Errorf("expected path %q, got %q", "/", found.Path)
	}
	if found.SameSite != http.SameSiteStrictMode {
		t.Errorf("expected SameSite Strict, got %v", found.SameSite)
	}
	expectedMaxAge := int(sessionDuration.Seconds())
	if found.MaxAge != expectedMaxAge {
		t.Errorf("expected MaxAge %d, got %d", expectedMaxAge, found.MaxAge)
	}
}

func TestClearSessionCookie(t *testing.T) {
	a, _ := newTestAuth(t)

	// Login to create a session
	sessionToken, ok := a.Login("test123")
	if !ok {
		t.Fatal("login failed")
	}

	// Verify session exists
	a.mu.RLock()
	_, exists := a.sessions[sessionToken]
	a.mu.RUnlock()
	if !exists {
		t.Fatal("session should exist after login")
	}

	// Clear the session
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{
		Name:  sessionCookieName,
		Value: sessionToken,
	})

	a.ClearSessionCookie(recorder, req)

	// Verify session is removed from the map
	a.mu.RLock()
	_, exists = a.sessions[sessionToken]
	a.mu.RUnlock()
	if exists {
		t.Error("session should be removed after clearing")
	}

	// Verify the cookie is cleared in the response
	resp := recorder.Result()
	cookies := resp.Cookies()
	var found *http.Cookie
	for _, c := range cookies {
		if c.Name == sessionCookieName {
			found = c
			break
		}
	}
	if found == nil {
		t.Fatal("expected a Set-Cookie header to clear the cookie")
	}
	if found.MaxAge != -1 {
		t.Errorf("expected MaxAge -1 to delete cookie, got %d", found.MaxAge)
	}
	if found.Value != "" {
		t.Errorf("expected empty cookie value, got %q", found.Value)
	}
}

func TestRequireSession_Authenticated(t *testing.T) {
	a, _ := newTestAuth(t)

	sessionToken, ok := a.Login("test123")
	if !ok {
		t.Fatal("login failed")
	}

	handlerCalled := false
	handler := a.RequireSession(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	req.AddCookie(&http.Cookie{
		Name:  sessionCookieName,
		Value: sessionToken,
	})
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)

	if !handlerCalled {
		t.Error("expected handler to be called for authenticated request")
	}
	if recorder.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", recorder.Code)
	}
}

func TestRequireSession_Unauthenticated(t *testing.T) {
	a, _ := newTestAuth(t)

	handlerCalled := false
	handler := a.RequireSession(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
	})

	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)

	if handlerCalled {
		t.Error("handler should not be called for unauthenticated request")
	}
	if recorder.Code != http.StatusSeeOther {
		t.Errorf("expected status %d (SeeOther), got %d", http.StatusSeeOther, recorder.Code)
	}
	location := recorder.Header().Get("Location")
	if location != "/login" {
		t.Errorf("expected redirect to /login, got %q", location)
	}
}

func TestCleanupSessions(t *testing.T) {
	a, _ := newTestAuth(t)

	// Create multiple sessions via login
	token1, ok := a.Login("test123")
	if !ok {
		t.Fatal("login 1 failed")
	}
	token2, ok := a.Login("test123")
	if !ok {
		t.Fatal("login 2 failed")
	}
	token3, ok := a.Login("test123")
	if !ok {
		t.Fatal("login 3 failed")
	}

	// Set token1 and token2 to expired
	a.mu.Lock()
	a.sessions[token1] = time.Now().Add(-2 * time.Hour)
	a.sessions[token2] = time.Now().Add(-1 * time.Hour)
	// token3 remains valid (set in the future by Login)
	a.mu.Unlock()

	// Verify we have 3 sessions
	a.mu.RLock()
	countBefore := len(a.sessions)
	a.mu.RUnlock()
	if countBefore != 3 {
		t.Fatalf("expected 3 sessions before cleanup, got %d", countBefore)
	}

	// Run cleanup
	a.CleanupSessions()

	// Verify only the valid session remains
	a.mu.RLock()
	countAfter := len(a.sessions)
	_, t1Exists := a.sessions[token1]
	_, t2Exists := a.sessions[token2]
	_, t3Exists := a.sessions[token3]
	a.mu.RUnlock()

	if countAfter != 1 {
		t.Errorf("expected 1 session after cleanup, got %d", countAfter)
	}
	if t1Exists {
		t.Error("expired token1 should have been removed")
	}
	if t2Exists {
		t.Error("expired token2 should have been removed")
	}
	if !t3Exists {
		t.Error("valid token3 should still exist")
	}
}

func TestErrorTypes(t *testing.T) {
	if ErrNoToken.Error() != "no authorization token provided" {
		t.Errorf("ErrNoToken message = %q, want %q", ErrNoToken.Error(), "no authorization token provided")
	}
	if ErrInvalidAuth.Error() != "invalid authorization header" {
		t.Errorf("ErrInvalidAuth message = %q, want %q", ErrInvalidAuth.Error(), "invalid authorization header")
	}
}

func TestValidateAPIToken_BearerCaseInsensitive(t *testing.T) {
	a, s := newTestAuth(t)

	rawToken, _, err := s.CreateToken("case-test")
	if err != nil {
		t.Fatalf("CreateToken failed: %v", err)
	}

	// Test with lowercase "bearer"
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "bearer "+rawToken)

	tok, err := a.ValidateAPIToken(req)
	if err != nil {
		t.Fatalf("ValidateAPIToken with lowercase bearer failed: %v", err)
	}
	if tok == nil {
		t.Fatal("expected non-nil token")
	}

	// Test with mixed case "BEARER"
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.Header.Set("Authorization", "BEARER "+rawToken)

	tok2, err := a.ValidateAPIToken(req2)
	if err != nil {
		t.Fatalf("ValidateAPIToken with uppercase BEARER failed: %v", err)
	}
	if tok2 == nil {
		t.Fatal("expected non-nil token")
	}
}

func TestValidateAPIToken_MalformedHeader(t *testing.T) {
	a, _ := newTestAuth(t)

	// Single word, no space
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "BearerNoSpace")

	_, err := a.ValidateAPIToken(req)
	if err != ErrInvalidAuth {
		t.Errorf("expected ErrInvalidAuth for malformed header, got %v", err)
	}
}

func TestClearSessionCookie_NoCookie(t *testing.T) {
	a, _ := newTestAuth(t)

	// Clear without any cookie set should still work and set a clearing cookie
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	a.ClearSessionCookie(recorder, req)

	resp := recorder.Result()
	cookies := resp.Cookies()
	var found *http.Cookie
	for _, c := range cookies {
		if c.Name == sessionCookieName {
			found = c
			break
		}
	}
	if found == nil {
		t.Fatal("expected a Set-Cookie header even when no session cookie was present")
	}
	if found.MaxAge != -1 {
		t.Errorf("expected MaxAge -1, got %d", found.MaxAge)
	}
}
