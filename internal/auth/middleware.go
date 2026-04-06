package auth

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/mkende/golink-url-shortener/internal/config"
)

// RequireAuth returns a middleware that enforces authentication.
//
// For API paths (those starting with "/api/") an unauthenticated request
// receives a 401 JSON response; REST clients cannot follow HTML redirects.
//
// For all other paths the user is redirected to /auth/login with the current
// path as the ?rd= parameter. For OIDC auth the redirect goes through the
// canonical domain first so the session cookie is set on the correct domain.
func RequireAuth(cfg *config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := FromContext(r.Context())
			if id == nil {
				if strings.HasPrefix(r.URL.Path, "/api/") {
					w.Header().Set("Content-Type", "application/json")
					http.Error(w, `{"error":"authentication required"}`, http.StatusUnauthorized)
					return
				}
				loginURL := "/auth/login?rd=" + url.QueryEscape(r.URL.RequestURI())
				if cfg.OIDC.Enabled && cfg.CanonicalDomain != "" {
					loginURL = "https://" + cfg.CanonicalDomain + loginURL
				}
				http.Redirect(w, r, loginURL, http.StatusFound)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireAdmin returns a middleware that enforces admin access. Non-admin
// requests receive a 403 Forbidden response.
func RequireAdmin() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := FromContext(r.Context())
			if id == nil || !id.IsAdmin {
				http.Error(w, "forbidden", http.StatusForbidden)
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
				w.Header().Set("Content-Type", "application/json")
				http.Error(w, `{"error":"read-only API key cannot perform write operations"}`, http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
