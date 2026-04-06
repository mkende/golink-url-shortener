package middleware

import (
	"net/http"
	"net/url"

	"github.com/mkende/golink-url-shortener/internal/config"
)

// RedirectToCanonical checks whether r is already on the canonical HTTPS
// domain configured in cfg. If not, it writes a 301 redirect and returns
// true. If no canonical domain is configured, or the request is already
// on it, it returns false and leaves w untouched.
func RedirectToCanonical(cfg *config.Config, w http.ResponseWriter, r *http.Request) bool {
	if cfg.CanonicalDomain == "" {
		return false
	}
	isHTTPS := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
	if isHTTPS && r.Host == cfg.CanonicalDomain {
		return false
	}
	target := &url.URL{
		Scheme:   "https",
		Host:     cfg.CanonicalDomain,
		Path:     r.URL.Path,
		RawQuery: r.URL.RawQuery,
	}
	http.Redirect(w, r, target.String(), http.StatusMovedPermanently)
	return true
}

// DomainRedirect returns middleware that redirects requests to the canonical
// HTTPS domain if they are not already on it. This applies to all UI and API
// routes regardless of auth source; link redirects are routed separately and
// bypass this middleware entirely.
func DomainRedirect(cfg *config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if cfg.AllowHTTP {
				next.ServeHTTP(w, r)
				return
			}
			if !RedirectToCanonical(cfg, w, r) {
				next.ServeHTTP(w, r)
			}
		})
	}
}
