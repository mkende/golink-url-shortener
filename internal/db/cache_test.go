package db

import (
	"context"
	"sync/atomic"
	"testing"
)

// countingLinkRepo wraps a LinkRepo and counts GetByName calls.
type countingLinkRepo struct {
	inner      LinkRepo
	getByNameN atomic.Int64
}

func (r *countingLinkRepo) GetByName(ctx context.Context, nameLower string) (*Link, error) {
	r.getByNameN.Add(1)
	return r.inner.GetByName(ctx, nameLower)
}

func (r *countingLinkRepo) Create(ctx context.Context, name, target, ownerEmail string, linkType LinkType, aliasTarget string, requireAuth bool) (*Link, error) {
	return r.inner.Create(ctx, name, target, ownerEmail, linkType, aliasTarget, requireAuth)
}

func (r *countingLinkRepo) Update(ctx context.Context, id int64, name, target string, linkType LinkType, requireAuth bool) (*Link, error) {
	return r.inner.Update(ctx, id, name, target, linkType, requireAuth)
}

func (r *countingLinkRepo) SetAlias(ctx context.Context, id int64, name, aliasTargetLower string, requireAuth bool, maxAliases int) (*Link, error) {
	return r.inner.SetAlias(ctx, id, name, aliasTargetLower, requireAuth, maxAliases)
}

func (r *countingLinkRepo) Delete(ctx context.Context, id int64) error {
	return r.inner.Delete(ctx, id)
}

func (r *countingLinkRepo) List(ctx context.Context, limit, offset int, sortField SortField, sortDir SortDir, publicOnly bool) ([]*Link, int, error) {
	return r.inner.List(ctx, limit, offset, sortField, sortDir, publicOnly)
}

func (r *countingLinkRepo) ListByOwner(ctx context.Context, ownerEmail string, limit, offset int, sortField SortField, sortDir SortDir) ([]*Link, int, error) {
	return r.inner.ListByOwner(ctx, ownerEmail, limit, offset, sortField, sortDir)
}

func (r *countingLinkRepo) Search(ctx context.Context, query string, limit, offset int, publicOnly bool) ([]*Link, int, error) {
	return r.inner.Search(ctx, query, limit, offset, publicOnly)
}

func (r *countingLinkRepo) GetShares(ctx context.Context, linkID int64) ([]string, error) {
	return r.inner.GetShares(ctx, linkID)
}

func (r *countingLinkRepo) AddShare(ctx context.Context, linkID int64, email string) error {
	return r.inner.AddShare(ctx, linkID, email)
}

func (r *countingLinkRepo) RemoveShare(ctx context.Context, linkID int64, email string) error {
	return r.inner.RemoveShare(ctx, linkID, email)
}

func (r *countingLinkRepo) IncrementUseCount(ctx context.Context, id int64) error {
	return r.inner.IncrementUseCount(ctx, id)
}

func (r *countingLinkRepo) GetAliases(ctx context.Context, nameLower string) ([]*Link, error) {
	return r.inner.GetAliases(ctx, nameLower)
}

func (r *countingLinkRepo) CountAliases(ctx context.Context, nameLower string) (int, error) {
	return r.inner.CountAliases(ctx, nameLower)
}

func (r *countingLinkRepo) SharedLinkIDs(ctx context.Context, identifiers []string) (map[int64]bool, error) {
	return r.inner.SharedLinkIDs(ctx, identifiers)
}

func (r *countingLinkRepo) ListOwnedOrSharedWith(ctx context.Context, ownerEmail string, identifiers []string, limit, offset int, sortField SortField, sortDir SortDir) ([]*Link, int, error) {
	return r.inner.ListOwnedOrSharedWith(ctx, ownerEmail, identifiers, limit, offset, sortField, sortDir)
}

func (r *countingLinkRepo) SearchOwnedOrSharedWith(ctx context.Context, ownerEmail string, identifiers []string, query string, limit, offset int) ([]*Link, int, error) {
	return r.inner.SearchOwnedOrSharedWith(ctx, ownerEmail, identifiers, query, limit, offset)
}

// TestCachingLinkRepo_HitAndMiss verifies that a second GetByName call for the
// same name does not reach the underlying repository.
func TestCachingLinkRepo_HitAndMiss(t *testing.T) {
	for _, b := range allBackends(t) {
		t.Run(b.name, func(t *testing.T) {
			base := NewLinkRepo(b.db)
			ctx := context.Background()

			_, err := base.Create(ctx, "cached", "https://example.com", "o@example.com", LinkTypeSimple, "", false)
			if err != nil {
				t.Fatalf("Create: %v", err)
			}

			counter := &countingLinkRepo{inner: base}
			cache, err := NewCachingLinkRepo(counter, 100)
			if err != nil {
				t.Fatalf("NewCachingLinkRepo: %v", err)
			}

			// First call — cache miss, should hit inner repo.
			l1, err := cache.GetByName(ctx, "cached")
			if err != nil {
				t.Fatalf("GetByName (miss): %v", err)
			}
			if counter.getByNameN.Load() != 1 {
				t.Errorf("inner GetByName calls after miss = %d, want 1", counter.getByNameN.Load())
			}

			// Second call — cache hit, inner repo must NOT be called again.
			l2, err := cache.GetByName(ctx, "cached")
			if err != nil {
				t.Fatalf("GetByName (hit): %v", err)
			}
			if counter.getByNameN.Load() != 1 {
				t.Errorf("inner GetByName calls after hit = %d, want 1 (no extra call)", counter.getByNameN.Load())
			}
			if l1.ID != l2.ID {
				t.Errorf("cached link ID mismatch: %d vs %d", l1.ID, l2.ID)
			}
		})
	}
}

// TestCachingLinkRepo_InvalidateOnUpdate verifies that after Update the next
// GetByName call reads fresh data from the inner repository.
func TestCachingLinkRepo_InvalidateOnUpdate(t *testing.T) {
	for _, b := range allBackends(t) {
		t.Run(b.name, func(t *testing.T) {
			base := NewLinkRepo(b.db)
			ctx := context.Background()

			orig, err := base.Create(ctx, "updateme", "https://old.example.com", "o@example.com", LinkTypeSimple, "", false)
			if err != nil {
				t.Fatalf("Create: %v", err)
			}

			counter := &countingLinkRepo{inner: base}
			cache, err := NewCachingLinkRepo(counter, 100)
			if err != nil {
				t.Fatalf("NewCachingLinkRepo: %v", err)
			}

			// Populate cache.
			if _, err := cache.GetByName(ctx, "updateme"); err != nil {
				t.Fatalf("GetByName before update: %v", err)
			}
			callsAfterFirst := counter.getByNameN.Load()

			// Update the link — this must invalidate the cache entry.
			if _, err := cache.Update(ctx, orig.ID, "updateme", "https://new.example.com", LinkTypeSimple, false); err != nil {
				t.Fatalf("Update: %v", err)
			}

			// Next GetByName must go to the inner repo.
			updated, err := cache.GetByName(ctx, "updateme")
			if err != nil {
				t.Fatalf("GetByName after update: %v", err)
			}
			if counter.getByNameN.Load() != callsAfterFirst+1 {
				t.Errorf("inner GetByName calls after invalidation = %d, want %d", counter.getByNameN.Load(), callsAfterFirst+1)
			}
			if updated.Target != "https://new.example.com" {
				t.Errorf("Target = %q, want https://new.example.com", updated.Target)
			}
		})
	}
}

// TestCachingLinkRepo_InvalidateOnDelete verifies that after Delete the cache
// no longer returns the stale entry.
func TestCachingLinkRepo_InvalidateOnDelete(t *testing.T) {
	for _, b := range allBackends(t) {
		t.Run(b.name, func(t *testing.T) {
			base := NewLinkRepo(b.db)
			ctx := context.Background()

			link, err := base.Create(ctx, "deleteme", "https://example.com", "o@example.com", LinkTypeSimple, "", false)
			if err != nil {
				t.Fatalf("Create: %v", err)
			}

			counter := &countingLinkRepo{inner: base}
			cache, err := NewCachingLinkRepo(counter, 100)
			if err != nil {
				t.Fatalf("NewCachingLinkRepo: %v", err)
			}

			// Warm the cache.
			if _, err := cache.GetByName(ctx, "deleteme"); err != nil {
				t.Fatalf("GetByName before delete: %v", err)
			}

			// Delete purges the cache.
			if err := cache.Delete(ctx, link.ID); err != nil {
				t.Fatalf("Delete: %v", err)
			}

			// After delete the link should not be found.
			_, err = cache.GetByName(ctx, "deleteme")
			if err == nil {
				t.Error("expected ErrNotFound after delete, got nil")
			}
		})
	}
}

// TestCachingLinkRepo_CreatePopulatesCache verifies that Create adds the new
// link to the cache so a subsequent GetByName does not hit the inner repo.
func TestCachingLinkRepo_CreatePopulatesCache(t *testing.T) {
	for _, b := range allBackends(t) {
		t.Run(b.name, func(t *testing.T) {
			base := NewLinkRepo(b.db)
			ctx := context.Background()

			counter := &countingLinkRepo{inner: base}
			cache, err := NewCachingLinkRepo(counter, 100)
			if err != nil {
				t.Fatalf("NewCachingLinkRepo: %v", err)
			}

			if _, err := cache.Create(ctx, "brand-new", "https://example.com", "o@example.com", LinkTypeSimple, "", false); err != nil {
				t.Fatalf("Create: %v", err)
			}
			callsAfterCreate := counter.getByNameN.Load()

			// GetByName should be served from cache — inner repo must not be called.
			if _, err := cache.GetByName(ctx, "brand-new"); err != nil {
				t.Fatalf("GetByName after create: %v", err)
			}
			if counter.getByNameN.Load() != callsAfterCreate {
				t.Errorf("inner GetByName calls after cached create = %d, want %d (no extra call)", counter.getByNameN.Load(), callsAfterCreate)
			}
		})
	}
}
