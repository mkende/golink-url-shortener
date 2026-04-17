package server_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/mkende/golink-url-shortener/internal/config"
	"github.com/mkende/golink-url-shortener/internal/db"
	"github.com/mkende/golink-url-shortener/internal/server"
	"github.com/mkende/golink-url-shortener/pkg/httpauth"
)

// doGet sends an authenticated GET request using an API key.
func doGet(handler http.Handler, path, apiKey string) *httptest.ResponseRecorder {
	return doRaw(handler, http.MethodGet, path, nil, "", apiKey)
}

// --- /api/users/search tests ---

func TestAPIUserSearch_Unauthenticated(t *testing.T) {
	env := newAPITestEnv(t)
	w := doGet(env.handler, "/api/users/search?email=alice", "")
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestAPIUserSearch_EmptyQuery(t *testing.T) {
	env := newAPITestEnv(t)
	key := createTestAPIKey(t, env, "k")

	w := doGet(env.handler, "/api/users/search", key)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if strings.TrimSpace(w.Body.String()) != "" {
		t.Errorf("expected empty body for empty query, got: %q", w.Body.String())
	}
}

func TestAPIUserSearch_NoMatches(t *testing.T) {
	env := newAPITestEnv(t)
	key := createTestAPIKey(t, env, "k")
	if _, err := env.users.Upsert(context.Background(), "alice@example.com", "Alice", ""); err != nil {
		t.Fatalf("upsert user: %v", err)
	}

	w := doGet(env.handler, "/api/users/search?email=nobody", key)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if strings.TrimSpace(w.Body.String()) != "" {
		t.Errorf("expected empty body when no users match, got: %q", w.Body.String())
	}
}

func TestAPIUserSearch_WithMatches(t *testing.T) {
	env := newAPITestEnv(t)
	key := createTestAPIKey(t, env, "k")
	if _, err := env.users.Upsert(context.Background(), "alice@example.com", "Alice Smith", ""); err != nil {
		t.Fatalf("upsert user: %v", err)
	}
	if _, err := env.users.Upsert(context.Background(), "bob@example.com", "Bob Jones", ""); err != nil {
		t.Fatalf("upsert user: %v", err)
	}

	w := doGet(env.handler, "/api/users/search?email=alice", key)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "alice@example.com") {
		t.Errorf("expected alice@example.com in response, got: %q", body)
	}
	if strings.Contains(body, "bob@example.com") {
		t.Errorf("bob@example.com should not appear for query 'alice', got: %q", body)
	}
}

func TestAPIUserSearch_MatchesByDisplayName(t *testing.T) {
	env := newAPITestEnv(t)
	key := createTestAPIKey(t, env, "k")
	if _, err := env.users.Upsert(context.Background(), "u@example.com", "Thomas Anderson", ""); err != nil {
		t.Fatalf("upsert user: %v", err)
	}

	// Query by first name, which is not in the email.
	w := doGet(env.handler, "/api/users/search?email=thomas", key)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "u@example.com") {
		t.Errorf("expected u@example.com in response for display-name match, got: %q", body)
	}
}

func TestAPIUserSearch_ContentType(t *testing.T) {
	env := newAPITestEnv(t)
	key := createTestAPIKey(t, env, "k")

	w := doGet(env.handler, "/api/users/search?email=x", key)
	ct := w.Result().Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "text/html") {
		t.Errorf("expected text/html content-type, got: %q", ct)
	}
}

// --- Share form validation tests ---

// shareTestEnv holds a server wired with proxy auth so tests can inject an
// authenticated identity via the Remote-Email header.
type shareTestEnv struct {
	handler http.Handler
	links   db.LinkRepo
}

// httptest.NewRequest uses 192.0.2.1:1234 as the remote address.
const httptestRemoteAddr = "192.0.2.1/32"

// newShareTestEnv builds a test server with proxy auth enabled. The optional
// cfg function can override config fields (e.g. DefaultDomain, RequiredDomain).
func newShareTestEnv(t *testing.T, cfg func(*config.Config)) *shareTestEnv {
	t.Helper()
	sqlDB, err := db.Open(context.Background(), "sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })

	c := &config.Config{
		ListenAddr:   ":8080",
		Title:        "Test GoLink",
		TrustedProxy: []string{httptestRemoteAddr},
		Auth: httpauth.AuthConfig{
			ProxyAuth: httpauth.ProxyAuthConfig{
				Enabled:      true,
				EmailHeader:  "Remote-Email",
				UserHeader:   "Remote-User",
				NameHeader:   "Remote-Name",
				GroupsHeader: "Remote-Groups",
			},
		},
	}
	if cfg != nil {
		cfg(c)
	}

	authManager, err := httpauth.New(context.Background(), c.Auth,
		httpauth.WithTrustedProxy(c.TrustedProxy),
	)
	if err != nil {
		t.Fatalf("create auth manager: %v", err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return &shareTestEnv{
		handler: server.New(c, sqlDB, logger, authManager),
		links:   db.NewLinkRepo(sqlDB),
	}
}

// doShare POSTs to the share endpoint as the given user with the given email
// field value. It handles the CSRF token automatically.
func doShare(handler http.Handler, linkName, asUser, emailField string) *httptest.ResponseRecorder {
	const csrfToken = "test-csrf-token"
	form := url.Values{
		"csrf_token": {csrfToken},
		"email":      {emailField},
	}
	req := httptest.NewRequest(http.MethodPost,
		"/details/"+linkName+"/share",
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Remote-Email", asUser)
	req.AddCookie(&http.Cookie{Name: "golink_csrf", Value: csrfToken})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	return w
}

func TestShare_BareNameNoDomain_Rejected(t *testing.T) {
	env := newShareTestEnv(t, nil) // no DefaultDomain
	_, err := env.links.Create(context.Background(), "mylink", "https://example.com",
		"owner@test.com", db.LinkTypeSimple, "", false)
	if err != nil {
		t.Fatalf("create link: %v", err)
	}

	w := doShare(env.handler, "mylink", "owner@test.com", "thomas")
	// Should re-render the details page (200) with an error notification, not redirect.
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 with error page, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "valid email") {
		t.Errorf("expected 'valid email' error in page body, got: %q", w.Body.String()[:min(500, w.Body.Len())])
	}
}

func TestShare_BareNameWithDomain_Accepted(t *testing.T) {
	env := newShareTestEnv(t, func(c *config.Config) {
		c.DefaultDomain = "corp.example.com"
	})
	link, err := env.links.Create(context.Background(), "mylink", "https://example.com",
		"owner@test.com", db.LinkTypeSimple, "", false)
	if err != nil {
		t.Fatalf("create link: %v", err)
	}

	w := doShare(env.handler, "mylink", "owner@test.com", "thomas")
	if w.Code != http.StatusFound {
		t.Fatalf("expected 302 redirect on success, got %d: %s", w.Code, w.Body.String())
	}

	shares, err := env.links.GetShares(context.Background(), link.ID)
	if err != nil {
		t.Fatalf("get shares: %v", err)
	}
	if len(shares) != 1 || shares[0] != "thomas@corp.example.com" {
		t.Errorf("expected share thomas@corp.example.com, got %v", shares)
	}
}

func TestShare_FullEmail_Accepted(t *testing.T) {
	env := newShareTestEnv(t, nil)
	link, err := env.links.Create(context.Background(), "mylink", "https://example.com",
		"owner@test.com", db.LinkTypeSimple, "", false)
	if err != nil {
		t.Fatalf("create link: %v", err)
	}

	w := doShare(env.handler, "mylink", "owner@test.com", "alice@example.com")
	if w.Code != http.StatusFound {
		t.Fatalf("expected 302 redirect on success, got %d: %s", w.Code, w.Body.String())
	}

	shares, err := env.links.GetShares(context.Background(), link.ID)
	if err != nil {
		t.Fatalf("get shares: %v", err)
	}
	if len(shares) != 1 || shares[0] != "alice@example.com" {
		t.Errorf("expected share alice@example.com, got %v", shares)
	}
}

func TestShare_RequiredDomain_Rejected(t *testing.T) {
	env := newShareTestEnv(t, func(c *config.Config) {
		c.RequiredDomain = "corp.example.com"
	})
	_, err := env.links.Create(context.Background(), "mylink", "https://example.com",
		"owner@test.com", db.LinkTypeSimple, "", false)
	if err != nil {
		t.Fatalf("create link: %v", err)
	}

	w := doShare(env.handler, "mylink", "owner@test.com", "alice@other.com")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 with error page, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "corp.example.com") {
		t.Errorf("expected domain name in error message, got body excerpt: %q",
			w.Body.String()[:min(500, w.Body.Len())])
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
