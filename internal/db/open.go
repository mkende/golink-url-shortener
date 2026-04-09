package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	_ "github.com/lib/pq"           // registers "postgres" driver
	_ "github.com/mattn/go-sqlite3" // registers "sqlite3" driver
)

// DB wraps sql.DB with a backend identifier and provides q, a portable
// placeholder helper for writing SQL queries using ? that works with both
// SQLite and Postgres.
type DB struct {
	*sql.DB
	backend string // "sqlite3" or "postgres"
}

// Open opens a database connection pool using the given driver and DSN, then
// runs all pending schema migrations.  Supported drivers: "sqlite3", "postgres".
func Open(ctx context.Context, driver, dsn string) (*DB, error) {
	var sqlDriver, backend string
	switch driver {
	case "sqlite", "sqlite3":
		sqlDriver = "sqlite3"
		backend = "sqlite3"
	case "postgres", "postgresql":
		sqlDriver = "postgres"
		backend = "postgres"
	default:
		return nil, fmt.Errorf("unsupported driver %q", driver)
	}

	sqlDB, err := sql.Open(sqlDriver, dsn)
	if err != nil {
		return nil, fmt.Errorf("open %s database: %w", driver, err)
	}

	// SQLite does not support concurrent writers and each in-memory ":memory:"
	// connection is a separate database.  Constrain the pool to a single
	// connection so that all operations share the same underlying database file
	// (or in-memory instance) and avoid "database is locked" errors.
	if backend == "sqlite3" {
		sqlDB.SetMaxOpenConns(1)
	}

	if err := sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("ping %s database: %w", driver, err)
	}

	// Enable WAL journal mode for SQLite. WAL improves write throughput and
	// crash durability without requiring any configuration from the user.
	// It is a no-op for in-memory databases (:memory:) where the mode stays
	// "memory".
	if backend == "sqlite3" {
		if _, err := sqlDB.ExecContext(ctx, "PRAGMA journal_mode=WAL"); err != nil {
			_ = sqlDB.Close()
			return nil, fmt.Errorf("enable WAL mode: %w", err)
		}
	}

	d := &DB{DB: sqlDB, backend: backend}
	if err := d.migrate(); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}

	return d, nil
}

// q rewrites ? placeholders to $N for Postgres. For all other backends the
// query is returned unchanged.
func (d *DB) q(query string) string {
	if d.backend != "postgres" {
		return query
	}
	var b strings.Builder
	n := 0
	for _, c := range query {
		if c == '?' {
			n++
			fmt.Fprintf(&b, "$%d", n)
		} else {
			b.WriteRune(c)
		}
	}
	return b.String()
}
