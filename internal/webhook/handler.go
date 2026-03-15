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

type SendRequest struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	Text    string `json:"text"`
	HTML    string `json:"html"`
}

type SendResponse struct {
	ID     int64  `json:"id"`
	Status string `json:"status"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/send", h.handleSend)
	mux.HandleFunc("GET /api/v1/health", h.handleHealth)
}

func (h *Handler) handleSend(w http.ResponseWriter, r *http.Request) {
	token, err := h.auth.ValidateAPIToken(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, ErrorResponse{Error: "unauthorized: " + err.Error()})
		return
	}

	var req SendRequest
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

	msg, err := h.store.EnqueueMessage(&token.ID, req.To, req.Subject, req.Text, req.HTML, h.maxRetries)
	if err != nil {
		log.Printf("ERROR enqueuing message: %v", err)
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "failed to enqueue message"})
		return
	}

	writeJSON(w, http.StatusAccepted, SendResponse{ID: msg.ID, Status: msg.Status})
}

func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
