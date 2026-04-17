package server_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/mkende/golink-url-shortener/internal/config"
	"github.com/mkende/golink-url-shortener/internal/db"
	"github.com/mkende/golink-url-shortener/internal/server"
	"github.com/mkende/golink-url-shortener/pkg/httpauth"
)

// httptestClientAddr is the remote address httptest.NewRequest uses.
const httptestClientAddr = "192.0.2.1/32"

// makeAuthManager creates an AuthManager from cfg. When no auth provider is
// configured it defaults to ProxyAuth with the trusted proxy set to the
// httptest client address, so requests without a Remote-User header arrive
// unauthenticated.
func makeAuthManager(t *testing.T, cfg *config.Config) *httpauth.AuthManager {
	t.Helper()
	authCfg := cfg.Auth
	trustedProxy := cfg.TrustedProxy
	if !authCfg.OIDC.Enabled && !authCfg.Tailscale.Enabled && !authCfg.ProxyAuth.Enabled && !authCfg.Anonymous.Enabled {
		trustedProxy = []string{httptestClientAddr}
		authCfg.ProxyAuth = httpauth.ProxyAuthConfig{Enabled: true}
	}
	opts := []httpauth.Option{}
	if cfg.CanonicalAddress != "" {
		opts = append(opts, httpauth.WithCanonicalAddress(cfg.CanonicalAddress))
	}
	if len(trustedProxy) > 0 {
		opts = append(opts, httpauth.WithTrustedProxy(trustedProxy))
	}
	m, err := httpauth.New(context.Background(), authCfg, opts...)
	if err != nil {
		t.Fatalf("create auth manager: %v", err)
	}
	return m
}

func newTestServer(t *testing.T) (http.Handler, db.LinkRepo) {
	t.Helper()
	sqlDB, err := db.Open(context.Background(), "sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })

	cfg := &config.Config{
		ListenAddr: ":8080",
		Title:      "Test GoLink",
	}
	authManager := makeAuthManager(t, cfg)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := server.New(cfg, sqlDB, logger, authManager)
	links := db.NewLinkRepo(sqlDB)
	return handler, links
}

func TestHealthz(t *testing.T) {
	handler, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestRedirectFound(t *testing.T) {
	handler, links := newTestServer(t)

	// Create a test link
	_, err := links.Create(context.Background(), "docs", "https://example.com/docs", "test@example.com", db.LinkTypeSimple, "", false)
	if err != nil {
		t.Fatalf("create link: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/docs", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Errorf("expected 302, got %d", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "https://example.com/docs" {
		t.Errorf("expected location https://example.com/docs, got %q", loc)
	}
}

func TestRedirectWithSuffix(t *testing.T) {
	handler, links := newTestServer(t)

	_, err := links.Create(context.Background(), "gh", "https://github.com", "test@example.com", db.LinkTypeSimple, "", false)
	if err != nil {
		t.Fatalf("create link: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/gh/myorg/myrepo", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Errorf("expected 302, got %d", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "https://github.com/myorg/myrepo" {
		t.Errorf("expected https://github.com/myorg/myrepo, got %q", loc)
	}
}

func TestRedirectNotFound(t *testing.T) {
	handler, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/nonexistent", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestRedirectNotFoundRedirectsToCanonicalDomain(t *testing.T) {
	sqlDB, err := db.Open(context.Background(), "sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })

	cfg := &config.Config{
		ListenAddr:       ":8080",
		Title:            "Test GoLink",
		CanonicalAddress: "https://go.example.com",
	}
	authManager := makeAuthManager(t, cfg)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := server.New(cfg, sqlDB, logger, authManager)

	// http://go/missing — link doesn't exist, non-canonical host
	req := httptest.NewRequest(http.MethodGet, "/missing", nil)
	req.Host = "go"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusMovedPermanently {
		t.Errorf("expected 301 redirect to canonical domain, got %d", w.Code)
	}
	const want = "https://go.example.com/missing"
	if loc := w.Header().Get("Location"); loc != want {
		t.Errorf("expected Location %q, got %q", want, loc)
	}
}

func TestRedirectRequireAuth_Unauthenticated(t *testing.T) {
	// No canonical address: step 3 is skipped, so an unauthenticated request
	// to a require_auth link immediately reaches step 5 → 401.
	handler, links := newTestServer(t)

	// Create a link that requires auth (requireAuth = true)
	_, err := links.Create(context.Background(), "secret", "https://secret.example.com", "owner@example.com", db.LinkTypeSimple, "", true)
	if err != nil {
		t.Fatalf("create link: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/secret", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	// Unauthenticated, OIDC not enabled → 401 unauthorized page.
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestRedirectRequireAuth_NonCanonical_RedirectsFirst(t *testing.T) {
	sqlDB, err := db.Open(context.Background(), "sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })

	cfg := &config.Config{
		ListenAddr:       ":8080",
		Title:            "Test GoLink",
		CanonicalAddress: "https://go.example.com",
	}
	authManager := makeAuthManager(t, cfg)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := server.New(cfg, sqlDB, logger, authManager)
	links := db.NewLinkRepo(sqlDB)

	_, err = links.Create(context.Background(), "secret", "https://secret.example.com", "owner@example.com", db.LinkTypeSimple, "", true)
	if err != nil {
		t.Fatalf("create link: %v", err)
	}

	// Request on non-canonical host — should redirect to canonical first (step 3).
	req := httptest.NewRequest(http.MethodGet, "/secret", nil)
	req.Host = "go"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusMovedPermanently {
		t.Errorf("expected 301 canonical redirect, got %d", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "https://go.example.com/secret" {
		t.Errorf("expected Location https://go.example.com/secret, got %q", loc)
	}
}

func TestUIRequiresAuth(t *testing.T) {
	handler, _ := newTestServer(t)
	// ProxyAuth without Remote-User header → no identity → 401.
	req := httptest.NewRequest(http.MethodGet, "/links", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for unauthenticated UI access, got %d", w.Code)
	}
}

func TestUIUnauthenticatedRedirectsToCanonicalDomain(t *testing.T) {
	// When the canonical address is configured and an unauthenticated request
	// arrives on a different host, DomainRedirect should send a 301 before
	// RequireUIAccess even runs.
	sqlDB, err := db.Open(context.Background(), "sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })

	cfg := &config.Config{
		ListenAddr:       ":8080",
		Title:            "Test GoLink",
		CanonicalAddress: "https://go.example.com",
	}
	authManager := makeAuthManager(t, cfg)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := server.New(cfg, sqlDB, logger, authManager)

	// Simulate http://go/ — plain HTTP, wrong host.
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "go"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusMovedPermanently {
		t.Errorf("expected 301 redirect to canonical domain, got %d", w.Code)
	}
	loc := w.Header().Get("Location")
	const want = "https://go.example.com/"
	if loc != want {
		t.Errorf("expected Location %q, got %q", want, loc)
	}
}

func TestUIWithAnonymousAuthAllows(t *testing.T) {
	sqlDB, err := db.Open(context.Background(), "sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })

	cfg := &config.Config{
		ListenAddr: ":8080",
		Title:      "Test GoLink",
		Auth:       httpauth.AuthConfig{Anonymous: httpauth.AnonymousConfig{Enabled: true}},
		UI:         config.UIConfig{LinksPerPage: 100},
	}
	authManager := makeAuthManager(t, cfg)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := server.New(cfg, sqlDB, logger, authManager)

	req := httptest.NewRequest(http.MethodGet, "/links", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 when anonymous auth enabled, got %d", w.Code)
	}
}

func TestRedirectRequireAuthForRedirects_Unauthenticated(t *testing.T) {
	// No canonical address: step 3 is skipped, so an unauthenticated request
	// immediately reaches step 5 → 401.
	sqlDB, err := db.Open(context.Background(), "sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })

	cfg := &config.Config{
		ListenAddr:              ":8080",
		Title:                   "Test GoLink",
		RequireAuthForRedirects: true,
	}
	authManager := makeAuthManager(t, cfg)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := server.New(cfg, sqlDB, logger, authManager)
	links := db.NewLinkRepo(sqlDB)

	_, err = links.Create(context.Background(), "pub", "https://public.example.com", "owner@example.com", db.LinkTypeSimple, "", false)
	if err != nil {
		t.Fatalf("create link: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/pub", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 when require_auth_for_redirects set and unauthenticated, got %d", w.Code)
	}
	if loc := w.Header().Get("Location"); loc == "https://public.example.com" {
		t.Error("unauthenticated request should not reach the target URL")
	}
}
