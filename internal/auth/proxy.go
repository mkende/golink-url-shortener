package auth

import (
	"context"
	"net"
	"net/http"
	"strings"

	"github.com/mkende/golink-redirector/internal/config"
	"github.com/mkende/golink-redirector/internal/db"
)

// ProxyAuthMiddleware reads forward-auth headers injected by a trusted reverse
// proxy (nginx, Caddy, Traefik, Authelia, …) and populates the identity
// context. The middleware is a no-op when proxy auth is disabled or when the
// request arrives from an IP outside the configured trusted CIDR ranges.
//
// Header names default to the de-facto standard used by Authelia:
//
//   - Remote-User   — username / login name (fallback identifier)
//   - Remote-Email  — email address (preferred primary identifier)
//   - Remote-Name   — display name
//   - Remote-Groups — comma-separated group memberships
//
// Identity.Email is set from EmailHeader when present; otherwise UserHeader is
// used as the identifier. All four header names are configurable via
// ProxyAuthConfig.
func ProxyAuthMiddleware(cfg *config.Config, users db.UserRepo) func(http.Handler) http.Handler {
	// Pre-parse CIDRs once at construction time.
	var trustedNets []*net.IPNet
	if len(cfg.ProxyAuth.TrustedCIDRs) > 0 {
		nets, err := parseCIDRs(cfg.ProxyAuth.TrustedCIDRs)
		if err != nil {
			// Config validation should have caught this.
			panic("proxy_auth: invalid trusted_cidrs in config: " + err.Error())
		}
		trustedNets = nets
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip entirely if another middleware already established an identity.
			if !cfg.ProxyAuth.Enabled || FromContext(r.Context()) != nil {
				next.ServeHTTP(w, r)
				return
			}

			// Only accept headers from trusted IP ranges.
			ip := remoteIP(r)
			if ip == nil || !ipInRanges(ip, trustedNets) {
				next.ServeHTTP(w, r)
				return
			}

			// Determine the primary identifier. Prefer the dedicated email header
			// (Remote-Email by default) over the username header (Remote-User),
			// because Identity.Email is the system-wide user key. Fall back to
			// UserHeader when EmailHeader is absent or empty.
			email := r.Header.Get(cfg.ProxyAuth.EmailHeader)
			if email == "" {
				email = r.Header.Get(cfg.ProxyAuth.UserHeader)
			}
			if email == "" {
				// Proxy is trusted but sent no identity headers — unauthenticated.
				next.ServeHTTP(w, r)
				return
			}

			id := &Identity{
				Email:       email,
				DisplayName: r.Header.Get(cfg.ProxyAuth.NameHeader),
				Source:      AuthSourceProxy,
			}

			if raw := r.Header.Get(cfg.ProxyAuth.GroupsHeader); raw != "" {
				id.Groups = splitGroups(raw)
			}

			id.IsAdmin = isAdmin(cfg, id)

			if users != nil {
				go func() {
					if _, err := users.Upsert(context.Background(), id.Email, id.DisplayName, ""); err != nil {
						_ = err
					}
				}()
			}

			next.ServeHTTP(w, r.WithContext(WithIdentity(r.Context(), id)))
		})
	}
}

// splitGroups splits a comma-separated groups string, trimming whitespace
// around each entry and omitting empty entries.
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
