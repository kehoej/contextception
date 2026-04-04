// Package db manages the SQLite index database lifecycle.
package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// Index wraps the SQLite database connection for the contextception index.
type Index struct {
	DB   *sql.DB
	Path string
}

// IndexDir is the directory name where the index is stored.
const IndexDir = ".contextception"

// IndexFile is the filename of the SQLite database.
const IndexFile = "index.sqlite"

// IndexPath returns the full path to the index file for a given repo root.
func IndexPath(repoRoot string) string {
	return filepath.Join(repoRoot, IndexDir, IndexFile)
}

// OpenIndex opens or creates the SQLite index database at the given path.
// It configures pragmas for WAL mode, foreign keys, and busy timeout.
// On first creation, it ensures .contextception/ is in .gitignore.
func OpenIndex(path string) (*Index, error) {
	// Ensure the parent directory exists.
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating index directory: %w", err)
	}

	// Ensure .contextception/ is in .gitignore.
	ensureGitignore(dir)

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	// Set pragmas for performance and correctness.
	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA foreign_keys=ON",
		"PRAGMA busy_timeout=5000",
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			db.Close()
			return nil, fmt.Errorf("setting pragma %q: %w", p, err)
		}
	}

	// Verify we can actually query the database (detects corruption early).
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("database may be corrupted — run `contextception reindex` to rebuild: %w", err)
	}

	return &Index{DB: db, Path: path}, nil
}

// Close closes the database connection.
func (idx *Index) Close() error {
	return idx.DB.Close()
}

// ensureGitignore checks if .contextception/ is in the repo's .gitignore
// and appends it if missing. Silently does nothing if the .gitignore can't
// be read or written (not a git repo, permissions, etc.).
func ensureGitignore(indexDir string) {
	repoRoot := filepath.Dir(indexDir)
	gitignorePath := filepath.Join(repoRoot, ".gitignore")

	entry := ".contextception/"

	// Read existing .gitignore.
	content, err := os.ReadFile(gitignorePath)
	if err != nil && !os.IsNotExist(err) {
		return // can't read, skip silently
	}

	// Check if already present.
	lines := string(content)
	for _, line := range splitLines(lines) {
		if line == entry || line == ".contextception" {
			return // already covered
		}
	}

	// Append the entry.
	f, err := os.OpenFile(gitignorePath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	defer f.Close()

	// Add a newline before the entry if the file doesn't end with one.
	if len(content) > 0 && content[len(content)-1] != '\n' {
		_, _ = f.WriteString("\n")
	}
	_, _ = f.WriteString(entry + "\n")
}

// splitLines splits a string into lines, handling both \n and \r\n.
func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			line := s[start:i]
			if len(line) > 0 && line[len(line)-1] == '\r' {
				line = line[:len(line)-1]
			}
			lines = append(lines, line)
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

// SchemaVersion reads the current schema version from the meta table.
// Returns 0 if the meta table does not exist (fresh database).
func (idx *Index) SchemaVersion() (int, error) {
	var version int
	err := idx.DB.QueryRow("SELECT value FROM meta WHERE key = 'schema_version'").Scan(&version)
	if err != nil {
		// Table doesn't exist yet — version 0.
		return 0, nil
	}
	return version, nil
}
