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
	// For alias links set linkType to LinkTypeAlias, aliasTarget to the
	// lower-cased canonical link name, and target to "".
	// For simple/advanced links set aliasTarget to "".
	Create(ctx context.Context, name, target, ownerEmail string, linkType LinkType, aliasTarget string, requireAuth bool) (*Link, error)
	// GetByName retrieves a link by its lower-cased name. Returns
	// ErrNotFound when no matching link exists.
	GetByName(ctx context.Context, nameLower string) (*Link, error)
	// Update replaces the mutable fields of a simple or advanced link.
	// To convert a link to alias type use SetAlias instead.
	Update(ctx context.Context, id int64, name, target string, linkType LinkType, requireAuth bool) (*Link, error)
	// SetAlias atomically converts the link to LinkTypeAlias pointing at
	// aliasTargetLower, and reparents any existing aliases of this link to
	// aliasTargetLower.  Returns ErrAliasLimitExceeded when the total number
	// of aliases that aliasTargetLower would have after the operation exceeds
	// maxAliases.
	SetAlias(ctx context.Context, id int64, name, aliasTargetLower string, requireAuth bool, maxAliases int) (*Link, error)
	// Delete removes a link by ID.
	Delete(ctx context.Context, id int64) error
	// List returns a paginated, sorted slice of all links plus the total count.
	// When publicOnly is true, links with require_auth set are excluded.
	List(ctx context.Context, limit, offset int, sortField SortField, sortDir SortDir, publicOnly bool) ([]*Link, int, error)
	// ListByOwner returns a paginated, sorted slice of links owned by ownerEmail
	// plus the total count for that owner.
	ListByOwner(ctx context.Context, ownerEmail string, limit, offset int, sortField SortField, sortDir SortDir) ([]*Link, int, error)
	// Search returns links whose lower-cased name contains query, paginated.
	// When publicOnly is true, links with require_auth set are excluded.
	Search(ctx context.Context, query string, limit, offset int, publicOnly bool) ([]*Link, int, error)
	// SearchOwnedOrSharedWith is like Search but restricted to links that are
	// owned by ownerEmail or shared with any of the given identifiers.
	SearchOwnedOrSharedWith(ctx context.Context, ownerEmail string, identifiers []string, query string, limit, offset int) ([]*Link, int, error)
	// GetShares returns the emails/group names a link is shared with.
	GetShares(ctx context.Context, linkID int64) ([]string, error)
	// AddShare grants access to email for the given link.
	AddShare(ctx context.Context, linkID int64, email string) error
	// RemoveShare revokes access to email for the given link.
	RemoveShare(ctx context.Context, linkID int64, email string) error
	// SharedLinkIDs returns the set of link IDs shared with any of the given
	// identifiers (user email plus group names). An empty slice returns an empty map.
	SharedLinkIDs(ctx context.Context, identifiers []string) (map[int64]bool, error)
	// ListOwnedOrSharedWith returns a paginated, sorted slice of links that are
	// either owned by ownerEmail or shared with any of the given identifiers
	// (excluding links already owned by ownerEmail from the shared set), plus
	// the total count.
	ListOwnedOrSharedWith(ctx context.Context, ownerEmail string, identifiers []string, limit, offset int, sortField SortField, sortDir SortDir) ([]*Link, int, error)
	// IncrementUseCount bumps the link's use counter and last-used timestamp.
	IncrementUseCount(ctx context.Context, id int64) error
	// GetAliases returns all links that alias the given canonical link name.
	GetAliases(ctx context.Context, nameLower string) ([]*Link, error)
	// CountAliases returns the number of alias links targeting nameLower.
	CountAliases(ctx context.Context, nameLower string) (int, error)
	// ReassignLinks changes the owner_email of all links owned by fromEmail to
	// toEmail. It returns the number of links updated.
	ReassignLinks(ctx context.Context, fromEmail, toEmail string) (int64, error)
	// TransferOwnership changes the owner_email of a single link to newOwnerEmail
	// and returns the updated record.
	TransferOwnership(ctx context.Context, id int64, newOwnerEmail string) (*Link, error)
}

// ErrNotFound is returned when a requested record does not exist.
var ErrNotFound = errors.New("not found")

// ErrAliasLimitExceeded is returned when adding an alias would exceed the
// configured per-link alias limit.
var ErrAliasLimitExceeded = errors.New("alias limit exceeded")

// SQLLinkRepo is a database/sql-backed implementation of LinkRepo.
type SQLLinkRepo struct {
	db *DB
}

// NewLinkRepo creates a new SQLLinkRepo backed by db.
func NewLinkRepo(db *DB) *SQLLinkRepo {
	return &SQLLinkRepo{db: db}
}

// selectCols is the shared column list used in all link SELECT statements.
const selectCols = `id, name, name_lower, target, owner_email, link_type, alias_target, require_auth,
	               created_at, last_used_at, use_count`

// Create inserts a new link record and returns it.
func (r *SQLLinkRepo) Create(ctx context.Context, name, target, ownerEmail string, linkType LinkType, aliasTarget string, requireAuth bool) (*Link, error) {
	nameLower := strings.ToLower(name)
	row := r.db.QueryRowContext(ctx, r.db.q(`
		INSERT INTO links (name, name_lower, target, owner_email, link_type, alias_target, require_auth)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		RETURNING `+selectCols),
		name, nameLower, target, ownerEmail, linkType, aliasTarget, requireAuth,
	)
	return scanLink(row)
}

// GetByName retrieves a link by lower-cased name.
func (r *SQLLinkRepo) GetByName(ctx context.Context, nameLower string) (*Link, error) {
	row := r.db.QueryRowContext(ctx, r.db.q(`
		SELECT `+selectCols+`
		FROM links WHERE name_lower = ? LIMIT 1`),
		nameLower,
	)
	link, err := scanLink(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return link, err
}

// Update modifies the mutable fields of a simple or advanced link and returns
// the updated record.  To convert a link to alias type use SetAlias.
func (r *SQLLinkRepo) Update(ctx context.Context, id int64, name, target string, linkType LinkType, requireAuth bool) (*Link, error) {
	nameLower := strings.ToLower(name)
	row := r.db.QueryRowContext(ctx, r.db.q(`
		UPDATE links
		SET name = ?, name_lower = ?, target = ?, link_type = ?, alias_target = '', require_auth = ?
		WHERE id = ?
		RETURNING `+selectCols),
		name, nameLower, target, linkType, requireAuth, id,
	)
	link, err := scanLink(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return link, err
}

// SetAlias atomically converts the link to an alias of aliasTargetLower,
// reparenting any existing aliases of this link to aliasTargetLower.
func (r *SQLLinkRepo) SetAlias(ctx context.Context, id int64, name, aliasTargetLower string, requireAuth bool, maxAliases int) (*Link, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// Find the current name_lower of this link so we can locate its aliases.
	var currentNameLower string
	if err := tx.QueryRowContext(ctx, r.db.q("SELECT name_lower FROM links WHERE id = ?"), id).Scan(&currentNameLower); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get link name: %w", err)
	}

	// Count aliases that the target already has.
	var targetAliasCount int
	if err := tx.QueryRowContext(ctx,
		r.db.q("SELECT COUNT(*) FROM links WHERE alias_target = ?"), aliasTargetLower,
	).Scan(&targetAliasCount); err != nil {
		return nil, fmt.Errorf("count target aliases: %w", err)
	}

	// Count aliases that this link currently has (they will be reparented).
	var ownAliasCount int
	if err := tx.QueryRowContext(ctx,
		r.db.q("SELECT COUNT(*) FROM links WHERE alias_target = ?"), currentNameLower,
	).Scan(&ownAliasCount); err != nil {
		return nil, fmt.Errorf("count own aliases: %w", err)
	}

	// After the operation the target will gain: this link (1) + its reparented
	// aliases (ownAliasCount).
	if targetAliasCount+ownAliasCount+1 > maxAliases {
		return nil, ErrAliasLimitExceeded
	}

	// Reparent existing aliases of this link to the new target.
	if ownAliasCount > 0 {
		if _, err := tx.ExecContext(ctx,
			r.db.q("UPDATE links SET alias_target = ? WHERE alias_target = ?"),
			aliasTargetLower, currentNameLower,
		); err != nil {
			return nil, fmt.Errorf("reparent aliases: %w", err)
		}
	}

	// Convert this link to an alias.
	nameLower := strings.ToLower(name)
	row := tx.QueryRowContext(ctx, r.db.q(`
		UPDATE links
		SET name = ?, name_lower = ?, target = '', link_type = ?, alias_target = ?, require_auth = ?
		WHERE id = ?
		RETURNING `+selectCols),
		name, nameLower, LinkTypeAlias, aliasTargetLower, requireAuth, id,
	)
	link, err := scanLink(row)
	if err != nil {
		return nil, fmt.Errorf("convert link to alias: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}
	return link, nil
}

// Delete removes the link with the given ID.
func (r *SQLLinkRepo) Delete(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, r.db.q("DELETE FROM links WHERE id = ?"), id)
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
// validated against an allow-list to prevent SQL injection. When publicOnly is
// true, links with require_auth set are excluded.
func (r *SQLLinkRepo) List(ctx context.Context, limit, offset int, sortField SortField, sortDir SortDir, publicOnly bool) ([]*Link, int, error) {
	if err := validateSort(sortField, sortDir); err != nil {
		return nil, 0, err
	}

	authFilter := ""
	if publicOnly {
		authFilter = " WHERE require_auth = 0"
	}

	var total int
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM links"+authFilter).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count links: %w", err)
	}

	// Safe to interpolate: values were validated against an allow-list above.
	query := r.db.q(fmt.Sprintf(`
		SELECT `+selectCols+`
		FROM links`+authFilter+` ORDER BY %s %s LIMIT ? OFFSET ?`, sortField, sortDir))

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

// ListByOwner returns a paginated, sorted slice of links for ownerEmail.
func (r *SQLLinkRepo) ListByOwner(ctx context.Context, ownerEmail string, limit, offset int, sortField SortField, sortDir SortDir) ([]*Link, int, error) {
	if err := validateSort(sortField, sortDir); err != nil {
		return nil, 0, err
	}

	var total int
	if err := r.db.QueryRowContext(ctx,
		r.db.q("SELECT COUNT(*) FROM links WHERE owner_email = ?"), ownerEmail,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count links by owner: %w", err)
	}

	// Safe to interpolate: values were validated against an allow-list above.
	rows, err := r.db.QueryContext(ctx, r.db.q(fmt.Sprintf(`
		SELECT `+selectCols+`
		FROM links WHERE owner_email = ?
		ORDER BY %s %s LIMIT ? OFFSET ?`, sortField, sortDir)),
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

// buildSearchMatchExpr returns the SQL WHERE expression and the LIKE argument
// slice for the given search field. For searchAll the expression matches name,
// target, and alias_target and the args slice contains the pattern three times.
func buildSearchMatchExpr(field searchField, pattern string) (expr string, args []any) {
	switch field {
	case searchName:
		return "name_lower LIKE ?", []any{pattern}
	case searchTarget:
		return "LOWER(target) LIKE ?", []any{pattern}
	case searchAlias:
		return "alias_target LIKE ?", []any{pattern}
	default:
		return "(name_lower LIKE ? OR LOWER(target) LIKE ? OR alias_target LIKE ?)", []any{pattern, pattern, pattern}
	}
}

// validateSort returns an error when sortField or sortDir are not in the
// allow-list, preventing SQL injection via interpolated ORDER BY clauses.
func validateSort(sortField SortField, sortDir SortDir) error {
	if !validSortFields[sortField] {
		return fmt.Errorf("invalid sort field: %q", sortField)
	}
	if !validSortDirs[sortDir] {
		return fmt.Errorf("invalid sort direction: %q", sortDir)
	}
	return nil
}

// searchField indicates which column(s) a search query targets.
type searchField int

const (
	searchAll    searchField = iota // default: name, target, and alias target
	searchName                      // name: or n: prefix
	searchTarget                    // target: or t: prefix
	searchAlias                     // alias: or a: prefix — alias_target column only
)

// parseSearchQuery parses an optional field prefix (name:/n:/target:/t:/alias:/a:) and
// ^ / $ anchors from a raw search string, returning the field scope and the
// LIKE pattern to use.
func parseSearchQuery(query string) (searchField, string) {
	field := searchAll
	q := query
	switch {
	case strings.HasPrefix(q, "name:"):
		field, q = searchName, q[len("name:"):]
	case strings.HasPrefix(q, "n:"):
		field, q = searchName, q[len("n:"):]
	case strings.HasPrefix(q, "target:"):
		field, q = searchTarget, q[len("target:"):]
	case strings.HasPrefix(q, "t:"):
		field, q = searchTarget, q[len("t:"):]
	case strings.HasPrefix(q, "alias:"):
		field, q = searchAlias, q[len("alias:"):]
	case strings.HasPrefix(q, "a:"):
		field, q = searchAlias, q[len("a:"):]
	}
	q = strings.ToLower(q)
	prefix, suffix := "%", "%"
	if strings.HasPrefix(q, "^") {
		prefix = ""
		q = q[1:]
	}
	if strings.HasSuffix(q, "$") {
		suffix = ""
		q = q[:len(q)-1]
	}
	return field, prefix + q + suffix
}

// Search returns links whose name, target, or alias target matches query.
// An optional prefix selects the field: "name:" or "n:" restricts to the link
// name; "target:" or "t:" restricts to the target URL; "alias:" or "a:"
// restricts to the alias target name. Without a prefix all three fields are
// searched. The remainder supports ^ and $ anchors.
// When publicOnly is true, links with require_auth set are excluded.
func (r *SQLLinkRepo) Search(ctx context.Context, query string, limit, offset int, publicOnly bool) ([]*Link, int, error) {
	field, pattern := parseSearchQuery(query)

	authCond := ""
	if publicOnly {
		authCond = " AND require_auth = 0"
	}

	matchExpr, baseArgs := buildSearchMatchExpr(field, pattern)
	countSQL := r.db.q("SELECT COUNT(*) FROM links WHERE " + matchExpr + authCond)
	listSQL := r.db.q("SELECT " + selectCols + " FROM links WHERE " + matchExpr + authCond + " ORDER BY name_lower ASC LIMIT ? OFFSET ?")

	var total int
	if err := r.db.QueryRowContext(ctx, countSQL, baseArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count search links: %w", err)
	}

	rows, err := r.db.QueryContext(ctx, listSQL, append(baseArgs, limit, offset)...)
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

// SearchOwnedOrSharedWith returns links matching query that are owned by
// ownerEmail or shared with any of the given identifiers, paginated.
// The query syntax is identical to Search (field prefixes and ^ / $ anchors).
func (r *SQLLinkRepo) SearchOwnedOrSharedWith(ctx context.Context, ownerEmail string, identifiers []string, query string, limit, offset int) ([]*Link, int, error) {
	field, pattern := parseSearchQuery(query)

	matchExpr, patternArgs := buildSearchMatchExpr(field, pattern)

	if len(identifiers) == 0 {
		// Fast path: no sharing, restrict to owner only.
		countSQL := r.db.q("SELECT COUNT(*) FROM links WHERE owner_email = ? AND " + matchExpr)
		listSQL := r.db.q("SELECT " + selectCols + " FROM links WHERE owner_email = ? AND " + matchExpr + " ORDER BY name_lower ASC LIMIT ? OFFSET ?")

		var total int
		countArgs := append([]any{ownerEmail}, patternArgs...)
		if err := r.db.QueryRowContext(ctx, countSQL, countArgs...).Scan(&total); err != nil {
			return nil, 0, fmt.Errorf("count search owned links: %w", err)
		}
		listArgs := append(countArgs, limit, offset)
		rows, err := r.db.QueryContext(ctx, listSQL, listArgs...)
		if err != nil {
			return nil, 0, fmt.Errorf("search owned links: %w", err)
		}
		defer rows.Close()
		links, err := scanLinks(rows)
		return links, total, err
	}

	placeholders := strings.Repeat("?,", len(identifiers))
	placeholders = placeholders[:len(placeholders)-1]
	sharedArgs := make([]any, len(identifiers))
	for i, id := range identifiers {
		sharedArgs[i] = id
	}

	// owned branch args: ownerEmail + pattern(s)
	ownedBranchArgs := append([]any{ownerEmail}, patternArgs...)
	// shared branch args: sharedArgs + ownerEmail (NOT owner) + pattern(s)
	sharedBranchArgs := append(sharedArgs, ownerEmail)
	sharedBranchArgs = append(sharedBranchArgs, patternArgs...)

	countArgs := append(ownedBranchArgs, sharedBranchArgs...)
	countSQL := r.db.q(`SELECT COUNT(*) FROM (
		SELECT id FROM links WHERE owner_email = ? AND ` + matchExpr + `
		UNION
		SELECT id FROM links
		WHERE id IN (SELECT link_id FROM link_shares WHERE shared_with_email IN (` + placeholders + `))
		AND owner_email != ? AND ` + matchExpr + `)`)
	var total int
	if err := r.db.QueryRowContext(ctx, countSQL, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count search owned-or-shared links: %w", err)
	}

	listArgs := append(ownedBranchArgs, sharedBranchArgs...)
	listArgs = append(listArgs, limit, offset)
	listSQL := r.db.q(`SELECT ` + selectCols + ` FROM links WHERE owner_email = ? AND ` + matchExpr + `
		UNION
		SELECT ` + selectCols + ` FROM links
		WHERE id IN (SELECT link_id FROM link_shares WHERE shared_with_email IN (` + placeholders + `))
		AND owner_email != ? AND ` + matchExpr + `
		ORDER BY name_lower ASC LIMIT ? OFFSET ?`)
	rows, err := r.db.QueryContext(ctx, listSQL, listArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("search owned-or-shared links: %w", err)
	}
	defer rows.Close()
	links, err := scanLinks(rows)
	return links, total, err
}

// GetShares returns all emails/groups the link is shared with.
func (r *SQLLinkRepo) GetShares(ctx context.Context, linkID int64) ([]string, error) {
	rows, err := r.db.QueryContext(ctx,
		r.db.q("SELECT shared_with_email FROM link_shares WHERE link_id = ?"), linkID,
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
		r.db.q("INSERT INTO link_shares (link_id, shared_with_email) VALUES (?, ?) ON CONFLICT DO NOTHING"),
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
		r.db.q("DELETE FROM link_shares WHERE link_id = ? AND shared_with_email = ?"),
		linkID, email,
	)
	if err != nil {
		return fmt.Errorf("remove share: %w", err)
	}
	return nil
}

// SharedLinkIDs returns the set of link IDs shared with any of the given
// identifiers (typically the user's email plus their group names). Using a
// single IN query is far more efficient than issuing one query per identifier.
func (r *SQLLinkRepo) SharedLinkIDs(ctx context.Context, identifiers []string) (map[int64]bool, error) {
	if len(identifiers) == 0 {
		return make(map[int64]bool), nil
	}
	placeholders := strings.Repeat("?,", len(identifiers))
	placeholders = placeholders[:len(placeholders)-1]
	args := make([]any, len(identifiers))
	for i, id := range identifiers {
		args[i] = id
	}
	rows, err := r.db.QueryContext(ctx,
		r.db.q("SELECT link_id FROM link_shares WHERE shared_with_email IN ("+placeholders+")"),
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("shared link IDs: %w", err)
	}
	defer rows.Close()

	ids := make(map[int64]bool)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan shared link ID: %w", err)
		}
		ids[id] = true
	}
	return ids, rows.Err()
}

// ListOwnedOrSharedWith returns links owned by ownerEmail UNION links shared
// with any of the given identifiers (excluding those already owned by ownerEmail).
func (r *SQLLinkRepo) ListOwnedOrSharedWith(ctx context.Context, ownerEmail string, identifiers []string, limit, offset int, sortField SortField, sortDir SortDir) ([]*Link, int, error) {
	if err := validateSort(sortField, sortDir); err != nil {
		return nil, 0, err
	}

	if len(identifiers) == 0 {
		return r.ListByOwner(ctx, ownerEmail, limit, offset, sortField, sortDir)
	}

	placeholders := strings.Repeat("?,", len(identifiers))
	placeholders = placeholders[:len(placeholders)-1]

	sharedArgs := make([]any, len(identifiers))
	for i, id := range identifiers {
		sharedArgs[i] = id
	}

	// Count via UNION (UNION deduplicates, so shared links that are also owned
	// are not double-counted; the owner_email != ? clause already excludes them
	// from the shared branch).
	countArgs := append([]any{ownerEmail}, sharedArgs...)
	countArgs = append(countArgs, ownerEmail)
	var total int
	if err := r.db.QueryRowContext(ctx, r.db.q(`
		SELECT COUNT(*) FROM (
			SELECT id FROM links WHERE owner_email = ?
			UNION
			SELECT id FROM links
			WHERE id IN (SELECT link_id FROM link_shares WHERE shared_with_email IN (`+placeholders+`))
			AND owner_email != ?
		)`), countArgs...,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count owned or shared links: %w", err)
	}

	// Fetch page.
	listArgs := append([]any{ownerEmail}, sharedArgs...)
	listArgs = append(listArgs, ownerEmail, limit, offset)
	rows, err := r.db.QueryContext(ctx, r.db.q(fmt.Sprintf(`
		SELECT `+selectCols+` FROM links WHERE owner_email = ?
		UNION
		SELECT `+selectCols+` FROM links
		WHERE id IN (SELECT link_id FROM link_shares WHERE shared_with_email IN (`+placeholders+`))
		AND owner_email != ?
		ORDER BY %s %s LIMIT ? OFFSET ?`, sortField, sortDir)),
		listArgs...,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("list owned or shared links: %w", err)
	}
	defer rows.Close()

	links, err := scanLinks(rows)
	if err != nil {
		return nil, 0, err
	}
	return links, total, nil
}

// IncrementUseCount bumps the hit counter and last-used timestamp for the link.
func (r *SQLLinkRepo) IncrementUseCount(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx,
		r.db.q("UPDATE links SET use_count = use_count + 1, last_used_at = CURRENT_TIMESTAMP WHERE id = ?"),
		id,
	)
	if err != nil {
		return fmt.Errorf("increment use count for link %d: %w", id, err)
	}
	return nil
}

// GetAliases returns all links that alias the given canonical link name,
// ordered by name.
func (r *SQLLinkRepo) GetAliases(ctx context.Context, nameLower string) ([]*Link, error) {
	rows, err := r.db.QueryContext(ctx, r.db.q(`
		SELECT `+selectCols+`
		FROM links WHERE alias_target = ?
		ORDER BY name_lower ASC`),
		nameLower,
	)
	if err != nil {
		return nil, fmt.Errorf("get aliases for %q: %w", nameLower, err)
	}
	defer rows.Close()
	return scanLinks(rows)
}

// CountAliases returns the number of alias links targeting nameLower.
func (r *SQLLinkRepo) CountAliases(ctx context.Context, nameLower string) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx,
		r.db.q("SELECT COUNT(*) FROM links WHERE alias_target = ?"), nameLower,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count aliases for %q: %w", nameLower, err)
	}
	return count, nil
}

// ReassignLinks bulk-updates owner_email for every link owned by fromEmail,
// setting it to toEmail. Returns the count of rows changed.
func (r *SQLLinkRepo) ReassignLinks(ctx context.Context, fromEmail, toEmail string) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		r.db.q("UPDATE links SET owner_email = ? WHERE owner_email = ?"),
		toEmail, fromEmail,
	)
	if err != nil {
		return 0, fmt.Errorf("reassign links from %q to %q: %w", fromEmail, toEmail, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("reassign links rows affected: %w", err)
	}
	return n, nil
}

// TransferOwnership sets the owner_email of the given link to newOwnerEmail.
func (r *SQLLinkRepo) TransferOwnership(ctx context.Context, id int64, newOwnerEmail string) (*Link, error) {
	row := r.db.QueryRowContext(ctx, r.db.q(`
		UPDATE links SET owner_email = ? WHERE id = ?
		RETURNING `+selectCols),
		newOwnerEmail, id,
	)
	link, err := scanLink(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return link, err
}

// scanLink reads a single Link from a *sql.Row.
func scanLink(row *sql.Row) (*Link, error) {
	var l Link
	err := row.Scan(
		&l.ID, &l.Name, &l.NameLower, &l.Target, &l.OwnerEmail,
		&l.LinkType, &l.AliasTarget, &l.RequireAuth,
		&l.CreatedAt, &l.LastUsedAt, &l.UseCount,
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
			&l.LinkType, &l.AliasTarget, &l.RequireAuth,
			&l.CreatedAt, &l.LastUsedAt, &l.UseCount,
		); err != nil {
			return nil, fmt.Errorf("scan link row: %w", err)
		}
		links = append(links, &l)
	}
	return links, rows.Err()
}
