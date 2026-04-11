package server_test

import (
	"bytes"
	"context"
	"encoding/json"
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

// apiTestEnv holds a test server and the repos needed to set up state.
type apiTestEnv struct {
	handler http.Handler
	links   db.LinkRepo
	apiKeys db.APIKeyRepo
}

// newAPITestEnv creates an in-memory SQLite database, wires up a Server, and
// returns an apiTestEnv with direct repo access for test setup.
func newAPITestEnv(t *testing.T) *apiTestEnv {
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
	return &apiTestEnv{
		handler: handler,
		links:   db.NewLinkRepo(sqlDB),
		apiKeys: db.NewAPIKeyRepo(sqlDB),
	}
}

// createTestAPIKey inserts a read-write API key and returns the raw key string.
func createTestAPIKey(t *testing.T, env *apiTestEnv, name string) string {
	t.Helper()
	return createTestAPIKeyWithAccess(t, env, name, false)
}

// createTestAPIKeyWithAccess inserts an API key with the given readOnly flag
// and returns the raw key string.
func createTestAPIKeyWithAccess(t *testing.T, env *apiTestEnv, name string, readOnly bool) string {
	t.Helper()
	raw := "testkey-" + name
	hash := server.HashAPIKey(raw)
	_, err := env.apiKeys.Create(context.Background(), name, hash, "admin@example.com", readOnly)
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}
	return raw
}

// mustCreate creates a simple link in the repo and calls t.Fatal on error.
func mustCreate(t *testing.T, env *apiTestEnv, name, target, owner string) {
	t.Helper()
	_, err := env.links.Create(context.Background(), name, target, owner, db.LinkTypeSimple, "", false)
	if err != nil {
		t.Fatalf("create link %q: %v", name, err)
	}
}

// doJSON sends a JSON API request with an optional API key header.
func doJSON(t *testing.T, handler http.Handler, method, path string, body any, apiKey string) *httptest.ResponseRecorder {
	t.Helper()
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("json.Marshal: %v", err)
		}
		reqBody = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, reqBody)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if apiKey != "" {
		req.Header.Set("X-API-Key", apiKey)
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	return w
}
