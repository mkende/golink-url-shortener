package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// LinkRepo defines the storage operations for short links.
type LinkRepo interface {
	// Create inserts a new link and returns the persisted record.
	Create(ctx context.Context, name, target, ownerEmail string, isAdvanced, requireAuth bool) (*Link, error)
	// GetByName retrieves a link by its lower-cased name. Returns
	// ErrNotFound when no matching link exists.
	GetByName(ctx context.Context, nameLower string) (*Link, error)
	// Update replaces the mutable fields of an existing link.
	Update(ctx context.Context, id int64, name, target string, isAdvanced, requireAuth bool) (*Link, error)
	// Delete removes a link by ID.
	Delete(ctx context.Context, id int64) error
	// List returns a paginated, sorted slice of all links plus the total count.
	List(ctx context.Context, limit, offset int, sortField SortField, sortDir SortDir) ([]*Link, int, error)
	// ListByOwner returns a paginated slice of links owned by ownerEmail plus
	// the total count for that owner.
	ListByOwner(ctx context.Context, ownerEmail string, limit, offset int) ([]*Link, int, error)
	// Search returns links whose lower-cased name contains query, paginated.
	Search(ctx context.Context, query string, limit, offset int) ([]*Link, int, error)
	// GetShares returns the emails/group names a link is shared with.
	GetShares(ctx context.Context, linkID int64) ([]string, error)
	// AddShare grants access to email for the given link.
	AddShare(ctx context.Context, linkID int64, email string) error
	// RemoveShare revokes access to email for the given link.
	RemoveShare(ctx context.Context, linkID int64, email string) error
	// IncrementUseCount bumps the link's use counter and last-used timestamp.
	// This is the synchronous version; async batching is added in phase 9.
	IncrementUseCount(ctx context.Context, id int64) error
}

// ErrNotFound is returned when a requested record does not exist.
var ErrNotFound = errors.New("not found")

// SQLLinkRepo is a database/sql-backed implementation of LinkRepo.
type SQLLinkRepo struct {
	db *sql.DB
}

// NewLinkRepo creates a new SQLLinkRepo backed by db.
func NewLinkRepo(db *sql.DB) *SQLLinkRepo {
	return &SQLLinkRepo{db: db}
}

// Create inserts a new link record and returns it.
func (r *SQLLinkRepo) Create(ctx context.Context, name, target, ownerEmail string, isAdvanced, requireAuth bool) (*Link, error) {
	nameLower := strings.ToLower(name)
	row := r.db.QueryRowContext(ctx, `
		INSERT INTO links (name, name_lower, target, owner_email, is_advanced, require_auth)
		VALUES (?, ?, ?, ?, ?, ?)
		RETURNING id, name, name_lower, target, owner_email, is_advanced, require_auth,
		          created_at, last_used_at, use_count`,
		name, nameLower, target, ownerEmail, isAdvanced, requireAuth,
	)
	return scanLink(row)
}

// GetByName retrieves a link by lower-cased name.
func (r *SQLLinkRepo) GetByName(ctx context.Context, nameLower string) (*Link, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, name, name_lower, target, owner_email, is_advanced, require_auth,
		       created_at, last_used_at, use_count
		FROM links WHERE name_lower = ? LIMIT 1`,
		nameLower,
	)
	link, err := scanLink(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return link, err
}

// Update modifies the mutable fields of an existing link and returns the
// updated record.
func (r *SQLLinkRepo) Update(ctx context.Context, id int64, name, target string, isAdvanced, requireAuth bool) (*Link, error) {
	nameLower := strings.ToLower(name)
	row := r.db.QueryRowContext(ctx, `
		UPDATE links
		SET name = ?, name_lower = ?, target = ?, is_advanced = ?, require_auth = ?
		WHERE id = ?
		RETURNING id, name, name_lower, target, owner_email, is_advanced, require_auth,
		          created_at, last_used_at, use_count`,
		name, nameLower, target, isAdvanced, requireAuth, id,
	)
	link, err := scanLink(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return link, err
}

// Delete removes the link with the given ID.
func (r *SQLLinkRepo) Delete(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, "DELETE FROM links WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete link %d: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete link %d rows affected: %w", id, err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// List returns all links, paginated and sorted.  sort field and direction are
// validated against an allow-list to prevent SQL injection.
func (r *SQLLinkRepo) List(ctx context.Context, limit, offset int, sortField SortField, sortDir SortDir) ([]*Link, int, error) {
	if !validSortFields[sortField] {
		return nil, 0, fmt.Errorf("invalid sort field: %q", sortField)
	}
	if !validSortDirs[sortDir] {
		return nil, 0, fmt.Errorf("invalid sort direction: %q", sortDir)
	}

	var total int
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM links").Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count links: %w", err)
	}

	// Safe to interpolate: values were validated against an allow-list above.
	query := fmt.Sprintf(`
		SELECT id, name, name_lower, target, owner_email, is_advanced, require_auth,
		       created_at, last_used_at, use_count
		FROM links ORDER BY %s %s LIMIT ? OFFSET ?`, sortField, sortDir)

	rows, err := r.db.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list links: %w", err)
	}
	defer rows.Close()

	links, err := scanLinks(rows)
	if err != nil {
		return nil, 0, err
	}
	return links, total, nil
}

// ListByOwner returns a paginated slice of links for ownerEmail.
func (r *SQLLinkRepo) ListByOwner(ctx context.Context, ownerEmail string, limit, offset int) ([]*Link, int, error) {
	var total int
	if err := r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM links WHERE owner_email = ?", ownerEmail,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count links by owner: %w", err)
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT id, name, name_lower, target, owner_email, is_advanced, require_auth,
		       created_at, last_used_at, use_count
		FROM links WHERE owner_email = ?
		ORDER BY name_lower ASC LIMIT ? OFFSET ?`,
		ownerEmail, limit, offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("list links by owner: %w", err)
	}
	defer rows.Close()

	links, err := scanLinks(rows)
	if err != nil {
		return nil, 0, err
	}
	return links, total, nil
}

// Search returns links whose lower-cased name contains query.
func (r *SQLLinkRepo) Search(ctx context.Context, query string, limit, offset int) ([]*Link, int, error) {
	pattern := "%" + strings.ToLower(query) + "%"

	var total int
	if err := r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM links WHERE name_lower LIKE ?", pattern,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count search links: %w", err)
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT id, name, name_lower, target, owner_email, is_advanced, require_auth,
		       created_at, last_used_at, use_count
		FROM links WHERE name_lower LIKE ?
		ORDER BY name_lower ASC LIMIT ? OFFSET ?`,
		pattern, limit, offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("search links: %w", err)
	}
	defer rows.Close()

	links, err := scanLinks(rows)
	if err != nil {
		return nil, 0, err
	}
	return links, total, nil
}

// GetShares returns all emails/groups the link is shared with.
func (r *SQLLinkRepo) GetShares(ctx context.Context, linkID int64) ([]string, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT shared_with_email FROM link_shares WHERE link_id = ?", linkID,
	)
	if err != nil {
		return nil, fmt.Errorf("get shares for link %d: %w", linkID, err)
	}
	defer rows.Close()

	var emails []string
	for rows.Next() {
		var email string
		if err := rows.Scan(&email); err != nil {
			return nil, fmt.Errorf("scan share email: %w", err)
		}
		emails = append(emails, email)
	}
	return emails, rows.Err()
}

// AddShare grants access for email on the link.  Duplicate entries are ignored.
func (r *SQLLinkRepo) AddShare(ctx context.Context, linkID int64, email string) error {
	_, err := r.db.ExecContext(ctx,
		"INSERT OR IGNORE INTO link_shares (link_id, shared_with_email) VALUES (?, ?)",
		linkID, email,
	)
	if err != nil {
		return fmt.Errorf("add share: %w", err)
	}
	return nil
}

// RemoveShare revokes access for email on the link.
func (r *SQLLinkRepo) RemoveShare(ctx context.Context, linkID int64, email string) error {
	_, err := r.db.ExecContext(ctx,
		"DELETE FROM link_shares WHERE link_id = ? AND shared_with_email = ?",
		linkID, email,
	)
	if err != nil {
		return fmt.Errorf("remove share: %w", err)
	}
	return nil
}

// IncrementUseCount bumps the hit counter and last-used timestamp for the link.
func (r *SQLLinkRepo) IncrementUseCount(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx,
		"UPDATE links SET use_count = use_count + 1, last_used_at = CURRENT_TIMESTAMP WHERE id = ?",
		id,
	)
	if err != nil {
		return fmt.Errorf("increment use count for link %d: %w", id, err)
	}
	return nil
}

// scanLink reads a single Link from a *sql.Row.
func scanLink(row *sql.Row) (*Link, error) {
	var l Link
	err := row.Scan(
		&l.ID, &l.Name, &l.NameLower, &l.Target, &l.OwnerEmail,
		&l.IsAdvanced, &l.RequireAuth, &l.CreatedAt, &l.LastUsedAt, &l.UseCount,
	)
	if err != nil {
		return nil, fmt.Errorf("scan link: %w", err)
	}
	return &l, nil
}

// scanLinks reads all Links from *sql.Rows.
func scanLinks(rows *sql.Rows) ([]*Link, error) {
	var links []*Link
	for rows.Next() {
		var l Link
		if err := rows.Scan(
			&l.ID, &l.Name, &l.NameLower, &l.Target, &l.OwnerEmail,
			&l.IsAdvanced, &l.RequireAuth, &l.CreatedAt, &l.LastUsedAt, &l.UseCount,
		); err != nil {
			return nil, fmt.Errorf("scan link row: %w", err)
		}
		links = append(links, &l)
	}
	return links, rows.Err()
}
