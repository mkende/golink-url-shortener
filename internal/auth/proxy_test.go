package auth_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mkende/golink-redirector/internal/auth"
	"github.com/mkende/golink-redirector/internal/config"
)

func proxyCfg(enabled bool, cidrs ...string) *config.Config {
	cfg := &config.Config{
		ProxyAuth: config.ProxyAuthConfig{
			Enabled:      enabled,
			TrustedCIDRs: cidrs,
			UserHeader:   "Remote-User",
			NameHeader:   "Remote-Name",
			GroupsHeader: "Remote-Groups",
		},
	}
	return cfg
}

// trustedReq creates a test request with the given remote addr saved as the
// original TCP address (simulating PreserveRemoteAddr middleware).
func trustedReq(remoteAddr string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := auth.WithOriginalRemoteAddr(context.Background(), remoteAddr)
	return req.WithContext(ctx)
}

func TestProxyAuthMiddleware_Disabled(t *testing.T) {
	cfg := proxyCfg(false, "127.0.0.0/8")
	var got *auth.Identity
	h := auth.ProxyAuthMiddleware(cfg, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = auth.FromContext(r.Context())
	}))

	req := trustedReq("127.0.0.1:1234")
	req.Header.Set("Remote-User", "user@example.com")
	h.ServeHTTP(httptest.NewRecorder(), req)

	if got != nil {
		t.Error("expected no identity when proxy auth is disabled")
	}
}

func TestProxyAuthMiddleware_UntrustedIP(t *testing.T) {
	cfg := proxyCfg(true, "127.0.0.0/8")
	var got *auth.Identity
	h := auth.ProxyAuthMiddleware(cfg, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = auth.FromContext(r.Context())
	}))

	req := trustedReq("1.2.3.4:5678") // outside trusted range
	req.Header.Set("Remote-User", "attacker@evil.com")
	h.ServeHTTP(httptest.NewRecorder(), req)

	if got != nil {
		t.Error("expected no identity from untrusted IP")
	}
}

func TestProxyAuthMiddleware_NoUserHeader(t *testing.T) {
	cfg := proxyCfg(true, "127.0.0.0/8")
	var got *auth.Identity
	h := auth.ProxyAuthMiddleware(cfg, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = auth.FromContext(r.Context())
	}))

	req := trustedReq("127.0.0.1:1234")
	// No Remote-User header set.
	h.ServeHTTP(httptest.NewRecorder(), req)

	if got != nil {
		t.Error("expected no identity when user header is absent")
	}
}

func TestProxyAuthMiddleware_BasicUser(t *testing.T) {
	cfg := proxyCfg(true, "127.0.0.0/8")
	var got *auth.Identity
	h := auth.ProxyAuthMiddleware(cfg, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = auth.FromContext(r.Context())
	}))

	req := trustedReq("127.0.0.1:1234")
	req.Header.Set("Remote-User", "alice@example.com")
	req.Header.Set("Remote-Name", "Alice")
	h.ServeHTTP(httptest.NewRecorder(), req)

	if got == nil {
		t.Fatal("expected identity, got nil")
	}
	if got.Email != "alice@example.com" {
		t.Errorf("email: got %q, want %q", got.Email, "alice@example.com")
	}
	if got.DisplayName != "Alice" {
		t.Errorf("display name: got %q, want %q", got.DisplayName, "Alice")
	}
	if got.Source != auth.AuthSourceProxy {
		t.Errorf("source: got %q, want %q", got.Source, auth.AuthSourceProxy)
	}
	if got.IsAdmin {
		t.Error("expected non-admin")
	}
}

func TestProxyAuthMiddleware_Groups(t *testing.T) {
	cfg := proxyCfg(true, "10.0.0.0/8")
	var got *auth.Identity
	h := auth.ProxyAuthMiddleware(cfg, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = auth.FromContext(r.Context())
	}))

	req := trustedReq("10.1.2.3:4444")
	req.Header.Set("Remote-User", "bob@example.com")
	req.Header.Set("Remote-Groups", "devs, admins, qa")
	h.ServeHTTP(httptest.NewRecorder(), req)

	if got == nil {
		t.Fatal("expected identity, got nil")
	}
	want := []string{"devs", "admins", "qa"}
	if len(got.Groups) != len(want) {
		t.Fatalf("groups: got %v, want %v", got.Groups, want)
	}
	for i, g := range want {
		if got.Groups[i] != g {
			t.Errorf("groups[%d]: got %q, want %q", i, got.Groups[i], g)
		}
	}
}

func TestProxyAuthMiddleware_AdminEmail(t *testing.T) {
	cfg := proxyCfg(true, "127.0.0.0/8")
	cfg.AdminEmails = []string{"admin@example.com"}
	var got *auth.Identity
	h := auth.ProxyAuthMiddleware(cfg, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = auth.FromContext(r.Context())
	}))

	req := trustedReq("127.0.0.1:1234")
	req.Header.Set("Remote-User", "admin@example.com")
	h.ServeHTTP(httptest.NewRecorder(), req)

	if got == nil || !got.IsAdmin {
		t.Error("expected admin identity")
	}
}

func TestProxyAuthMiddleware_AdminGroup(t *testing.T) {
	cfg := proxyCfg(true, "127.0.0.0/8")
	cfg.AdminGroup = "superusers"
	var got *auth.Identity
	h := auth.ProxyAuthMiddleware(cfg, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = auth.FromContext(r.Context())
	}))

	req := trustedReq("127.0.0.1:1234")
	req.Header.Set("Remote-User", "carol@example.com")
	req.Header.Set("Remote-Groups", "devs,superusers")
	h.ServeHTTP(httptest.NewRecorder(), req)

	if got == nil || !got.IsAdmin {
		t.Error("expected admin identity via group membership")
	}
}

func TestProxyAuthMiddleware_IPv6Trusted(t *testing.T) {
	cfg := proxyCfg(true, "::1/128")
	var got *auth.Identity
	h := auth.ProxyAuthMiddleware(cfg, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = auth.FromContext(r.Context())
	}))

	req := trustedReq("[::1]:5000")
	req.Header.Set("Remote-User", "dan@example.com")
	h.ServeHTTP(httptest.NewRecorder(), req)

	if got == nil || got.Email != "dan@example.com" {
		t.Error("expected identity from trusted IPv6 address")
	}
}

func TestProxyAuthMiddleware_SkipsIfAlreadyAuthenticated(t *testing.T) {
	cfg := proxyCfg(true, "127.0.0.0/8")
	existing := &auth.Identity{Email: "first@example.com", Source: auth.AuthSourceTailscale}

	var got *auth.Identity
	h := auth.ProxyAuthMiddleware(cfg, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = auth.FromContext(r.Context())
	}))

	req := trustedReq("127.0.0.1:1234")
	req.Header.Set("Remote-User", "second@example.com")
	ctx := auth.WithIdentity(req.Context(), existing)
	req = req.WithContext(ctx)

	h.ServeHTTP(httptest.NewRecorder(), req)
	if got == nil || got.Email != "first@example.com" {
		t.Error("expected existing identity to be preserved")
	}
}
