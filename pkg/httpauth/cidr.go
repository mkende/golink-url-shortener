package httpauth

import (
	"context"
	"net"
	"net/http"
)

type origRemoteAddrKey struct{}

// WithOriginalRemoteAddr returns a context carrying the raw TCP remote address.
// Call this (or use [AuthManager.Middleware] which calls it automatically)
// before any RealIP middleware so that CIDR checks use the actual connecting
// IP rather than the spoofable X-Forwarded-For value.
func WithOriginalRemoteAddr(ctx context.Context, addr string) context.Context {
	return context.WithValue(ctx, origRemoteAddrKey{}, addr)
}

// originalRemoteAddr returns the raw TCP remote address stored by
// [WithOriginalRemoteAddr], or "" if not set.
func originalRemoteAddr(ctx context.Context) string {
	v, _ := ctx.Value(origRemoteAddrKey{}).(string)
	return v
}

// parseCIDRs parses a slice of CIDR strings. Returns an error on the first
// invalid entry.
func parseCIDRs(cidrs []string) ([]*net.IPNet, error) {
	nets := make([]*net.IPNet, 0, len(cidrs))
	for _, cidr := range cidrs {
		_, ipnet, err := net.ParseCIDR(cidr)
		if err != nil {
			return nil, err
		}
		nets = append(nets, ipnet)
	}
	return nets, nil
}

// ipInRanges reports whether ip falls within any of nets.
func ipInRanges(ip net.IP, nets []*net.IPNet) bool {
	for _, n := range nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// peerIP returns the connecting IP of the request. It prefers the original TCP
// address saved by [WithOriginalRemoteAddr] (before any RealIP middleware could
// overwrite r.RemoteAddr), falling back to r.RemoteAddr. The port is stripped.
func peerIP(r *http.Request) net.IP {
	addr := originalRemoteAddr(r.Context())
	if addr == "" {
		addr = r.RemoteAddr
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr // no port present
	}
	return net.ParseIP(host)
}
