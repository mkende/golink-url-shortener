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
	switch driver {
	case "sqlite", "sqlite3":
		sqlDriver = "sqlite3"
	case "postgres", "postgresql":
		sqlDriver = "postgres"
	}

	db, err := sql.Open(sqlDriver, dsn)
	if err != nil {
		return nil, fmt.Errorf("open %s database: %w", driver, err)
	}

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping %s database: %w", driver, err)
	}

	if err := runMigrations(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}

	return db, nil
}
