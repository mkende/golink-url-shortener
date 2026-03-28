package db

import (
	"embed"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/database/sqlite3"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed migrations/sqlite/*.sql migrations/postgres/*.sql
var migrationsFS embed.FS

// migrate applies all pending up-migrations for the configured backend.
func (d *DB) migrate() error {
	subDir := map[string]string{
		"sqlite3":  "migrations/sqlite",
		"postgres": "migrations/postgres",
	}[d.backend]
	if subDir == "" {
		return fmt.Errorf("unsupported backend %q", d.backend)
	}

	src, err := iofs.New(migrationsFS, subDir)
	if err != nil {
		return fmt.Errorf("migrations source: %w", err)
	}

	var m *migrate.Migrate
	switch d.backend {
	case "sqlite3":
		drv, err := sqlite3.WithInstance(d.DB, &sqlite3.Config{})
		if err != nil {
			return fmt.Errorf("migrate sqlite3 driver: %w", err)
		}
		m, err = migrate.NewWithInstance("iofs", src, "sqlite3", drv)
		if err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
	case "postgres":
		drv, err := postgres.WithInstance(d.DB, &postgres.Config{})
		if err != nil {
			return fmt.Errorf("migrate postgres driver: %w", err)
		}
		m, err = migrate.NewWithInstance("iofs", src, "postgres", drv)
		if err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("run migrations: %w", err)
	}
	return nil
}
