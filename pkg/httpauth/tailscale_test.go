package httpauth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// newTailscaleManager builds a minimal AuthManager for Tailscale middleware tests.
func newTailscaleManager(t *testing.T, cfg AuthConfig, trustedProxy ...string) *AuthManager {
	t.Helper()
	// Construct directly to avoid contacting an OIDC provider.
	nets, _ := parseCIDRs(trustedProxy)
	return &AuthManager{
		cfg:         cfg,
		trustedNets: nets,
		logger:      noopLogger(),
	}
}

func TestTailscaleMiddleware_Disabled(t *testing.T) {
	m := newTailscaleManager(t, AuthConfig{Tailscale: TailscaleConfig{Enabled: false}, Anonymous: AnonymousConfig{Enabled: true}})
	called := false
	handler := m.tailscaleMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if id := IdentityFromContext(r.Context()); id != nil {
			t.Error("expected no identity when Tailscale is disabled")
		}
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Tailscale-User-Login", "user@example.com")
	handler.ServeHTTP(httptest.NewRecorder(), req)
	if !called {
		t.Error("handler not called")
	}
}

func TestTailscaleMiddleware_NoHeader(t *testing.T) {
	m := newTailscaleManager(t, AuthConfig{Tailscale: TailscaleConfig{Enabled: true}})
	handler := m.tailscaleMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if id := IdentityFromContext(r.Context()); id != nil {
			t.Error("expected no identity without headers")
		}
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	handler.ServeHTTP(httptest.NewRecorder(), req)
}

func TestTailscaleMiddleware_ParsesHeaders(t *testing.T) {
	m := newTailscaleManager(t, AuthConfig{Tailscale: TailscaleConfig{Enabled: true}})
	var got *Identity
	handler := m.tailscaleMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = IdentityFromContext(r.Context())
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Tailscale-User-Login", "alice@example.com")
	req.Header.Set("Tailscale-User-Name", "Alice")
	req.Header.Set("Tailscale-User-Profile-Pic", "https://example.com/pic.png")
	handler.ServeHTTP(httptest.NewRecorder(), req)

	if got == nil {
		t.Fatal("expected identity, got nil")
	}
	if got.Email != "alice@example.com" {
		t.Errorf("email: got %q, want %q", got.Email, "alice@example.com")
	}
	if got.DisplayName != "Alice" {
		t.Errorf("display name: got %q, want %q", got.DisplayName, "Alice")
	}
	if got.AvatarURL != "https://example.com/pic.png" {
		t.Errorf("avatar: got %q, want %q", got.AvatarURL, "https://example.com/pic.png")
	}
	if got.IsAdmin {
		t.Error("expected non-admin")
	}
}

func TestTailscaleMiddleware_AdminEmail(t *testing.T) {
	m := newTailscaleManager(t, AuthConfig{
		Tailscale:   TailscaleConfig{Enabled: true},
		AdminEmails: []string{"admin@example.com"},
	})
	var got *Identity
	handler := m.tailscaleMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = IdentityFromContext(r.Context())
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Tailscale-User-Login", "admin@example.com")
	handler.ServeHTTP(httptest.NewRecorder(), req)

	if got == nil || !got.IsAdmin {
		t.Error("expected admin identity")
	}
}

func TestTailscaleMiddleware_TrustedCIDR_Accepted(t *testing.T) {
	m := newTailscaleManager(t, AuthConfig{Tailscale: TailscaleConfig{Enabled: true}}, "100.64.0.0/10")
	var got *Identity
	handler := m.tailscaleMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = IdentityFromContext(r.Context())
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Tailscale-User-Login", "alice@example.com")
	ctx := WithOriginalRemoteAddr(context.Background(), "100.64.1.5:12345")
	req = req.WithContext(ctx)

	handler.ServeHTTP(httptest.NewRecorder(), req)
	if got == nil || got.Email != "alice@example.com" {
		t.Error("expected identity from trusted CIDR")
	}
}

func TestTailscaleMiddleware_TrustedCIDR_Rejected(t *testing.T) {
	m := newTailscaleManager(t, AuthConfig{Tailscale: TailscaleConfig{Enabled: true}}, "100.64.0.0/10")
	var got *Identity
	handler := m.tailscaleMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = IdentityFromContext(r.Context())
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Tailscale-User-Login", "alice@example.com")
	ctx := WithOriginalRemoteAddr(context.Background(), "1.2.3.4:9999")
	req = req.WithContext(ctx)

	handler.ServeHTTP(httptest.NewRecorder(), req)
	if got != nil {
		t.Error("expected no identity from untrusted IP")
	}
}

func TestTailscaleMiddleware_TrustedCIDR_IPv6Accepted(t *testing.T) {
	m := newTailscaleManager(t, AuthConfig{Tailscale: TailscaleConfig{Enabled: true}}, "::1/128")
	var got *Identity
	handler := m.tailscaleMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = IdentityFromContext(r.Context())
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Tailscale-User-Login", "alice@example.com")
	ctx := WithOriginalRemoteAddr(context.Background(), "[::1]:45534")
	req = req.WithContext(ctx)

	handler.ServeHTTP(httptest.NewRecorder(), req)
	if got == nil || got.Email != "alice@example.com" {
		t.Error("expected identity from trusted IPv6 address")
	}
}

func TestTailscaleMiddleware_TrustedCIDR_IPv6Rejected(t *testing.T) {
	m := newTailscaleManager(t, AuthConfig{Tailscale: TailscaleConfig{Enabled: true}}, "::1/128")
	var got *Identity
	handler := m.tailscaleMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = IdentityFromContext(r.Context())
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Tailscale-User-Login", "alice@example.com")
	ctx := WithOriginalRemoteAddr(context.Background(), "[2001:db8::1]:9999")
	req = req.WithContext(ctx)

	handler.ServeHTTP(httptest.NewRecorder(), req)
	if got != nil {
		t.Error("expected no identity from untrusted IPv6 address")
	}
}
