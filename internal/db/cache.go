package db

import (
	"context"

	lru "github.com/hashicorp/golang-lru/v2"
)

// CachingLinkRepo wraps a LinkRepo with an LRU cache for GetByName lookups,
// dramatically reducing database reads on the hot redirect path.
type CachingLinkRepo struct {
	inner LinkRepo
	cache *lru.Cache[string, *Link] // key: name_lower
}

// NewCachingLinkRepo creates a CachingLinkRepo with the given LRU cache size.
// size must be > 0.
func NewCachingLinkRepo(inner LinkRepo, size int) (*CachingLinkRepo, error) {
	c, err := lru.New[string, *Link](size)
	if err != nil {
		return nil, err
	}
	return &CachingLinkRepo{inner: inner, cache: c}, nil
}

// GetByName checks the LRU cache before hitting the database. On a cache miss
// the result is stored in the cache for subsequent lookups.
func (r *CachingLinkRepo) GetByName(ctx context.Context, nameLower string) (*Link, error) {
	if v, ok := r.cache.Get(nameLower); ok {
		return v, nil
	}
	link, err := r.inner.GetByName(ctx, nameLower)
	if err != nil {
		return nil, err
	}
	r.cache.Add(nameLower, link)
	return link, nil
}

// Create delegates to the inner repo and adds the resulting link to the cache.
func (r *CachingLinkRepo) Create(ctx context.Context, name, target, ownerEmail string, linkType LinkType, aliasTarget string, requireAuth bool) (*Link, error) {
	link, err := r.inner.Create(ctx, name, target, ownerEmail, linkType, aliasTarget, requireAuth)
	if err == nil {
		r.cache.Add(link.NameLower, link)
	}
	return link, err
}

// Update delegates to the inner repo and removes the stale cache entry so the
// next GetByName call reads fresh data from the database.
func (r *CachingLinkRepo) Update(ctx context.Context, id int64, name, target string, linkType LinkType, requireAuth bool) (*Link, error) {
	link, err := r.inner.Update(ctx, id, name, target, linkType, requireAuth)
	if err == nil {
		r.cache.Remove(link.NameLower)
	}
	return link, err
}

// SetAlias delegates to the inner repo and purges the entire cache because
// multiple alias and canonical link entries may become stale simultaneously.
func (r *CachingLinkRepo) SetAlias(ctx context.Context, id int64, name, aliasTargetLower string, requireAuth bool, maxAliases int) (*Link, error) {
	link, err := r.inner.SetAlias(ctx, id, name, aliasTargetLower, requireAuth, maxAliases)
	if err == nil {
		r.cache.Purge()
	}
	return link, err
}

// Delete purges the entire cache because we would need an extra DB query to
// find the name_lower for the deleted ID, and deletes are rare enough that a
// full purge is acceptable.
func (r *CachingLinkRepo) Delete(ctx context.Context, id int64) error {
	err := r.inner.Delete(ctx, id)
	if err == nil {
		r.cache.Purge()
	}
	return err
}

// List delegates to the inner repo without caching (results are paginated and
// not part of the hot redirect path).
func (r *CachingLinkRepo) List(ctx context.Context, limit, offset int, sortField SortField, sortDir SortDir) ([]*Link, int, error) {
	return r.inner.List(ctx, limit, offset, sortField, sortDir)
}

// ListByOwner delegates to the inner repo without caching.
func (r *CachingLinkRepo) ListByOwner(ctx context.Context, ownerEmail string, limit, offset int, sortField SortField, sortDir SortDir) ([]*Link, int, error) {
	return r.inner.ListByOwner(ctx, ownerEmail, limit, offset, sortField, sortDir)
}

// Search delegates to the inner repo without caching.
func (r *CachingLinkRepo) Search(ctx context.Context, query string, limit, offset int) ([]*Link, int, error) {
	return r.inner.Search(ctx, query, limit, offset)
}

// GetShares delegates to the inner repo without caching.
func (r *CachingLinkRepo) GetShares(ctx context.Context, linkID int64) ([]string, error) {
	return r.inner.GetShares(ctx, linkID)
}

// AddShare delegates to the inner repo without caching.
func (r *CachingLinkRepo) AddShare(ctx context.Context, linkID int64, email string) error {
	return r.inner.AddShare(ctx, linkID, email)
}

// RemoveShare delegates to the inner repo without caching.
func (r *CachingLinkRepo) RemoveShare(ctx context.Context, linkID int64, email string) error {
	return r.inner.RemoveShare(ctx, linkID, email)
}

// IncrementUseCount delegates to the inner repo. In practice the server uses
// UseCounter for batched async updates; this path exists to satisfy the
// LinkRepo interface.
func (r *CachingLinkRepo) IncrementUseCount(ctx context.Context, id int64) error {
	return r.inner.IncrementUseCount(ctx, id)
}

// GetAliases delegates to the inner repo without caching.
func (r *CachingLinkRepo) GetAliases(ctx context.Context, nameLower string) ([]*Link, error) {
	return r.inner.GetAliases(ctx, nameLower)
}

// CountAliases delegates to the inner repo without caching.
func (r *CachingLinkRepo) CountAliases(ctx context.Context, nameLower string) (int, error) {
	return r.inner.CountAliases(ctx, nameLower)
}

// SharedLinkIDs delegates to the inner repo without caching.
func (r *CachingLinkRepo) SharedLinkIDs(ctx context.Context, email string) (map[int64]bool, error) {
	return r.inner.SharedLinkIDs(ctx, email)
}
