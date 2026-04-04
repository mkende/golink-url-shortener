package middleware

import (
	"net/http"
	"net/url"

	"github.com/mkende/golink-redirector/internal/config"
)

// DomainRedirect returns middleware that redirects requests to the canonical
// HTTPS domain if they are not already on it.
// Requests with the Tailscale-User-Login header are exempt (Tailscale auth).
func DomainRedirect(cfg *config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if cfg.CanonicalDomain == "" || cfg.AllowHTTP {
				next.ServeHTTP(w, r)
				return
			}

			// Tailscale: skip domain redirect if the Tailscale header is present.
			if cfg.Tailscale.Enabled && r.Header.Get("Tailscale-User-Login") != "" {
				next.ServeHTTP(w, r)
				return
			}

			// Check if already on canonical HTTPS.
			host := r.Host
			if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
				if host == cfg.CanonicalDomain {
					next.ServeHTTP(w, r)
					return
				}
			}

			// Redirect to canonical domain.
			target := &url.URL{
				Scheme:   "https",
				Host:     cfg.CanonicalDomain,
				Path:     r.URL.Path,
				RawQuery: r.URL.RawQuery,
			}
			http.Redirect(w, r, target.String(), http.StatusMovedPermanently)
		})
	}
}
