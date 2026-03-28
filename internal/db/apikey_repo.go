package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// APIKeyRepo defines the storage operations for API keys.
// Keys are stored only as hashes; the plaintext is never persisted.
type APIKeyRepo interface {
	// Create stores a new API key record (hash only) and returns it.
	Create(ctx context.Context, name, keyHash, createdBy string) (*APIKey, error)
	// GetByHash looks up a key record by its hash. Returns ErrNotFound when
	// the hash is not present.
	GetByHash(ctx context.Context, hash string) (*APIKey, error)
	// List returns all API key records ordered newest first.
	List(ctx context.Context) ([]*APIKey, error)
	// Delete removes the API key with the given ID.
	Delete(ctx context.Context, id int64) error
	// UpdateLastUsed refreshes the last_used_at timestamp for the key.
	UpdateLastUsed(ctx context.Context, id int64) error
}

// SQLAPIKeyRepo is a database/sql-backed implementation of APIKeyRepo.
type SQLAPIKeyRepo struct {
	db *DB
}

// NewAPIKeyRepo creates a new SQLAPIKeyRepo backed by db.
func NewAPIKeyRepo(db *DB) *SQLAPIKeyRepo {
	return &SQLAPIKeyRepo{db: db}
}

// Create inserts a new API key record.
func (r *SQLAPIKeyRepo) Create(ctx context.Context, name, keyHash, createdBy string) (*APIKey, error) {
	row := r.db.QueryRowContext(ctx, r.db.q(`
		INSERT INTO api_keys (name, key_hash, created_by)
		VALUES (?, ?, ?)
		RETURNING id, name, key_hash, created_by, created_at, last_used_at`),
		name, keyHash, createdBy,
	)
	return scanAPIKey(row)
}

// GetByHash retrieves an API key by its hash value.
func (r *SQLAPIKeyRepo) GetByHash(ctx context.Context, hash string) (*APIKey, error) {
	row := r.db.QueryRowContext(ctx, r.db.q(`
		SELECT id, name, key_hash, created_by, created_at, last_used_at
		FROM api_keys WHERE key_hash = ? LIMIT 1`),
		hash,
	)
	k, err := scanAPIKey(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return k, err
}

// List returns all API key records ordered by creation time descending.
func (r *SQLAPIKeyRepo) List(ctx context.Context) ([]*APIKey, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, name, key_hash, created_by, created_at, last_used_at
		FROM api_keys ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list api keys: %w", err)
	}
	defer rows.Close()

	var keys []*APIKey
	for rows.Next() {
		var k APIKey
		if err := rows.Scan(&k.ID, &k.Name, &k.KeyHash, &k.CreatedBy, &k.CreatedAt, &k.LastUsedAt); err != nil {
			return nil, fmt.Errorf("scan api key row: %w", err)
		}
		keys = append(keys, &k)
	}
	return keys, rows.Err()
}

// Delete removes the API key with the given ID.
func (r *SQLAPIKeyRepo) Delete(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, r.db.q("DELETE FROM api_keys WHERE id = ?"), id)
	if err != nil {
		return fmt.Errorf("delete api key %d: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete api key %d rows affected: %w", id, err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateLastUsed sets last_used_at to the current timestamp for the given key.
func (r *SQLAPIKeyRepo) UpdateLastUsed(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx,
		r.db.q("UPDATE api_keys SET last_used_at = CURRENT_TIMESTAMP WHERE id = ?"), id,
	)
	if err != nil {
		return fmt.Errorf("update last used for api key %d: %w", id, err)
	}
	return nil
}

// scanAPIKey reads a single APIKey from a *sql.Row.
func scanAPIKey(row *sql.Row) (*APIKey, error) {
	var k APIKey
	if err := row.Scan(&k.ID, &k.Name, &k.KeyHash, &k.CreatedBy, &k.CreatedAt, &k.LastUsedAt); err != nil {
		return nil, fmt.Errorf("scan api key: %w", err)
	}
	return &k, nil
}
