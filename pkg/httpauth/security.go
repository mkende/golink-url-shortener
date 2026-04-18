package httpauth

import "net/http"

// cspPolicy is the Content-Security-Policy applied to every response.
// All script and style assets are self-hosted, so 'self' is sufficient for
// both directives — no CDN origins or 'unsafe-inline' are needed.
const cspPolicy = "default-src 'self'; " +
	"script-src 'self'; " +
	"style-src 'self'; " +
	"img-src 'self' data:; " +
	"connect-src 'self'; " +
	"font-src 'self'; " +
	"object-src 'none'; " +
	"base-uri 'self'; " +
	"form-action 'self'; " +
	"frame-ancestors 'self'"

// securityHeaders returns middleware that adds defensive HTTP headers to every
// response. When httpsOnly is true, Strict-Transport-Security is also set; the
// header is intentionally omitted for plain-HTTP deployments so it does not
// interfere with HTTP access.
func securityHeaders(httpsOnly bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("X-Frame-Options", "SAMEORIGIN")
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
			h.Set("Content-Security-Policy", cspPolicy)
			if httpsOnly {
				h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			}
			next.ServeHTTP(w, r)
		})
	}
}
