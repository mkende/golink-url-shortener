package httpauth

import (
	"net"
	"net/http"
	"strings"
)

// realIPMiddleware reads the X-Real-IP or X-Forwarded-For header and updates
// r.RemoteAddr to the client IP. It must run after preserveRemoteAddr so that
// CIDR-based auth middleware can still access the original TCP address via
// [WithOriginalRemoteAddr].
func realIPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ip := extractRealIP(r); ip != "" {
			r.RemoteAddr = ip
		}
		next.ServeHTTP(w, r)
	})
}

// extractRealIP returns the client IP from X-Real-IP or the first address in
// X-Forwarded-For. Returns "" if neither header is present or the value is not
// a valid IP address.
func extractRealIP(r *http.Request) string {
	var ip string
	if xrip := r.Header.Get("X-Real-IP"); xrip != "" {
		ip = xrip
	} else if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		i := strings.Index(xff, ", ")
		if i == -1 {
			i = len(xff)
		}
		ip = xff[:i]
	}
	if ip == "" || net.ParseIP(ip) == nil {
		return ""
	}
	return ip
}

// preserveRemoteAddr saves r.RemoteAddr into the request context before
// realIPMiddleware can overwrite it with the X-Forwarded-For value. The saved
// address is later read by CIDR-based auth middlewares (Tailscale, proxy auth)
// to verify that headers arrive from a trusted network range, using the actual
// TCP connection address rather than the spoofable forwarded header.
func preserveRemoteAddr(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := WithOriginalRemoteAddr(r.Context(), r.RemoteAddr)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
