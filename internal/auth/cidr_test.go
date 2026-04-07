package auth

import (
	"context"
	"net"
	"net/http/httptest"
	"testing"
)

func TestParseCIDRs(t *testing.T) {
	tests := []struct {
		name    string
		cidrs   []string
		wantErr bool
	}{
		{"empty", []string{}, false},
		{"valid ipv4", []string{"192.168.1.0/24", "10.0.0.0/8"}, false},
		{"valid ipv6", []string{"::1/128", "fc00::/7"}, false},
		{"mixed", []string{"127.0.0.0/8", "::1/128"}, false},
		{"invalid", []string{"not-a-cidr"}, true},
		{"plain ip no mask", []string{"192.168.1.1"}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseCIDRs(tc.cidrs)
			if (err != nil) != tc.wantErr {
				t.Errorf("parseCIDRs(%v) error = %v, wantErr %v", tc.cidrs, err, tc.wantErr)
			}
		})
	}
}

func TestIPInRanges(t *testing.T) {
	nets, err := ParseCIDRs([]string{"192.168.0.0/16", "10.0.0.0/8", "::1/128"})
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		ip   string
		want bool
	}{
		{"192.168.1.50", true},
		{"192.168.0.1", true},
		{"10.255.255.1", true},
		{"172.16.0.1", false},
		{"8.8.8.8", false},
		{"::1", true},
		{"fe80::1", false},
	}
	for _, tc := range tests {
		ip := net.ParseIP(tc.ip)
		if got := IPInRanges(ip, nets); got != tc.want {
			t.Errorf("IPInRanges(%q) = %v, want %v", tc.ip, got, tc.want)
		}
	}
}

func TestRemoteIP_FromContext(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	// Simulate PreserveRemoteAddr saving the original addr.
	ctx := WithOriginalRemoteAddr(context.Background(), "10.0.0.1:5555")
	req = req.WithContext(ctx)
	// Set r.RemoteAddr to something different (as RealIP would).
	req.RemoteAddr = "1.2.3.4:9999"

	got := remoteIP(req)
	if got.String() != "10.0.0.1" {
		t.Errorf("got %q, want 10.0.0.1", got)
	}
}

func TestRemoteIP_Fallback(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.0.2.5:1234"
	// No context value set.

	got := remoteIP(req)
	if got.String() != "192.0.2.5" {
		t.Errorf("got %q, want 192.0.2.5", got)
	}
}
