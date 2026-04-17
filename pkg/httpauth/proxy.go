package httpauth

import (
	"net/http"
	"strings"
)

// proxyAuthMiddleware reads forward-auth headers injected by a trusted reverse
// proxy and populates the identity context. It is a no-op when proxy auth is
// disabled, when the request originates outside the configured trusted CIDRs,
// or when a prior provider has already set an identity.
func (m *AuthManager) proxyAuthMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !m.cfg.ProxyAuth.Enabled || IdentityFromContext(r.Context()) != nil {
				next.ServeHTTP(w, r)
				return
			}

			if len(m.trustedNets) > 0 && m.trustedPeerIP(r) == nil {
				m.logger.DebugContext(r.Context(), "proxy_auth: request from untrusted IP, ignoring headers",
					"remote_ip", peerIPStr(r),
				)
				next.ServeHTTP(w, r)
				return
			}

			email := r.Header.Get(m.cfg.ProxyAuth.EmailHeader)
			if email == "" {
				email = r.Header.Get(m.cfg.ProxyAuth.UserHeader)
			}
			if email == "" {
				m.logger.DebugContext(r.Context(), "proxy_auth: trusted IP but no identity headers present")
				next.ServeHTTP(w, r)
				return
			}

			id := &Identity{
				Email:       email,
				DisplayName: r.Header.Get(m.cfg.ProxyAuth.NameHeader),
				Source:      AuthSourceProxy,
			}
			if raw := r.Header.Get(m.cfg.ProxyAuth.GroupsHeader); raw != "" {
				id.Groups = splitGroups(raw)
			}
			id.IsAdmin = isAdmin(m.cfg, id)
			m.logger.DebugContext(r.Context(), "proxy_auth: identity established",
				"email", id.Email,
				"is_admin", id.IsAdmin,
			)
			m.notifyAuth(id.Email, id.DisplayName, "")
			next.ServeHTTP(w, r.WithContext(WithIdentity(r.Context(), id)))
		})
	}
}

// splitGroups splits a comma-separated groups string, trimming whitespace and
// omitting empty entries.
func splitGroups(raw string) []string {
	parts := strings.Split(raw, ",")
	groups := make([]string, 0, len(parts))
	for _, p := range parts {
		if g := strings.TrimSpace(p); g != "" {
			groups = append(groups, g)
		}
	}
	return groups
}
