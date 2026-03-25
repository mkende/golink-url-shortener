// Package db provides database access for golink-redirector.
package db

import (
	"database/sql"
	"time"
)

// Link represents a short link stored in the database.
type Link struct {
	ID          int64
	Name        string
	NameLower   string
	Target      string
	OwnerEmail  string
	IsAdvanced  bool
	RequireAuth bool
	CreatedAt   time.Time
	LastUsedAt  sql.NullTime
	UseCount    int64
}

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
