package auth

import (
	"context"
	"net"
	"net/http"

	"github.com/mkende/golink-url-shortener/internal/config"
	"github.com/mkende/golink-url-shortener/internal/db"
)

// TailscaleMiddleware reads Tailscale-User-* headers and populates the identity
// context. If the header is absent or Tailscale auth is disabled, it passes
// through unchanged. When a UserRepo is provided, the user record is upserted
// asynchronously on each authenticated request.
//
// When cfg.TrustedProxy is non-empty the headers are only accepted from
// requests whose original TCP remote address falls within one of those ranges;
// headers from other IPs are silently ignored.
func TailscaleMiddleware(cfg *config.Config, users db.UserRepo) func(http.Handler) http.Handler {
	// Pre-parse CIDRs once at construction time so the hot path is allocation-free.
	var trustedNets []*net.IPNet
	if len(cfg.TrustedProxy) > 0 {
		nets, err := ParseCIDRs(cfg.TrustedProxy)
		if err != nil {
			// Config validation should have caught this; panic loudly in case it
			// slips through (programmer error, not a runtime error).
			panic("tailscale: invalid trusted_proxy in config: " + err.Error())
		}
		trustedNets = nets
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !cfg.Tailscale.Enabled {
				next.ServeHTTP(w, r)
				return
			}
			login := r.Header.Get("Tailscale-User-Login")
			if login == "" {
				next.ServeHTTP(w, r)
				return
			}
			// If trusted CIDRs are configured, reject headers from untrusted IPs.
			if len(trustedNets) > 0 {
				ip := remoteIP(r)
				if ip == nil || !IPInRanges(ip, trustedNets) {
					next.ServeHTTP(w, r)
					return
				}
			}

			id := &Identity{
				Email:       login,
				DisplayName: r.Header.Get("Tailscale-User-Name"),
				AvatarURL:   r.Header.Get("Tailscale-User-Profile-Pic"),
				Source:      AuthSourceTailscale,
			}
			id.IsAdmin = isAdmin(cfg, id)

			if users != nil {
				go func() {
					if _, err := users.Upsert(context.Background(), id.Email, id.DisplayName, id.AvatarURL); err != nil {
						// Best-effort; errors are not surfaced to the caller.
						_ = err
					}
				}()
			}

			next.ServeHTTP(w, r.WithContext(WithIdentity(r.Context(), id)))
		})
	}
}

// isAdmin reports whether the given identity has admin privileges according to
// the config's admin_emails list and admin_groups setting.
func isAdmin(cfg *config.Config, id *Identity) bool {
	for _, email := range cfg.AdminEmails {
		if email == id.Email {
			return true
		}
	}
	for _, adminGroup := range cfg.AdminGroups {
		for _, g := range id.Groups {
			if g == adminGroup {
				return true
			}
		}
	}
	return false
}
