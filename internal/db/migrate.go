package db

import (
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// MigrateToLatest applies all pending migrations to bring the schema up to date.
// Each migration runs in its own transaction.
// Returns true when any migration was applied, false when already up to date.
func (idx *Index) MigrateToLatest() (bool, error) {
	current, err := idx.SchemaVersion()
	if err != nil {
		return false, fmt.Errorf("reading schema version: %w", err)
	}

	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return false, fmt.Errorf("reading embedded migrations: %w", err)
	}

	// Sort migration files by name to ensure order.
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	applied := false
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".sql") {
			continue
		}

		// Extract version number from filename (e.g., "0001_init.sql" → 1).
		version, err := parseMigrationVersion(name)
		if err != nil {
			return false, fmt.Errorf("parsing migration filename %q: %w", name, err)
		}

		if version <= current {
			continue // already applied
		}

		content, err := fs.ReadFile(migrationsFS, "migrations/"+name)
		if err != nil {
			return false, fmt.Errorf("reading migration %q: %w", name, err)
		}

		if err := idx.applyMigration(version, string(content)); err != nil {
			return false, fmt.Errorf("applying migration %q: %w", name, err)
		}
		applied = true
	}

	return applied, nil
}

// applyMigration executes a single migration SQL in a transaction and updates the schema version.
func (idx *Index) applyMigration(version int, sql string) error {
	tx, err := idx.DB.Begin()
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(sql); err != nil {
		return fmt.Errorf("executing SQL: %w", err)
	}

	// The migration SQL itself sets schema_version via INSERT into meta for v1.
	// For subsequent migrations, we update it explicitly here.
	if version > 1 {
		if _, err := tx.Exec("UPDATE meta SET value = ? WHERE key = 'schema_version'", strconv.Itoa(version)); err != nil {
			return fmt.Errorf("updating schema version: %w", err)
		}
	}

	return tx.Commit()
}

// parseMigrationVersion extracts the version number from a migration filename.
// Expected format: "0001_description.sql" → 1.
func parseMigrationVersion(name string) (int, error) {
	parts := strings.SplitN(name, "_", 2)
	if len(parts) < 2 {
		return 0, fmt.Errorf("expected format NNNN_description.sql, got %q", name)
	}
	v, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, fmt.Errorf("invalid version number in %q: %w", name, err)
	}
	return v, nil
}
