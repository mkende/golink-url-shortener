package auth_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mkende/golink-redirector/internal/auth"
	"github.com/mkende/golink-redirector/internal/config"
)

func TestTailscaleMiddleware_Disabled(t *testing.T) {
	cfg := &config.Config{Tailscale: config.TailscaleConfig{Enabled: false}}
	called := false
	handler := auth.TailscaleMiddleware(cfg, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if id := auth.FromContext(r.Context()); id != nil {
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
	cfg := &config.Config{Tailscale: config.TailscaleConfig{Enabled: true}}
	handler := auth.TailscaleMiddleware(cfg, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if id := auth.FromContext(r.Context()); id != nil {
			t.Error("expected no identity without headers")
		}
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	handler.ServeHTTP(httptest.NewRecorder(), req)
}

func TestTailscaleMiddleware_ParsesHeaders(t *testing.T) {
	cfg := &config.Config{Tailscale: config.TailscaleConfig{Enabled: true}}
	var got *auth.Identity
	handler := auth.TailscaleMiddleware(cfg, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = auth.FromContext(r.Context())
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
	cfg := &config.Config{
		Tailscale:   config.TailscaleConfig{Enabled: true},
		AdminEmails: []string{"admin@example.com"},
	}
	var got *auth.Identity
	handler := auth.TailscaleMiddleware(cfg, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = auth.FromContext(r.Context())
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Tailscale-User-Login", "admin@example.com")
	handler.ServeHTTP(httptest.NewRecorder(), req)

	if got == nil || !got.IsAdmin {
		t.Error("expected admin identity")
	}
}
