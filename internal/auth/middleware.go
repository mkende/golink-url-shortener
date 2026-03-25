package auth

import (
	"net/http"
	"net/url"

	"github.com/mkende/golink-redirector/internal/config"
)

// RequireAuth returns a middleware that enforces authentication. If the user is
// not authenticated they are redirected to /auth/login with the current path as
// the ?rd= parameter. For OIDC auth, the redirect goes through the canonical
// domain first so the session cookie is issued on the correct domain.
func RequireAuth(cfg *config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := FromContext(r.Context())
			if id == nil {
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
