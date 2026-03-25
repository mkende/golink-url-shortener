package db

import (
	"context"
	"database/sql"
	"fmt"

	_ "github.com/mattn/go-sqlite3" // registers "sqlite3" driver
)

// Open opens a database connection pool using the given driver and DSN, then
// runs all pending schema migrations.  Supported drivers: "sqlite3", "postgres".
func Open(ctx context.Context, driver, dsn string) (*sql.DB, error) {
	sqlDriver := driver
	isSQLite := false
	switch driver {
	case "sqlite", "sqlite3":
		sqlDriver = "sqlite3"
		isSQLite = true
	case "postgres", "postgresql":
		sqlDriver = "postgres"
	}

	db, err := sql.Open(sqlDriver, dsn)
	if err != nil {
		return nil, fmt.Errorf("open %s database: %w", driver, err)
	}

	// SQLite does not support concurrent writers and each in-memory ":memory:"
	// connection is a separate database.  Constrain the pool to a single
	// connection so that all operations share the same underlying database file
	// (or in-memory instance) and avoid "database is locked" errors.
	if isSQLite {
		db.SetMaxOpenConns(1)
	}

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping %s database: %w", driver, err)
	}

	// Enable WAL journal mode for SQLite. WAL improves write throughput and
	// crash durability without requiring any configuration from the user.
	// It is a no-op for in-memory databases (:memory:) where the mode stays
	// "memory".
	if isSQLite {
		if _, err := db.ExecContext(ctx, "PRAGMA journal_mode=WAL"); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("enable WAL mode: %w", err)
		}
	}

	if err := runMigrations(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}

	return db, nil
}
