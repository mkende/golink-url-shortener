package auth_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/mkende/golink-url-shortener/internal/auth"
	"github.com/mkende/golink-url-shortener/internal/config"
)

const testJWTSecret = "test-secret-for-unit-tests"

// makeSessionCookie creates a signed JWT session cookie for testing.
func makeSessionCookie(t *testing.T, email string, expiry time.Time) *http.Cookie {
	t.Helper()
	type sessionClaims struct {
		jwt.RegisteredClaims
		Email       string   `json:"email"`
		DisplayName string   `json:"name"`
		AvatarURL   string   `json:"picture"`
		Groups      []string `json:"groups"`
	}
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
	return &http.Cookie{Name: "golink_session", Value: signed}
}

func TestOIDCMiddleware_Disabled(t *testing.T) {
	cfg := &config.Config{JWTSecret: testJWTSecret, OIDC: config.OIDCConfig{Enabled: false}}
	called := false
	handler := auth.OIDCMiddleware(cfg, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if id := auth.FromContext(r.Context()); id != nil {
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
	cfg := &config.Config{JWTSecret: testJWTSecret, OIDC: config.OIDCConfig{Enabled: true}}
	handler := auth.OIDCMiddleware(cfg, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if id := auth.FromContext(r.Context()); id != nil {
			t.Error("expected no identity without cookie")
		}
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	handler.ServeHTTP(httptest.NewRecorder(), req)
}

func TestOIDCMiddleware_ValidCookie(t *testing.T) {
	cfg := &config.Config{
		JWTSecret:   testJWTSecret,
		OIDC:        config.OIDCConfig{Enabled: true},
		AdminEmails: []string{"bob@example.com"},
	}
	var got *auth.Identity
	handler := auth.OIDCMiddleware(cfg, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = auth.FromContext(r.Context())
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
	// Config has no admin emails — user should NOT be admin even if they held
	// admin rights when the JWT was originally issued.
	cfg := &config.Config{JWTSecret: testJWTSecret, OIDC: config.OIDCConfig{Enabled: true}}
	var got *auth.Identity
	handler := auth.OIDCMiddleware(cfg, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = auth.FromContext(r.Context())
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
	cfg := &config.Config{JWTSecret: testJWTSecret, OIDC: config.OIDCConfig{Enabled: true}}
	handler := auth.OIDCMiddleware(cfg, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if id := auth.FromContext(r.Context()); id != nil {
			t.Error("expected no identity for expired token")
		}
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(makeSessionCookie(t, "user@example.com", time.Now().Add(-time.Hour)))
	handler.ServeHTTP(httptest.NewRecorder(), req)
}

func TestOIDCMiddleware_WrongSecret(t *testing.T) {
	cfg := &config.Config{JWTSecret: "wrong-secret", OIDC: config.OIDCConfig{Enabled: true}}
	handler := auth.OIDCMiddleware(cfg, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if id := auth.FromContext(r.Context()); id != nil {
			t.Error("expected no identity with wrong secret")
		}
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(makeSessionCookie(t, "user@example.com", time.Now().Add(time.Hour)))
	handler.ServeHTTP(httptest.NewRecorder(), req)
}
