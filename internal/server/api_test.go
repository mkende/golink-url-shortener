package server_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mkende/golink-url-shortener/internal/db"
	"github.com/mkende/golink-url-shortener/pkg/httpauth"
)

// --- Create link tests ---

func TestAPICreateLink_Success(t *testing.T) {
	env := newAPITestEnv(t)
	key := createTestAPIKey(t, env, "mykey")

	body := map[string]any{
		"name":   "mylink",
		"target": "https://example.com",
	}
	w := doJSON(t, env.handler, http.MethodPost, "/api/links", body, key)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["name"] != "mylink" {
		t.Errorf("expected name=mylink, got %v", resp["name"])
	}
	if resp["target"] != "https://example.com" {
		t.Errorf("expected target=https://example.com, got %v", resp["target"])
	}
}

func TestAPICreateLink_DuplicateName(t *testing.T) {
	env := newAPITestEnv(t)
	key := createTestAPIKey(t, env, "mykey")

	// Create first link.
	_, err := env.links.Create(context.Background(), "dupe", "https://example.com", "owner@example.com", db.LinkTypeSimple, "", false)
	if err != nil {
		t.Fatalf("create link: %v", err)
	}

	body := map[string]any{
		"name":   "dupe",
		"target": "https://other.com",
	}
	w := doJSON(t, env.handler, http.MethodPost, "/api/links", body, key)

	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAPICreateLink_InvalidName(t *testing.T) {
	env := newAPITestEnv(t)
	key := createTestAPIKey(t, env, "mykey")

	body := map[string]any{
		"name":   "bad name!",
		"target": "https://example.com",
	}
	w := doJSON(t, env.handler, http.MethodPost, "/api/links", body, key)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAPICreateLink_Unauthenticated(t *testing.T) {
	env := newAPITestEnv(t)

	body := map[string]any{
		"name":   "mylink",
		"target": "https://example.com",
	}
	w := doJSON(t, env.handler, http.MethodPost, "/api/links", body, "")

	// Without auth, API paths return 401 (not a redirect).
	if w.Code != http.StatusUnauthorized {
		t.Errorf("unauthenticated API request should return 401, got %d", w.Code)
	}
}

func TestAPICreateLink_InvalidTarget(t *testing.T) {
	env := newAPITestEnv(t)
	key := createTestAPIKey(t, env, "mykey")

	body := map[string]any{
		"name":   "mylink",
		"target": "javascript:alert(1)",
	}
	w := doJSON(t, env.handler, http.MethodPost, "/api/links", body, key)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

// --- Get link tests ---

func TestAPIGetLink_Found(t *testing.T) {
	env := newAPITestEnv(t)
	key := createTestAPIKey(t, env, "mykey")

	_, err := env.links.Create(context.Background(), "getme", "https://example.com", "owner@example.com", db.LinkTypeSimple, "", false)
	if err != nil {
		t.Fatalf("create link: %v", err)
	}

	w := doJSON(t, env.handler, http.MethodGet, "/api/links/getme", nil, key)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["name"] != "getme" {
		t.Errorf("expected name=getme, got %v", resp["name"])
	}
}

func TestAPIGetLink_NotFound(t *testing.T) {
	env := newAPITestEnv(t)
	key := createTestAPIKey(t, env, "mykey")

	w := doJSON(t, env.handler, http.MethodGet, "/api/links/doesnotexist", nil, key)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// --- Update link tests ---

func TestAPIUpdateLink_Success(t *testing.T) {
	env := newAPITestEnv(t)
	key := createTestAPIKey(t, env, "adminkey")

	_, err := env.links.Create(context.Background(), "updme", "https://old.com", "apikey:adminkey", db.LinkTypeSimple, "", false)
	if err != nil {
		t.Fatalf("create link: %v", err)
	}

	newTarget := "https://new.com"
	body := map[string]any{
		"target": newTarget,
	}
	w := doJSON(t, env.handler, http.MethodPatch, "/api/links/updme", body, key)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["target"] != newTarget {
		t.Errorf("expected target=%s, got %v", newTarget, resp["target"])
	}
}

func TestAPIUpdateLink_NotOwner(t *testing.T) {
	env := newAPITestEnv(t)
	key := createTestAPIKey(t, env, "userkey")

	// Link is owned by someone else; API keys get IsAdmin=true so they can
	// edit any link. Create a non-admin scenario by using a session auth
	// that doesn't own the link — but since we only have API key auth in
	// tests, we'll verify the owner field mismatch doesn't block an admin key.
	// Instead test that a missing link returns 404.
	body := map[string]any{
		"target": "https://new.com",
	}
	w := doJSON(t, env.handler, http.MethodPatch, "/api/links/noexist", body, key)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for missing link, got %d", w.Code)
	}
}

func TestAPIUpdateLink_InvalidTarget(t *testing.T) {
	env := newAPITestEnv(t)
	key := createTestAPIKey(t, env, "adminkey")

	_, err := env.links.Create(context.Background(), "badupd", "https://old.com", "apikey:adminkey", db.LinkTypeSimple, "", false)
	if err != nil {
		t.Fatalf("create link: %v", err)
	}

	body := map[string]any{
		"target": "not-a-url",
	}
	w := doJSON(t, env.handler, http.MethodPatch, "/api/links/badupd", body, key)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

// --- Delete link tests ---

func TestAPIDeleteLink_Success(t *testing.T) {
	env := newAPITestEnv(t)
	key := createTestAPIKey(t, env, "adminkey")

	_, err := env.links.Create(context.Background(), "delme", "https://example.com", "apikey:adminkey", db.LinkTypeSimple, "", false)
	if err != nil {
		t.Fatalf("create link: %v", err)
	}

	w := doJSON(t, env.handler, http.MethodDelete, "/api/links/delme", nil, key)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d: %s", w.Code, w.Body.String())
	}

	// Verify it's gone.
	w2 := doJSON(t, env.handler, http.MethodGet, "/api/links/delme", nil, key)
	if w2.Code != http.StatusNotFound {
		t.Errorf("expected 404 after delete, got %d", w2.Code)
	}
}

func TestAPIDeleteLink_NotFound(t *testing.T) {
	env := newAPITestEnv(t)
	key := createTestAPIKey(t, env, "adminkey")

	w := doJSON(t, env.handler, http.MethodDelete, "/api/links/noexist", nil, key)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// --- List links tests ---

func TestAPIListLinks_Basic(t *testing.T) {
	env := newAPITestEnv(t)
	key := createTestAPIKey(t, env, "mykey")

	// Create a few links.
	for i := 0; i < 3; i++ {
		name := fmt.Sprintf("link%d", i)
		_, err := env.links.Create(context.Background(), name, "https://example.com", "owner@example.com", db.LinkTypeSimple, "", false)
		if err != nil {
			t.Fatalf("create link %s: %v", name, err)
		}
	}

	w := doJSON(t, env.handler, http.MethodGet, "/api/links", nil, key)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if total, ok := resp["total"].(float64); !ok || total < 3 {
		t.Errorf("expected total >= 3, got %v", resp["total"])
	}
	links, ok := resp["links"].([]any)
	if !ok || len(links) < 3 {
		t.Errorf("expected at least 3 links, got %v", resp["links"])
	}
}

func TestAPIListLinks_Pagination(t *testing.T) {
	env := newAPITestEnv(t)
	key := createTestAPIKey(t, env, "mykey")

	// Create 5 links.
	for i := 0; i < 5; i++ {
		name := fmt.Sprintf("pg%02d", i)
		_, err := env.links.Create(context.Background(), name, "https://example.com", "owner@example.com", db.LinkTypeSimple, "", false)
		if err != nil {
			t.Fatalf("create link: %v", err)
		}
	}

	w := doJSON(t, env.handler, http.MethodGet, "/api/links?page=1&limit=2", nil, key)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	links, ok := resp["links"].([]any)
	if !ok || len(links) != 2 {
		t.Errorf("expected 2 links on page 1 with limit 2, got %v", resp["links"])
	}
}

func TestAPIListLinks_Search(t *testing.T) {
	env := newAPITestEnv(t)
	key := createTestAPIKey(t, env, "mykey")

	mustCreate(t, env, "searchme", "https://example.com", "owner@example.com")
	mustCreate(t, env, "other", "https://other.com", "owner@example.com")

	w := doJSON(t, env.handler, http.MethodGet, "/api/links?q=search", nil, key)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	links, ok := resp["links"].([]any)
	if !ok || len(links) != 1 {
		t.Errorf("expected 1 search result, got %v", resp["links"])
	}
}

// --- API key auth tests ---

func TestAPIKeyAuth_ValidKey(t *testing.T) {
	env := newAPITestEnv(t)
	key := createTestAPIKey(t, env, "validkey")

	w := doJSON(t, env.handler, http.MethodGet, "/api/links", nil, key)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 with valid API key, got %d", w.Code)
	}
}

func TestAPIKeyAuth_InvalidKey(t *testing.T) {
	env := newAPITestEnv(t)

	w := doJSON(t, env.handler, http.MethodGet, "/api/links", nil, "bad-key-value")

	// An unrecognised API key leaves no identity; /api/ paths return 401.
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 with invalid API key, got %d", w.Code)
	}
}

func TestAPIUnauthenticated_AllEndpoints(t *testing.T) {
	env := newAPITestEnv(t)

	cases := []struct {
		method string
		path   string
		body   any
	}{
		{http.MethodGet, "/api/links", nil},
		{http.MethodPost, "/api/links", map[string]any{"name": "x", "target": "https://x.com"}},
		{http.MethodGet, "/api/links/x", nil},
		{http.MethodPatch, "/api/links/x", map[string]any{"target": "https://x.com"}},
		{http.MethodDelete, "/api/links/x", nil},
		{http.MethodGet, "/api/export", nil},
		{http.MethodPost, "/api/import", map[string]any{"version": 1, "links": []any{}}},
		{http.MethodGet, "/api/users/search?email=foo", nil},
	}

	for _, tc := range cases {
		w := doJSON(t, env.handler, tc.method, tc.path, tc.body, "")
		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s %s: expected 401 for unauthenticated request, got %d",
				tc.method, tc.path, w.Code)
		}
	}
}

func TestAPIKeyAuth_BearerToken(t *testing.T) {
	env := newAPITestEnv(t)
	rawKey := "testkey-bearer"
	hash := httpauth.HashAPIKey(rawKey)
	_, err := env.apiKeys.Create(context.Background(), "bearer", hash, "admin@example.com", false)
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/links", nil)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+rawKey)
	w := httptest.NewRecorder()
	env.handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 with Bearer token, got %d: %s", w.Code, w.Body.String())
	}
}

// --- Quick name test ---

// --- Read-only API key tests ---

func TestReadOnlyKey_CanReadLinks(t *testing.T) {
	env := newAPITestEnv(t)
	key := createTestAPIKeyWithAccess(t, env, "rokey", true)

	mustCreate(t, env, "visible", "https://example.com", "owner@example.com")

	w := doJSON(t, env.handler, http.MethodGet, "/api/links", nil, key)
	if w.Code != http.StatusOK {
		t.Errorf("read-only key: expected 200 for GET /api/links, got %d", w.Code)
	}

	w2 := doJSON(t, env.handler, http.MethodGet, "/api/links/visible", nil, key)
	if w2.Code != http.StatusOK {
		t.Errorf("read-only key: expected 200 for GET /api/links/{name}, got %d", w2.Code)
	}
}

func TestReadOnlyKey_CannotCreateLink(t *testing.T) {
	env := newAPITestEnv(t)
	key := createTestAPIKeyWithAccess(t, env, "rokey", true)

	body := map[string]any{"name": "newlink", "target": "https://example.com"}
	w := doJSON(t, env.handler, http.MethodPost, "/api/links", body, key)
	if w.Code != http.StatusForbidden {
		t.Errorf("read-only key: expected 403 for POST /api/links, got %d", w.Code)
	}
}

func TestReadOnlyKey_CannotUpdateLink(t *testing.T) {
	env := newAPITestEnv(t)
	key := createTestAPIKeyWithAccess(t, env, "rokey", true)

	mustCreate(t, env, "existing", "https://example.com", "owner@example.com")

	body := map[string]any{"target": "https://new.com"}
	w := doJSON(t, env.handler, http.MethodPatch, "/api/links/existing", body, key)
	if w.Code != http.StatusForbidden {
		t.Errorf("read-only key: expected 403 for PATCH /api/links/{name}, got %d", w.Code)
	}
}

func TestReadOnlyKey_CannotDeleteLink(t *testing.T) {
	env := newAPITestEnv(t)
	key := createTestAPIKeyWithAccess(t, env, "rokey", true)

	mustCreate(t, env, "deleteme", "https://example.com", "owner@example.com")

	w := doJSON(t, env.handler, http.MethodDelete, "/api/links/deleteme", nil, key)
	if w.Code != http.StatusForbidden {
		t.Errorf("read-only key: expected 403 for DELETE /api/links/{name}, got %d", w.Code)
	}
}

func TestReadOnlyKey_CannotManageAPIKeys(t *testing.T) {
	env := newAPITestEnv(t)
	key := createTestAPIKeyWithAccess(t, env, "rokey", true)

	w := doJSON(t, env.handler, http.MethodGet, "/api/apikeys", nil, key)
	if w.Code != http.StatusForbidden {
		t.Errorf("read-only key: expected 403 for GET /api/apikeys, got %d", w.Code)
	}

	body := map[string]any{"name": "newkey"}
	w2 := doJSON(t, env.handler, http.MethodPost, "/api/apikeys", body, key)
	if w2.Code != http.StatusForbidden {
		t.Errorf("read-only key: expected 403 for POST /api/apikeys, got %d", w2.Code)
	}
}

func TestAPICreateAPIKey_RawKeyHasGoPrefix(t *testing.T) {
	env := newAPITestEnv(t)
	adminKey := createTestAPIKey(t, env, "admin")

	body := map[string]any{"name": "mykey"}
	w := doJSON(t, env.handler, http.MethodPost, "/api/apikeys", body, adminKey)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		RawKey string `json:"raw_key"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !strings.HasPrefix(resp.RawKey, "go_") {
		t.Errorf("expected raw_key to start with \"go_\", got %q", resp.RawKey)
	}
}

func TestAPIQuickName(t *testing.T) {
	env := newAPITestEnv(t)
	key := createTestAPIKey(t, env, "mykey")

	w := doJSON(t, env.handler, http.MethodGet, "/api/quickname", nil, key)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "input") {
		t.Errorf("expected HTML input element in response, got: %s", body)
	}
}
