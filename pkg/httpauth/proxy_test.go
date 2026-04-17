package httpauth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// newProxyManager builds a minimal AuthManager for proxy-auth middleware tests.
func newProxyManager(t *testing.T, cfg AuthConfig, trustedProxy ...string) *AuthManager {
	t.Helper()
	nets, _ := parseCIDRs(trustedProxy)
	applyAuthDefaults(&cfg)
	return &AuthManager{
		cfg:         cfg,
		trustedNets: nets,
		logger:      noopLogger(),
	}
}

// trustedReq creates a test request with the given remote addr saved as the
// original TCP address (simulating preserveRemoteAddr middleware).
func trustedReq(remoteAddr string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := WithOriginalRemoteAddr(context.Background(), remoteAddr)
	return req.WithContext(ctx)
}

func TestProxyAuthMiddleware_Disabled(t *testing.T) {
	m := newProxyManager(t, AuthConfig{ProxyAuth: ProxyAuthConfig{Enabled: false}}, "127.0.0.0/8")
	var got *Identity
	h := m.proxyAuthMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = IdentityFromContext(r.Context())
	}))

	req := trustedReq("127.0.0.1:1234")
	req.Header.Set("Remote-User", "user@example.com")
	h.ServeHTTP(httptest.NewRecorder(), req)

	if got != nil {
		t.Error("expected no identity when proxy auth is disabled")
	}
}

func TestProxyAuthMiddleware_UntrustedIP(t *testing.T) {
	m := newProxyManager(t, AuthConfig{ProxyAuth: ProxyAuthConfig{Enabled: true}}, "127.0.0.0/8")
	var got *Identity
	h := m.proxyAuthMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = IdentityFromContext(r.Context())
	}))

	req := trustedReq("1.2.3.4:5678")
	req.Header.Set("Remote-User", "attacker@evil.com")
	h.ServeHTTP(httptest.NewRecorder(), req)

	if got != nil {
		t.Error("expected no identity from untrusted IP")
	}
}

func TestProxyAuthMiddleware_NoUserHeader(t *testing.T) {
	m := newProxyManager(t, AuthConfig{ProxyAuth: ProxyAuthConfig{Enabled: true}}, "127.0.0.0/8")
	var got *Identity
	h := m.proxyAuthMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = IdentityFromContext(r.Context())
	}))

	req := trustedReq("127.0.0.1:1234")
	h.ServeHTTP(httptest.NewRecorder(), req)

	if got != nil {
		t.Error("expected no identity when user header is absent")
	}
}

func TestProxyAuthMiddleware_EmailHeaderTakesPrecedence(t *testing.T) {
	m := newProxyManager(t, AuthConfig{ProxyAuth: ProxyAuthConfig{Enabled: true}}, "127.0.0.0/8")
	var got *Identity
	h := m.proxyAuthMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = IdentityFromContext(r.Context())
	}))

	req := trustedReq("127.0.0.1:1234")
	req.Header.Set("Remote-User", "alice")
	req.Header.Set("Remote-Email", "alice@example.com")
	req.Header.Set("Remote-Name", "Alice")
	h.ServeHTTP(httptest.NewRecorder(), req)

	if got == nil {
		t.Fatal("expected identity, got nil")
	}
	if got.Email != "alice@example.com" {
		t.Errorf("email: got %q, want alice@example.com (Remote-Email should take precedence)", got.Email)
	}
	if got.DisplayName != "Alice" {
		t.Errorf("display name: got %q, want %q", got.DisplayName, "Alice")
	}
	if got.Source != AuthSourceProxy {
		t.Errorf("source: got %q, want %q", got.Source, AuthSourceProxy)
	}
	if got.IsAdmin {
		t.Error("expected non-admin")
	}
}

func TestProxyAuthMiddleware_FallsBackToUserHeader(t *testing.T) {
	m := newProxyManager(t, AuthConfig{ProxyAuth: ProxyAuthConfig{Enabled: true}}, "127.0.0.0/8")
	var got *Identity
	h := m.proxyAuthMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = IdentityFromContext(r.Context())
	}))

	req := trustedReq("127.0.0.1:1234")
	req.Header.Set("Remote-User", "alice@example.com")
	h.ServeHTTP(httptest.NewRecorder(), req)

	if got == nil {
		t.Fatal("expected identity, got nil")
	}
	if got.Email != "alice@example.com" {
		t.Errorf("email: got %q, want alice@example.com", got.Email)
	}
}

func TestProxyAuthMiddleware_Groups(t *testing.T) {
	m := newProxyManager(t, AuthConfig{ProxyAuth: ProxyAuthConfig{Enabled: true}}, "10.0.0.0/8")
	var got *Identity
	h := m.proxyAuthMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = IdentityFromContext(r.Context())
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
	m := newProxyManager(t, AuthConfig{
		ProxyAuth:   ProxyAuthConfig{Enabled: true},
		AdminEmails: []string{"admin@example.com"},
	}, "127.0.0.0/8")
	var got *Identity
	h := m.proxyAuthMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = IdentityFromContext(r.Context())
	}))

	req := trustedReq("127.0.0.1:1234")
	req.Header.Set("Remote-User", "admin@example.com")
	h.ServeHTTP(httptest.NewRecorder(), req)

	if got == nil || !got.IsAdmin {
		t.Error("expected admin identity")
	}
}

func TestProxyAuthMiddleware_AdminGroup(t *testing.T) {
	m := newProxyManager(t, AuthConfig{
		ProxyAuth:   ProxyAuthConfig{Enabled: true},
		AdminGroups: []string{"superusers"},
	}, "127.0.0.0/8")
	var got *Identity
	h := m.proxyAuthMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = IdentityFromContext(r.Context())
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
	m := newProxyManager(t, AuthConfig{ProxyAuth: ProxyAuthConfig{Enabled: true}}, "::1/128")
	var got *Identity
	h := m.proxyAuthMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = IdentityFromContext(r.Context())
	}))

	req := trustedReq("[::1]:5000")
	req.Header.Set("Remote-User", "dan@example.com")
	h.ServeHTTP(httptest.NewRecorder(), req)

	if got == nil || got.Email != "dan@example.com" {
		t.Error("expected identity from trusted IPv6 address")
	}
}

func TestProxyAuthMiddleware_SkipsIfAlreadyAuthenticated(t *testing.T) {
	m := newProxyManager(t, AuthConfig{ProxyAuth: ProxyAuthConfig{Enabled: true}}, "127.0.0.0/8")
	existing := &Identity{Email: "first@example.com", Source: AuthSourceTailscale}

	var got *Identity
	h := m.proxyAuthMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = IdentityFromContext(r.Context())
	}))

	req := trustedReq("127.0.0.1:1234")
	req.Header.Set("Remote-User", "second@example.com")
	ctx := WithIdentity(req.Context(), existing)
	req = req.WithContext(ctx)

	h.ServeHTTP(httptest.NewRecorder(), req)
	if got == nil || got.Email != "first@example.com" {
		t.Error("expected existing identity to be preserved")
	}
}
