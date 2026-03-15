package webhook

import (
	"encoding/json"
	"log"
	"net/http"

	"c0de-webhook/internal/auth"
	"c0de-webhook/internal/store"
)

type Handler struct {
	store      *store.Store
	auth       *auth.Auth
	maxRetries int
}

func NewHandler(st *store.Store, a *auth.Auth, maxRetries int) *Handler {
	return &Handler{store: st, auth: a, maxRetries: maxRetries}
}

// --- Request/Response types ---

type MailRequest struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	Text    string `json:"text"`
	HTML    string `json:"html"`
}

type WhatsAppRequest struct {
	Phone string `json:"phone"`
	Text  string `json:"text"`
}

type TelegramRequest struct {
	ChatID string `json:"chat_id"`
	Text   string `json:"text"`
}

type SendRequest = MailRequest // backwards compat

type SendResponse struct {
	ID      int64  `json:"id"`
	Status  string `json:"status"`
	Channel string `json:"channel"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

// --- Routes ---

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	// v1 (backwards compat)
	mux.HandleFunc("POST /api/v1/send", h.handleMailSend)
	// v2
	mux.HandleFunc("POST /api/v2/mail", h.handleMailSend)
	mux.HandleFunc("POST /api/v2/whatsapp", h.handleWhatsAppSend)
	mux.HandleFunc("POST /api/v2/telegram", h.handleTelegramSend)
	// health
	mux.HandleFunc("GET /api/v1/health", h.handleHealth)
	mux.HandleFunc("GET /api/v2/health", h.handleHealth)
}

// --- Handlers ---

func (h *Handler) handleMailSend(w http.ResponseWriter, r *http.Request) {
	token, err := h.auth.ValidateAPIToken(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, ErrorResponse{Error: "unauthorized: " + err.Error()})
		return
	}

	var req MailRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid JSON: " + err.Error()})
		return
	}

	if req.To == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "\"to\" is required"})
		return
	}
	if req.Subject == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "\"subject\" is required"})
		return
	}
	if req.Text == "" && req.HTML == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "\"text\" or \"html\" is required"})
		return
	}

	msg, err := h.store.EnqueueMessage(&token.ID, "mail", req.To, req.Subject, req.Text, req.HTML, h.maxRetries)
	if err != nil {
		log.Printf("ERROR enqueuing mail: %v", err)
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "failed to enqueue message"})
		return
	}

	writeJSON(w, http.StatusAccepted, SendResponse{ID: msg.ID, Status: msg.Status, Channel: "mail"})
}

func (h *Handler) handleWhatsAppSend(w http.ResponseWriter, r *http.Request) {
	token, err := h.auth.ValidateAPIToken(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, ErrorResponse{Error: "unauthorized: " + err.Error()})
		return
	}

	var req WhatsAppRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid JSON: " + err.Error()})
		return
	}

	if req.Phone == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "\"phone\" is required"})
		return
	}
	if req.Text == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "\"text\" is required"})
		return
	}

	msg, err := h.store.EnqueueMessage(&token.ID, "whatsapp", req.Phone, "", req.Text, "", h.maxRetries)
	if err != nil {
		log.Printf("ERROR enqueuing whatsapp: %v", err)
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "failed to enqueue message"})
		return
	}

	writeJSON(w, http.StatusAccepted, SendResponse{ID: msg.ID, Status: msg.Status, Channel: "whatsapp"})
}

func (h *Handler) handleTelegramSend(w http.ResponseWriter, r *http.Request) {
	token, err := h.auth.ValidateAPIToken(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, ErrorResponse{Error: "unauthorized: " + err.Error()})
		return
	}

	var req TelegramRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid JSON: " + err.Error()})
		return
	}

	if req.ChatID == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "\"chat_id\" is required"})
		return
	}
	if req.Text == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "\"text\" is required"})
		return
	}

	msg, err := h.store.EnqueueMessage(&token.ID, "telegram", req.ChatID, "", req.Text, "", h.maxRetries)
	if err != nil {
		log.Printf("ERROR enqueuing telegram: %v", err)
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "failed to enqueue message"})
		return
	}

	writeJSON(w, http.StatusAccepted, SendResponse{ID: msg.ID, Status: msg.Status, Channel: "telegram"})
}

func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
