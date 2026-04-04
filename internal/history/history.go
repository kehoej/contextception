// Package history provides persistent storage for analysis results over time.
// This enables blast radius trends, hotspot evolution, and coupling drift tracking.
//
// History is stored in a separate SQLite database (.contextception/history.sqlite)
// to keep it independent from the index, which gets rebuilt frequently.
package history

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/kehoej/contextception/internal/model"
	_ "modernc.org/sqlite"
)

// Store manages the historical analysis database.
type Store struct {
	db *sql.DB
}

// HistoryFile is the filename of the history database.
const HistoryFile = "history.sqlite"

// HistoryPath returns the path to the history database for a repo.
func HistoryPath(repoRoot string) string {
	return filepath.Join(repoRoot, ".contextception", HistoryFile)
}

// Open opens or creates the history database.
func Open(repoRoot string) (*Store, error) {
	path := HistoryPath(repoRoot)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating history directory: %w", err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("opening history database: %w", err)
	}

	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA busy_timeout=5000",
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			db.Close()
			return nil, fmt.Errorf("setting pragma %q: %w", p, err)
		}
	}

	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrating history database: %w", err)
	}

	return &Store{db: db}, nil
}

// Close closes the database.
func (s *Store) Close() error {
	return s.db.Close()
}

// migrate creates the history tables if they don't exist.
func migrate(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS analysis_runs (
		id            INTEGER PRIMARY KEY AUTOINCREMENT,
		created_at    TEXT    NOT NULL DEFAULT (datetime('now')),
		ref_range     TEXT    NOT NULL,
		commit_sha    TEXT,
		branch        TEXT,
		total_files   INTEGER NOT NULL DEFAULT 0,
		blast_level   TEXT    NOT NULL,
		blast_detail  TEXT,
		fragility     REAL    NOT NULL DEFAULT 0,
		high_risk     INTEGER NOT NULL DEFAULT 0,
		test_gaps     INTEGER NOT NULL DEFAULT 0,
		coupling      INTEGER NOT NULL DEFAULT 0,
		hotspots      INTEGER NOT NULL DEFAULT 0,
		report_json   TEXT
	);

	CREATE TABLE IF NOT EXISTS file_risks (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		run_id      INTEGER NOT NULL REFERENCES analysis_runs(id) ON DELETE CASCADE,
		file_path   TEXT    NOT NULL,
		status      TEXT    NOT NULL,
		blast_level TEXT,
		UNIQUE(run_id, file_path)
	);

	CREATE TABLE IF NOT EXISTS hotspot_history (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		run_id      INTEGER NOT NULL REFERENCES analysis_runs(id) ON DELETE CASCADE,
		file_path   TEXT    NOT NULL,
		UNIQUE(run_id, file_path)
	);

	CREATE INDEX IF NOT EXISTS idx_runs_created ON analysis_runs(created_at);
	CREATE INDEX IF NOT EXISTS idx_runs_branch ON analysis_runs(branch);
	CREATE INDEX IF NOT EXISTS idx_file_risks_path ON file_risks(file_path);
	CREATE INDEX IF NOT EXISTS idx_hotspot_path ON hotspot_history(file_path);
	`
	_, err := db.Exec(schema)
	return err
}

// RecordRun stores a change report in the history database.
func (s *Store) RecordRun(report *model.ChangeReport, commitSHA, branch string) (int64, error) {
	reportJSON, err := json.Marshal(report)
	if err != nil {
		return 0, fmt.Errorf("marshaling report: %w", err)
	}

	tx, err := s.db.Begin()
	if err != nil {
		return 0, fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	fragility := 0.0
	blastLevel := "low"
	blastDetail := ""
	if report.BlastRadius != nil {
		blastLevel = report.BlastRadius.Level
		blastDetail = report.BlastRadius.Detail
		fragility = report.BlastRadius.Fragility
	}

	result, err := tx.Exec(`
		INSERT INTO analysis_runs
			(ref_range, commit_sha, branch, total_files, blast_level, blast_detail,
			 fragility, high_risk, test_gaps, coupling, hotspots, report_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		report.RefRange, commitSHA, branch,
		report.Summary.TotalFiles, blastLevel, blastDetail,
		fragility, report.Summary.HighRiskFiles,
		len(report.TestGaps), len(report.Coupling), len(report.Hotspots),
		string(reportJSON),
	)
	if err != nil {
		return 0, fmt.Errorf("inserting run: %w", err)
	}

	runID, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("getting run ID: %w", err)
	}

	// Store per-file risks.
	for _, f := range report.ChangedFiles {
		bl := ""
		if f.BlastRadius != nil {
			bl = f.BlastRadius.Level
		}
		_, err := tx.Exec(`
			INSERT OR IGNORE INTO file_risks (run_id, file_path, status, blast_level)
			VALUES (?, ?, ?, ?)`,
			runID, f.File, f.Status, bl,
		)
		if err != nil {
			return 0, fmt.Errorf("inserting file risk: %w", err)
		}
	}

	// Store hotspots.
	for _, h := range report.Hotspots {
		_, err := tx.Exec(`
			INSERT OR IGNORE INTO hotspot_history (run_id, file_path)
			VALUES (?, ?)`,
			runID, h,
		)
		if err != nil {
			return 0, fmt.Errorf("inserting hotspot: %w", err)
		}
	}

	return runID, tx.Commit()
}

// BlastRadiusTrend returns blast radius levels over time.
type BlastRadiusTrend struct {
	Date    string `json:"date"`
	Level   string `json:"level"`
	Files   int    `json:"files"`
	Branch  string `json:"branch,omitempty"`
}

// GetBlastRadiusTrend returns the last N analysis runs' blast radius.
func (s *Store) GetBlastRadiusTrend(limit int, branch string) ([]BlastRadiusTrend, error) {
	query := `
		SELECT created_at, blast_level, total_files, branch
		FROM analysis_runs
		WHERE (? = '' OR branch = ?)
		ORDER BY created_at DESC
		LIMIT ?`

	rows, err := s.db.Query(query, branch, branch, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var trends []BlastRadiusTrend
	for rows.Next() {
		var t BlastRadiusTrend
		if err := rows.Scan(&t.Date, &t.Level, &t.Files, &t.Branch); err != nil {
			return nil, err
		}
		trends = append(trends, t)
	}
	return trends, rows.Err()
}

// HotspotFrequency represents how often a file appears as a hotspot.
type HotspotFrequency struct {
	File      string `json:"file"`
	Count     int    `json:"count"`
	LastSeen  string `json:"last_seen"`
}

// GetHotspotEvolution returns files that appear most frequently as hotspots.
func (s *Store) GetHotspotEvolution(limit int, since time.Time) ([]HotspotFrequency, error) {
	query := `
		SELECT h.file_path, COUNT(*) as cnt, MAX(r.created_at) as last_seen
		FROM hotspot_history h
		JOIN analysis_runs r ON h.run_id = r.id
		WHERE r.created_at >= ?
		GROUP BY h.file_path
		ORDER BY cnt DESC
		LIMIT ?`

	rows, err := s.db.Query(query, since.Format(time.RFC3339), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []HotspotFrequency
	for rows.Next() {
		var h HotspotFrequency
		if err := rows.Scan(&h.File, &h.Count, &h.LastSeen); err != nil {
			return nil, err
		}
		result = append(result, h)
	}
	return result, rows.Err()
}

// FileRiskHistory shows how a file's blast radius has changed over time.
type FileRiskHistory struct {
	Date       string `json:"date"`
	BlastLevel string `json:"blast_level"`
	Status     string `json:"status"`
}

// GetFileRiskHistory returns the risk history for a specific file.
func (s *Store) GetFileRiskHistory(filePath string, limit int) ([]FileRiskHistory, error) {
	query := `
		SELECT r.created_at, f.blast_level, f.status
		FROM file_risks f
		JOIN analysis_runs r ON f.run_id = r.id
		WHERE f.file_path = ?
		ORDER BY r.created_at DESC
		LIMIT ?`

	rows, err := s.db.Query(query, filePath, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []FileRiskHistory
	for rows.Next() {
		var h FileRiskHistory
		if err := rows.Scan(&h.Date, &h.BlastLevel, &h.Status); err != nil {
			return nil, err
		}
		result = append(result, h)
	}
	return result, rows.Err()
}

// BlastDistribution shows what percentage of runs had each blast level.
type BlastDistribution struct {
	Level   string  `json:"level"`
	Count   int     `json:"count"`
	Percent float64 `json:"percent"`
}

// GetBlastDistribution returns the distribution of blast radius levels.
func (s *Store) GetBlastDistribution(since time.Time) ([]BlastDistribution, error) {
	query := `
		SELECT blast_level, COUNT(*) as cnt
		FROM analysis_runs
		WHERE created_at >= ?
		GROUP BY blast_level
		ORDER BY
			CASE blast_level
				WHEN 'high' THEN 1
				WHEN 'medium' THEN 2
				WHEN 'low' THEN 3
			END`

	rows, err := s.db.Query(query, since.Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []BlastDistribution
	total := 0
	for rows.Next() {
		var d BlastDistribution
		if err := rows.Scan(&d.Level, &d.Count); err != nil {
			return nil, err
		}
		total += d.Count
		results = append(results, d)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i := range results {
		if total > 0 {
			results[i].Percent = float64(results[i].Count) / float64(total) * 100
		}
	}
	return results, nil
}

// RunCount returns the total number of recorded runs.
func (s *Store) RunCount() (int, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM analysis_runs").Scan(&count)
	return count, err
}
