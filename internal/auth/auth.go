package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"sync"
	"time"

	"c0de-webhook/internal/store"
)

const (
	sessionCookieName = "webhook_session"
	sessionDuration   = 24 * time.Hour
)

type Auth struct {
	store         *store.Store
	adminPassword string
	secretKey     []byte
	sessions      map[string]time.Time
	mu            sync.RWMutex
}

func New(st *store.Store, adminPassword, secretKey string) *Auth {
	return &Auth{
		store:         st,
		adminPassword: adminPassword,
		secretKey:     []byte(secretKey),
		sessions:      make(map[string]time.Time),
	}
}

// ValidateAPIToken extracts and validates a Bearer token from the request.
func (a *Auth) ValidateAPIToken(r *http.Request) (*store.Token, error) {
	header := r.Header.Get("Authorization")
	if header == "" {
		return nil, ErrNoToken
	}
	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
		return nil, ErrInvalidAuth
	}
	return a.store.ValidateToken(parts[1])
}

// Login checks the admin password and creates a session.
// It checks the DB-stored password first, then falls back to the config password.
func (a *Auth) Login(password string) (string, bool) {
	if !a.checkPassword(password) {
		return "", false
	}

	token := generateSessionToken()
	a.mu.Lock()
	a.sessions[token] = time.Now().Add(sessionDuration)
	a.mu.Unlock()

	return token, true
}

func (a *Auth) checkPassword(password string) bool {
	// Check DB-stored password hash first
	if storedHash, err := a.store.GetSetting("admin_password_hash"); err == nil && storedHash != "" {
		h := sha256.Sum256([]byte(password))
		return hex.EncodeToString(h[:]) == storedHash
	}
	// Fall back to config password
	return password == a.adminPassword
}

// ChangePassword sets a new admin password (stored as SHA-256 hash in DB).
func (a *Auth) ChangePassword(currentPassword, newPassword string) error {
	if !a.checkPassword(currentPassword) {
		return ErrWrongPassword
	}
	h := sha256.Sum256([]byte(newPassword))
	return a.store.SetSetting("admin_password_hash", hex.EncodeToString(h[:]))
}

// ValidateSession checks if the request has a valid session cookie.
func (a *Auth) ValidateSession(r *http.Request) bool {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return false
	}

	a.mu.RLock()
	expiry, ok := a.sessions[cookie.Value]
	a.mu.RUnlock()

	if !ok || time.Now().After(expiry) {
		if ok {
			a.mu.Lock()
			delete(a.sessions, cookie.Value)
			a.mu.Unlock()
		}
		return false
	}
	return true
}

// SetSessionCookie sets the session cookie on the response.
func (a *Auth) SetSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(sessionDuration.Seconds()),
	})
}

// ClearSessionCookie removes the session cookie.
func (a *Auth) ClearSessionCookie(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		a.mu.Lock()
		delete(a.sessions, cookie.Value)
		a.mu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})
}

// RequireSession is middleware that redirects to login if not authenticated.
func (a *Auth) RequireSession(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !a.ValidateSession(r) {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next(w, r)
	}
}

// CleanupSessions removes expired sessions. Call periodically.
func (a *Auth) CleanupSessions() {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := time.Now()
	for token, expiry := range a.sessions {
		if now.After(expiry) {
			delete(a.sessions, token)
		}
	}
}

func generateSessionToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	mac := hmac.New(sha256.New, b)
	mac.Write([]byte("session"))
	return hex.EncodeToString(mac.Sum(nil))
}

// Sentinel errors
type authError string

func (e authError) Error() string { return string(e) }

const (
	ErrNoToken       = authError("no authorization token provided")
	ErrInvalidAuth   = authError("invalid authorization header")
	ErrWrongPassword = authError("current password is incorrect")
)
