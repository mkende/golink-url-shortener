package middleware

import (
	"net/http"
	"net/url"

	"github.com/mkende/golink-url-shortener/internal/config"
)

// DomainRedirect returns middleware that redirects requests to the canonical
// HTTPS domain if they are not already on it. This applies to all UI and API
// routes regardless of auth source; link redirects are routed separately and
// bypass this middleware entirely.
func DomainRedirect(cfg *config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if cfg.CanonicalDomain == "" || cfg.AllowHTTP {
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
