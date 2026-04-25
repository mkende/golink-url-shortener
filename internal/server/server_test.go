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
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := server.New(cfg, sqlDB, logger, nil)

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
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := server.New(cfg, sqlDB, logger, nil)
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
	// No auth providers enabled and OIDC disabled, so an unauthenticated
	// request to a UI page should return 401.
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

func TestUIWithAnonymousAuthAllows(t *testing.T) {
	sqlDB, err := db.Open(context.Background(), "sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })

	cfg := &config.Config{
		ListenAddr: ":8080",
		Title:      "Test GoLink",
		Anonymous:  config.AnonymousConfig{Enabled: true},
		UI:         config.UIConfig{LinksPerPage: 100},
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := server.New(cfg, sqlDB, logger, nil)

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

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 when require_auth_for_redirects set and unauthenticated, got %d", w.Code)
	}
	if loc := w.Header().Get("Location"); loc == "https://public.example.com" {
		t.Error("unauthenticated request should not reach the target URL")
	}
}

// TestAdvancedLinkAliasVar_Direct verifies that .alias resolves to the
// canonical link name when the link is accessed directly (not via alias).
func TestAdvancedLinkAliasVar_Direct(t *testing.T) {
	handler, links := newTestServer(t)

	_, err := links.Create(context.Background(), "canonical", "https://example.com/{{.alias}}", "owner@example.com", db.LinkTypeAdvanced, "", false)
	if err != nil {
		t.Fatalf("create link: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/canonical", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", w.Code)
	}
	const want = "https://example.com/canonical"
	if loc := w.Header().Get("Location"); loc != want {
		t.Errorf("Location = %q, want %q", loc, want)
	}
}

// TestAdvancedLinkAliasVar_ViaAlias verifies that .alias resolves to the alias
// name (not the canonical name) when the link is reached through an alias.
func TestAdvancedLinkAliasVar_ViaAlias(t *testing.T) {
	handler, links := newTestServer(t)

	canonical, err := links.Create(context.Background(), "canonical", "https://example.com/{{.alias}}", "owner@example.com", db.LinkTypeAdvanced, "", false)
	if err != nil {
		t.Fatalf("create canonical link: %v", err)
	}

	aliasLink, err := links.Create(context.Background(), "myalias", "", "owner@example.com", db.LinkTypeSimple, "", false)
	if err != nil {
		t.Fatalf("create alias placeholder: %v", err)
	}
	if _, err := links.SetAlias(context.Background(), aliasLink.ID, "myalias", canonical.NameLower, false, 10); err != nil {
		t.Fatalf("SetAlias: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/myalias", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", w.Code)
	}
	const want = "https://example.com/myalias"
	if loc := w.Header().Get("Location"); loc != want {
		t.Errorf("Location = %q, want %q", loc, want)
	}
}
