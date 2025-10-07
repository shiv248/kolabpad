package database

import (
	"database/sql"
	"embed"
	"fmt"
	"path/filepath"
	"sort"
	"time"

	"github.com/shiv248/kolabpad/pkg/logger"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// migrate applies all pending database migrations.
// Migrations are applied in alphabetical order based on filename.
// Each migration is tracked in the schema_migrations table.
func migrate(db *sql.DB) error {
	// Create migrations tracking table
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			filename TEXT NOT NULL,
			applied_at INTEGER NOT NULL
		)
	`)
	if err != nil {
		return fmt.Errorf("create migrations table: %w", err)
	}

	// Get current version
	var currentVersion int
	db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&currentVersion)

	// Read migration files
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("read migrations: %w", err)
	}

	// Sort by filename (1_xxx.sql, 2_xxx.sql, ...)
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	// Apply pending migrations
	appliedCount := 0
	for i, entry := range entries {
		version := i + 1
		if version <= currentVersion {
			continue // Already applied
		}

		filename := entry.Name()
		logger.Info("Applying migration %d: %s", version, filename)

		// Read SQL file
		content, err := migrationsFS.ReadFile(filepath.Join("migrations", filename))
		if err != nil {
			return fmt.Errorf("read migration %s: %w", filename, err)
		}

		// Execute migration
		if _, err := db.Exec(string(content)); err != nil {
			return fmt.Errorf("migration %s: %w", filename, err)
		}

		// Record migration
		_, err = db.Exec(
			"INSERT INTO schema_migrations (version, filename, applied_at) VALUES (?, ?, ?)",
			version, filename, time.Now().Unix(),
		)
		if err != nil {
			return fmt.Errorf("record migration %s: %w", filename, err)
		}

		appliedCount++
	}

	if appliedCount > 0 {
		logger.Info("Applied %d migration(s)", appliedCount)
	} else {
		logger.Debug("Database schema is up to date (version %d)", currentVersion)
	}

	return nil
}
