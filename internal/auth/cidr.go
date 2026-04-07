package auth

import (
	"context"
	"net"
	"net/http"
)

// origRemoteAddrKey is the context key for the original TCP remote address,
// saved before any RealIP middleware has a chance to overwrite r.RemoteAddr.
type origRemoteAddrKey struct{}

// WithOriginalRemoteAddr returns a context carrying the raw TCP remote address.
// This must be called (by PreserveRemoteAddr middleware) before chi's RealIP
// middleware runs so that CIDR checks use the actual connecting IP, not the
// spoofable X-Forwarded-For value.
func WithOriginalRemoteAddr(ctx context.Context, addr string) context.Context {
	return context.WithValue(ctx, origRemoteAddrKey{}, addr)
}

// OriginalRemoteAddr returns the raw TCP remote address stored in ctx by
// WithOriginalRemoteAddr, or "" if not set.
func OriginalRemoteAddr(ctx context.Context) string {
	v, _ := ctx.Value(origRemoteAddrKey{}).(string)
	return v
}

// ParseCIDRs parses a slice of CIDR strings and returns the corresponding
// network list. It returns an error if any entry is not a valid CIDR.
func ParseCIDRs(cidrs []string) ([]*net.IPNet, error) {
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

// IPInRanges reports whether ip falls within any of the provided networks.
func IPInRanges(ip net.IP, nets []*net.IPNet) bool {
	for _, n := range nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// PeerIP extracts the connecting IP address from the request. It prefers the
// original TCP address saved in context (before RealIP overrides r.RemoteAddr),
// falling back to r.RemoteAddr when the context value is not present (e.g. in
// unit tests). The port suffix is stripped.
func PeerIP(r *http.Request) net.IP {
	addr := OriginalRemoteAddr(r.Context())
	if addr == "" {
		addr = r.RemoteAddr
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		// addr had no port (unusual but handle gracefully).
		host = addr
	}
	return net.ParseIP(host)
}

// remoteIP is an unexported alias kept for use within the auth package only.
func remoteIP(r *http.Request) net.IP { return PeerIP(r) }
