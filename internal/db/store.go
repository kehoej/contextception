package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/kehoej/contextception/internal/classify"
)

// InsertFileTx inserts a file record within an existing transaction.
func InsertFileTx(tx *sql.Tx, path, hash string, mtime int64, lang string, size int64) error {
	_, err := tx.Exec(
		`INSERT INTO files (path, content_hash, last_modified, language, size_bytes) VALUES (?, ?, ?, ?, ?)`,
		path, hash, mtime, lang, size,
	)
	return err
}

// UpsertFileTx inserts or updates a file record within an existing transaction.
func UpsertFileTx(tx *sql.Tx, path, hash string, mtime int64, lang string, size int64) error {
	_, err := tx.Exec(
		`INSERT INTO files (path, content_hash, last_modified, language, size_bytes)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(path) DO UPDATE SET
		   content_hash = excluded.content_hash,
		   last_modified = excluded.last_modified,
		   language = excluded.language,
		   size_bytes = excluded.size_bytes`,
		path, hash, mtime, lang, size,
	)
	return err
}

// InsertEdgeTx inserts a dependency edge within an existing transaction.
// Ignores duplicate edges silently. importedNames may be nil for bare imports.
func InsertEdgeTx(tx *sql.Tx, src, dst, edgeType, spec, method string, line int, importedNames []string) error {
	var namesJSON *string
	if len(importedNames) > 0 {
		b, err := json.Marshal(importedNames)
		if err != nil {
			return fmt.Errorf("marshaling imported_names: %w", err)
		}
		s := string(b)
		namesJSON = &s
	}
	_, err := tx.Exec(
		`INSERT OR IGNORE INTO edges (src_file, dst_file, edge_type, specifier, resolution_method, line_number, imported_names)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		src, dst, edgeType, spec, method, line, namesJSON,
	)
	return err
}

// InsertUnresolvedTx inserts an unresolved import within an existing transaction.
// Ignores duplicates silently.
func InsertUnresolvedTx(tx *sql.Tx, src, spec, reason string, line int) error {
	_, err := tx.Exec(
		`INSERT OR IGNORE INTO unresolved (src_file, specifier, reason, line_number)
		 VALUES (?, ?, ?, ?)`,
		src, spec, reason, line,
	)
	return err
}

// SetMetaTx sets a metadata key within an existing transaction.
func SetMetaTx(tx *sql.Tx, key, value string) error {
	_, err := tx.Exec(
		`INSERT INTO meta (key, value) VALUES (?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		key, value,
	)
	return err
}

// GetMeta reads a metadata value by key.
func (idx *Index) GetMeta(key string) (string, error) {
	var value string
	err := idx.DB.QueryRow("SELECT value FROM meta WHERE key = ?", key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}

// SetMeta sets a metadata key-value pair.
func (idx *Index) SetMeta(key, value string) error {
	_, err := idx.DB.Exec(
		`INSERT INTO meta (key, value) VALUES (?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		key, value,
	)
	return err
}

// GetAllFileHashes returns a map of path→content_hash for all indexed files.
func (idx *Index) GetAllFileHashes() (map[string]string, error) {
	rows, err := idx.DB.Query("SELECT path, content_hash FROM files")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	hashes := make(map[string]string)
	for rows.Next() {
		var path, hash string
		if err := rows.Scan(&path, &hash); err != nil {
			return nil, err
		}
		hashes[path] = hash
	}
	return hashes, rows.Err()
}

// FileCount returns the number of indexed files.
func (idx *Index) FileCount() (int, error) {
	var count int
	err := idx.DB.QueryRow("SELECT COUNT(*) FROM files").Scan(&count)
	return count, err
}

// EdgeCount returns the number of dependency edges.
func (idx *Index) EdgeCount() (int, error) {
	var count int
	err := idx.DB.QueryRow("SELECT COUNT(*) FROM edges").Scan(&count)
	return count, err
}

// UnresolvedCount returns the number of unresolved imports.
func (idx *Index) UnresolvedCount() (int, error) {
	var count int
	err := idx.DB.QueryRow("SELECT COUNT(*) FROM unresolved").Scan(&count)
	return count, err
}

// EntrypointCount returns the number of files flagged as entrypoints.
func (idx *Index) EntrypointCount() (int, error) {
	var count int
	err := idx.DB.QueryRow("SELECT COUNT(*) FROM signals WHERE is_entrypoint = 1").Scan(&count)
	return count, err
}

// UtilityCount returns the number of files flagged as utilities.
func (idx *Index) UtilityCount() (int, error) {
	var count int
	err := idx.DB.QueryRow("SELECT COUNT(*) FROM signals WHERE is_utility = 1").Scan(&count)
	return count, err
}

// PackageRootCount returns the number of detected package roots.
func (idx *Index) PackageRootCount() (int, error) {
	var count int
	err := idx.DB.QueryRow("SELECT COUNT(*) FROM package_roots").Scan(&count)
	return count, err
}

// PackageRootRecord represents a package root for bulk upsert.
type PackageRootRecord struct {
	Path            string
	DetectionMethod string
	Language        string
}

// UpsertPackageRoots replaces all package roots for a given language.
func (idx *Index) UpsertPackageRoots(roots []PackageRootRecord) error {
	tx, err := idx.DB.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// Clear existing roots (we rebuild them each index run).
	if _, err := tx.Exec("DELETE FROM package_roots"); err != nil {
		return fmt.Errorf("clearing package roots: %w", err)
	}

	for _, r := range roots {
		if _, err := tx.Exec(
			`INSERT INTO package_roots (path, detection_method, language) VALUES (?, ?, ?)`,
			r.Path, r.DetectionMethod, r.Language,
		); err != nil {
			return fmt.Errorf("inserting package root %q: %w", r.Path, err)
		}
	}

	return tx.Commit()
}

// DeleteFileTx removes a file and its associated edges/signals/unresolved (via CASCADE) within a transaction.
func DeleteFileTx(tx *sql.Tx, path string) error {
	_, err := tx.Exec("DELETE FROM files WHERE path = ?", path)
	return err
}

// DeleteFileEdgesTx removes all edges originating from a file within a transaction.
func DeleteFileEdgesTx(tx *sql.Tx, path string) error {
	_, err := tx.Exec("DELETE FROM edges WHERE src_file = ?", path)
	return err
}

// DeleteFileUnresolvedTx removes all unresolved imports for a file within a transaction.
func DeleteFileUnresolvedTx(tx *sql.Tx, path string) error {
	_, err := tx.Exec("DELETE FROM unresolved WHERE src_file = ?", path)
	return err
}

// DeleteFileSignalsTx removes signals for a file within a transaction.
func DeleteFileSignalsTx(tx *sql.Tx, path string) error {
	_, err := tx.Exec("DELETE FROM signals WHERE file_path = ?", path)
	return err
}

// FileExistsTx checks if a file path exists in the index.
func FileExistsTx(tx *sql.Tx, path string) (bool, error) {
	var count int
	err := tx.QueryRow("SELECT COUNT(*) FROM files WHERE path = ?", path).Scan(&count)
	return count > 0, err
}

// SignalRow holds precomputed structural signals for a file.
type SignalRow struct {
	Indegree     int
	Outdegree    int
	IsEntrypoint bool
	IsUtility    bool
}

// FileExists checks if a file path exists in the index (non-transaction version).
func (idx *Index) FileExists(path string) (bool, error) {
	var count int
	err := idx.DB.QueryRow("SELECT COUNT(*) FROM files WHERE path = ?", path).Scan(&count)
	return count > 0, err
}

// GetDirectImports returns all files imported by the given file.
func (idx *Index) GetDirectImports(path string) ([]string, error) {
	rows, err := idx.DB.Query("SELECT DISTINCT dst_file FROM edges WHERE src_file = ?", path)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []string
	for rows.Next() {
		var dst string
		if err := rows.Scan(&dst); err != nil {
			return nil, err
		}
		results = append(results, dst)
	}
	return results, rows.Err()
}

// GetForwardEdgesBatch returns forward edges (imports) for multiple files in a single query.
// Returns a map of source file → list of destination files.
func (idx *Index) GetForwardEdgesBatch(paths []string) (map[string][]string, error) {
	if len(paths) == 0 {
		return map[string][]string{}, nil
	}
	ph, args := inPlaceholders(paths)
	query := fmt.Sprintf("SELECT src_file, dst_file FROM edges WHERE src_file IN (%s)", ph)
	rows, err := idx.DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string][]string)
	for rows.Next() {
		var src, dst string
		if err := rows.Scan(&src, &dst); err != nil {
			return nil, err
		}
		result[src] = append(result[src], dst)
	}
	return result, rows.Err()
}

// GetDirectImporters returns all files that import the given file.
func (idx *Index) GetDirectImporters(path string) ([]string, error) {
	rows, err := idx.DB.Query("SELECT DISTINCT src_file FROM edges WHERE dst_file = ?", path)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []string
	for rows.Next() {
		var src string
		if err := rows.Scan(&src); err != nil {
			return nil, err
		}
		results = append(results, src)
	}
	return results, rows.Err()
}

// inPlaceholders builds "?,?,?" and []any for an IN clause.
func inPlaceholders(paths []string) (string, []any) {
	placeholders := make([]string, len(paths))
	args := make([]any, len(paths))
	for i, p := range paths {
		placeholders[i] = "?"
		args[i] = p
	}
	return strings.Join(placeholders, ","), args
}

// getEdgeNeighbors queries distinct neighbors along edges, excluding a set.
// selectCol is the column to return, filterCol is the column to match against paths.
func (idx *Index) getEdgeNeighbors(paths []string, exclude map[string]bool, selectCol, filterCol string) ([]string, error) {
	if len(paths) == 0 {
		return nil, nil
	}
	ph, args := inPlaceholders(paths)
	query := fmt.Sprintf("SELECT DISTINCT %s FROM edges WHERE %s IN (%s)", selectCol, filterCol, ph)
	rows, err := idx.DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		if !exclude[p] {
			results = append(results, p)
		}
	}
	return results, rows.Err()
}

// GetImportsOf returns forward edges from multiple files, excluding a set.
func (idx *Index) GetImportsOf(paths []string, exclude map[string]bool) ([]string, error) {
	return idx.getEdgeNeighbors(paths, exclude, "dst_file", "src_file")
}

// GetImportersOf returns reverse edges from multiple files, excluding a set.
func (idx *Index) GetImportersOf(paths []string, exclude map[string]bool) ([]string, error) {
	return idx.getEdgeNeighbors(paths, exclude, "src_file", "dst_file")
}

// GetSignals returns signals for the given file paths.
func (idx *Index) GetSignals(paths []string) (map[string]SignalRow, error) {
	if len(paths) == 0 {
		return map[string]SignalRow{}, nil
	}
	ph, args := inPlaceholders(paths)
	query := fmt.Sprintf("SELECT file_path, indegree, outdegree, is_entrypoint, is_utility FROM signals WHERE file_path IN (%s)", ph)
	rows, err := idx.DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]SignalRow)
	for rows.Next() {
		var path string
		var s SignalRow
		var ep, ut int
		if err := rows.Scan(&path, &s.Indegree, &s.Outdegree, &ep, &ut); err != nil {
			return nil, err
		}
		s.IsEntrypoint = ep == 1
		s.IsUtility = ut == 1
		result[path] = s
	}
	return result, rows.Err()
}

// GetMaxIndegree returns the maximum indegree across all signals.
func (idx *Index) GetMaxIndegree() (int, error) {
	var maxVal sql.NullInt64
	err := idx.DB.QueryRow("SELECT MAX(indegree) FROM signals").Scan(&maxVal)
	if err != nil {
		return 0, err
	}
	if !maxVal.Valid {
		return 0, nil
	}
	return int(maxVal.Int64), nil
}

// UnresolvedRecord represents a single unresolved import row.
type UnresolvedRecord struct {
	SrcFile    string
	Specifier  string
	Reason     string
	LineNumber int
}

// GetUnresolvedByFile returns all unresolved imports for a given file.
func (idx *Index) GetUnresolvedByFile(path string) ([]UnresolvedRecord, error) {
	rows, err := idx.DB.Query(
		"SELECT src_file, specifier, reason, line_number FROM unresolved WHERE src_file = ?",
		path,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []UnresolvedRecord
	for rows.Next() {
		var r UnresolvedRecord
		if err := rows.Scan(&r.SrcFile, &r.Specifier, &r.Reason, &r.LineNumber); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// UnresolvedSummary holds aggregate unresolved import statistics.
type UnresolvedSummary struct {
	Total    int
	ByReason map[string]int
}

// GetUnresolvedSummary returns aggregate unresolved import counts grouped by reason.
func (idx *Index) GetUnresolvedSummary() (UnresolvedSummary, error) {
	rows, err := idx.DB.Query("SELECT reason, COUNT(*) FROM unresolved GROUP BY reason")
	if err != nil {
		return UnresolvedSummary{}, err
	}
	defer rows.Close()

	summary := UnresolvedSummary{ByReason: make(map[string]int)}
	for rows.Next() {
		var reason string
		var count int
		if err := rows.Scan(&reason, &count); err != nil {
			return UnresolvedSummary{}, err
		}
		summary.ByReason[reason] = count
		summary.Total += count
	}
	return summary, rows.Err()
}

// ClearAllDataTx removes all data from files, edges, unresolved, signals, and git signal tables within a transaction.
func ClearAllDataTx(tx *sql.Tx) error {
	tables := []string{"co_change_pairs", "git_signals", "signals", "unresolved", "edges", "files"}
	for _, t := range tables {
		if _, err := tx.Exec("DELETE FROM " + t); err != nil {
			return fmt.Errorf("clearing %s: %w", t, err)
		}
	}
	return nil
}

// GitSignalRecord represents a row in the git_signals table.
type GitSignalRecord struct {
	FilePath   string
	SignalType string
	Value      float64
	WindowDays int
	ComputedAt string
}

// CoChangePairRecord represents a row in the co_change_pairs table.
type CoChangePairRecord struct {
	FileA      string
	FileB      string
	Frequency  int
	WindowDays int
	ComputedAt string
}

// CoChangePartner represents a file that co-changes with a given file.
type CoChangePartner struct {
	Path      string
	Frequency int
}

// InsertGitSignalTx inserts a git signal record within an existing transaction.
func InsertGitSignalTx(tx *sql.Tx, r GitSignalRecord) error {
	_, err := tx.Exec(
		`INSERT OR REPLACE INTO git_signals (file_path, signal_type, value, window_days, computed_at)
		 VALUES (?, ?, ?, ?, ?)`,
		r.FilePath, r.SignalType, r.Value, r.WindowDays, r.ComputedAt,
	)
	return err
}

// InsertCoChangePairTx inserts a co-change pair record within an existing transaction.
func InsertCoChangePairTx(tx *sql.Tx, r CoChangePairRecord) error {
	_, err := tx.Exec(
		`INSERT OR REPLACE INTO co_change_pairs (file_a, file_b, frequency, window_days, computed_at)
		 VALUES (?, ?, ?, ?, ?)`,
		r.FileA, r.FileB, r.Frequency, r.WindowDays, r.ComputedAt,
	)
	return err
}

// ClearGitDataTx clears git_signals and co_change_pairs within a transaction.
func ClearGitDataTx(tx *sql.Tx) error {
	if _, err := tx.Exec("DELETE FROM co_change_pairs"); err != nil {
		return fmt.Errorf("clearing co_change_pairs: %w", err)
	}
	if _, err := tx.Exec("DELETE FROM git_signals"); err != nil {
		return fmt.Errorf("clearing git_signals: %w", err)
	}
	return nil
}

// GetChurnForFiles returns churn values for the given paths and window.
func (idx *Index) GetChurnForFiles(paths []string, windowDays int) (map[string]float64, error) {
	if len(paths) == 0 {
		return map[string]float64{}, nil
	}
	ph, args := inPlaceholders(paths)
	args = append(args, windowDays)
	query := fmt.Sprintf(
		`SELECT file_path, value FROM git_signals
		 WHERE file_path IN (%s) AND signal_type = 'churn' AND window_days = ?`, ph)
	rows, err := idx.DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]float64)
	for rows.Next() {
		var path string
		var value float64
		if err := rows.Scan(&path, &value); err != nil {
			return nil, err
		}
		result[path] = value
	}
	return result, rows.Err()
}

// GetCoChangePartners returns files that co-change with the given path, sorted by frequency desc.
// Pass limit=0 to return all partners (no limit).
func (idx *Index) GetCoChangePartners(path string, windowDays int, limit int) ([]CoChangePartner, error) {
	// Use UNION of two indexed lookups instead of OR to allow index use.
	query := `SELECT file_a, file_b, frequency FROM (
		SELECT file_a, file_b, frequency FROM co_change_pairs
		 WHERE file_a = ? AND window_days = ?
		UNION ALL
		SELECT file_a, file_b, frequency FROM co_change_pairs
		 WHERE file_b = ? AND window_days = ?
	) ORDER BY frequency DESC, file_a ASC, file_b ASC`
	args := []interface{}{path, windowDays, path, windowDays}
	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}
	rows, err := idx.DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []CoChangePartner
	for rows.Next() {
		var a, b string
		var freq int
		if err := rows.Scan(&a, &b, &freq); err != nil {
			return nil, err
		}
		partner := a
		if a == path {
			partner = b
		}
		results = append(results, CoChangePartner{Path: partner, Frequency: freq})
	}
	return results, rows.Err()
}

// GitSignalCount returns the number of files with git signals for the given window.
func (idx *Index) GitSignalCount(windowDays int) (int, error) {
	var count int
	err := idx.DB.QueryRow(
		"SELECT COUNT(*) FROM git_signals WHERE signal_type = 'churn' AND window_days = ?",
		windowDays,
	).Scan(&count)
	return count, err
}

// CoChangePairCount returns the number of co-change pairs for the given window.
func (idx *Index) CoChangePairCount(windowDays int) (int, error) {
	var count int
	err := idx.DB.QueryRow(
		"SELECT COUNT(*) FROM co_change_pairs WHERE window_days = ?",
		windowDays,
	).Scan(&count)
	return count, err
}

// GetMaxChurn returns the maximum churn value across all files for the given window.
func (idx *Index) GetMaxChurn(windowDays int) (float64, error) {
	var maxVal sql.NullFloat64
	err := idx.DB.QueryRow(
		"SELECT MAX(value) FROM git_signals WHERE signal_type = 'churn' AND window_days = ?",
		windowDays,
	).Scan(&maxVal)
	if err != nil {
		return 0, err
	}
	if !maxVal.Valid {
		return 0, nil
	}
	return maxVal.Float64, nil
}

// GetOrchestrators returns files with high connectivity in both directions
// (indegree >= minIn AND outdegree >= minOut), ordered by total degree descending.
// Files in the exclude set are skipped.
func (idx *Index) GetOrchestrators(minIn, minOut, limit int, exclude map[string]bool) ([]EntrypointFile, error) {
	if limit <= 0 {
		limit = 20
	}
	// Fetch more than limit to account for excluded files.
	fetchLimit := limit + len(exclude)
	rows, err := idx.DB.Query(
		`SELECT f.path, f.language, s.indegree, s.outdegree
		 FROM signals s
		 JOIN files f ON f.path = s.file_path
		 WHERE s.indegree >= ? AND s.outdegree >= ?
		 ORDER BY (s.indegree + s.outdegree) DESC, f.path ASC
		 LIMIT ?`,
		minIn, minOut, fetchLimit,
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
		if exclude[e.Path] {
			continue
		}
		results = append(results, e)
		if len(results) >= limit {
			break
		}
	}
	return results, rows.Err()
}

// GetHighestChurnFile returns the file with the highest churn value for the given window,
// excluding files in the exclude set. Returns empty string if no churn data exists.
func (idx *Index) GetHighestChurnFile(windowDays int, exclude map[string]bool) (string, float64, error) {
	// Fetch a few extra to account for excluded files.
	fetchLimit := 10 + len(exclude)
	rows, err := idx.DB.Query(
		`SELECT g.file_path, g.value
		 FROM git_signals g
		 JOIN files f ON f.path = g.file_path
		 WHERE g.signal_type = 'churn' AND g.window_days = ?
		 ORDER BY g.value DESC
		 LIMIT ?`,
		windowDays, fetchLimit,
	)
	if err != nil {
		return "", 0, err
	}
	defer rows.Close()

	for rows.Next() {
		var path string
		var value float64
		if err := rows.Scan(&path, &value); err != nil {
			return "", 0, err
		}
		if exclude[path] {
			continue
		}
		return path, value, nil
	}
	return "", 0, rows.Err()
}

// getSymbolMap queries edge symbol names grouped by the neighbor file.
// selectCol is returned as the map key, filterCol is matched against subject.
// Names are deduplicated, "*" is filtered, and results are sorted.
func (idx *Index) getSymbolMap(subject, selectCol, filterCol string) (map[string][]string, error) {
	query := fmt.Sprintf(
		"SELECT %s, imported_names FROM edges WHERE %s = ? AND imported_names IS NOT NULL",
		selectCol, filterCol,
	)
	rows, err := idx.DB.Query(query, subject)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string][]string)
	for rows.Next() {
		var neighbor, namesJSON string
		if err := rows.Scan(&neighbor, &namesJSON); err != nil {
			return nil, err
		}
		var names []string
		if err := json.Unmarshal([]byte(namesJSON), &names); err != nil {
			continue // skip malformed data
		}
		result[neighbor] = append(result[neighbor], names...)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Deduplicate, filter "*", sort.
	for key, names := range result {
		seen := make(map[string]bool, len(names))
		filtered := make([]string, 0, len(names))
		for _, n := range names {
			if n != "*" && !seen[n] {
				seen[n] = true
				filtered = append(filtered, n)
			}
		}
		sort.Strings(filtered)
		result[key] = filtered
	}

	return result, nil
}

// GetImportedNamesForSubject returns dst_file → symbol names for all edges from subject.
func (idx *Index) GetImportedNamesForSubject(subject string) (map[string][]string, error) {
	return idx.getSymbolMap(subject, "dst_file", "src_file")
}

// GetImporterSymbols returns src_file → symbol names for all edges into subject.
func (idx *Index) GetImporterSymbols(subject string) (map[string][]string, error) {
	return idx.getSymbolMap(subject, "src_file", "dst_file")
}

// GetSubjectResolutionRate returns (resolvedCount, notFoundCount) for a subject file.
// resolvedCount = edges from subject (internal resolved imports).
// notFoundCount = unresolved from subject with reason 'not_found' (internal misses),
// plus unresolved crate::, super::, and self:: specifiers (always internal in Rust).
// External and stdlib unresolved are excluded (expected, not failures).
func (idx *Index) GetSubjectResolutionRate(subject string) (int, int, error) {
	var resolved int
	err := idx.DB.QueryRow("SELECT COUNT(*) FROM edges WHERE src_file = ?", subject).Scan(&resolved)
	if err != nil {
		return 0, 0, err
	}

	var notFound int
	err = idx.DB.QueryRow(
		`SELECT COUNT(*) FROM unresolved WHERE src_file = ? AND (
			reason = 'not_found'
			OR specifier LIKE 'crate::%'
			OR specifier LIKE 'super::%'
			OR specifier LIKE 'self::%'
		)`,
		subject,
	).Scan(&notFound)
	if err != nil {
		return 0, 0, err
	}

	return resolved, notFound, nil
}

// GetPackageImporterCount returns the count of distinct files from OTHER
// directories that have edges pointing to any file in the given directory.
// Approximates "how many files outside this Go package depend on this package".
func (idx *Index) GetPackageImporterCount(dir string) (int, error) {
	var pattern string
	if dir == "." || dir == "" {
		pattern = "%"
	} else {
		pattern = dir + "/%"
	}
	var count int
	err := idx.DB.QueryRow(
		`SELECT COUNT(DISTINCT src_file)
		 FROM edges
		 WHERE dst_file LIKE ?
		 AND dst_file NOT LIKE ?
		 AND src_file NOT LIKE ?`,
		pattern, dir+"/%/%", pattern,
	).Scan(&count)
	return count, err
}

// getSiblingsInDir returns files in the given directory matching the extension,
// excluding the specified file and subdirectories. If filterTest is true,
// results are filtered through classify.IsTestFile. If sqlTestFilter is non-empty,
// it is appended as an additional SQL condition (used for Go's _test.go pattern).
func (idx *Index) getSiblingsInDir(dir, exclude, ext, sqlTestFilter string, filterTest bool) ([]string, error) {
	var pattern string
	if dir == "." || dir == "" {
		pattern = "%"
	} else {
		pattern = dir + "/%"
	}

	query := `SELECT path FROM files
		 WHERE path LIKE ? AND path NOT LIKE ?
		 AND path LIKE ?
		 AND path != ?`
	if sqlTestFilter != "" {
		query += " " + sqlTestFilter
	}
	query += " ORDER BY path"

	rows, err := idx.DB.Query(query, pattern, dir+"/%/%", "%."+ext, exclude)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		if filterTest && classify.IsTestFile(p) {
			continue
		}
		results = append(results, p)
	}
	return results, rows.Err()
}

// GetGoSiblingsInDir returns non-test .go files in the given directory,
// excluding the specified file and subdirectories.
func (idx *Index) GetGoSiblingsInDir(dir, exclude string) ([]string, error) {
	return idx.getSiblingsInDir(dir, exclude, "go", `AND path NOT LIKE '%\_test.go' ESCAPE '\'`, false)
}

// GetJavaSiblingsInDir returns non-test .java files in the given directory,
// excluding the specified file and subdirectories.
func (idx *Index) GetJavaSiblingsInDir(dir, exclude string) ([]string, error) {
	return idx.getSiblingsInDir(dir, exclude, "java", "", true)
}

// GetRustSiblingsInDir returns non-test .rs files in the given directory,
// excluding the specified file and subdirectories.
func (idx *Index) GetRustSiblingsInDir(dir, exclude string) ([]string, error) {
	return idx.getSiblingsInDir(dir, exclude, "rs", "", true)
}

// GetCSharpSiblingsInDir returns non-test .cs files in the given directory,
// excluding the specified file and subdirectories.
func (idx *Index) GetCSharpSiblingsInDir(dir, exclude string) ([]string, error) {
	return idx.getSiblingsInDir(dir, exclude, "csharp", "", true)
}

// GetTestFilesInDir returns files in the given directory (not subdirectories) matching a suffix.
// Used for Go (_test.go) and Rust test discovery.
func (idx *Index) GetTestFilesInDir(dir, suffix string) ([]string, error) {
	// Match files where path starts with "dir/" and has no further "/" after the dir prefix.
	// Also handle root dir case.
	var pattern string
	if dir == "." || dir == "" {
		pattern = "%"
	} else {
		pattern = dir + "/%"
	}
	suffixPattern := "%" + suffix

	rows, err := idx.DB.Query(
		`SELECT path FROM files WHERE path LIKE ? AND path LIKE ? AND path NOT LIKE ?`,
		pattern, suffixPattern, dir+"/%/%",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		results = append(results, p)
	}
	return results, rows.Err()
}

// GetTestFilesUnderDir returns files under the given directory (including subdirectories)
// matching a suffix. Used for C# test project discovery where tests are organized
// in subdirectories mirroring the source project structure.
func (idx *Index) GetTestFilesUnderDir(dir, suffix string) ([]string, error) {
	var pattern string
	if dir == "." || dir == "" {
		pattern = "%"
	} else {
		pattern = dir + "/%"
	}
	suffixPattern := "%" + suffix

	rows, err := idx.DB.Query(
		`SELECT path FROM files WHERE path LIKE ? AND path LIKE ? ORDER BY path`,
		pattern, suffixPattern,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		results = append(results, p)
	}
	return results, rows.Err()
}

// FindCSharpTestDirs searches the index for directories that look like C# test
// projects matching the given source project name. It checks for patterns like:
// ProjectName.Tests/, ProjectName.Test/, ProjectName.UnitTests/, ProjectNameTests/.
// Returns repo-relative directory paths that contain indexed .cs files.
func (idx *Index) FindCSharpTestDirs(projectName string) ([]string, error) {
	// Build test project name variants.
	variants := []string{
		projectName + ".Tests",
		projectName + ".Test",
		projectName + ".UnitTests",
		projectName + ".IntegrationTests",
		projectName + "Tests",
		projectName + "Test",
	}

	// Query for directories that contain .cs files and match test project patterns.
	dirSet := make(map[string]bool)
	for _, variant := range variants {
		// Look for files under any path containing this test project directory name.
		patterns := []string{
			variant + "/%.cs",       // root level: ProjectName.Tests/...
			"%/" + variant + "/%.cs", // nested: test/ProjectName.Tests/... or tests/ProjectName.Tests/...
		}
		for _, pat := range patterns {
			rows, err := idx.DB.Query(`SELECT DISTINCT path FROM files WHERE path LIKE ? LIMIT 1`, pat)
			if err != nil {
				continue
			}
			for rows.Next() {
				var p string
				if err := rows.Scan(&p); err == nil {
					// Extract the directory up to and including the test project name.
					varIdx := strings.Index(p, variant+"/")
					if varIdx >= 0 {
						dir := p[:varIdx+len(variant)]
						dirSet[dir] = true
					}
				}
			}
			rows.Close()
		}
	}

	var dirs []string
	for d := range dirSet {
		dirs = append(dirs, d)
	}
	sort.Strings(dirs)
	return dirs, nil
}

// GetTestFilesByName searches recursively under a directory prefix for files
// matching any of the given basenames. Used for Python tests/ directory discovery
// where test files (e.g., test_pipelines.py) live in a separate tests/ tree.
func (idx *Index) GetTestFilesByName(dir string, basenames []string) ([]string, error) {
	if len(basenames) == 0 {
		return nil, nil
	}

	// Build OR conditions for each basename.
	var conditions []string
	var args []any
	for _, name := range basenames {
		if dir == "." || dir == "" {
			// Root-level: match files named exactly or under subdirectories.
			conditions = append(conditions, "(path = ? OR path LIKE ?)")
			args = append(args, name, "%/"+name)
		} else {
			// Under specific directory: path LIKE 'dir/%' AND basename matches.
			conditions = append(conditions, "(path LIKE ? AND (path LIKE ? OR path = ?))")
			args = append(args, dir+"/%", "%/"+name, dir+"/"+name)
		}
	}

	query := fmt.Sprintf("SELECT path FROM files WHERE %s ORDER BY path",
		strings.Join(conditions, " OR "))
	rows, err := idx.DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		results = append(results, p)
	}
	return results, rows.Err()
}

// DirectedEdge represents a directed dependency edge between two files.
type DirectedEdge struct {
	Src string
	Dst string
}

// GetEdgesAmong returns directed edges where both src and dst are in the given path set.
func (idx *Index) GetEdgesAmong(paths []string) ([]DirectedEdge, error) {
	if len(paths) == 0 {
		return nil, nil
	}
	ph, args := inPlaceholders(paths)
	// Both src and dst must be in the set.
	query := fmt.Sprintf(
		"SELECT DISTINCT src_file, dst_file FROM edges WHERE src_file IN (%s) AND dst_file IN (%s)",
		ph, ph,
	)
	// Duplicate the args for both IN clauses.
	allArgs := make([]any, 0, len(args)*2)
	allArgs = append(allArgs, args...)
	allArgs = append(allArgs, args...)

	rows, err := idx.DB.Query(query, allArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []DirectedEdge
	for rows.Next() {
		var e DirectedEdge
		if err := rows.Scan(&e.Src, &e.Dst); err != nil {
			return nil, err
		}
		results = append(results, e)
	}
	return results, rows.Err()
}
