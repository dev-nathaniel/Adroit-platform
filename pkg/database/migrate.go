package database

import (
	"errors"
	"fmt"
	"io/fs"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

// ErrNoChange indicates that the database is already at the latest migration version.
var ErrNoChange = migrate.ErrNoChange

// newMigrateWithFS creates a golang-migrate instance backed by an fs.FS source and Postgres DSN.
func newMigrateWithFS(dsn string, fsys fs.FS) (*migrate.Migrate, error) {
	driver, err := iofs.New(fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("failed to initialize migration filesystem driver: %w", err)
	}

	m, err := migrate.NewWithSourceInstance("iofs", driver, dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize migration instance: %w", err)
	}

	return m, nil
}

// MigrateUp applies all pending migrations from the provided filesystem.
func MigrateUp(dsn string, fsys fs.FS) error {
	m, err := newMigrateWithFS(dsn, fsys)
	if err != nil {
		return err
	}
	defer m.Close()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migration up failed: %w", err)
	}

	return nil
}

// MigrateDown rolls back migrations. If steps <= 0, rolls back all migrations.
func MigrateDown(dsn string, fsys fs.FS, steps int) error {
	m, err := newMigrateWithFS(dsn, fsys)
	if err != nil {
		return err
	}
	defer m.Close()

	if steps <= 0 {
		if err := m.Down(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
			return fmt.Errorf("migration down failed: %w", err)
		}
	} else {
		if err := m.Steps(-steps); err != nil && !errors.Is(err, migrate.ErrNoChange) {
			return fmt.Errorf("migration down steps (%d) failed: %w", steps, err)
		}
	}

	return nil
}

// MigrateVersion retrieves the active migration version and dirty status.
func MigrateVersion(dsn string, fsys fs.FS) (version uint, dirty bool, err error) {
	m, err := newMigrateWithFS(dsn, fsys)
	if err != nil {
		return 0, false, err
	}
	defer m.Close()

	v, d, err := m.Version()
	if err != nil && !errors.Is(err, migrate.ErrNilVersion) {
		return 0, false, fmt.Errorf("failed to fetch migration version: %w", err)
	}

	return v, d, nil
}

// MigrateForce sets a specific migration version and clears the dirty flag if a previous migration failed.
func MigrateForce(dsn string, fsys fs.FS, version int) error {
	m, err := newMigrateWithFS(dsn, fsys)
	if err != nil {
		return err
	}
	defer m.Close()

	if err := m.Force(version); err != nil {
		return fmt.Errorf("failed to force migration version %d: %w", version, err)
	}

	return nil
}
