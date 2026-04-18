package httpauth

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
)

// isAPIRequest reports whether the request targets an API path or the client
// signals it expects JSON via the Accept header.
func isAPIRequest(r *http.Request) bool {
	return strings.HasPrefix(r.URL.Path, "/api/") ||
		strings.Contains(r.Header.Get("Accept"), "application/json")
}

// writeJSONError writes a {"error": message} JSON response.
func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message}) //nolint:errcheck
}

// loginRedirect redirects to the OIDC login page, encoding the current
// request URI as the ?rd= post-login destination.
func loginRedirect(w http.ResponseWriter, r *http.Request) {
	target := loginPath + "?rd=" + url.QueryEscape(r.URL.RequestURI())
	http.Redirect(w, r, target, http.StatusFound)
}

// RequireAuth returns a middleware that enforces authentication.
//
// For API paths (those starting with "/api/") or requests that declare
// "application/json" in their Accept header, an unauthenticated request
// receives a 401 JSON response.
//
// For all other paths: when a login flow is available (OIDC is enabled) the
// browser is redirected to the login page; otherwise denied is invoked.
// Callers control the denied response — typically a 401 or 403 HTML page.
func (m *AuthManager) RequireAuth(denied http.HandlerFunc) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if IdentityFromContext(r.Context()) != nil {
				next.ServeHTTP(w, r)
				return
			}
			if isAPIRequest(r) {
				writeJSONError(w, http.StatusUnauthorized, "authentication required")
				return
			}
			if m.oidcH != nil {
				loginRedirect(w, r)
				return
			}
			denied(w, r)
		})
	}
}

// RequireAdmin returns a middleware that enforces admin access. Requests from
// non-admin identities are passed to denied. It must run after an auth
// provider has had the chance to set the identity.
func (m *AuthManager) RequireAdmin(denied http.HandlerFunc) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := IdentityFromContext(r.Context())
			if id == nil || !id.IsAdmin {
				denied(w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireWriteScope returns a middleware that blocks read-only API key bearers
// from accessing endpoints that mutate state. It must run after
// [AuthManager.APIKeyMiddleware] so that Identity.APIKeyReadOnly is populated.
// Identities established via session (OIDC, Tailscale, proxy) are never
// read-only and always pass through.
func (m *AuthManager) RequireWriteScope() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if id := IdentityFromContext(r.Context()); id != nil && id.APIKeyReadOnly {
				writeJSONError(w, http.StatusForbidden, "read-only API key cannot perform write operations")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
