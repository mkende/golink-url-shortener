package server_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/mkende/golink-redirector/internal/config"
	"github.com/mkende/golink-redirector/internal/db"
	"github.com/mkende/golink-redirector/internal/server"
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
	handler := server.New(cfg, sqlDB, logger)
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
	_, err := links.Create(context.Background(), "docs", "https://example.com/docs", "test@example.com", false, false)
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

	_, err := links.Create(context.Background(), "gh", "https://github.com", "test@example.com", false, false)
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
