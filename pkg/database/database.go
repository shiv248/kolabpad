// Package database provides SQLite persistence for documents.
package database

import (
	"database/sql"
	"fmt"

	_ "github.com/mattn/go-sqlite3"
)

// PersistedDocument represents a document stored in the database.
type PersistedDocument struct {
	ID       string
	Text     string
	Language *string
}

// Database wraps a SQLite connection.
type Database struct {
	db *sql.DB
}

// New creates a new database connection and runs migrations.
func New(uri string) (*Database, error) {
	db, err := sql.Open("sqlite3", uri)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Run migrations
	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return &Database{db: db}, nil
}

// Close closes the database connection.
func (d *Database) Close() error {
	return d.db.Close()
}

// Load retrieves a document from the database.
func (d *Database) Load(id string) (*PersistedDocument, error) {
	var doc PersistedDocument
	var language sql.NullString

	err := d.db.QueryRow(
		"SELECT id, text, language FROM document WHERE id = ?",
		id,
	).Scan(&doc.ID, &doc.Text, &language)

	if err == sql.ErrNoRows {
		return nil, nil // Document doesn't exist
	}
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}

	if language.Valid {
		doc.Language = &language.String
	}

	return &doc, nil
}

// Store saves a document to the database (INSERT or UPDATE).
func (d *Database) Store(doc *PersistedDocument) error {
	query := `
	INSERT INTO document (id, text, language)
	VALUES (?, ?, ?)
	ON CONFLICT(id) DO UPDATE SET
		text = excluded.text,
		language = excluded.language
	`

	result, err := d.db.Exec(query, doc.ID, doc.Text, doc.Language)
	if err != nil {
		return fmt.Errorf("exec: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}

	if rows != 1 {
		return fmt.Errorf("expected 1 row affected, got %d", rows)
	}

	return nil
}

// Count returns the total number of documents in the database.
func (d *Database) Count() (int, error) {
	var count int
	err := d.db.QueryRow("SELECT COUNT(*) FROM document").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count: %w", err)
	}
	return count, nil
}

// Delete removes a document from the database.
func (d *Database) Delete(id string) error {
	_, err := d.db.Exec("DELETE FROM document WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete: %w", err)
	}
	return nil
}
