package db

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
)

// openTestDB opens an in-memory SQLite database and returns the *sql.DB.
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := Open(context.Background(), "sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// ---- LinkRepo tests ---------------------------------------------------------

func TestLinkRepo_CreateAndGet(t *testing.T) {
	repo := NewLinkRepo(openTestDB(t))
	ctx := context.Background()

	link, err := repo.Create(ctx, "Docs", "https://example.com/docs", "alice@example.com", false, false)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if link.ID == 0 {
		t.Error("expected non-zero ID after create")
	}
	if link.NameLower != "docs" {
		t.Errorf("NameLower = %q, want %q", link.NameLower, "docs")
	}
	if link.Name != "Docs" {
		t.Errorf("Name = %q, want %q", link.Name, "Docs")
	}

	got, err := repo.GetByName(ctx, "docs")
	if err != nil {
		t.Fatalf("GetByName: %v", err)
	}
	if got.Target != "https://example.com/docs" {
		t.Errorf("Target = %q, want %q", got.Target, "https://example.com/docs")
	}
	if got.OwnerEmail != "alice@example.com" {
		t.Errorf("OwnerEmail = %q", got.OwnerEmail)
	}
}

func TestLinkRepo_GetByName_NotFound(t *testing.T) {
	repo := NewLinkRepo(openTestDB(t))
	_, err := repo.GetByName(context.Background(), "nosuchlink")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestLinkRepo_Update(t *testing.T) {
	repo := NewLinkRepo(openTestDB(t))
	ctx := context.Background()

	orig, err := repo.Create(ctx, "old", "https://old.example.com", "bob@example.com", false, false)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	updated, err := repo.Update(ctx, orig.ID, "New", "https://new.example.com", true, true)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Name != "New" {
		t.Errorf("Name = %q, want %q", updated.Name, "New")
	}
	if updated.NameLower != "new" {
		t.Errorf("NameLower = %q, want %q", updated.NameLower, "new")
	}
	if !updated.IsAdvanced {
		t.Error("expected IsAdvanced = true")
	}
	if !updated.RequireAuth {
		t.Error("expected RequireAuth = true")
	}
}

func TestLinkRepo_Update_NotFound(t *testing.T) {
	repo := NewLinkRepo(openTestDB(t))
	_, err := repo.Update(context.Background(), 99999, "x", "https://x.com", false, false)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestLinkRepo_Delete(t *testing.T) {
	repo := NewLinkRepo(openTestDB(t))
	ctx := context.Background()

	link, err := repo.Create(ctx, "todelete", "https://example.com", "c@example.com", false, false)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := repo.Delete(ctx, link.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, err = repo.GetByName(ctx, "todelete")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestLinkRepo_Delete_NotFound(t *testing.T) {
	repo := NewLinkRepo(openTestDB(t))
	err := repo.Delete(context.Background(), 99999)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestLinkRepo_List_Pagination(t *testing.T) {
	repo := NewLinkRepo(openTestDB(t))
	ctx := context.Background()

	names := []string{"alpha", "beta", "gamma", "delta", "epsilon"}
	for _, n := range names {
		if _, err := repo.Create(ctx, n, "https://"+n+".example.com", "owner@example.com", false, false); err != nil {
			t.Fatalf("Create %s: %v", n, err)
		}
	}

	page1, total, err := repo.List(ctx, 3, 0, SortByName, SortAsc)
	if err != nil {
		t.Fatalf("List page 1: %v", err)
	}
	if total != 5 {
		t.Errorf("total = %d, want 5", total)
	}
	if len(page1) != 3 {
		t.Fatalf("page1 len = %d, want 3", len(page1))
	}

	page2, _, err := repo.List(ctx, 3, 3, SortByName, SortAsc)
	if err != nil {
		t.Fatalf("List page 2: %v", err)
	}
	if len(page2) != 2 {
		t.Fatalf("page2 len = %d, want 2", len(page2))
	}
}

func TestLinkRepo_List_SortFields(t *testing.T) {
	repo := NewLinkRepo(openTestDB(t))
	ctx := context.Background()

	if _, err := repo.Create(ctx, "aaa", "https://a.example.com", "owner@example.com", false, false); err != nil {
		t.Fatalf("Create: %v", err)
	}

	cases := []struct {
		field SortField
		dir   SortDir
	}{
		{SortByName, SortAsc},
		{SortByName, SortDesc},
		{SortByCreated, SortAsc},
		{SortByCreated, SortDesc},
		{SortByUseCount, SortAsc},
		{SortByUseCount, SortDesc},
		{SortByLastUsed, SortAsc},
		{SortByLastUsed, SortDesc},
	}
	for _, tc := range cases {
		_, _, err := repo.List(ctx, 10, 0, tc.field, tc.dir)
		if err != nil {
			t.Errorf("List(%s, %s): %v", tc.field, tc.dir, err)
		}
	}
}

func TestLinkRepo_List_InvalidSort(t *testing.T) {
	repo := NewLinkRepo(openTestDB(t))
	_, _, err := repo.List(context.Background(), 10, 0, "bad_column; DROP TABLE links--", SortAsc)
	if err == nil {
		t.Error("expected error for invalid sort field")
	}
}

func TestLinkRepo_Search(t *testing.T) {
	repo := NewLinkRepo(openTestDB(t))
	ctx := context.Background()

	for _, n := range []string{"go-docs", "go-api", "go-home", "other"} {
		if _, err := repo.Create(ctx, n, "https://"+n+".example.com", "o@example.com", false, false); err != nil {
			t.Fatalf("Create %s: %v", n, err)
		}
	}

	results, total, err := repo.Search(ctx, "go-", 10, 0)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if total != 3 {
		t.Errorf("total = %d, want 3", total)
	}
	if len(results) != 3 {
		t.Errorf("results len = %d, want 3", len(results))
	}
	for _, l := range results {
		if !strings.HasPrefix(l.NameLower, "go-") {
			t.Errorf("unexpected result: %s", l.Name)
		}
	}
}

func TestLinkRepo_Shares(t *testing.T) {
	repo := NewLinkRepo(openTestDB(t))
	ctx := context.Background()

	link, err := repo.Create(ctx, "shared", "https://example.com", "owner@example.com", false, false)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Initially no shares.
	shares, err := repo.GetShares(ctx, link.ID)
	if err != nil {
		t.Fatalf("GetShares (empty): %v", err)
	}
	if len(shares) != 0 {
		t.Errorf("expected 0 shares, got %d", len(shares))
	}

	// Add shares.
	for _, email := range []string{"a@example.com", "b@example.com"} {
		if err := repo.AddShare(ctx, link.ID, email); err != nil {
			t.Fatalf("AddShare %s: %v", email, err)
		}
	}
	// Duplicate should not error.
	if err := repo.AddShare(ctx, link.ID, "a@example.com"); err != nil {
		t.Fatalf("AddShare duplicate: %v", err)
	}

	shares, err = repo.GetShares(ctx, link.ID)
	if err != nil {
		t.Fatalf("GetShares: %v", err)
	}
	if len(shares) != 2 {
		t.Errorf("expected 2 shares, got %d", len(shares))
	}

	// Remove one share.
	if err := repo.RemoveShare(ctx, link.ID, "a@example.com"); err != nil {
		t.Fatalf("RemoveShare: %v", err)
	}
	shares, err = repo.GetShares(ctx, link.ID)
	if err != nil {
		t.Fatalf("GetShares after remove: %v", err)
	}
	if len(shares) != 1 || shares[0] != "b@example.com" {
		t.Errorf("unexpected shares after remove: %v", shares)
	}
}

func TestLinkRepo_IncrementUseCount(t *testing.T) {
	repo := NewLinkRepo(openTestDB(t))
	ctx := context.Background()

	link, err := repo.Create(ctx, "popular", "https://example.com", "o@example.com", false, false)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if link.UseCount != 0 {
		t.Errorf("UseCount initial = %d, want 0", link.UseCount)
	}

	for i := 0; i < 3; i++ {
		if err := repo.IncrementUseCount(ctx, link.ID); err != nil {
			t.Fatalf("IncrementUseCount: %v", err)
		}
	}

	got, err := repo.GetByName(ctx, "popular")
	if err != nil {
		t.Fatalf("GetByName: %v", err)
	}
	if got.UseCount != 3 {
		t.Errorf("UseCount = %d, want 3", got.UseCount)
	}
	if !got.LastUsedAt.Valid {
		t.Error("expected LastUsedAt to be set after increment")
	}
}

func TestLinkRepo_ListByOwner(t *testing.T) {
	repo := NewLinkRepo(openTestDB(t))
	ctx := context.Background()

	for _, n := range []string{"link1", "link2", "link3"} {
		if _, err := repo.Create(ctx, n, "https://example.com", "alice@example.com", false, false); err != nil {
			t.Fatalf("Create %s: %v", n, err)
		}
	}
	if _, err := repo.Create(ctx, "other", "https://example.com", "bob@example.com", false, false); err != nil {
		t.Fatalf("Create other: %v", err)
	}

	links, total, err := repo.ListByOwner(ctx, "alice@example.com", 10, 0)
	if err != nil {
		t.Fatalf("ListByOwner: %v", err)
	}
	if total != 3 {
		t.Errorf("total = %d, want 3", total)
	}
	if len(links) != 3 {
		t.Errorf("len = %d, want 3", len(links))
	}
	for _, l := range links {
		if l.OwnerEmail != "alice@example.com" {
			t.Errorf("unexpected owner: %s", l.OwnerEmail)
		}
	}
}

// ---- UserRepo tests ---------------------------------------------------------

func TestUserRepo_UpsertAndGet(t *testing.T) {
	repo := NewUserRepo(openTestDB(t))
	ctx := context.Background()

	u, err := repo.Upsert(ctx, "alice@example.com", "Alice", "https://example.com/avatar.png")
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if u.Email != "alice@example.com" {
		t.Errorf("Email = %q", u.Email)
	}
	if u.DisplayName != "Alice" {
		t.Errorf("DisplayName = %q", u.DisplayName)
	}

	// Upsert again with updated name.
	u2, err := repo.Upsert(ctx, "alice@example.com", "Alice Updated", "")
	if err != nil {
		t.Fatalf("Upsert (update): %v", err)
	}
	if u2.DisplayName != "Alice Updated" {
		t.Errorf("DisplayName after update = %q, want %q", u2.DisplayName, "Alice Updated")
	}

	got, err := repo.Get(ctx, "alice@example.com")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.DisplayName != "Alice Updated" {
		t.Errorf("DisplayName = %q", got.DisplayName)
	}
}

func TestUserRepo_Get_NotFound(t *testing.T) {
	repo := NewUserRepo(openTestDB(t))
	_, err := repo.Get(context.Background(), "noone@example.com")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestUserRepo_List(t *testing.T) {
	repo := NewUserRepo(openTestDB(t))
	ctx := context.Background()

	emails := []string{"charlie@example.com", "alice@example.com", "bob@example.com"}
	for _, e := range emails {
		if _, err := repo.Upsert(ctx, e, "", ""); err != nil {
			t.Fatalf("Upsert %s: %v", e, err)
		}
	}

	users, err := repo.List(ctx, 10, 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(users) != 3 {
		t.Fatalf("len = %d, want 3", len(users))
	}
	// Should be sorted by email ASC.
	if users[0].Email != "alice@example.com" {
		t.Errorf("first = %q, want alice@example.com", users[0].Email)
	}
}

func TestUserRepo_Search(t *testing.T) {
	repo := NewUserRepo(openTestDB(t))
	ctx := context.Background()

	if _, err := repo.Upsert(ctx, "alice@example.com", "Alice Smith", ""); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if _, err := repo.Upsert(ctx, "bob@example.com", "Bob Jones", ""); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	results, err := repo.Search(ctx, "alice", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 || results[0].Email != "alice@example.com" {
		t.Errorf("unexpected search results: %v", results)
	}
}

// ---- APIKeyRepo tests -------------------------------------------------------

func TestAPIKeyRepo_CreateAndGetByHash(t *testing.T) {
	repo := NewAPIKeyRepo(openTestDB(t))
	ctx := context.Background()

	k, err := repo.Create(ctx, "my-key", "hashvalue123", "admin@example.com")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if k.ID == 0 {
		t.Error("expected non-zero ID")
	}
	if k.Name != "my-key" {
		t.Errorf("Name = %q", k.Name)
	}

	got, err := repo.GetByHash(ctx, "hashvalue123")
	if err != nil {
		t.Fatalf("GetByHash: %v", err)
	}
	if got.KeyHash != "hashvalue123" {
		t.Errorf("KeyHash = %q", got.KeyHash)
	}
	if got.CreatedBy != "admin@example.com" {
		t.Errorf("CreatedBy = %q", got.CreatedBy)
	}
}

func TestAPIKeyRepo_GetByHash_NotFound(t *testing.T) {
	repo := NewAPIKeyRepo(openTestDB(t))
	_, err := repo.GetByHash(context.Background(), "nonexistent")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestAPIKeyRepo_List(t *testing.T) {
	repo := NewAPIKeyRepo(openTestDB(t))
	ctx := context.Background()

	for i, name := range []string{"key-a", "key-b", "key-c"} {
		hash := strings.Repeat("x", i+1) // distinct hashes
		if _, err := repo.Create(ctx, name, hash, "admin@example.com"); err != nil {
			t.Fatalf("Create %s: %v", name, err)
		}
	}

	keys, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(keys) != 3 {
		t.Errorf("len = %d, want 3", len(keys))
	}
}

func TestAPIKeyRepo_Delete(t *testing.T) {
	repo := NewAPIKeyRepo(openTestDB(t))
	ctx := context.Background()

	k, err := repo.Create(ctx, "to-delete", "hash-del", "admin@example.com")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := repo.Delete(ctx, k.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, err = repo.GetByHash(ctx, "hash-del")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestAPIKeyRepo_Delete_NotFound(t *testing.T) {
	repo := NewAPIKeyRepo(openTestDB(t))
	err := repo.Delete(context.Background(), 99999)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestAPIKeyRepo_UpdateLastUsed(t *testing.T) {
	repo := NewAPIKeyRepo(openTestDB(t))
	ctx := context.Background()

	k, err := repo.Create(ctx, "ts-key", "hash-ts", "admin@example.com")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if k.LastUsedAt.Valid {
		t.Error("expected LastUsedAt to be null initially")
	}
	if err := repo.UpdateLastUsed(ctx, k.ID); err != nil {
		t.Fatalf("UpdateLastUsed: %v", err)
	}
	got, err := repo.GetByHash(ctx, "hash-ts")
	if err != nil {
		t.Fatalf("GetByHash: %v", err)
	}
	if !got.LastUsedAt.Valid {
		t.Error("expected LastUsedAt to be set after UpdateLastUsed")
	}
}
