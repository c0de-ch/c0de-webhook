package ui

import (
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"c0de-webhook/internal/auth"
	"c0de-webhook/internal/config"
	"c0de-webhook/internal/store"
)

type Handler struct {
	store         *store.Store
	auth          *auth.Auth
	config        *config.Config
	webFS         fs.FS
	templates     map[string]*template.Template
	pendingTokens map[string]string // sessionID -> raw token
	mu            sync.Mutex
}

type PageData struct {
	Title     string
	ActiveNav string
	Flash     string
	Content   interface{}
}

type DashboardContent struct {
	Stats       *store.DashboardStats
	HourlyStats []store.HourlyStat
	MaxHourly   int
	RecentMsgs  []store.Message
	TokenStats  []store.TokenStats
}

type TokensContent struct {
	Tokens   []store.Token
	NewToken string
}

type QueueContent struct {
	Messages []store.Message
	Filter   string
	Total    int64
	Page     int
	Pages    int
}

type TokenDetailContent struct {
	Token    store.Token
	Stats    store.TokenStats
	Messages []store.Message
	Total    int64
	Page     int
	Pages    int
}

type SettingsContent struct {
	Config       *config.Config
	PasswordMsg  string
	PasswordErr  bool
}

func NewHandler(st *store.Store, a *auth.Auth, cfg *config.Config, webFS fs.FS) *Handler {
	h := &Handler{
		store:         st,
		auth:          a,
		config:        cfg,
		webFS:         webFS,
		pendingTokens: make(map[string]string),
	}
	h.templates = h.loadTemplates(webFS)
	return h
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	// Static files
	mux.Handle("GET /static/", http.FileServerFS(h.webFS))

	// Auth routes
	mux.HandleFunc("GET /login", h.handleLogin)
	mux.HandleFunc("POST /login", h.handleLoginPost)
	mux.HandleFunc("GET /logout", h.handleLogout)

	// Protected routes
	mux.HandleFunc("GET /{$}", h.auth.RequireSession(h.handleDashboard))
	mux.HandleFunc("GET /dashboard", h.auth.RequireSession(h.handleDashboard))
	mux.HandleFunc("GET /tokens", h.auth.RequireSession(h.handleTokens))
	mux.HandleFunc("POST /tokens", h.auth.RequireSession(h.handleCreateToken))
	mux.HandleFunc("POST /tokens/{id}/toggle", h.auth.RequireSession(h.handleToggleToken))
	mux.HandleFunc("POST /tokens/{id}/delete", h.auth.RequireSession(h.handleDeleteToken))
	mux.HandleFunc("GET /tokens/{id}", h.auth.RequireSession(h.handleTokenDetail))
	mux.HandleFunc("GET /queue", h.auth.RequireSession(h.handleQueue))
	mux.HandleFunc("POST /queue/{id}/retry", h.auth.RequireSession(h.handleRetryMessage))
	mux.HandleFunc("POST /queue/{id}/delete", h.auth.RequireSession(h.handleDeleteMessage))
	mux.HandleFunc("GET /settings", h.auth.RequireSession(h.handleSettings))
	mux.HandleFunc("POST /settings/password", h.auth.RequireSession(h.handleChangePassword))
}

func (h *Handler) loadTemplates(webFS fs.FS) map[string]*template.Template {
	funcMap := template.FuncMap{
		"formatTime": func(t time.Time) string {
			if t.IsZero() {
				return "-"
			}
			return t.Format("2006-01-02 15:04:05")
		},
		"formatTimePtr": func(t *time.Time) string {
			if t == nil {
				return "-"
			}
			return t.Format("2006-01-02 15:04:05")
		},
		"statusClass": func(s string) string {
			switch s {
			case "sent":
				return "status-sent"
			case "failed":
				return "status-failed"
			case "queued":
				return "status-queued"
			case "sending":
				return "status-sending"
			default:
				return ""
			}
		},
		"percentage": func(a, b int64) float64 {
			if b == 0 {
				return 0
			}
			return float64(a) / float64(b) * 100
		},
		"barHeight": func(val, max int) int {
			if max == 0 {
				return 0
			}
			return val * 100 / max
		},
		"truncate": func(s string, n int) string {
			if len(s) <= n {
				return s
			}
			return s[:n] + "..."
		},
		"formatHour": func(t time.Time) string {
			return t.Format("15:04")
		},
		"seq": func(n int) []int {
			s := make([]int, n)
			for i := range s {
				s[i] = i + 1
			}
			return s
		},
		"maskPassword": func(s string) string {
			if s == "" {
				return "(not set)"
			}
			return "********"
		},
		"add": func(a, b int) int {
			return a + b
		},
		"sub": func(a, b int) int {
			return a - b
		},
	}

	templates := make(map[string]*template.Template)

	pages := []string{"dashboard", "tokens", "token_detail", "queue", "settings"}
	for _, page := range pages {
		t := template.Must(
			template.New("").Funcs(funcMap).ParseFS(webFS,
				"templates/layout.html",
				"templates/"+page+".html",
			),
		)
		templates[page] = t
	}

	// Login is standalone (no layout)
	templates["login"] = template.Must(
		template.New("").Funcs(funcMap).ParseFS(webFS, "templates/login.html"),
	)

	return templates
}

func (h *Handler) render(w http.ResponseWriter, name string, data PageData) {
	tmpl, ok := h.templates[name]
	if !ok {
		http.Error(w, "template not found", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	var err error
	if name == "login" {
		err = tmpl.ExecuteTemplate(w, "login", data)
	} else {
		err = tmpl.ExecuteTemplate(w, "layout", data)
	}
	if err != nil {
		log.Printf("ERROR rendering template %s: %v", name, err)
	}
}

// --- Auth handlers ---

func (h *Handler) handleLogin(w http.ResponseWriter, r *http.Request) {
	if h.auth.ValidateSession(r) {
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		return
	}
	h.render(w, "login", PageData{Title: "Login", Flash: r.URL.Query().Get("error")})
}

func (h *Handler) handleLoginPost(w http.ResponseWriter, r *http.Request) {
	password := r.FormValue("password")
	token, ok := h.auth.Login(password)
	if !ok {
		http.Redirect(w, r, "/login?error=Invalid+password", http.StatusSeeOther)
		return
	}
	h.auth.SetSessionCookie(w, token)
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

func (h *Handler) handleLogout(w http.ResponseWriter, r *http.Request) {
	h.auth.ClearSessionCookie(w, r)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// --- Dashboard ---

func (h *Handler) handleDashboard(w http.ResponseWriter, r *http.Request) {
	stats, err := h.store.GetDashboardStats()
	if err != nil {
		log.Printf("ERROR getting stats: %v", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	hourly, _ := h.store.GetHourlyStats(24)
	recent, _ := h.store.GetRecentMessages(10)
	tokenStats, _ := h.store.GetTokenStats()

	maxHourly := 0
	for _, s := range hourly {
		if s.Total > maxHourly {
			maxHourly = s.Total
		}
	}

	h.render(w, "dashboard", PageData{
		Title:     "Dashboard",
		ActiveNav: "dashboard",
		Content: DashboardContent{
			Stats:       stats,
			HourlyStats: hourly,
			MaxHourly:   maxHourly,
			RecentMsgs:  recent,
			TokenStats:  tokenStats,
		},
	})
}

// --- Tokens ---

func (h *Handler) handleTokens(w http.ResponseWriter, r *http.Request) {
	tokens, err := h.store.ListTokens()
	if err != nil {
		log.Printf("ERROR listing tokens: %v", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	var newToken string
	if r.URL.Query().Get("new") == "1" {
		cookie, _ := r.Cookie("webhook_session")
		if cookie != nil {
			h.mu.Lock()
			newToken = h.pendingTokens[cookie.Value]
			delete(h.pendingTokens, cookie.Value)
			h.mu.Unlock()
		}
	}

	h.render(w, "tokens", PageData{
		Title:     "API Tokens",
		ActiveNav: "tokens",
		Flash:     r.URL.Query().Get("msg"),
		Content: TokensContent{
			Tokens:   tokens,
			NewToken: newToken,
		},
	})
}

func (h *Handler) handleCreateToken(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		http.Redirect(w, r, "/tokens?msg=Name+is+required", http.StatusSeeOther)
		return
	}

	rawToken, _, err := h.store.CreateToken(name)
	if err != nil {
		log.Printf("ERROR creating token: %v", err)
		http.Redirect(w, r, "/tokens?msg=Failed+to+create+token", http.StatusSeeOther)
		return
	}

	// Store raw token for display after redirect
	cookie, _ := r.Cookie("webhook_session")
	if cookie != nil {
		h.mu.Lock()
		h.pendingTokens[cookie.Value] = rawToken
		h.mu.Unlock()
	}

	http.Redirect(w, r, "/tokens?new=1", http.StatusSeeOther)
}

func (h *Handler) handleToggleToken(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Redirect(w, r, "/tokens?msg=Invalid+token+ID", http.StatusSeeOther)
		return
	}
	if err := h.store.ToggleToken(id); err != nil {
		log.Printf("ERROR toggling token: %v", err)
	}
	http.Redirect(w, r, "/tokens", http.StatusSeeOther)
}

func (h *Handler) handleDeleteToken(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Redirect(w, r, "/tokens?msg=Invalid+token+ID", http.StatusSeeOther)
		return
	}
	if err := h.store.DeleteToken(id); err != nil {
		log.Printf("ERROR deleting token: %v", err)
	}
	http.Redirect(w, r, "/tokens?msg=Token+deleted", http.StatusSeeOther)
}

func (h *Handler) handleTokenDetail(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Redirect(w, r, "/tokens?msg=Invalid+token+ID", http.StatusSeeOther)
		return
	}

	tokens, err := h.store.ListTokens()
	if err != nil {
		log.Printf("ERROR listing tokens: %v", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	var token store.Token
	found := false
	for _, t := range tokens {
		if t.ID == id {
			token = t
			found = true
			break
		}
	}
	if !found {
		http.Redirect(w, r, "/tokens?msg=Token+not+found", http.StatusSeeOther)
		return
	}

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	perPage := 25

	messages, total, err := h.store.GetTokenMessages(id, perPage, (page-1)*perPage)
	if err != nil {
		log.Printf("ERROR getting token messages: %v", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	pages := int(total) / perPage
	if int(total)%perPage > 0 {
		pages++
	}
	if pages < 1 {
		pages = 1
	}

	allTokenStats, _ := h.store.GetTokenStats()
	var stats store.TokenStats
	for _, ts := range allTokenStats {
		if ts.TokenID == id {
			stats = ts
			break
		}
	}

	h.render(w, "token_detail", PageData{
		Title:     "Token: " + token.Name,
		ActiveNav: "tokens",
		Content: TokenDetailContent{
			Token:    token,
			Stats:    stats,
			Messages: messages,
			Total:    total,
			Page:     page,
			Pages:    pages,
		},
	})
}

// --- Queue ---

func (h *Handler) handleQueue(w http.ResponseWriter, r *http.Request) {
	filter := r.URL.Query().Get("status")
	if filter == "" {
		filter = "all"
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	perPage := 25

	messages, total, err := h.store.ListMessages(filter, perPage, (page-1)*perPage)
	if err != nil {
		log.Printf("ERROR listing messages: %v", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	pages := int(total) / perPage
	if int(total)%perPage > 0 {
		pages++
	}
	if pages < 1 {
		pages = 1
	}

	h.render(w, "queue", PageData{
		Title:     "Message Queue",
		ActiveNav: "queue",
		Flash:     r.URL.Query().Get("msg"),
		Content: QueueContent{
			Messages: messages,
			Filter:   filter,
			Total:    total,
			Page:     page,
			Pages:    pages,
		},
	})
}

func (h *Handler) handleRetryMessage(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Redirect(w, r, "/queue?msg=Invalid+message+ID", http.StatusSeeOther)
		return
	}
	if err := h.store.RetryMessage(id); err != nil {
		log.Printf("ERROR retrying message: %v", err)
	}
	http.Redirect(w, r, fmt.Sprintf("/queue?msg=Message+%d+requeued", id), http.StatusSeeOther)
}

func (h *Handler) handleDeleteMessage(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Redirect(w, r, "/queue?msg=Invalid+message+ID", http.StatusSeeOther)
		return
	}
	if err := h.store.DeleteMessage(id); err != nil {
		log.Printf("ERROR deleting message: %v", err)
	}
	http.Redirect(w, r, "/queue?msg=Message+deleted", http.StatusSeeOther)
}

// --- Settings ---

func (h *Handler) handleSettings(w http.ResponseWriter, r *http.Request) {
	msg := r.URL.Query().Get("msg")
	isErr := r.URL.Query().Get("err") == "1"
	h.render(w, "settings", PageData{
		Title:     "Settings",
		ActiveNav: "settings",
		Content: SettingsContent{
			Config:      h.config,
			PasswordMsg: msg,
			PasswordErr: isErr,
		},
	})
}

func (h *Handler) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	current := r.FormValue("current_password")
	newPass := r.FormValue("new_password")
	confirm := r.FormValue("confirm_password")

	if newPass == "" {
		http.Redirect(w, r, "/settings?err=1&msg=New+password+cannot+be+empty", http.StatusSeeOther)
		return
	}
	if newPass != confirm {
		http.Redirect(w, r, "/settings?err=1&msg=New+passwords+do+not+match", http.StatusSeeOther)
		return
	}
	if err := h.auth.ChangePassword(current, newPass); err != nil {
		log.Printf("ERROR changing password: %v", err)
		http.Redirect(w, r, "/settings?err=1&msg=Current+password+is+incorrect", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/settings?msg=Password+changed+successfully", http.StatusSeeOther)
}
