package middleware

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

// mustCIDRs parses CIDR strings for tests, failing on any parse error.
func mustCIDRs(t *testing.T, cidrs ...string) []*net.IPNet {
	t.Helper()
	nets := make([]*net.IPNet, 0, len(cidrs))
	for _, c := range cidrs {
		_, n, err := net.ParseCIDR(c)
		if err != nil {
			t.Fatalf("parse CIDR %q: %v", c, err)
		}
		nets = append(nets, n)
	}
	return nets
}

// runTrustedRealIP runs the middleware against a request with the given peer
// address and headers, returning the r.RemoteAddr seen by the next handler.
func runTrustedRealIP(t *testing.T, trustedNets []*net.IPNet, remoteAddr string, headers map[string][]string) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = remoteAddr
	for k, vals := range headers {
		for _, v := range vals {
			req.Header.Add(k, v)
		}
	}
	var seen string
	next := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { seen = r.RemoteAddr })
	TrustedRealIP(trustedNets)(next).ServeHTTP(httptest.NewRecorder(), req)
	return seen
}

func TestTrustedRealIP(t *testing.T) {
	trusted := mustCIDRs(t, "10.0.0.0/8")

	cases := []struct {
		name        string
		trustedNets []*net.IPNet
		remoteAddr  string
		headers     map[string][]string
		want        string
	}{
		{
			name:        "untrusted peer is not rewritten",
			trustedNets: trusted,
			remoteAddr:  "203.0.113.50:4444",
			headers:     map[string][]string{"X-Forwarded-For": {"9.9.9.9"}},
			want:        "203.0.113.50:4444",
		},
		{
			name:        "no trusted nets disables rewriting",
			trustedNets: nil,
			remoteAddr:  "10.0.0.2:1234",
			headers:     map[string][]string{"X-Forwarded-For": {"203.0.113.7"}},
			want:        "10.0.0.2:1234",
		},
		{
			name:        "single proxy returns the client",
			trustedNets: trusted,
			remoteAddr:  "10.0.0.2:1234",
			headers:     map[string][]string{"X-Forwarded-For": {"203.0.113.7"}},
			want:        "203.0.113.7",
		},
		{
			name:        "spoofed leftmost entry is ignored",
			trustedNets: trusted,
			remoteAddr:  "10.0.0.2:1234",
			// Client injected 9.9.9.9; the trusted proxy appended the real peer.
			headers: map[string][]string{"X-Forwarded-For": {"9.9.9.9, 203.0.113.7"}},
			want:    "203.0.113.7",
		},
		{
			name:        "proxy chain strips trusted hops right-to-left",
			trustedNets: trusted,
			remoteAddr:  "10.0.0.2:1234",
			headers:     map[string][]string{"X-Forwarded-For": {"203.0.113.7, 10.0.0.5"}},
			want:        "203.0.113.7",
		},
		{
			name:        "multiple XFF header lines are flattened",
			trustedNets: trusted,
			remoteAddr:  "10.0.0.2:1234",
			headers:     map[string][]string{"X-Forwarded-For": {"203.0.113.7", "10.0.0.5"}},
			want:        "203.0.113.7",
		},
		{
			name:        "all hops trusted falls back to leftmost",
			trustedNets: trusted,
			remoteAddr:  "10.0.0.2:1234",
			headers:     map[string][]string{"X-Forwarded-For": {"10.0.0.9, 10.0.0.5"}},
			want:        "10.0.0.9",
		},
		{
			name:        "X-Real-IP used when no XFF present",
			trustedNets: trusted,
			remoteAddr:  "10.0.0.2:1234",
			headers:     map[string][]string{"X-Real-IP": {"203.0.113.7"}},
			want:        "203.0.113.7",
		},
		{
			name:        "True-Client-IP preferred over X-Real-IP",
			trustedNets: trusted,
			remoteAddr:  "10.0.0.2:1234",
			headers: map[string][]string{
				"True-Client-IP": {"203.0.113.7"},
				"X-Real-IP":      {"198.51.100.9"},
			},
			want: "203.0.113.7",
		},
		{
			name:        "IPv6 client with port is parsed",
			trustedNets: trusted,
			remoteAddr:  "10.0.0.2:1234",
			headers:     map[string][]string{"X-Forwarded-For": {"[2001:db8::1]:443, 10.0.0.5"}},
			want:        "2001:db8::1",
		},
		{
			name:        "no usable header leaves peer untouched",
			trustedNets: trusted,
			remoteAddr:  "10.0.0.2:1234",
			headers:     nil,
			want:        "10.0.0.2:1234",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := runTrustedRealIP(t, tc.trustedNets, tc.remoteAddr, tc.headers)
			if got != tc.want {
				t.Errorf("RemoteAddr = %q, want %q", got, tc.want)
			}
		})
	}
}
