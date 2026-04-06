package middleware

import (
	"net/http"
	"net/url"

	"github.com/mkende/golink-url-shortener/internal/auth"
	"github.com/mkende/golink-url-shortener/internal/config"
)

// RequireUIAccess returns a middleware that gates access to UI pages based on
// the AllowLoggedOutUIAccess configuration option (default false).
//
// When AllowLoggedOutUIAccess is false, unauthenticated requests are handled in
// priority order:
//  1. OIDC enabled → 302 redirect to /auth/login (via canonical domain if set).
//  2. Canonical domain set and request not already on it over HTTPS → 301
//     redirect to the canonical HTTPS URL (via RedirectToCanonical). This
//     covers the AllowHTTP=true case where DomainRedirect is a no-op.
//  3. Otherwise → deniedHandler is invoked (caller controls the response).
//
// Anonymous users, Tailscale users, and proxy-auth users always have a non-nil
// Identity and pass through unconditionally.
func RequireUIAccess(cfg *config.Config, deniedHandler http.Handler) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if cfg.UI.AllowLoggedOutUIAccess {
				next.ServeHTTP(w, r)
				return
			}
			if auth.FromContext(r.Context()) != nil {
				next.ServeHTTP(w, r)
				return
			}
			if cfg.OIDC.Enabled {
				loginURL := "/auth/login?rd=" + url.QueryEscape(r.URL.RequestURI())
				if cfg.CanonicalDomain != "" {
					loginURL = "https://" + cfg.CanonicalDomain + loginURL
				}
				http.Redirect(w, r, loginURL, http.StatusFound)
				return
			}
			if RedirectToCanonical(cfg, w, r) {
				return
			}
			deniedHandler.ServeHTTP(w, r)
		})
	}
}
