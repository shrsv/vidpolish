// Package store persists vidpolish "Projects" and their "Cells" (the
// notebook-style UI's data model) in a SQLite database at
// ~/.vidpolish/vidpolish.db, using the pure-Go modernc.org/sqlite driver
// (no CGO, keeps cross-compilation as simple as the rest of vidpolish).
package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// DB wraps a database/sql handle for the vidpolish schema.
type DB struct {
	sql *sql.DB
}

const schema = `
CREATE TABLE IF NOT EXISTS projects (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	next_seq INTEGER NOT NULL DEFAULT 1,
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS cells (
	id TEXT PRIMARY KEY,
	project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
	seq INTEGER NOT NULL,
	parent_cell_id TEXT REFERENCES cells(id),
	name TEXT,
	kind TEXT NOT NULL,
	params_json TEXT NOT NULL,
	status TEXT NOT NULL DEFAULT 'idle',
	status_message TEXT,
	output_path TEXT,
	source_filename TEXT,
	youtube_url TEXT,
	position INTEGER NOT NULL,
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_cells_project ON cells(project_id);
`

// Path returns the path to ~/.vidpolish/vidpolish.db.
func Path() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home directory: %w", err)
	}
	return filepath.Join(home, ".vidpolish", "vidpolish.db"), nil
}

// Open opens (creating if needed) the vidpolish SQLite database and
// applies the schema.
func Open() (*DB, error) {
	path, err := Path()
	if err != nil {
		return nil, err
	}
	return OpenAt(path)
}

// OpenAt opens the database at an explicit path, mainly for tests.
func OpenAt(path string) (*DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("creating db dir: %w", err)
	}
	sqlDB, err := sql.Open("sqlite", path+"?_pragma=foreign_keys(1)")
	if err != nil {
		return nil, fmt.Errorf("opening db: %w", err)
	}
	// modernc.org/sqlite does not support concurrent writers well; a
	// single connection avoids "database is locked" errors for this
	// tool's modest local concurrency (a handful of running cells).
	sqlDB.SetMaxOpenConns(1)
	if _, err := sqlDB.Exec(schema); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("applying schema: %w", err)
	}
	return &DB{sql: sqlDB}, nil
}

// Close closes the underlying database handle.
func (db *DB) Close() error {
	return db.sql.Close()
}
