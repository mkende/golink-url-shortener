package auth

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"github.com/mkende/golink-url-shortener/internal/config"
)

// isAPIRequest reports whether the request targets an API path or the client
// signals it expects JSON via the Accept header.
func isAPIRequest(r *http.Request) bool {
	return strings.HasPrefix(r.URL.Path, "/api/") ||
		strings.Contains(r.Header.Get("Accept"), "application/json")
}

// writeJSONMiddlewareError writes a JSON {"error": message} response. It is
// used by auth middleware which cannot import the server package.
func writeJSONMiddlewareError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message}) //nolint:errcheck
}

// LoginRedirect redirects the user to the OIDC login page, encoding the
// current request URI as the ?rd= post-login destination.
func LoginRedirect(w http.ResponseWriter, r *http.Request) {
	loginURL := "/auth/login?rd=" + url.QueryEscape(r.URL.RequestURI())
	http.Redirect(w, r, loginURL, http.StatusFound)
}

// RequireAuth returns a middleware that enforces authentication.
//
// For API paths (those starting with "/api/") an unauthenticated request
// receives a 401 JSON response; REST clients cannot follow HTML redirects.
//
// For all other paths: if OIDC is enabled the user is redirected to the login
// page; otherwise a 403 response is written.
//
// When DomainRedirect runs before this middleware (as it does for all UI and
// API routes), the request is already on the canonical domain so the login
// redirect will set the session cookie on the correct domain.
func RequireAuth(cfg *config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if FromContext(r.Context()) != nil {
				next.ServeHTTP(w, r)
				return
			}
			if isAPIRequest(r) {
				writeJSONMiddlewareError(w, http.StatusUnauthorized, "authentication required")
				return
			}
			if cfg.OIDC.Enabled {
				LoginRedirect(w, r)
			} else {
				http.Error(w, "forbidden", http.StatusForbidden)
			}
		})
	}
}

// RequireAdmin returns a middleware that enforces admin access. Non-admin
// requests are passed to deniedHandler.
func RequireAdmin(deniedHandler http.Handler) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := FromContext(r.Context())
			if id == nil || !id.IsAdmin {
				deniedHandler.ServeHTTP(w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireWriteScope returns a middleware that blocks read-only API key bearers
// from accessing endpoints that mutate state. It must run after APIKeyMiddleware
// so the identity's APIKeyReadOnly field is populated.
//
// Identities established via session (OIDC, Tailscale) are never read-only and
// always pass through.
func RequireWriteScope() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if id := FromContext(r.Context()); id != nil && id.APIKeyReadOnly {
				writeJSONMiddlewareError(w, http.StatusForbidden, "read-only API key cannot perform write operations")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
