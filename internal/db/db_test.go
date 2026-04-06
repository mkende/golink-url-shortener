package db

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"testing"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
)

// testPostgresDSN is set by TestMain when embedded Postgres starts successfully,
// or when TEST_POSTGRES_DSN is provided externally.
var testPostgresDSN string

// TestMain starts an embedded Postgres instance shared across all tests in this
// package, then runs the tests.  If Postgres cannot start the tests still run
// against SQLite only.
func TestMain(m *testing.M) {
	os.Exit(runTests(m))
}

func runTests(m *testing.M) int {
	// Allow an external DSN override (e.g. from CI with a managed Postgres).
	if dsn := os.Getenv("TEST_POSTGRES_DSN"); dsn != "" {
		testPostgresDSN = dsn
		return m.Run()
	}

	port, err := getFreePort()
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: find free port: %v; running SQLite tests only\n", err)
		return m.Run()
	}

	tmpDir, err := os.MkdirTemp("", "embpg-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: create temp dir: %v; running SQLite tests only\n", err)
		return m.Run()
	}
	defer os.RemoveAll(tmpDir)

	pg := embeddedpostgres.NewDatabase(embeddedpostgres.DefaultConfig().
		Username("test").
		Password("test").
		Database("test").
		Port(uint32(port)).
		RuntimePath(tmpDir))

	if err := pg.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: start embedded postgres: %v; running SQLite tests only\n", err)
		return m.Run()
	}
	defer pg.Stop()

	testPostgresDSN = fmt.Sprintf("host=localhost port=%d user=test password=test dbname=test sslmode=disable", port)
	return m.Run()
}

func getFreePort() (int, error) {
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		return 0, err
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	return port, nil
}

// testBackend pairs a backend name with an open *DB for parameterised testing.
type testBackend struct {
	name string
	db   *DB
}

// allBackends returns backends to run each test against.
// SQLite (in-memory) is always included.  Postgres is added when the
// TEST_POSTGRES_DSN environment variable is set.
func allBackends(t *testing.T) []testBackend {
	t.Helper()
	bs := []testBackend{{"sqlite", openSQLiteDB(t)}}
	if testPostgresDSN != "" {
		bs = append(bs, testBackend{"postgres", openPostgresDB(t, testPostgresDSN)})
	}
	return bs
}

func openSQLiteDB(t *testing.T) *DB {
	t.Helper()
	db, err := Open(context.Background(), "sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite test database: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func openPostgresDB(t *testing.T, dsn string) *DB {
	t.Helper()
	db, err := Open(context.Background(), "postgres", dsn)
	if err != nil {
		t.Fatalf("open postgres test database: %v", err)
	}
	t.Cleanup(func() {
		// TRUNCATE resets sequences and cascades through FK relationships.
		db.Exec("TRUNCATE api_keys, group_members, groups, link_shares, links, users RESTART IDENTITY CASCADE") //nolint:errcheck
		db.Close()
	})
	return db
}

// ---- LinkRepo tests ---------------------------------------------------------

func TestLinkRepo_CreateAndGet(t *testing.T) {
	for _, b := range allBackends(t) {
		t.Run(b.name, func(t *testing.T) {
			repo := NewLinkRepo(b.db)
			ctx := context.Background()

			link, err := repo.Create(ctx, "Docs", "https://example.com/docs", "alice@example.com", LinkTypeSimple, "", false)
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
		})
	}
}

func TestLinkRepo_GetByName_NotFound(t *testing.T) {
	for _, b := range allBackends(t) {
		t.Run(b.name, func(t *testing.T) {
			repo := NewLinkRepo(b.db)
			_, err := repo.GetByName(context.Background(), "nosuchlink")
			if !errors.Is(err, ErrNotFound) {
				t.Errorf("expected ErrNotFound, got %v", err)
			}
		})
	}
}

func TestLinkRepo_Update(t *testing.T) {
	for _, b := range allBackends(t) {
		t.Run(b.name, func(t *testing.T) {
			repo := NewLinkRepo(b.db)
			ctx := context.Background()

			orig, err := repo.Create(ctx, "old", "https://old.example.com", "bob@example.com", LinkTypeSimple, "", false)
			if err != nil {
				t.Fatalf("Create: %v", err)
			}

			updated, err := repo.Update(ctx, orig.ID, "New", "https://new.example.com", LinkTypeAdvanced, true)
			if err != nil {
				t.Fatalf("Update: %v", err)
			}
			if updated.Name != "New" {
				t.Errorf("Name = %q, want %q", updated.Name, "New")
			}
			if updated.NameLower != "new" {
				t.Errorf("NameLower = %q, want %q", updated.NameLower, "new")
			}
			if !updated.IsAdvanced() {
				t.Error("expected IsAdvanced = true")
			}
			if !updated.RequireAuth {
				t.Error("expected RequireAuth = true")
			}
		})
	}
}

func TestLinkRepo_Update_NotFound(t *testing.T) {
	for _, b := range allBackends(t) {
		t.Run(b.name, func(t *testing.T) {
			repo := NewLinkRepo(b.db)
			_, err := repo.Update(context.Background(), 99999, "x", "https://x.com", LinkTypeSimple, false)
			if !errors.Is(err, ErrNotFound) {
				t.Errorf("expected ErrNotFound, got %v", err)
			}
		})
	}
}

func TestLinkRepo_Delete(t *testing.T) {
	for _, b := range allBackends(t) {
		t.Run(b.name, func(t *testing.T) {
			repo := NewLinkRepo(b.db)
			ctx := context.Background()

			link, err := repo.Create(ctx, "todelete", "https://example.com", "c@example.com", LinkTypeSimple, "", false)
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
		})
	}
}

func TestLinkRepo_Delete_NotFound(t *testing.T) {
	for _, b := range allBackends(t) {
		t.Run(b.name, func(t *testing.T) {
			repo := NewLinkRepo(b.db)
			err := repo.Delete(context.Background(), 99999)
			if !errors.Is(err, ErrNotFound) {
				t.Errorf("expected ErrNotFound, got %v", err)
			}
		})
	}
}

func TestLinkRepo_List_Pagination(t *testing.T) {
	for _, b := range allBackends(t) {
		t.Run(b.name, func(t *testing.T) {
			repo := NewLinkRepo(b.db)
			ctx := context.Background()

			names := []string{"alpha", "beta", "gamma", "delta", "epsilon"}
			for _, n := range names {
				if _, err := repo.Create(ctx, n, "https://"+n+".example.com", "owner@example.com", LinkTypeSimple, "", false); err != nil {
					t.Fatalf("Create %s: %v", n, err)
				}
			}

			page1, total, err := repo.List(ctx, 3, 0, SortByName, SortAsc, false)
			if err != nil {
				t.Fatalf("List page 1: %v", err)
			}
			if total != 5 {
				t.Errorf("total = %d, want 5", total)
			}
			if len(page1) != 3 {
				t.Fatalf("page1 len = %d, want 3", len(page1))
			}

			page2, _, err := repo.List(ctx, 3, 3, SortByName, SortAsc, false)
			if err != nil {
				t.Fatalf("List page 2: %v", err)
			}
			if len(page2) != 2 {
				t.Fatalf("page2 len = %d, want 2", len(page2))
			}
		})
	}
}

func TestLinkRepo_List_SortFields(t *testing.T) {
	for _, b := range allBackends(t) {
		t.Run(b.name, func(t *testing.T) {
			repo := NewLinkRepo(b.db)
			ctx := context.Background()

			if _, err := repo.Create(ctx, "aaa", "https://a.example.com", "owner@example.com", LinkTypeSimple, "", false); err != nil {
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
				_, _, err := repo.List(ctx, 10, 0, tc.field, tc.dir, false)
				if err != nil {
					t.Errorf("List(%s, %s): %v", tc.field, tc.dir, err)
				}
			}
		})
	}
}

func TestLinkRepo_List_InvalidSort(t *testing.T) {
	for _, b := range allBackends(t) {
		t.Run(b.name, func(t *testing.T) {
			repo := NewLinkRepo(b.db)
			_, _, err := repo.List(context.Background(), 10, 0, "bad_column; DROP TABLE links--", SortAsc, false)
			if err == nil {
				t.Error("expected error for invalid sort field")
			}
		})
	}
}

func TestLinkRepo_Search(t *testing.T) {
	for _, b := range allBackends(t) {
		t.Run(b.name, func(t *testing.T) {
			repo := NewLinkRepo(b.db)
			ctx := context.Background()

			links := []struct{ name, target string }{
				{"go-docs", "https://docs.example.com"},
				{"go-api", "https://api.example.com"},
				{"go-home", "https://home.example.com"},
				{"other", "https://other.example.com/go-extra"},
			}
			for _, l := range links {
				if _, err := repo.Create(ctx, l.name, l.target, "o@example.com", LinkTypeSimple, "", false); err != nil {
					t.Fatalf("Create %s: %v", l.name, err)
				}
			}

			cases := []struct {
				query  string
				want   int
				checkFn func(l *Link) bool
			}{
				// Plain substring — name or target
				{"go-", 4, nil}, // "go-" appears in all 4 names or targets
				// Anchor on name
				{"^go-", 3, func(l *Link) bool { return strings.HasPrefix(l.NameLower, "go-") }},
				{"docs$", 1, func(l *Link) bool { return strings.HasSuffix(l.NameLower, "docs") }},
				{"^go-docs$", 1, func(l *Link) bool { return l.NameLower == "go-docs" }},
				{"^nomatch$", 0, nil},
				// name: prefix
				{"name:go-", 3, func(l *Link) bool { return strings.Contains(l.NameLower, "go-") }},
				{"n:^go-", 3, func(l *Link) bool { return strings.HasPrefix(l.NameLower, "go-") }},
				{"n:^other$", 1, func(l *Link) bool { return l.NameLower == "other" }},
				// target: prefix
				{"target:docs", 1, func(l *Link) bool { return strings.Contains(strings.ToLower(l.Target), "docs") }},
				{"t:^https://", 4, func(l *Link) bool { return strings.HasPrefix(strings.ToLower(l.Target), "https://") }},
				{"t:example.com$", 3, func(l *Link) bool { return strings.HasSuffix(strings.ToLower(l.Target), "example.com") }},
				{"t:extra$", 1, func(l *Link) bool { return strings.HasSuffix(strings.ToLower(l.Target), "extra") }},
			}
			for _, tc := range cases {
				results, total, err := repo.Search(ctx, tc.query, 10, 0, false)
				if err != nil {
					t.Fatalf("Search(%q): %v", tc.query, err)
				}
				if total != tc.want {
					t.Errorf("Search(%q) total = %d, want %d", tc.query, total, tc.want)
				}
				if tc.checkFn != nil {
					for _, l := range results {
						if !tc.checkFn(l) {
							t.Errorf("Search(%q) unexpected result: name=%s target=%s", tc.query, l.Name, l.Target)
						}
					}
				}
			}
		})
	}
}

func TestLinkRepo_Shares(t *testing.T) {
	for _, b := range allBackends(t) {
		t.Run(b.name, func(t *testing.T) {
			repo := NewLinkRepo(b.db)
			ctx := context.Background()

			link, err := repo.Create(ctx, "shared", "https://example.com", "owner@example.com", LinkTypeSimple, "", false)
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
		})
	}
}

func TestLinkRepo_IncrementUseCount(t *testing.T) {
	for _, b := range allBackends(t) {
		t.Run(b.name, func(t *testing.T) {
			repo := NewLinkRepo(b.db)
			ctx := context.Background()

			link, err := repo.Create(ctx, "popular", "https://example.com", "o@example.com", LinkTypeSimple, "", false)
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
		})
	}
}

func TestLinkRepo_ListByOwner(t *testing.T) {
	for _, b := range allBackends(t) {
		t.Run(b.name, func(t *testing.T) {
			repo := NewLinkRepo(b.db)
			ctx := context.Background()

			for _, n := range []string{"link1", "link2", "link3"} {
				if _, err := repo.Create(ctx, n, "https://example.com", "alice@example.com", LinkTypeSimple, "", false); err != nil {
					t.Fatalf("Create %s: %v", n, err)
				}
			}
			if _, err := repo.Create(ctx, "other", "https://example.com", "bob@example.com", LinkTypeSimple, "", false); err != nil {
				t.Fatalf("Create other: %v", err)
			}

			links, total, err := repo.ListByOwner(ctx, "alice@example.com", 10, 0, SortByName, SortAsc)
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
		})
	}
}

func TestLinkRepo_Alias(t *testing.T) {
	for _, b := range allBackends(t) {
		t.Run(b.name, func(t *testing.T) {
			repo := NewLinkRepo(b.db)
			ctx := context.Background()

			// Create a canonical link.
			canonical, err := repo.Create(ctx, "docs", "https://docs.example.com", "alice@example.com", LinkTypeSimple, "", false)
			if err != nil {
				t.Fatalf("Create canonical: %v", err)
			}

			// Create an alias link pointing at the canonical.
			alias, err := repo.Create(ctx, "d", "", "bob@example.com", LinkTypeAlias, "docs", false)
			if err != nil {
				t.Fatalf("Create alias: %v", err)
			}
			if !alias.IsAlias() {
				t.Error("expected IsAlias = true")
			}
			if alias.AliasTarget != "docs" {
				t.Errorf("AliasTarget = %q, want %q", alias.AliasTarget, "docs")
			}

			// GetAliases should return the alias.
			aliases, err := repo.GetAliases(ctx, "docs")
			if err != nil {
				t.Fatalf("GetAliases: %v", err)
			}
			if len(aliases) != 1 || aliases[0].Name != "d" {
				t.Errorf("unexpected aliases: %v", aliases)
			}

			// CountAliases should return 1.
			count, err := repo.CountAliases(ctx, "docs")
			if err != nil {
				t.Fatalf("CountAliases: %v", err)
			}
			if count != 1 {
				t.Errorf("CountAliases = %d, want 1", count)
			}

			// SetAlias: convert the canonical to an alias of a new link.
			newLink, err := repo.Create(ctx, "newdocs", "https://newdocs.example.com", "alice@example.com", LinkTypeSimple, "", false)
			if err != nil {
				t.Fatalf("Create newdocs: %v", err)
			}

			converted, err := repo.SetAlias(ctx, canonical.ID, "docs", "newdocs", false, 100)
			if err != nil {
				t.Fatalf("SetAlias: %v", err)
			}
			if !converted.IsAlias() {
				t.Error("expected converted link to be alias")
			}
			if converted.AliasTarget != "newdocs" {
				t.Errorf("AliasTarget = %q, want %q", converted.AliasTarget, "newdocs")
			}

			// The existing alias "d" should now point at "newdocs" (reparented).
			reparented, err := repo.GetByName(ctx, "d")
			if err != nil {
				t.Fatalf("GetByName d: %v", err)
			}
			if reparented.AliasTarget != "newdocs" {
				t.Errorf("reparented alias_target = %q, want %q", reparented.AliasTarget, "newdocs")
			}

			// newLink should now have 2 aliases: "docs" and "d".
			aliases, err = repo.GetAliases(ctx, "newdocs")
			if err != nil {
				t.Fatalf("GetAliases newdocs: %v", err)
			}
			if len(aliases) != 2 {
				t.Errorf("aliases of newdocs = %d, want 2", len(aliases))
			}
			_ = newLink
		})
	}
}

func TestLinkRepo_SetAlias_LimitExceeded(t *testing.T) {
	for _, b := range allBackends(t) {
		t.Run(b.name, func(t *testing.T) {
			repo := NewLinkRepo(b.db)
			ctx := context.Background()

			_, err := repo.Create(ctx, "canon", "https://canon.example.com", "alice@example.com", LinkTypeSimple, "", false)
			if err != nil {
				t.Fatalf("Create canonical: %v", err)
			}
			// Fill the canonical's aliases up to the limit.
			for i := 0; i < 3; i++ {
				name := "a" + string(rune('0'+i))
				if _, err := repo.Create(ctx, name, "", "bob@example.com", LinkTypeAlias, "canon", false); err != nil {
					t.Fatalf("Create alias %s: %v", name, err)
				}
			}

			// Try to convert another link to alias with maxAliases=3; should fail.
			other, err := repo.Create(ctx, "other", "https://other.example.com", "alice@example.com", LinkTypeSimple, "", false)
			if err != nil {
				t.Fatalf("Create other: %v", err)
			}
			_, err = repo.SetAlias(ctx, other.ID, "other", "canon", false, 3)
			if !errors.Is(err, ErrAliasLimitExceeded) {
				t.Errorf("expected ErrAliasLimitExceeded, got %v", err)
			}
		})
	}
}

// ---- UserRepo tests ---------------------------------------------------------

func TestUserRepo_UpsertAndGet(t *testing.T) {
	for _, b := range allBackends(t) {
		t.Run(b.name, func(t *testing.T) {
			repo := NewUserRepo(b.db)
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
		})
	}
}

func TestUserRepo_Get_NotFound(t *testing.T) {
	for _, b := range allBackends(t) {
		t.Run(b.name, func(t *testing.T) {
			repo := NewUserRepo(b.db)
			_, err := repo.Get(context.Background(), "noone@example.com")
			if !errors.Is(err, ErrNotFound) {
				t.Errorf("expected ErrNotFound, got %v", err)
			}
		})
	}
}

func TestUserRepo_List(t *testing.T) {
	for _, b := range allBackends(t) {
		t.Run(b.name, func(t *testing.T) {
			repo := NewUserRepo(b.db)
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
		})
	}
}

func TestUserRepo_Search(t *testing.T) {
	for _, b := range allBackends(t) {
		t.Run(b.name, func(t *testing.T) {
			repo := NewUserRepo(b.db)
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
		})
	}
}

// ---- APIKeyRepo tests -------------------------------------------------------

func TestAPIKeyRepo_CreateAndGetByHash(t *testing.T) {
	for _, b := range allBackends(t) {
		t.Run(b.name, func(t *testing.T) {
			repo := NewAPIKeyRepo(b.db)
			ctx := context.Background()

			k, err := repo.Create(ctx, "my-key", "hashvalue123", "admin@example.com", false)
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
		})
	}
}

func TestAPIKeyRepo_GetByHash_NotFound(t *testing.T) {
	for _, b := range allBackends(t) {
		t.Run(b.name, func(t *testing.T) {
			repo := NewAPIKeyRepo(b.db)
			_, err := repo.GetByHash(context.Background(), "nonexistent")
			if !errors.Is(err, ErrNotFound) {
				t.Errorf("expected ErrNotFound, got %v", err)
			}
		})
	}
}

func TestAPIKeyRepo_List(t *testing.T) {
	for _, b := range allBackends(t) {
		t.Run(b.name, func(t *testing.T) {
			repo := NewAPIKeyRepo(b.db)
			ctx := context.Background()

			for i, name := range []string{"key-a", "key-b", "key-c"} {
				hash := strings.Repeat("x", i+1) // distinct hashes
				if _, err := repo.Create(ctx, name, hash, "admin@example.com", false); err != nil {
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
		})
	}
}

func TestAPIKeyRepo_Delete(t *testing.T) {
	for _, b := range allBackends(t) {
		t.Run(b.name, func(t *testing.T) {
			repo := NewAPIKeyRepo(b.db)
			ctx := context.Background()

			k, err := repo.Create(ctx, "to-delete", "hash-del", "admin@example.com", false)
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
		})
	}
}

func TestAPIKeyRepo_Delete_NotFound(t *testing.T) {
	for _, b := range allBackends(t) {
		t.Run(b.name, func(t *testing.T) {
			repo := NewAPIKeyRepo(b.db)
			err := repo.Delete(context.Background(), 99999)
			if !errors.Is(err, ErrNotFound) {
				t.Errorf("expected ErrNotFound, got %v", err)
			}
		})
	}
}

func TestAPIKeyRepo_UpdateLastUsed(t *testing.T) {
	for _, b := range allBackends(t) {
		t.Run(b.name, func(t *testing.T) {
			repo := NewAPIKeyRepo(b.db)
			ctx := context.Background()

			k, err := repo.Create(ctx, "ts-key", "hash-ts", "admin@example.com", false)
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
		})
	}
}
