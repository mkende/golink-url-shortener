package auth

import (
	"log/slog"
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
// cfg.TrustedProxy must be non-empty (enforced by config validation); headers
// are only accepted from requests whose original TCP remote address falls
// within one of those ranges. Headers from other IPs are silently ignored.
//
// Note: Tailscale only injects Tailscale-User-* headers when using
// `tailscale serve` in HTTP proxy mode (i.e. via Handlers, not TCPForward).
// Plain TCP forwarding does not inject these headers.
//
// If logger is nil, slog.Default() is used.
func TailscaleMiddleware(cfg *config.Config, users db.UserRepo, logger *slog.Logger) func(http.Handler) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
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
				logger.DebugContext(r.Context(), "tailscale: no Tailscale-User-Login header; headers are only injected by `tailscale serve` HTTP proxy mode, not TCPForward")
				next.ServeHTTP(w, r)
				return
			}
			// If trusted CIDRs are configured, reject headers from untrusted IPs.
			if len(trustedNets) > 0 {
				ip := PeerIP(r)
				if ip == nil || !IPInRanges(ip, trustedNets) {
					logger.DebugContext(r.Context(), "tailscale: request from untrusted IP, ignoring headers",
						"remote_ip", ip,
						"trusted_cidrs", cfg.TrustedProxy,
					)
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
			logger.DebugContext(r.Context(), "tailscale: identity established",
				"email", id.Email,
				"is_admin", id.IsAdmin,
			)

			upsertUserAsync(logger, users, id.Email, id.DisplayName, id.AvatarURL)

			next.ServeHTTP(w, r.WithContext(WithIdentity(r.Context(), id)))
		})
	}
}

