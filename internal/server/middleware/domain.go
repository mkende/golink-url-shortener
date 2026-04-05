package middleware

import (
	"net/http"
	"net/url"

	"github.com/mkende/golink-redirector/internal/auth"
	"github.com/mkende/golink-redirector/internal/config"
)

// DomainRedirect returns middleware that redirects requests to the canonical
// HTTPS domain if they are not already on it.
//
// Requests authenticated via Tailscale or reverse-proxy forward-auth headers
// are exempt: those deployments sit behind an internal proxy that controls the
// hostname, so enforcing the canonical domain would create a redirect loop.
func DomainRedirect(cfg *config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if cfg.CanonicalDomain == "" || cfg.AllowHTTP {
				next.ServeHTTP(w, r)
				return
			}

			// Skip domain redirect for header-based auth sources: the request
			// already passed through a trusted proxy and the hostname may differ
			// from the canonical domain by design.
			if id := auth.FromContext(r.Context()); id != nil {
				switch id.Source {
				case auth.AuthSourceTailscale, auth.AuthSourceProxy:
					next.ServeHTTP(w, r)
					return
				}
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
