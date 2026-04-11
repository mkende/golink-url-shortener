package server

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

const csrfCookieName = "golink_csrf"

// generateCSRFToken returns a new random base64-encoded CSRF token.
func generateCSRFToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// setCSRFCookie writes the CSRF token to a cookie on w.
func setCSRFCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     csrfCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}

// validateCSRF returns true when the form's csrf_token matches the cookie.
func validateCSRF(r *http.Request) bool {
	cookie, err := r.Cookie(csrfCookieName)
	if err != nil {
		return false
	}
	formToken := r.FormValue("csrf_token")
	return formToken != "" && formToken == cookie.Value
}

// requireCSRF validates the CSRF token and writes a 403 response if invalid.
// Returns true when the token is valid and the handler should continue.
func requireCSRF(w http.ResponseWriter, r *http.Request) bool {
	if !validateCSRF(r) {
		http.Error(w, "invalid CSRF token", http.StatusForbidden)
		return false
	}
	return true
}

// urlParamLower returns the named chi URL parameter converted to lower-case.
func urlParamLower(r *http.Request, key string) string {
	return strings.ToLower(chi.URLParam(r, key))
}
