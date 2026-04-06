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
)

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
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := server.New(cfg, sqlDB, logger, nil)
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

func TestRedirectRequireAuth_Unauthenticated(t *testing.T) {
	handler, links := newTestServer(t)

	// Create a link that requires auth (requireAuth = true)
	_, err := links.Create(context.Background(), "secret", "https://secret.example.com", "owner@example.com", db.LinkTypeSimple, "", true)
	if err != nil {
		t.Fatalf("create link: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/secret", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Errorf("expected 302 redirect to login, got %d", w.Code)
	}
	loc := w.Header().Get("Location")
	if loc == "" {
		t.Error("expected Location header")
	}
	// Should redirect to login, not to the target
	if loc == "https://secret.example.com" {
		t.Error("unauthenticated request should not reach the target URL")
	}
}

func TestUIRequiresAuthByDefault(t *testing.T) {
	handler, _ := newTestServer(t)
	// The default config has AllowLoggedOutUIAccess=false and OIDC disabled, so
	// an unauthenticated request to a UI page should get a 404.
	req := httptest.NewRequest(http.MethodGet, "/links", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for unauthenticated UI access, got %d", w.Code)
	}
}

func TestUIUnauthenticatedRedirectsToCanonicalDomain(t *testing.T) {
	// When allow_logged_out_ui_access=false (default) and the canonical domain
	// is configured, an unauthenticated request that is not already on the
	// canonical HTTPS domain must be redirected there — not shown a 404.
	sqlDB, err := db.Open(context.Background(), "sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })

	cfg := &config.Config{
		ListenAddr:      ":8080",
		Title:           "Test GoLink",
		CanonicalDomain: "go.example.com",
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := server.New(cfg, sqlDB, logger, nil)

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

func TestUIAllowsLoggedOutAccessWhenEnabled(t *testing.T) {
	sqlDB, err := db.Open(context.Background(), "sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })

	cfg := &config.Config{
		ListenAddr: ":8080",
		Title:      "Test GoLink",
		UI:         config.UIConfig{AllowLoggedOutUIAccess: true, LinksPerPage: 100},
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := server.New(cfg, sqlDB, logger, nil)

	req := httptest.NewRequest(http.MethodGet, "/links", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 when AllowLoggedOutUIAccess=true, got %d", w.Code)
	}
}

func TestRedirectRequireAuthForRedirects_Unauthenticated(t *testing.T) {
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
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := server.New(cfg, sqlDB, logger, nil)
	links := db.NewLinkRepo(sqlDB)

	_, err = links.Create(context.Background(), "pub", "https://public.example.com", "owner@example.com", db.LinkTypeSimple, "", false)
	if err != nil {
		t.Fatalf("create link: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/pub", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Errorf("expected 302 redirect to login, got %d", w.Code)
	}
	loc := w.Header().Get("Location")
	if loc == "https://public.example.com" {
		t.Error("unauthenticated request should not reach the target URL when require_auth_for_redirects is set")
	}
}
