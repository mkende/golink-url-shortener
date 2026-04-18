package server

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/mkende/golink-url-shortener/internal/db"
	"github.com/mkende/golink-url-shortener/pkg/httpauth"
)

// apiKeysPageData is the template data for /apikeys.
type apiKeysPageData struct {
	baseData
	Keys   []*db.APIKey
	NewKey string
	Error  string
}

// renderAPIKeysError re-renders the /apikeys page with an error message.
func (s *Server) renderAPIKeysError(w http.ResponseWriter, r *http.Request, base baseData, msg string) {
	keys, _ := s.apiKeys.List(r.Context())
	s.renderer.Render(w, "apikeys", apiKeysPageData{baseData: base, Keys: keys, Error: msg})
}

// handleAPIKeysPage serves GET /apikeys (admin only).
func (s *Server) handleAPIKeysPage(w http.ResponseWriter, r *http.Request) {
	base, err := s.newBaseData(w, r)
	if err != nil {
		s.logr(r.Context()).Error("apikeys: baseData", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	keys, err := s.apiKeys.List(r.Context())
	if err != nil {
		s.logr(r.Context()).Error("apikeys: list", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	s.renderer.Render(w, "apikeys", apiKeysPageData{
		baseData: base,
		Keys:     keys,
	})
}

// handleCreateAPIKey serves POST /apikeys (admin only).
// Creates a new API key, shows the raw value once in the response page.
func (s *Server) handleCreateAPIKey(w http.ResponseWriter, r *http.Request) {
	base, err := s.newBaseData(w, r)
	if err != nil {
		s.logr(r.Context()).Error("apikeys create: baseData", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	if !requireCSRF(w, r) {
		return
	}

	id := httpauth.IdentityFromContext(r.Context())
	if id == nil {
		http.Redirect(w, r, "/auth/login?rd=/apikeys", http.StatusFound)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	// The form sends "true" for read-only (default) and "false" for read-write.
	readOnly := r.FormValue("read_only") != "false"

	if name == "" {
		s.renderAPIKeysError(w, r, base, "Key description is required.")
		return
	}

	rawKey, err := generateAPIKey()
	if err != nil {
		s.logr(r.Context()).Error("apikeys create: generate key", "error", err)
		s.renderAPIKeysError(w, r, base, "Could not generate API key.")
		return
	}

	if _, err := s.apiKeys.Create(r.Context(), name, httpauth.HashAPIKey(rawKey), id.Email, readOnly); err != nil {
		s.logr(r.Context()).Error("apikeys create: db create", "error", err)
		s.renderAPIKeysError(w, r, base, "Could not create API key. Please try again.")
		return
	}

	keys, err := s.apiKeys.List(r.Context())
	if err != nil {
		s.logr(r.Context()).Error("apikeys create: list after create", "error", err)
	}

	s.renderer.Render(w, "apikeys", apiKeysPageData{
		baseData: base,
		Keys:     keys,
		NewKey:   rawKey,
	})
}

// handleDeleteAPIKey serves POST /apikeys/{id}/delete (admin only).
func (s *Server) handleDeleteAPIKey(w http.ResponseWriter, r *http.Request) {
	if !requireCSRF(w, r) {
		return
	}

	idParam := chi.URLParam(r, "id")
	keyID, err := strconv.ParseInt(idParam, 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	if err := s.apiKeys.Delete(r.Context(), keyID); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		s.logr(r.Context()).Error("apikeys delete: db delete", "id", keyID, "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/apikeys", http.StatusFound)
}
