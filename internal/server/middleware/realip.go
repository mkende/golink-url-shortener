package middleware

import (
	"net"
	"net/http"
	"strings"

	"github.com/mkende/golink-url-shortener/internal/auth"
)

// PreserveRemoteAddr saves r.RemoteAddr into the request context before any
// RealIP middleware can overwrite it with the X-Forwarded-For value. The saved
// address is later read by CIDR-based auth middlewares (Tailscale, proxy auth)
// to verify that headers arrive from a trusted network range, using the actual
// TCP connection address rather than the spoofable forwarded header.
//
// This middleware must be registered before TrustedRealIP.
func PreserveRemoteAddr(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := auth.WithOriginalRemoteAddr(r.Context(), r.RemoteAddr)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// TrustedRealIP returns middleware that rewrites r.RemoteAddr from forwarded
// headers only when the actual TCP peer falls within one of trustedNets.
// Requests from untrusted peers keep their real connection address, so a direct
// client cannot spoof its logged IP.
//
// It must be registered after PreserveRemoteAddr so the trust decision is based
// on the genuine peer address. When trustedNets is empty no rewriting is done.
func TrustedRealIP(trustedNets []*net.IPNet) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if len(trustedNets) > 0 {
				if peer := auth.PeerIP(r); peer != nil && auth.IPInRanges(peer, trustedNets) {
					if ip := forwardedClientIP(r, trustedNets); ip != "" {
						r.RemoteAddr = ip
					}
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// forwardedClientIP resolves the real client IP from forwarded headers, given
// the set of trusted proxy networks. Returns "" when no usable address is found.
//
// X-Forwarded-For (across every header line, comma-split) is preferred and is
// walked right-to-left, skipping addresses that are themselves trusted proxies.
// The first non-trusted address is the genuine client: because each proxy
// appends the address it received the request from, any value a client injects
// ends up to the left of the address its trusted proxy appended, so it can never
// be mistaken for the real client. If every hop is a trusted proxy the leftmost
// (original) entry is used. When no X-Forwarded-For is present, the single-value
// True-Client-IP and X-Real-IP headers (set by the trusted proxy) are used as a
// fallback, in that order.
func forwardedClientIP(r *http.Request, trustedNets []*net.IPNet) string {
	if entries := xffEntries(r); len(entries) > 0 {
		for i := len(entries) - 1; i >= 0; i-- {
			ip := parseForwardedIP(entries[i])
			if ip == nil {
				continue
			}
			if !auth.IPInRanges(ip, trustedNets) {
				return ip.String()
			}
		}
		// Every hop was a trusted proxy: fall back to the original (leftmost) entry.
		if ip := parseForwardedIP(entries[0]); ip != nil {
			return ip.String()
		}
	}
	for _, header := range []string{"True-Client-IP", "X-Real-IP"} {
		if ip := parseForwardedIP(r.Header.Get(header)); ip != nil {
			return ip.String()
		}
	}
	return ""
}

// xffEntries returns every X-Forwarded-For address in order, flattening
// multiple header lines and comma-separated lists and dropping empty entries.
func xffEntries(r *http.Request) []string {
	var entries []string
	for _, line := range r.Header.Values("X-Forwarded-For") {
		for _, part := range strings.Split(line, ",") {
			if p := strings.TrimSpace(part); p != "" {
				entries = append(entries, p)
			}
		}
	}
	return entries
}

// parseForwardedIP parses a single forwarded-header value into an IP, accepting
// a bare address, a host:port pair, or a bracketed IPv6 address. Returns nil
// when the value is empty or not a valid IP.
func parseForwardedIP(s string) net.IP {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	if ip := net.ParseIP(s); ip != nil {
		return ip
	}
	if host, _, err := net.SplitHostPort(s); err == nil {
		return net.ParseIP(host)
	}
	return nil
}
