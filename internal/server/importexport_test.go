package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"

	"github.com/mkende/golink-url-shortener/internal/db"
	"github.com/mkende/golink-url-shortener/internal/server"
)

// makeAdminAPIKey creates an API key whose identity is treated as an admin.
// The API key middleware marks any valid API key bearer as IsAdmin=true, so any
// valid key suffices for admin-restricted endpoints in the test server.
func makeAdminAPIKey(t *testing.T, env *apiTestEnv, name string) string {
	t.Helper()
	return createTestAPIKey(t, env, name)
}

// doRaw sends a raw-body request with optional API key header.
func doRaw(handler http.Handler, method, path string, body []byte, contentType, apiKey string) *httptest.ResponseRecorder {
	var reqBody io.Reader
	if body != nil {
		reqBody = bytes.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reqBody)
	req.Header.Set("Accept", "application/json")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if apiKey != "" {
		req.Header.Set("X-API-Key", apiKey)
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	return w
}

// exportAll calls GET /api/export and returns the parsed ExportData.
func exportAll(t *testing.T, env *apiTestEnv, apiKey string) *server.ExportData {
	t.Helper()
	w := doRaw(env.handler, http.MethodGet, "/api/export", nil, "", apiKey)
	if w.Code != http.StatusOK {
		t.Fatalf("export: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var data server.ExportData
	if err := json.NewDecoder(w.Body).Decode(&data); err != nil {
		t.Fatalf("export: decode response: %v", err)
	}
	return &data
}

// importData calls POST /api/import with the given ExportData and returns the
// raw JSON response body decoded into a map.
func importData(t *testing.T, env *apiTestEnv, data *server.ExportData, apiKey string) map[string]any {
	t.Helper()
	body, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("import: marshal: %v", err)
	}
	w := doRaw(env.handler, http.MethodPost, "/api/import", body, "application/json", apiKey)
	if w.Code != http.StatusOK {
		t.Fatalf("import: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var result map[string]any
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("import: decode response: %v", err)
	}
	return result
}

// sortExportLinks sorts links by name for deterministic comparison.
func sortExportLinks(links []server.ExportLink) {
	sort.Slice(links, func(i, j int) bool {
		return links[i].Name < links[j].Name
	})
	for i := range links {
		sort.Strings(links[i].Shares)
	}
}

// --- Round-trip test ---

func TestExportImportRoundTrip(t *testing.T) {
	env := newAPITestEnv(t)
	key := makeAdminAPIKey(t, env, "adminkey")
	ctx := context.Background()

	// Create several links, some with shares.
	link1, err := env.links.Create(ctx, "alpha", "https://alpha.example.com", "alice@example.com", db.LinkTypeSimple, "", false)
	if err != nil {
		t.Fatalf("create alpha: %v", err)
	}
	if err := env.links.AddShare(ctx, link1.ID, "bob@example.com"); err != nil {
		t.Fatalf("add share: %v", err)
	}

	_, err = env.links.Create(ctx, "beta", "https://beta.example.com", "bob@example.com", db.LinkTypeAdvanced, "", true)
	if err != nil {
		t.Fatalf("create beta: %v", err)
	}

	link3, err := env.links.Create(ctx, "gamma", "https://gamma.example.com", "carol@example.com", db.LinkTypeSimple, "", false)
	if err != nil {
		t.Fatalf("create gamma: %v", err)
	}
	if err := env.links.AddShare(ctx, link3.ID, "alice@example.com"); err != nil {
		t.Fatalf("add share: %v", err)
	}
	if err := env.links.AddShare(ctx, link3.ID, "dave@example.com"); err != nil {
		t.Fatalf("add share: %v", err)
	}

	// Export.
	first := exportAll(t, env, key)
	if first.Version != 1 {
		t.Errorf("expected version 1, got %d", first.Version)
	}
	if len(first.Links) != 3 {
		t.Fatalf("expected 3 links in export, got %d", len(first.Links))
	}

	// Delete all links.
	for _, el := range first.Links {
		link, err := env.links.GetByName(ctx, el.Name)
		if err != nil {
			t.Fatalf("get link %s: %v", el.Name, err)
		}
		if err := env.links.Delete(ctx, link.ID); err != nil {
			t.Fatalf("delete link %s: %v", el.Name, err)
		}
	}

	// Import.
	result := importData(t, env, first, key)
	if created, ok := result["created"].(float64); !ok || int(created) != 3 {
		t.Errorf("expected created=3, got %v", result["created"])
	}
	if updated, ok := result["updated"].(float64); !ok || int(updated) != 0 {
		t.Errorf("expected updated=0, got %v", result["updated"])
	}
	if skipped, ok := result["skipped"].(float64); !ok || int(skipped) != 0 {
		t.Errorf("expected skipped=0, got %v", result["skipped"])
	}

	// Export again and compare.
	second := exportAll(t, env, key)
	if len(second.Links) != len(first.Links) {
		t.Fatalf("second export has %d links, want %d", len(second.Links), len(first.Links))
	}

	sortExportLinks(first.Links)
	sortExportLinks(second.Links)

	for i := range first.Links {
		a, b := first.Links[i], second.Links[i]
		if a.Name != b.Name {
			t.Errorf("link %d: name mismatch: %q vs %q", i, a.Name, b.Name)
		}
		if a.Target != b.Target {
			t.Errorf("link %s: target mismatch: %q vs %q", a.Name, a.Target, b.Target)
		}
		if a.OwnerEmail != b.OwnerEmail {
			t.Errorf("link %s: owner_email mismatch: %q vs %q", a.Name, a.OwnerEmail, b.OwnerEmail)
		}
		if a.LinkType != b.LinkType {
			t.Errorf("link %s: link_type mismatch: %v vs %v", a.Name, a.LinkType, b.LinkType)
		}
		if a.RequireAuth != b.RequireAuth {
			t.Errorf("link %s: require_auth mismatch: %v vs %v", a.Name, a.RequireAuth, b.RequireAuth)
		}
		if fmt.Sprintf("%v", a.Shares) != fmt.Sprintf("%v", b.Shares) {
			t.Errorf("link %s: shares mismatch: %v vs %v", a.Name, a.Shares, b.Shares)
		}
	}
}

// --- Import with an existing link (update path) ---

func TestImport_UpdateExistingLink(t *testing.T) {
	env := newAPITestEnv(t)
	key := makeAdminAPIKey(t, env, "adminkey")
	ctx := context.Background()

	_, err := env.links.Create(ctx, "existing", "https://old.example.com", "owner@example.com", db.LinkTypeSimple, "", false)
	if err != nil {
		t.Fatalf("create link: %v", err)
	}

	data := &server.ExportData{
		Version: 1,
		Links: []server.ExportLink{
			{
				Name:       "existing",
				Target:     "https://new.example.com",
				OwnerEmail: "owner@example.com",
			},
		},
	}

	result := importData(t, env, data, key)
	if updated, ok := result["updated"].(float64); !ok || int(updated) != 1 {
		t.Errorf("expected updated=1, got %v", result["updated"])
	}
	if created, ok := result["created"].(float64); !ok || int(created) != 0 {
		t.Errorf("expected created=0, got %v", result["created"])
	}

	// Verify the target was actually updated.
	link, err := env.links.GetByName(ctx, "existing")
	if err != nil {
		t.Fatalf("get link: %v", err)
	}
	if link.Target != "https://new.example.com" {
		t.Errorf("expected updated target, got %q", link.Target)
	}
}

// --- Invalid name is skipped ---

func TestImport_InvalidName(t *testing.T) {
	env := newAPITestEnv(t)
	key := makeAdminAPIKey(t, env, "adminkey")

	data := &server.ExportData{
		Version: 1,
		Links: []server.ExportLink{
			{
				Name:       "bad name!",
				Target:     "https://example.com",
				OwnerEmail: "owner@example.com",
			},
		},
	}

	result := importData(t, env, data, key)
	if skipped, ok := result["skipped"].(float64); !ok || int(skipped) != 1 {
		t.Errorf("expected skipped=1, got %v", result["skipped"])
	}
	errs, _ := result["errors"].([]any)
	if len(errs) == 0 {
		t.Errorf("expected at least one error message")
	}
}

// --- Invalid target is skipped ---

func TestImport_InvalidTarget(t *testing.T) {
	env := newAPITestEnv(t)
	key := makeAdminAPIKey(t, env, "adminkey")

	data := &server.ExportData{
		Version: 1,
		Links: []server.ExportLink{
			{
				Name:       "validname",
				Target:     "javascript:alert(1)",
				OwnerEmail: "owner@example.com",
			},
		},
	}

	result := importData(t, env, data, key)
	if skipped, ok := result["skipped"].(float64); !ok || int(skipped) != 1 {
		t.Errorf("expected skipped=1, got %v", result["skipped"])
	}
	errs, _ := result["errors"].([]any)
	if len(errs) == 0 {
		t.Errorf("expected at least one error message")
	}
}

// --- Access control ---

func TestExport_RequiresAuth(t *testing.T) {
	env := newAPITestEnv(t)
	// No API key => not authenticated at all.
	w := doRaw(env.handler, http.MethodGet, "/api/export", nil, "", "")
	if w.Code != http.StatusUnauthorized {
		t.Errorf("unauthenticated export should return 401, got %d", w.Code)
	}
}

func TestImport_RequiresAuth(t *testing.T) {
	env := newAPITestEnv(t)
	data := &server.ExportData{Version: 1}
	body, _ := json.Marshal(data)
	w := doRaw(env.handler, http.MethodPost, "/api/import", body, "application/json", "")
	if w.Code != http.StatusUnauthorized {
		t.Errorf("unauthenticated import should return 401, got %d", w.Code)
	}
}

func TestExport_EmptyDatabase(t *testing.T) {
	env := newAPITestEnv(t)
	key := makeAdminAPIKey(t, env, "adminkey")

	data := exportAll(t, env, key)
	if data.Version != 1 {
		t.Errorf("expected version 1, got %d", data.Version)
	}
	if len(data.Links) != 0 {
		t.Errorf("expected 0 links in empty export, got %d", len(data.Links))
	}
}

func TestReadOnlyKey_CanExport(t *testing.T) {
	env := newAPITestEnv(t)
	key := createTestAPIKeyWithAccess(t, env, "rokey", true)

	w := doRaw(env.handler, http.MethodGet, "/api/export", nil, "", key)
	if w.Code != http.StatusOK {
		t.Errorf("read-only key: expected 200 for GET /api/export, got %d: %s", w.Code, w.Body.String())
	}
}

func TestReadOnlyKey_CannotImport(t *testing.T) {
	env := newAPITestEnv(t)
	key := createTestAPIKeyWithAccess(t, env, "rokey", true)

	data := &server.ExportData{Version: 1}
	body, _ := json.Marshal(data)
	w := doRaw(env.handler, http.MethodPost, "/api/import", body, "application/json", key)
	if w.Code != http.StatusForbidden {
		t.Errorf("read-only key: expected 403 for POST /api/import, got %d", w.Code)
	}
}

func TestImport_MixedValidAndInvalid(t *testing.T) {
	env := newAPITestEnv(t)
	key := makeAdminAPIKey(t, env, "adminkey")

	data := &server.ExportData{
		Version: 1,
		Links: []server.ExportLink{
			{Name: "good", Target: "https://good.example.com", OwnerEmail: "a@example.com"},
			{Name: "bad name!", Target: "https://example.com", OwnerEmail: "a@example.com"},
			{Name: "also-good", Target: "https://also.example.com", OwnerEmail: "b@example.com"},
		},
	}

	result := importData(t, env, data, key)

	if created, ok := result["created"].(float64); !ok || int(created) != 2 {
		t.Errorf("expected created=2, got %v", result["created"])
	}
	if skipped, ok := result["skipped"].(float64); !ok || int(skipped) != 1 {
		t.Errorf("expected skipped=1, got %v", result["skipped"])
	}
}
