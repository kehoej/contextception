package db

import (
	"database/sql"
	"encoding/json"
	"strings"
)

// SearchResult represents a file found by a search query.
type SearchResult struct {
	Path      string
	Language  string
	SizeBytes int64
}

// EntrypointFile represents a file with structural graph signals.
type EntrypointFile struct {
	Path      string
	Language  string
	Indegree  int
	Outdegree int
}

// DirLanguageCount represents a directory's file count for a specific language.
type DirLanguageCount struct {
	Dir       string
	Language  string
	FileCount int
}

// SearchByPath returns files whose path contains the query substring (case-insensitive).
func (idx *Index) SearchByPath(query string, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := idx.DB.Query(
		`SELECT path, language, size_bytes FROM files
		 WHERE LOWER(path) LIKE '%' || LOWER(?) || '%'
		 ORDER BY path LIMIT ?`,
		query, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var r SearchResult
		if err := rows.Scan(&r.Path, &r.Language, &r.SizeBytes); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// SearchBySymbol returns files that export the given symbol name (exact match).
// Searches the imported_names JSON column on edges — finds dst_file where
// any importer references the given symbol name.
func (idx *Index) SearchBySymbol(symbol string, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 50
	}

	// Try json_each first (available in modernc.org/sqlite).
	results, err := idx.searchBySymbolJSON(symbol, limit)
	if err != nil {
		// Fall back to Go-side filtering if json_each isn't available.
		return idx.searchBySymbolFallback(symbol, limit)
	}
	return results, nil
}

func (idx *Index) searchBySymbolJSON(symbol string, limit int) ([]SearchResult, error) {
	rows, err := idx.DB.Query(
		`SELECT DISTINCT f.path, f.language, f.size_bytes
		 FROM edges e, json_each(e.imported_names) j
		 JOIN files f ON f.path = e.dst_file
		 WHERE j.value = ? AND e.imported_names IS NOT NULL
		 ORDER BY f.path LIMIT ?`,
		symbol, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var r SearchResult
		if err := rows.Scan(&r.Path, &r.Language, &r.SizeBytes); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

func (idx *Index) searchBySymbolFallback(symbol string, limit int) ([]SearchResult, error) {
	rows, err := idx.DB.Query(
		`SELECT DISTINCT e.dst_file, f.language, f.size_bytes, e.imported_names
		 FROM edges e
		 JOIN files f ON f.path = e.dst_file
		 WHERE e.imported_names IS NOT NULL
		 ORDER BY e.dst_file`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	seen := make(map[string]bool)
	var results []SearchResult
	for rows.Next() {
		var path, lang string
		var size int64
		var namesJSON sql.NullString
		if err := rows.Scan(&path, &lang, &size, &namesJSON); err != nil {
			return nil, err
		}
		if seen[path] || !namesJSON.Valid {
			continue
		}
		var names []string
		if err := json.Unmarshal([]byte(namesJSON.String), &names); err != nil {
			continue
		}
		for _, n := range names {
			if n == symbol {
				seen[path] = true
				results = append(results, SearchResult{Path: path, Language: lang, SizeBytes: size})
				break
			}
		}
		if len(results) >= limit {
			break
		}
	}
	return results, rows.Err()
}

// GetEntrypoints returns files flagged as entrypoints (is_entrypoint=1 in signals).
func (idx *Index) GetEntrypoints() ([]EntrypointFile, error) {
	rows, err := idx.DB.Query(
		`SELECT f.path, f.language, s.indegree, s.outdegree
		 FROM signals s
		 JOIN files f ON f.path = s.file_path
		 WHERE s.is_entrypoint = 1
		 ORDER BY f.path`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []EntrypointFile
	for rows.Next() {
		var e EntrypointFile
		if err := rows.Scan(&e.Path, &e.Language, &e.Indegree, &e.Outdegree); err != nil {
			return nil, err
		}
		results = append(results, e)
	}
	return results, rows.Err()
}

// GetFoundations returns the top-N files by indegree, excluding entrypoints and utilities.
// These are the most depended-upon source files in the project.
func (idx *Index) GetFoundations(limit int) ([]EntrypointFile, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := idx.DB.Query(
		`SELECT f.path, f.language, s.indegree, s.outdegree
		 FROM signals s
		 JOIN files f ON f.path = s.file_path
		 WHERE s.is_entrypoint = 0 AND s.is_utility = 0 AND s.indegree > 0
		 ORDER BY s.indegree DESC, f.path ASC
		 LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []EntrypointFile
	for rows.Next() {
		var e EntrypointFile
		if err := rows.Scan(&e.Path, &e.Language, &e.Indegree, &e.Outdegree); err != nil {
			return nil, err
		}
		results = append(results, e)
	}
	return results, rows.Err()
}

// GetDirectoryStructure returns file counts grouped by top-level directory and language.
func (idx *Index) GetDirectoryStructure() ([]DirLanguageCount, error) {
	rows, err := idx.DB.Query(
		`SELECT
		   CASE
		     WHEN INSTR(path, '/') > 0 THEN SUBSTR(path, 1, INSTR(path, '/') - 1)
		     ELSE '.'
		   END AS dir,
		   language,
		   COUNT(*) AS file_count
		 FROM files
		 GROUP BY dir, language
		 ORDER BY file_count DESC, dir ASC, language ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []DirLanguageCount
	for rows.Next() {
		var d DirLanguageCount
		if err := rows.Scan(&d.Dir, &d.Language, &d.FileCount); err != nil {
			return nil, err
		}
		// Normalize empty dir to "."
		if strings.TrimSpace(d.Dir) == "" {
			d.Dir = "."
		}
		results = append(results, d)
	}
	return results, rows.Err()
}
