package httpauth

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const testJWTSecret = "test-secret-for-unit-tests"

// noopLogger returns a logger that discards all output, for use in tests.
func noopLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(noopWriter{}, nil))
}

type noopWriter struct{}

func (noopWriter) Write(p []byte) (int, error) { return len(p), nil }

// newOIDCManager builds a minimal AuthManager with an oidcHandler stub so that
// oidcMiddleware tests can run without contacting a real OIDC provider.
func newOIDCManager(t *testing.T, cfg AuthConfig, jwtSecret string) *AuthManager {
	t.Helper()
	applyAuthDefaults(&cfg)
	m := &AuthManager{
		cfg:       cfg,
		jwtSecret: jwtSecret,
		logger:    noopLogger(),
	}
	if cfg.OIDC.Enabled {
		// Minimal stub — only the fields read by oidcMiddleware are needed.
		m.oidcH = &oidcHandler{
			cfg:       cfg,
			jwtSecret: jwtSecret,
			logger:    noopLogger(),
		}
	}
	return m
}

// makeSessionCookie creates a signed JWT session cookie for testing.
func makeSessionCookie(t *testing.T, email string, expiry time.Time) *http.Cookie {
	t.Helper()
	claims := sessionClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiry),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
		Email: email,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(testJWTSecret))
	if err != nil {
		t.Fatalf("sign JWT: %v", err)
	}
	return &http.Cookie{Name: sessionCookieName, Value: signed}
}

func TestOIDCMiddleware_Disabled(t *testing.T) {
	m := newOIDCManager(t, AuthConfig{OIDC: OIDCConfig{Enabled: false}, Anonymous: AnonymousConfig{Enabled: true}}, testJWTSecret)
	called := false
	handler := m.oidcMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if id := IdentityFromContext(r.Context()); id != nil {
			t.Error("expected no identity when OIDC disabled")
		}
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(makeSessionCookie(t, "user@example.com", time.Now().Add(time.Hour)))
	handler.ServeHTTP(httptest.NewRecorder(), req)
	if !called {
		t.Error("handler not called")
	}
}

func TestOIDCMiddleware_NoCookie(t *testing.T) {
	m := newOIDCManager(t, AuthConfig{OIDC: OIDCConfig{Enabled: true}}, testJWTSecret)
	handler := m.oidcMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if id := IdentityFromContext(r.Context()); id != nil {
			t.Error("expected no identity without cookie")
		}
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	handler.ServeHTTP(httptest.NewRecorder(), req)
}

func TestOIDCMiddleware_ValidCookie(t *testing.T) {
	m := newOIDCManager(t, AuthConfig{
		OIDC:        OIDCConfig{Enabled: true},
		AdminEmails: []string{"bob@example.com"},
	}, testJWTSecret)
	var got *Identity
	handler := m.oidcMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = IdentityFromContext(r.Context())
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(makeSessionCookie(t, "bob@example.com", time.Now().Add(time.Hour)))
	handler.ServeHTTP(httptest.NewRecorder(), req)

	if got == nil {
		t.Fatal("expected identity, got nil")
	}
	if got.Email != "bob@example.com" {
		t.Errorf("email: got %q, want %q", got.Email, "bob@example.com")
	}
	if !got.IsAdmin {
		t.Error("expected admin: email is in AdminEmails")
	}
}

// TestOIDCMiddleware_AdminReevaluated verifies that IsAdmin is determined from
// the current config on every request, not from the value baked into the JWT.
func TestOIDCMiddleware_AdminReevaluated(t *testing.T) {
	m := newOIDCManager(t, AuthConfig{OIDC: OIDCConfig{Enabled: true}}, testJWTSecret)
	var got *Identity
	handler := m.oidcMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = IdentityFromContext(r.Context())
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(makeSessionCookie(t, "former-admin@example.com", time.Now().Add(time.Hour)))
	handler.ServeHTTP(httptest.NewRecorder(), req)

	if got == nil {
		t.Fatal("expected identity, got nil")
	}
	if got.IsAdmin {
		t.Error("IsAdmin must be re-evaluated from config, not read from JWT")
	}
}

func TestOIDCMiddleware_ExpiredCookie(t *testing.T) {
	m := newOIDCManager(t, AuthConfig{OIDC: OIDCConfig{Enabled: true}}, testJWTSecret)
	handler := m.oidcMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if id := IdentityFromContext(r.Context()); id != nil {
			t.Error("expected no identity for expired token")
		}
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(makeSessionCookie(t, "user@example.com", time.Now().Add(-time.Hour)))
	handler.ServeHTTP(httptest.NewRecorder(), req)
}

func TestOIDCMiddleware_WrongSecret(t *testing.T) {
	m := newOIDCManager(t, AuthConfig{OIDC: OIDCConfig{Enabled: true}}, "wrong-secret")
	handler := m.oidcMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if id := IdentityFromContext(r.Context()); id != nil {
			t.Error("expected no identity with wrong secret")
		}
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(makeSessionCookie(t, "user@example.com", time.Now().Add(time.Hour)))
	handler.ServeHTTP(httptest.NewRecorder(), req)
}
