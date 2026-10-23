package store

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"sort"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// migrate applies every migration newer than the database's PRAGMA
// user_version, in file name order. Each one runs in its own transaction
// together with the version bump, so a failure leaves the database at the
// last good version.
func migrate(ctx context.Context, db *sql.DB) error {
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("read migrations: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)

	var current int
	if err := db.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&current); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}
	if current > len(names) {
		return fmt.Errorf("database schema version %d is newer than this farrun (knows %d)", current, len(names))
	}

	for i := current; i < len(names); i++ {
		version := i + 1

		body, err := migrationsFS.ReadFile("migrations/" + names[i])
		if err != nil {
			return fmt.Errorf("read migration %s: %w", names[i], err)
		}

		if err := applyMigration(ctx, db, version, string(body)); err != nil {
			return fmt.Errorf("apply migration %s: %w", names[i], err)
		}
	}

	return nil
}

func applyMigration(ctx context.Context, db *sql.DB, version int, body string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, body); err != nil {
		return err
	}
	// PRAGMA does not accept bound parameters; version is our own integer.
	if _, err := tx.ExecContext(ctx, fmt.Sprintf(`PRAGMA user_version = %d`, version)); err != nil {
		return err
	}

	return tx.Commit()
}
