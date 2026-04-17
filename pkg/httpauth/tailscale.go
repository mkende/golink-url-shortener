package httpauth

import (
	"net"
	"net/http"
)

// tailscaleMiddleware reads Tailscale-User-* headers and populates the
// identity context. It is a no-op when Tailscale auth is disabled, when the
// Tailscale-User-Login header is absent, or when the request originates from
// an IP outside the configured trusted proxy CIDRs.
//
// Note: Tailscale only injects these headers when using `tailscale serve` in
// HTTP proxy mode; plain TCP forwarding does not inject them.
func (m *AuthManager) tailscaleMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !m.cfg.Tailscale.Enabled {
				next.ServeHTTP(w, r)
				return
			}
			login := r.Header.Get("Tailscale-User-Login")
			if login == "" {
				m.logger.DebugContext(r.Context(), "tailscale: no Tailscale-User-Login header")
				next.ServeHTTP(w, r)
				return
			}
			if !m.fromTrustedPeer(r) {
				m.logger.DebugContext(r.Context(), "tailscale: request from untrusted IP, ignoring headers",
					"remote_ip", peerIPStr(r),
				)
				next.ServeHTTP(w, r)
				return
			}

			id := &Identity{
				Email:       login,
				DisplayName: r.Header.Get("Tailscale-User-Name"),
				AvatarURL:   r.Header.Get("Tailscale-User-Profile-Pic"),
				Source:      AuthSourceTailscale,
			}
			id.IsAdmin = isAdmin(m.cfg, id)
			m.logger.DebugContext(r.Context(), "tailscale: identity established",
				"email", id.Email,
				"is_admin", id.IsAdmin,
			)
			m.notifyAuth(id.Email, id.DisplayName, id.AvatarURL)
			next.ServeHTTP(w, r.WithContext(WithIdentity(r.Context(), id)))
		})
	}
}

// fromTrustedPeer returns true when no trusted networks are configured (open)
// or when the peer IP falls within the trusted networks.
func (m *AuthManager) fromTrustedPeer(r *http.Request) bool {
	if len(m.trustedNets) == 0 {
		return true
	}
	ip := peerIP(r)
	return ip != nil && ipInRanges(ip, m.trustedNets)
}

// peerIPStr returns the peer IP as a string for logging.
func peerIPStr(r *http.Request) string {
	ip := peerIP(r)
	if ip == nil {
		return "<unknown>"
	}
	return ip.String()
}

// trustedPeerIP returns the peer IP only when it falls within trusted networks,
// or nil otherwise. Used by providers that require the request to come from a
// trusted proxy.
func (m *AuthManager) trustedPeerIP(r *http.Request) net.IP {
	ip := peerIP(r)
	if ip == nil || (len(m.trustedNets) > 0 && !ipInRanges(ip, m.trustedNets)) {
		return nil
	}
	return ip
}
