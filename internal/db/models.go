// Package db provides database access for golink-redirector.
package db

import (
	"database/sql"
	"time"
)

// LinkType identifies the kind of redirect behaviour for a short link.
type LinkType int

const (
	// LinkTypeSimple is a plain URL redirect; the target is appended with any
	// extra path suffix from the request.
	LinkTypeSimple LinkType = 0
	// LinkTypeAdvanced uses a Go template as the target, giving access to path,
	// query, user-agent, and authenticated email variables.
	LinkTypeAdvanced LinkType = 1
	// LinkTypeAlias redirects via a canonical link identified by name.  The
	// canonical link's own redirect logic (simple or advanced) is applied.
	LinkTypeAlias LinkType = 2
)

// Link represents a short link stored in the database.
type Link struct {
	ID          int64
	Name        string
	NameLower   string
	// Target is the redirect destination for simple and advanced links.
	// It is empty for alias links.
	Target      string
	OwnerEmail  string
	LinkType    LinkType
	// AliasTarget is the lower-cased name of the canonical link for alias links.
	// It is empty for simple and advanced links.
	AliasTarget string
	RequireAuth bool
	CreatedAt   time.Time
	LastUsedAt  sql.NullTime
	UseCount    int64
}

// IsSimple reports whether the link is a plain URL redirect.
func (l *Link) IsSimple() bool { return l.LinkType == LinkTypeSimple }

// IsAdvanced reports whether the link uses a Go template redirect.
func (l *Link) IsAdvanced() bool { return l.LinkType == LinkTypeAdvanced }

// IsAlias reports whether the link is an alias for another link.
func (l *Link) IsAlias() bool { return l.LinkType == LinkTypeAlias }

// LinkShare represents a link shared with a specific user or group.
type LinkShare struct {
	LinkID          int64
	SharedWithEmail string
}

// User represents an authenticated user known to the system.
type User struct {
	Email       string
	DisplayName string
	AvatarURL   string
	LastSeenAt  time.Time
}

// Group represents a named group of users from an identity provider.
type Group struct {
	Name   string
	Source string
}

// APIKey represents an API key record; the key itself is stored only as a hash.
type APIKey struct {
	ID         int64
	Name       string
	KeyHash    string
	CreatedBy  string
	CreatedAt  time.Time
	LastUsedAt sql.NullTime
}

// SortField is a column name used to order link list results.
type SortField string

const (
	// SortByName sorts links alphabetically by lower-cased name.
	SortByName SortField = "name_lower"
	// SortByCreated sorts links by creation time, newest first.
	SortByCreated SortField = "created_at"
	// SortByLastUsed sorts links by last access time, most recent first.
	SortByLastUsed SortField = "last_used_at"
	// SortByUseCount sorts links by total use count, highest first.
	SortByUseCount SortField = "use_count"
)

// SortDir is the direction of a sort: ascending or descending.
type SortDir string

const (
	// SortAsc sorts from smallest to largest (A→Z, oldest→newest).
	SortAsc SortDir = "ASC"
	// SortDesc sorts from largest to smallest (Z→A, newest→oldest).
	SortDesc SortDir = "DESC"
)

// validSortFields is the set of columns that may be used for link ordering.
var validSortFields = map[SortField]bool{
	SortByName:     true,
	SortByCreated:  true,
	SortByLastUsed: true,
	SortByUseCount: true,
}

// validSortDirs is the set of allowed sort directions.
var validSortDirs = map[SortDir]bool{
	SortAsc:  true,
	SortDesc: true,
}
