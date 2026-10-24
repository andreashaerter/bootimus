package admin

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"bootimus/internal/auth"
	"bootimus/internal/models"

	"gorm.io/gorm"
)

const maxAPITokenNameLength = 64

func (h *Handler) APITokens(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.listAPITokens(w)
	case http.MethodPost:
		h.createAPIToken(w, r)
	case http.MethodDelete:
		h.revokeAPIToken(w, r)
	default:
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
	}
}

func (h *Handler) listAPITokens(w http.ResponseWriter) {
	tokens, err := h.storage.ListAPITokens()
	if err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}
	if tokens == nil {
		tokens = []*models.APIToken{}
	}
	h.sendJSON(w, http.StatusOK, Response{Success: true, Data: tokens})
}

func (h *Handler) createAPIToken(w http.ResponseWriter, r *http.Request) {
	actor := auth.UsernameFromContext(r.Context())
	if auth.IsTokenPrincipal(actor) {
		h.sendJSON(w, http.StatusForbidden, Response{Success: false, Error: "API tokens cannot create other tokens; sign in as a user"})
		return
	}
	var req struct {
		Name          string `json:"name"`
		Scope         string `json:"scope"`
		ExpiresInDays int    `json:"expires_in_days"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Invalid request"})
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || len(req.Name) > maxAPITokenNameLength {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "A token name of up to 64 characters is required"})
		return
	}
	if req.Scope == "" {
		req.Scope = models.APITokenScopeRead
	}
	if req.Scope != models.APITokenScopeRead && req.Scope != models.APITokenScopeAdmin {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Scope must be read or admin"})
		return
	}
	if req.ExpiresInDays < 0 {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Expiry cannot be negative"})
		return
	}
	secret, err := auth.GenerateAPISecret()
	if err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: "Failed to generate token"})
		return
	}
	token := &models.APIToken{
		Name:      req.Name,
		Prefix:    auth.APISecretPrefix(secret),
		Hash:      auth.HashAPISecret(secret),
		Scope:     req.Scope,
		CreatedBy: actor,
	}
	if req.ExpiresInDays > 0 {
		expires := time.Now().AddDate(0, 0, req.ExpiresInDays)
		token.ExpiresAt = &expires
	}
	if err := h.storage.CreateAPIToken(token); err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}
	log.Printf("API token created: %s (scope=%s, by=%s)", token.Name, token.Scope, actor)
	h.sendJSON(w, http.StatusCreated, Response{Success: true, Message: "Token created. Copy it now; it will not be shown again.", Data: map[string]interface{}{
		"token":  token,
		"secret": secret,
	}})
}

func (h *Handler) revokeAPIToken(w http.ResponseWriter, r *http.Request) {
	actor := auth.UsernameFromContext(r.Context())
	if auth.IsTokenPrincipal(actor) {
		h.sendJSON(w, http.StatusForbidden, Response{Success: false, Error: "API tokens cannot revoke tokens; sign in as a user"})
		return
	}
	id, err := strconv.ParseUint(r.URL.Query().Get("id"), 10, 64)
	if err != nil || id == 0 {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Token id is required"})
		return
	}
	token, err := h.storage.GetAPIToken(uint(id))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		h.sendJSON(w, http.StatusNotFound, Response{Success: false, Error: "Token not found"})
		return
	}
	if err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}
	if err := h.storage.RevokeAPIToken(token.ID); err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}
	log.Printf("API token revoked: %s (by=%s)", token.Name, actor)
	h.sendJSON(w, http.StatusOK, Response{Success: true, Message: "Token revoked"})
}
