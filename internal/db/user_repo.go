package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// UserRepo defines the storage operations for user records.
type UserRepo interface {
	// Upsert creates or updates the user identified by email.
	Upsert(ctx context.Context, email, displayName, avatarURL string) (*User, error)
	// Get returns the user for email. Returns ErrNotFound if absent.
	Get(ctx context.Context, email string) (*User, error)
	// List returns a paginated slice of all users ordered by email.
	List(ctx context.Context, limit, offset int) ([]*User, error)
	// Search returns users whose email or display name contains query.
	Search(ctx context.Context, query string, limit int) ([]*User, error)
}

// SQLUserRepo is a database/sql-backed implementation of UserRepo.
type SQLUserRepo struct {
	db *DB
}

// NewUserRepo creates a new SQLUserRepo backed by db.
func NewUserRepo(db *DB) *SQLUserRepo {
	return &SQLUserRepo{db: db}
}

// Upsert inserts or updates a user record, refreshing last_seen_at.
func (r *SQLUserRepo) Upsert(ctx context.Context, email, displayName, avatarURL string) (*User, error) {
	row := r.db.QueryRowContext(ctx, r.db.q(`
		INSERT INTO users (email, display_name, avatar_url, last_seen_at)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(email) DO UPDATE SET
			display_name = excluded.display_name,
			avatar_url   = excluded.avatar_url,
			last_seen_at = CURRENT_TIMESTAMP
		RETURNING email, display_name, avatar_url, last_seen_at`),
		email, displayName, avatarURL,
	)
	return scanUser(row)
}

// Get returns the user for the given email address.
func (r *SQLUserRepo) Get(ctx context.Context, email string) (*User, error) {
	row := r.db.QueryRowContext(ctx, r.db.q(`
		SELECT email, display_name, avatar_url, last_seen_at
		FROM users WHERE email = ? LIMIT 1`),
		email,
	)
	u, err := scanUser(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return u, err
}

// List returns a paginated list of users sorted by email.
func (r *SQLUserRepo) List(ctx context.Context, limit, offset int) ([]*User, error) {
	rows, err := r.db.QueryContext(ctx, r.db.q(`
		SELECT email, display_name, avatar_url, last_seen_at
		FROM users ORDER BY email ASC LIMIT ? OFFSET ?`),
		limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()
	return scanUsers(rows)
}

// Search returns users whose email or display name contain query (case-insensitive).
func (r *SQLUserRepo) Search(ctx context.Context, query string, limit int) ([]*User, error) {
	pattern := "%" + query + "%"
	rows, err := r.db.QueryContext(ctx, r.db.q(`
		SELECT email, display_name, avatar_url, last_seen_at
		FROM users
		WHERE email LIKE ? OR display_name LIKE ?
		ORDER BY email ASC LIMIT ?`),
		pattern, pattern, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("search users: %w", err)
	}
	defer rows.Close()
	return scanUsers(rows)
}

// scanUser reads a single User from a *sql.Row.
func scanUser(row *sql.Row) (*User, error) {
	var u User
	if err := row.Scan(&u.Email, &u.DisplayName, &u.AvatarURL, &u.LastSeenAt); err != nil {
		return nil, fmt.Errorf("scan user: %w", err)
	}
	return &u, nil
}

// scanUsers reads all User rows from *sql.Rows.
func scanUsers(rows *sql.Rows) ([]*User, error) {
	var users []*User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.Email, &u.DisplayName, &u.AvatarURL, &u.LastSeenAt); err != nil {
			return nil, fmt.Errorf("scan user row: %w", err)
		}
		users = append(users, &u)
	}
	return users, rows.Err()
}
